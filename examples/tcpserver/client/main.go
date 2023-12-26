package main

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"time"
)

const (
	// Edit to match server's listening TCP addr:port
	server = "192.168.1.120:1234"
)

func main() {
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
			time.Sleep(time.Second / 100)
			n, err := conn.Read(dd)
			if err != nil {
				fmt.Println("rerr", err.Error())
			}
			if n > 0 {
				fmt.Printf("%s", string(dd[:n]))
			}
		}
	}()
	baseMessage := []byte("hello ")
	i := 0
	for {

		i++
		msg := strconv.AppendInt(baseMessage, int64(i), 10)
		if i%13 == 0 {
			// msg = append(msg, ' ') // Add some entropy to length of message for stress testing.
		}
		msg = append(msg, '\n')
		_, err = conn.Write(msg)
		if err != nil {
			fmt.Println("werr", err.Error())
		}
		time.Sleep(time.Second / 100)
	}
}
