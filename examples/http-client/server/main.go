package main

import (
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
)

func main() {
	const portStr = ":8080"
	myaddr := getLocalAddr()
	host, _ := os.Hostname()
	svAddr := myaddr.String() + portStr
	sv := http.NewServeMux()
	sv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request from %s: %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		w.Write([]byte("Dear " + r.RemoteAddr + ",\nGreetings from " + host + " @ " + svAddr + "\n"))
	})
	log.Println("Listening on", svAddr)
	err := http.ListenAndServe(portStr, sv)
	if err != nil {
		panic(err)
	}
}

func getLocalAddr() netip.Addr {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	addr, ok := netip.AddrFromSlice(localAddr.IP)
	if !ok {
		log.Fatal("failed to convert local address")
	}
	return addr
}
