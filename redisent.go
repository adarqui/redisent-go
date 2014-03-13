package redisent

import (
	"container/list"
	"time"
	"net"
	"fmt"
	"errors"
	"strings"
	"strconv"
	"bufio"
	"io"
)

const (
	CRLF = "\r\n"
)	

const (
	REPLY_ERR = 0
	REPLY_INLINE = 1
	REPLY_BULK = 2
	REPLY_MULTI_BULK = 3
	REPLY_INT = 4
)

type RedisReply struct {
	Type uint
	Data string
	Len uint
	Next *RedisReply
}

type RedisCommand struct {
	command string
}

type RedisChan struct {
	Cmd string
	Reply chan []RedisReply
}

type Redis struct {
	ctype string
	server string
	host string
	port int
	threads int
	timeout time.Duration
	user string
	pass string
	ch chan *RedisChan
	sock net.Conn
	pipelined bool
	queue *list.List
}

func New (server string, threads int) (*Redis, error) {
	var err error
	r := new(Redis)
	r.Defaults()
	r.server = server
	err = r.parseServer()
	if err != nil {
		return nil, err
	}
	r.queue = list.New()

	/*
	 * create useless thread pool: no longer useless, works with non-multi go routined commands
	 */
	if threads <= 0 {
		threads = 1
	}
	r.threads = threads
	r.ch = make(chan *RedisChan, r.threads)
	r.connectSockets()
	return r, nil
}

func (r *Redis) connectSockets () (error) {
	for i := 0; i < r.threads; i++ {
		go r.connectSocket()
	}
	return nil
}

func (r *Redis) connectSocket () (error) {

	for {
		sock, err := net.Dial(r.ctype, r.server)
		if err != nil {
			time.Sleep(1*time.Second)
			continue
		}

		for m := range r.ch {
			if m.Cmd == "" {
				quit_msg := fmt.Sprintf("*1\r\n$4\r\nQUIT\r\n")
				_, _ = sock.Write([]byte(quit_msg))
				time.Sleep(1*time.Second)
				return nil
			}
			_, err := sock.Write([]byte(m.Cmd))

			if err != nil {
				m.Reply <- nil
				break
			}
			
			res, err := r.ReadResponse(sock)
			if err != nil {
				m.Reply <- nil
				break
			}

			m.Reply <- res
		}
	}

	return nil
}

func (r *Redis) parseServer () (error) {
	/*
	 * TODO: add username/password: tcp://user:pass@blah:port
	 */
	if strings.HasPrefix(r.server, "tcp://") == true {
		r.ctype = "tcp"
	} else if strings.HasPrefix(r.server, "unix://") == true {
		r.ctype = "unix"
	} else {
		return errors.New("invalid redis connection type: specify unix:// or tcp://")
	}

	r.server = strings.TrimPrefix(r.server, fmt.Sprintf("%s://", r.ctype))
	return nil
}

func (r *Redis) Defaults() {
	r.host = "127.0.0.1"
	r.port = 6379
	r.timeout = 30*time.Second
}

func (r *Redis) Close() {

	if r == nil {
		return
	}

	for i := 0 ; i < r.threads; i++ {
		rchan := new (RedisChan)
		rchan.Cmd = ""
		rchan.Reply = make(chan []RedisReply, 1)
		r.ch <- rchan
	}

}

func (r *Redis) Multi() {
	r.pipeline()
}

func (r *Redis) pipeline() {
	r.pipelined = true
}

func (r *Redis) Flush() {
	r.Uncork()
}

func (r *Redis) Uncork() ([]RedisReply, error) {
	resp := make ([]RedisReply, 0)
	queue_len := r.queue.Len()

	for {
		f := r.queue.Front()
		if f == nil {
			break
		}
		e := r.queue.Remove(f)

		for i := 0; i < queue_len; i++ {
		switch v := e.(type) {
			case string: {
				rchan := new (RedisChan)
				rchan.Cmd = v
				rchan.Reply = make(chan []RedisReply, 1)
				r.ch <- rchan
				rr := <-rchan.Reply
				for _,d := range rr {
					resp = append(resp, d)
				}
			}
		}
	}
	}

	return resp, nil
}

func fmtCount(argc int) (string) {
	command := fmt.Sprintf("*%d%s", argc, CRLF)
	return command
}

func fmtArgument(arg string) (string) {
	argument := fmt.Sprintf("$%d%s%s%s", len(arg), CRLF, arg, CRLF)
	return argument
}

func (r *Redis) Call(name string, args []string) ([]RedisReply, error) {
	nargs := make([]string, len(args)+2)
	nargs[0] = fmtCount(len(args)+1)
	nargs[1] = fmtArgument(name)
	argc := 2
	for _,v := range(args) {
		nargs[argc] = fmtArgument(v)
		argc += 1
	}

	command := strings.Join(nargs, "")

	r.queue.PushBack(command)

	if r.pipelined == true {
		/*
		 * Let commands fill our queue
		 */
		return nil, nil
	} else {
		return r.Uncork()
	}
}

func (r *Redis) CallFmt(name string, v ... string) ([]RedisReply, error) {
	argc := 0
	for _,_ = range (v) {
		argc += 1
	}
	nargs := make([]string, argc)
	argc = 0
	for _,s := range (v) {
		nargs[argc] = s
		argc += 1
	}
	return r.Call(name, nargs)
}


func (r *Redis) ReadResponse(sock net.Conn) ([]RedisReply, error) {

	var indx uint

	rr := make([]RedisReply, 0)
	block := make([]byte, 1024)

	/*
	r.sock.SetDeadline(time.Now().Add(r.timeout))
	*/
	n, err := sock.Read(block)
	if err != nil {
		return nil, err
	}

	s := string(block)

	switch s[0] {
		case '-' : {
			rr = append(rr, RedisReply{REPLY_ERR, s, uint(len(s)), nil})
			return rr, nil
		}
		case '+' : {
			indx = uint(strings.Index(s, CRLF))
			rr = append(rr, RedisReply{REPLY_INLINE, s[1:indx], uint(indx-1), nil})
			return rr, nil
		}
		case '$' : {
			if strings.HasPrefix(s, "$-1") == true {
				return nil, nil
				break
			}
			indx = uint(strings.Index(s, CRLF))
			l := s[1:indx]
			indx = indx + 2
			size, err := strconv.Atoi(l)
			if err != nil {
				return nil, err
			}

			rest :=  0
			if n == int(indx) {
				/*
				 * Sometimes redis sends a small packet
				 */
				rest = size+2
			} else if (size + n + 2) > n {
				rest = size-n+2+int(indx)
			}

			if rest > 0 {
				big_block := make([]byte, rest)
				n2, err2 := io.ReadFull(bufio.NewReader(sock), big_block)
				if err2 != nil {
					return nil, errors.New("bulk reply: invalid response")
				}

				if n2 != rest {
					return nil, errors.New("bulk reply: n2 != size")
				}

				block = append(block[0:n], big_block...)
			}
			s = string(block)
			s = s[indx:uint(size)+indx]
			rr = append(rr, RedisReply{REPLY_BULK, s, uint(size), nil})
			return rr, nil
		}
		case '*' : {
			return nil, errors.New("multi bulk: not implemented")
		}
		case ':' : {
			indx = uint(strings.Index(s, CRLF))
			l := s[1:indx]
			rr = append(rr, RedisReply{REPLY_INT, l, uint(len(l)), nil})
			return rr, nil
		}
		default: {
			return nil, errors.New("unknown redis reply")
		}
	}

	return nil, errors.New("unknown response")
}
