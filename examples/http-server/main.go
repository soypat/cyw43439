package main

import (
	"bufio"
	"io"
	"log/slog"
	"machine"
	"net/netip"
	"time"

	_ "embed"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/common"
	"github.com/soypat/seqs/httpx"
	"github.com/soypat/seqs/stacks"
)

const connTimeout = 3 * time.Second
const maxconns = 3
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr
const hostname = "http-pico"

var (
	// We embed the html file in the binary so that we can edit
	// index.html with pretty syntax highlighting.
	//
	//go:embed index.html
	webPage      []byte
	dev          *cyw43439.Device
	lastLedState bool
)

// This is our HTTP hander. It handles ALL incoming requests. Path routing is left
// as an excercise to the reader.
func HTTPHandler(respWriter io.Writer, resp *httpx.ResponseHeader, req *httpx.RequestHeader) {
	uri := string(req.RequestURI())
	resp.SetConnectionClose()
	switch uri {
	case "/":
		println("Got webpage request!")
		resp.SetContentType("text/html")
		resp.SetContentLength(len(webPage))
		respWriter.Write(resp.Header())
		respWriter.Write(webPage)

	case "/toggle-led":
		println("Got toggle led request!")
		respWriter.Write(resp.Header())
		lastLedState = !lastLedState
		dev.GPIOSet(0, lastLedState)

	default:
		println("Path not found:", uri)
		resp.SetStatusCode(404)
		respWriter.Write(resp.Header())
	}
}

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelDebug - 1,
	}))
	time.Sleep(time.Second)
	_, stack, devlocal, err := common.SetupWithDHCP(common.SetupConfig{
		Hostname: "TCP-pico",
		Logger:   logger,
		TCPPorts: 1,
	})
	dev = devlocal
	if err != nil {
		panic("setup DHCP:" + err.Error())
	}
	// Start TCP server.
	const listenPort = 80
	listenAddr := netip.AddrPortFrom(stack.Addr(), listenPort)
	listener, err := stacks.NewTCPListener(stack, stacks.TCPListenerConfig{
		MaxConnections: maxconns,
		ConnTxBufSize:  tcpbufsize,
		ConnRxBufSize:  tcpbufsize,
	})
	if err != nil {
		panic("listener create:" + err.Error())
	}
	err = listener.StartListening(listenPort)
	if err != nil {
		panic("listener start:" + err.Error())
	}
	// Reuse the same buffers for each connection to avoid heap allocations.
	var req httpx.RequestHeader
	var resp httpx.ResponseHeader
	buf := bufio.NewReaderSize(nil, 1024)
	logger.Info("listening",
		slog.String("addr", "http://"+listenAddr.String()),
	)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error("listener accept:", slog.String("err", err.Error()))
			time.Sleep(time.Second)
			continue
		}
		logger.Info("new connection", slog.String("remote", conn.RemoteAddr().String()))
		err = conn.SetDeadline(time.Now().Add(connTimeout))
		if err != nil {
			conn.Close()
			logger.Error("conn set deadline:", slog.String("err", err.Error()))
			continue
		}
		buf.Reset(conn)
		err = req.Read(buf)
		if err != nil {
			logger.Error("hdr read:", slog.String("err", err.Error()))
			conn.Close()
			continue
		}
		resp.Reset()
		HTTPHandler(conn, &resp, &req)
		// time.Sleep(100 * time.Millisecond)
		conn.Close()
	}
}
