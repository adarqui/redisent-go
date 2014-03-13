package redisent

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"strconv"
	"testing"
	"time"
)

func bigString(N int) []byte {
	s := make([]byte, N)
	for i := 0; i < N; i++ {
		s[i] = fmt.Sprintf("%d", i%9)[0]
	}
	return s
}

func Test_New_Closed(t *testing.T) {
	_, err := New("tcp://127.0.0.1:6371", 20)
	if err == nil {
		t.Error("New (closed): tcp socket: failure")
	} else {
		t.Log("New (closed): tcp socket: success")
	}

	_, err = New("unix:///tmp/redis-sokgsdogksd.sock", 20)
	if err == nil {
		t.Error("New (closed): unix socket: failure")
	} else {
		t.Log("New: (closed): tcp socket: success")
	}
}

func Test_New_Open(t *testing.T) {
	r, err := New("tcp://127.0.0.1:6370", 20)
	if err != nil {
		t.Error("New (open): tcp socket: failure")
	} else {
		t.Log("New (open): tcp socket: success")
		r.Close()
	}

	r, err = New("unix:///tmp/redis-test.sock", 20)
	if err != nil {
		t.Error("New (open): unix socket: failure")
	} else {
		t.Log("New (open): tcp socket: success")
		r.Close()
	}
}

func Test_Null_Close(t *testing.T) {
	r := new(*Redis)
	r.Close()
	t.Log("Test null close: If we made it here, we're safe")
}

func Test_Set_and_Get(t *testing.T) {
	Set_and_Get(t, "world", 5, 2)
	Set_and_Get_N(t, 1024, 1)
	Set_and_Get_N(t, 1024*20, 10) // 20480
	Set_and_Get_N(t, 1024*30, 20)
	Set_and_Get_N(t, 10024, 100)
	Set_and_Get_N(t, 10024*10, 5)
	Set_and_Get_N(t, 100024, 1)
	Set_and_Get_N(t, 1000024, 2)
	t.Log("Done")
}

func Set_and_Get_N(t *testing.T, n int, threads int) {
	b := bigString(n)
	bs := string(b)
	bsn := uint(len(bs))
	Set_and_Get(t, bs, bsn, threads)
}

func Set_and_Get(t *testing.T, bs string, bsn uint, threads int) {

	// generic this into a string builder, for big/small strings etc

	r, err := New("tcp://127.0.0.1:6370", threads)
	if err != nil {
		t.Error(err)
	}

	defer r.Close()

	rr, err := r.Call("set", []string{"poop", bs})
	if err != nil {
		t.Error(err)
		return
	}

	if len(rr) != 1 {
		t.Error("set_and_get: len(rr) != 1")
		return
	}

	rr_p := rr[0]
	if rr_p.Type != REPLY_INLINE {
		t.Error("set_and_get: rr_p.Type != REPLY_INLINE")
		return
	}

	if rr_p.Len != 2 {
		t.Error("set_and_get: rr_p.Len != 2")
		return
	}

	if len(rr_p.Data) != 2 {
		t.Error("set_and_get: rr_p.Data != 2")
		return
	}

	if rr_p.Data != "OK" {
		t.Error(`set_and_get: rr_p.Data != "OK"`)
		return
	}

	rr, err = r.Call("get", []string{"poop"})
	if err != nil {
		t.Error(err)
	}

	if len(rr) == 0 {
		t.Error("set_and_get: len(rr_p) == 0")
		return
	}

	rr_p = rr[0]

	if rr_p.Type != REPLY_BULK {
		t.Error("set_and_get: rr_p.Type != REPLY_BULK")
		return
	}

	if rr_p.Len != bsn {
		log.Printf("set_and_get: rr_p.Len=%d, len(bs)=%d\n", rr_p.Len, len(bs))
		t.Error("set_and_get: rr_p.Len != len(bs)")
		return
	}

	if len(rr_p.Data) != len(bs) {
		log.Printf("set_and_get: len(rr_p.Data)=%d, len(bs)=%d\n", len(rr_p.Data), len(bs))
		t.Error("set_and_get: rr_p.Data != len(bs)")
		return
	}

	hash_in := md5.New()
	io.WriteString(hash_in, bs)

	hash_recv := md5.New()
	io.WriteString(hash_recv, rr_p.Data)

	log.Printf("hash_in: %x\n", hash_in.Sum(nil))
	log.Printf("hash_recv: %x\n", hash_recv.Sum(nil))

	if rr_p.Data != bs {
		t.Error(`set_and_get: rr_p.Data != bs`)
		return
	}
	t.Log(bsn)
	t.Log(len(rr_p.Data))

	rr, err = r.Call("del", []string{"poop"})
	if err != nil {
		return
	}
}

func Test_Set_and_Get_Pool(t *testing.T) {
	r, err := New("tcp://127.0.0.1:6370", 100)
	if err != nil {
		t.Error(err)
	}
	defer r.Close()

	for i := 0; i < 1000; i++ {
		go func(i int) {
			for j := 0; j < 100; j++ {
				s := fmt.Sprintf("%d.%d", i, j)
				_, err := r.Call("set", []string{s, s})
				if err != nil {
					t.Error(err)
				}
			}
		}(i)
	}
	time.Sleep(5 * time.Second)
}

func Test_Set_and_Get_Pool_Incr_Threaded_100(t *testing.T) {
	_Test_Set_and_Get_Pool_Incr_Threaded_N(t, 100, 1000)
}

func Test_Set_and_Get_Pool_Incr_NonThreaded_1000(t *testing.T) {
	_Test_Set_and_Get_Pool_Incr_NonThreaded_N(t, 1000)
}

/* now compare these vs nonthreaded */

func Test_Set_and_Get_Pool_Incr_Threaded_by10000(t *testing.T) {
	_Test_Set_and_Get_Pool_Incr_Threaded_N(t, 1000, 10000)
}

func Test_Set_and_Get_Pool_Incr_NonThreaded_by10000(t *testing.T) {
	_Test_Set_and_Get_Pool_Incr_NonThreaded_N(t, 10000)
}

// another

func Test_Set_and_Get_Pool_Incr_Threaded_by100000(t *testing.T) {
	_Test_Set_and_Get_Pool_Incr_Threaded_N(t, 1000, 100000)
}

func Test_Set_and_Get_Pool_Incr_NonThreaded_by100000(t *testing.T) {
	_Test_Set_and_Get_Pool_Incr_NonThreaded_N(t, 100000)
}

func _Test_Set_and_Get_Pool_Incr_Threaded_N(t *testing.T, threads int, loop int) {
	r, err := New("tcp://127.0.0.1:6370", threads)
	if err != nil {
		t.Error(err)
	}
	defer r.Close()

	r.Call("del", []string{"poopincr"})
	for i := 0; i < loop; i++ {
		go func(i int) {
			for j := 0; j < 100; j++ {
				_, err := r.Call("incr", []string{"poopincr"})
				if err != nil {
					t.Error(err)
				}
			}
		}(i)
	}
	//time.Sleep(5*time.Second)

	cnt := 0
	for {
		if cnt > 512 {
			t.Error("poopincr threaded failure: key will most likely never hit target value")
			return
		}

		cnt += 1
		time.Sleep(1 * time.Second)
		rr, _ := r.Call("get", []string{"poopincr"})
		if len(rr) != 1 {
			continue
		}

		count, err := strconv.Atoi(rr[0].Data)
		if err != nil {
			t.Error(err)
		}

		if count == (loop * 100) {
			break
		}

		fmt.Println("count", count)
	}

	t.Log("poopincr threaded success")
}

func _Test_Set_and_Get_Pool_Incr_NonThreaded_N(t *testing.T, loop int) {
	r, err := New("tcp://127.0.0.1:6370", 1)
	if err != nil {
		t.Error(err)
	}
	defer r.Close()

	r.Call("del", []string{"poopincr"})
	for i := 0; i < loop; i++ {
		for j := 0; j < 100; j++ {
			_, err := r.Call("incr", []string{"poopincr"})
			if err != nil {
				t.Error(err)
			}
		}
	}

	rr, err := r.Call("get", []string{"poopincr"})
	if err != nil {
		t.Error(err)
	}
	count, err := strconv.Atoi(rr[0].Data)
	if err != nil {
		t.Error(err)
	}

	if count != (loop * 100) {
		t.Error("poopincr nonthreaded failed")
		return
	}

	t.Log("poopincr nonthreaded success")
}
