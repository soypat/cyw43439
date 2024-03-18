package main

import (
	"log/slog"
	"machine"
	"math/rand"
	"net/netip"
	"time"

	_ "embed"

	"github.com/soypat/cyw43439/examples/common"
	"github.com/soypat/seqs"
	"github.com/soypat/seqs/httpx"
	"github.com/soypat/seqs/stacks"
)

const connTimeout = 5 * time.Second
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr
// Set this address to the server's address.
// You can run the server example in this same directory to test this client.
const serverAddrStr = "192.168.0.44:8080"
const ourHostname = "tinygo-http-client"

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	_, stack, _, err := common.SetupWithDHCP(common.SetupConfig{
		Hostname: ourHostname,
		Logger:   logger,
		TCPPorts: 1, // For HTTP over TCP.
		UDPPorts: 1, // For DNS.
	})
	start := time.Now()
	if err != nil {
		panic("setup DHCP:" + err.Error())
	}
	svAddr, err := netip.ParseAddrPort(serverAddrStr)
	if err != nil {
		panic("parsing server address:" + err.Error())
	}
	// Resolver router's hardware address to dial outside our network to internet.
	routerhw, err := common.ResolveHardwareAddr(stack, svAddr.Addr())
	if err != nil {
		panic("router hwaddr resolving:" + err.Error())
	}

	rng := rand.New(rand.NewSource(int64(time.Now().Sub(start))))
	// Start TCP server.
	clientAddr := netip.AddrPortFrom(stack.Addr(), uint16(rng.Intn(65535-1024)+1024))
	conn, err := stacks.NewTCPConn(stack, stacks.TCPConnConfig{
		TxBufSize: tcpbufsize,
		RxBufSize: tcpbufsize,
	})

	if err != nil {
		panic("conn create:" + err.Error())
	}

	closeConn := func(err string) {
		slog.Error("tcpconn:closing", slog.String("err", err))
		conn.Close()
		for !conn.State().IsClosed() {
			slog.Info("tcpconn:waiting", slog.String("state", conn.State().String()))
			time.Sleep(1000 * time.Millisecond)
		}
	}

	// Here we create the HTTP request and generate the bytes. The Header method
	// returns the raw header bytes as should be sent over the wire.
	var req httpx.RequestHeader
	req.SetRequestURI("/")
	req.SetMethod("GET")
	req.SetHost(svAddr.Addr().String())
	reqbytes := req.Header()

	logger.Info("tcp:ready",
		slog.String("clientaddr", clientAddr.String()),
		slog.String("serveraddr", serverAddrStr),
	)
	rxBuf := make([]byte, 4096)
	for {
		time.Sleep(5 * time.Second)
		slog.Info("dialing", slog.String("serveraddr", serverAddrStr))

		// Make sure to timeout the connection if it takes too long.
		conn.SetDeadline(time.Now().Add(connTimeout))
		err = conn.OpenDialTCP(clientAddr.Port(), routerhw, svAddr, seqs.Value(rng.Intn(65535-1024)+1024))
		if err != nil {
			closeConn("opening TCP: " + err.Error())
			continue
		}
		retries := 50
		for conn.State() != seqs.StateEstablished && retries > 0 {
			time.Sleep(100 * time.Millisecond)
			retries--
		}
		conn.SetDeadline(time.Time{}) // Disable the deadline.
		if retries == 0 {
			closeConn("tcp establish retry limit exceeded")
			continue
		}

		// Send the request.
		_, err = conn.Write(reqbytes)
		if err != nil {
			closeConn("writing request: " + err.Error())
			continue
		}
		time.Sleep(500 * time.Millisecond)
		conn.SetDeadline(time.Now().Add(connTimeout))
		n, err := conn.Read(rxBuf)
		if n == 0 && err != nil {
			closeConn("reading response: " + err.Error())
			continue
		} else if n == 0 {
			closeConn("no response")
			continue
		}
		println("got HTTP response!")
		println(string(rxBuf[:n]))
		closeConn("done")
		return // exit program.
	}
}
