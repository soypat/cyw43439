package main

import (
	"fmt"
	"net"
	"net/netip"
	"time"
)

func main() {
	const server = "10.0.0.122:1234"
	raddr := netip.MustParseAddrPort(server)
	conn, err := net.DialTCP("tcp", nil, net.TCPAddrFromAddrPort(raddr))
	if err != nil {
		panic(err)
	}
	// wait a second for SYN/ACK stuff.
	time.Sleep(time.Second)
	dd := make([]byte, 1024)
	go func() {
		for {
			time.Sleep(time.Second)
			n, err := conn.Read(dd)
			if err != nil {
				fmt.Println("rerr", err.Error())
			}
			if n > 0 {
				fmt.Printf("read %q\n", string(dd[:n]))
			}
		}
	}()
	for {
		_, err = conn.Write([]byte("hello"))
		if err != nil {
			fmt.Println("werr", err.Error())
		}
		time.Sleep(time.Second)
	}
}
