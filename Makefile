all:
	go build

test:
	go test -bench=".*" -test.v

clean:
	rm -f redisent-go
