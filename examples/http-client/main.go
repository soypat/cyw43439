package main

// WARNING: default -scheduler=cores unsupported, compile with -scheduler=tasks set!

import (
	"log/slog"
	"machine"
	"net/netip"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/cyw43439/examples/cywnet/credentials"
	"github.com/soypat/lneto/http/httpraw"
	"github.com/soypat/lneto/tcp"
)

// Setup Wifi Password and SSID by creating ssid.text and password.text files in
// ../cywnet/credentials/ directory. Credentials are used for examples in this repo.

const connTimeout = 5 * time.Second
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr

// Set this address to the server's address.
// You can run the server example in this same directory to test this client.
const serverAddrStr = "192.168.1.53:8080"
const ourHostname = "tinygo-http-client"

var requestedIP = [4]byte{192, 168, 1, 99}

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	println("starting HTTP client example")

	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	cystack, err := cywnet.NewConfiguredPicoWithStack(credentials.SSID(), credentials.Password(), devcfg, cywnet.StackConfig{
		Hostname:    ourHostname,
		MaxTCPPorts: 1,
	})
	if err != nil {
		panic("setup failed:" + err.Error())
	}

	// Background loop needed to process packets.
	go loopForeverStack(cystack)

	dhcpResults, err := cystack.SetupWithDHCP(cywnet.DHCPConfig{
		RequestedAddr: netip.AddrFrom4(requestedIP),
	})
	if err != nil {
		panic("DHCP failed:" + err.Error())
	}
	logger.Info("DHCP complete", slog.String("addr", dhcpResults.AssignedAddr.String()))

	svAddr, err := netip.ParseAddrPort(serverAddrStr)
	if err != nil {
		panic("parsing server address:" + err.Error())
	}

	stack := cystack.LnetoStack()
	const pollTime = 5 * time.Millisecond
	rstack := stack.StackRetrying(pollTime)

	// Configure TCP connection.
	var conn tcp.Conn
	err = conn.Configure(tcp.ConnConfig{
		RxBuf:             make([]byte, tcpbufsize),
		TxBuf:             make([]byte, tcpbufsize),
		TxPacketQueueSize: 3,
	})
	if err != nil {
		panic("conn configure:" + err.Error())
	}

	// Build HTTP request using httpraw.Header.
	var hdr httpraw.Header
	hdr.SetMethod("GET")
	hdr.SetRequestURI("/")
	hdr.SetProtocol("HTTP/1.1")
	hdr.Set("Host", svAddr.Addr().String())
	hdr.Set("Connection", "close")
	reqbytes, err := hdr.AppendRequest(nil)
	if err != nil {
		panic("building HTTP request:" + err.Error())
	}

	logger.Info("http:ready",
		slog.String("serveraddr", serverAddrStr),
	)
	rxBuf := make([]byte, 4096)

	for {
		time.Sleep(5 * time.Second)
		lport := uint16(stack.Prand32()>>17) + 1024 // Random local port.
		logger.Info("dialing", slog.String("serveraddr", serverAddrStr), slog.Uint64("localport", uint64(lport)))

		// Dial TCP with retries.
		err = rstack.DoDialTCP(&conn, lport, svAddr, connTimeout, 3)
		if err != nil {
			logger.Error("tcp dial failed", slog.String("err", err.Error()))
			closeConn(&conn)
			continue
		}

		// Send the HTTP request.
		_, err = conn.Write(reqbytes)
		if err != nil {
			logger.Error("writing request", slog.String("err", err.Error()))
			closeConn(&conn)
			continue
		}

		// Wait for response.
		time.Sleep(500 * time.Millisecond)
		n, err := conn.Read(rxBuf)
		if n == 0 && err != nil {
			logger.Error("reading response", slog.String("err", err.Error()))
			closeConn(&conn)
			continue
		} else if n == 0 {
			logger.Error("no response received")
			closeConn(&conn)
			continue
		}

		println("got HTTP response!")
		machine.Serial.Write(rxBuf[:n])
		closeConn(&conn)
		return // Exit program after successful request.
	}
}

func closeConn(conn *tcp.Conn) {
	conn.Close()
	for i := 0; i < 50 && !conn.State().IsClosed(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	conn.Abort()
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}
