package main

import (
	"log/slog"
	"machine"
	"net/netip"
	"time"

	_ "embed"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/common"
	"github.com/soypat/seqs"
	"github.com/soypat/seqs/httpx"
	"github.com/soypat/seqs/stacks"
)

const connTimeout = 5 * time.Second
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr
const hostname = "http-client-pico"
const serverHostname = "httpbin.org"
const serverURI = "http://" + serverHostname + "/get" // Testing GET method.

var (
	dev *cyw43439.Device
)

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	dhcpc, stack, devlocal, err := common.SetupWithDHCP(common.SetupConfig{
		Hostname: hostname,
		Logger:   logger,
		TCPPorts: 1, // For HTTP over TCP.
		UDPPorts: 1, // For DNS.
	})
	dev = devlocal
	if err != nil {
		panic("setup DHCP:" + err.Error())
	}

	// Resolver router's hardware address:
	routerhw, err := common.ResolveHardwareAddr(stack, dhcpc.Router())
	if err != nil {
		panic("router hwaddr resolving:" + err.Error())
	}

	// Resolver the server's IP address:
	resolver, err := common.NewResolver(stack, dhcpc)
	if err != nil {
		panic("resolver create:" + err.Error())
	}
	addrs, err := resolver.LookupNetIP(serverHostname)
	if err != nil {
		panic("DNS lookup failed:" + err.Error())
	}
	serverAddr := netip.AddrPortFrom(addrs[0], 80)

	// Start TCP server.
	const clientPort = 80
	clientAddr := netip.AddrPortFrom(stack.Addr(), clientPort)
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

	var req httpx.RequestHeader
	req.SetRequestURI(serverURI)
	req.SetMethod("GET")
	reqbytes := req.Header()

	logger.Info("tcp:ready",
		slog.String("clientaddr", clientAddr.String()),
		slog.String("serveraddr", serverAddr.String()),
	)
	rxBuf := make([]byte, 4096)
	for {
		time.Sleep(5 * time.Second)
		slog.Info("dialing", slog.String("serveraddr", serverAddr.String()))

		// Make sure to timeout the connection if it takes too long.
		conn.SetDeadline(time.Now().Add(connTimeout))
		err = conn.OpenDialTCP(clientAddr.Port(), routerhw, serverAddr, 0x123456)
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
		logger.Info("response", slog.String("response", string(rxBuf[:n])))
		closeConn("done")
	}
}
