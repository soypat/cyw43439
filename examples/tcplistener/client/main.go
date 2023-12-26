package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"time"
)

const (
	// Edit to match server's listening TCP addr:port
	server        = "192.168.1.120:1234"
	messageperiod = time.Second / 100
)

func main() {
	raddr := netip.MustParseAddrPort(server)
	for {
		conn, err := net.DialTCP("tcp", nil, net.TCPAddrFromAddrPort(raddr))
		if err != nil {
			panic(err)
		}
		pushDataWhileOpen(conn)
		time.Sleep(time.Second)
	}
}

func pushDataWhileOpen(conn net.Conn) {
	dd := make([]byte, 1024)
	go func() {
		for {
			time.Sleep(10 * time.Millisecond)
			n, err := conn.Read(dd)
			if isCloseErr(err) {
				conn.Close()
				return
			}
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
			msg = append(msg, ' ') // Add some entropy to length of message for stress testing.
		}
		msg = append(msg, '\n')
		_, err := conn.Write(msg)
		if isCloseErr(err) {
			fmt.Println("closed connection")
			return
		}
		if err != nil {
			fmt.Println("werr", err.Error())
		}
		time.Sleep(messageperiod)
	}
}

func isCloseErr(err error) bool { return errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) }
