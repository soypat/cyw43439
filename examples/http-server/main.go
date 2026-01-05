package main

import (
	"log/slog"
	"machine"
	"net/netip"
	"strconv"
	"time"

	_ "embed"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/cyw43439/examples/cywnet/credentials"
	"github.com/soypat/lneto/http/httpraw"
	"github.com/soypat/lneto/tcp"
	"github.com/soypat/lneto/x/xnet"
)

// Setup Wifi Password and SSID by creating ssid.text and password.text files in
// ../cywnet/credentials/ directory. Credentials are used for examples in this repo.

const connTimeout = 3 * time.Second
const maxconns = 3
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr
const hostname = "http-pico"

var requestedIP = [4]byte{192, 168, 1, 99}

var (
	// We embed the html file in the binary so that we can edit
	// index.html with pretty syntax highlighting.
	//
	//go:embed index.html
	webPage      []byte
	dev          *cyw43439.Device
	lastLedState bool
)

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelDebug - 2,
	}))
	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	println("starting HTTP server example")

	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	cystack, err := cywnet.NewConfiguredPicoWithStack(credentials.SSID(), credentials.Password(), devcfg, cywnet.StackConfig{
		Hostname:    hostname,
		MaxTCPConns: maxconns,
	})
	if err != nil {
		panic("setup failed:" + err.Error())
	}
	dev = cystack.Device()

	// Background loop needed to process packets.
	go loopForeverStack(cystack)

	dhcpResults, err := cystack.SetupWithDHCP(cywnet.DHCPConfig{
		RequestedAddr: netip.AddrFrom4(requestedIP),
	})
	if err != nil {
		panic("DHCP failed:" + err.Error())
	}

	stack := cystack.LnetoStack()
	const listenPort = 80
	listenAddr := netip.AddrPortFrom(dhcpResults.AssignedAddr, listenPort)

	// Create TCP pool for connection management.
	tcpPool, err := xnet.NewTCPPool(xnet.TCPPoolConfig{
		PoolSize:           maxconns,
		QueueSize:          3,
		BufferSize:         tcpbufsize,
		EstablishedTimeout: connTimeout,
		ClosingTimeout:     connTimeout,
	})
	if err != nil {
		panic("tcppool create:" + err.Error())
	}

	// Create and register TCP listener.
	var listener tcp.Listener
	err = listener.Reset(listenPort, tcpPool)
	if err != nil {
		panic("listener reset:" + err.Error())
	}
	err = stack.RegisterListener(&listener)
	if err != nil {
		panic("listener register:" + err.Error())
	}

	// Buffers for HTTP handling (reused for each connection).
	var hdr httpraw.Header
	rxBuf := make([]byte, 2048)
	txBuf := make([]byte, 512)

	logger.Info("listening",
		slog.String("addr", "http://"+listenAddr.String()),
	)

	for {
		if listener.NumberOfReadyToAccept() == 0 {
			time.Sleep(5 * time.Millisecond)
			tcpPool.CheckTimeouts()
			continue
		}

		conn, err := listener.TryAccept()
		if err != nil {
			logger.Error("listener accept:", slog.String("err", err.Error()))
			time.Sleep(time.Second)
			continue
		}

		remoteAddr, _ := netip.AddrFromSlice(conn.RemoteAddr())
		logger.Info("new connection", slog.String("remote", remoteAddr.String()))

		// Read HTTP request.
		n, err := conn.Read(rxBuf)
		if err != nil || n == 0 {
			logger.Error("read failed", slog.String("err", errStr(err)))
			conn.Close()
			continue
		}

		// Parse HTTP request.
		hdr.Reset(rxBuf[:0])
		hdr.ReadFromBytes(rxBuf[:n])
		needMore, err := hdr.TryParse(false) // false = parse as request
		if err != nil && !needMore {
			logger.Error("parse failed", slog.String("err", err.Error()))
			conn.Close()
			continue
		}

		// Handle HTTP request.
		handleHTTP(conn, &hdr, txBuf)
		conn.Close()
	}
}

// handleHTTP processes an HTTP request and writes the response.
func handleHTTP(conn *tcp.Conn, reqHdr *httpraw.Header, buf []byte) {
	uri := string(reqHdr.RequestURI())

	// Build response header.
	var respHdr httpraw.Header
	respHdr.SetProtocol("HTTP/1.1")
	respHdr.Set("Connection", "close")

	switch uri {
	case "/":
		println("Got webpage request!")
		respHdr.SetStatus("200", "OK")
		respHdr.Set("Content-Type", "text/html")
		respHdr.Set("Content-Length", strconv.Itoa(len(webPage)))
		resp, _ := respHdr.AppendResponse(buf[:0])
		conn.Write(resp)
		conn.Write(webPage)

	case "/toggle-led":
		println("Got toggle led request!")
		respHdr.SetStatus("200", "OK")
		resp, _ := respHdr.AppendResponse(buf[:0])
		conn.Write(resp)
		lastLedState = !lastLedState
		dev.GPIOSet(0, lastLedState)

	default:
		println("Path not found:", uri)
		respHdr.SetStatus("404", "Not Found")
		resp, _ := respHdr.AppendResponse(buf[:0])
		conn.Write(resp)
	}
}

func errStr(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}
