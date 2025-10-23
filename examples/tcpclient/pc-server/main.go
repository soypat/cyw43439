package main

import (
	"fmt"
	"log"
	"net"
	"time"
)

func main() {
	l, err := net.Listen("tcp4", ":8080")
	if err != nil {
		log.Fatal(err)
	}
	i := 0
	var buf [2048]byte
	for {
		log.Println("accepting on", l.Addr())
		conn, err := l.Accept()
		if err != nil {
			log.Fatal("accepting:", err)
		}
		log.Println("received conn", conn.RemoteAddr(), "(remote) -> ", conn.LocalAddr(), "(local)")
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		fmt.Fprintf(conn, "connection %d", i)
		n, err := conn.Read(buf[:])
		if err != nil {
			log.Println("reading:", err)
			continue
		}
		log.Println("received data %q", buf[:n])
		err = conn.Close()
		if err != nil {
			log.Println("closing:", err)
		}
	}
}
