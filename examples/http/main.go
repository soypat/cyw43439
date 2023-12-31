package main

import (
	"bufio"
	"io"
	"log/slog"
	"net/netip"
	"time"

	_ "embed"

	"github.com/soypat/seqs/httpx"
	"github.com/soypat/seqs/stacks"
)

const tcpbufsize = 1024 // MTU - ethhdr - iphdr - tcphdr
const hostname = "http-pico"

// We embed the html file in the binary so that we can edit
// index.html with pretty syntax highlighting.
//
//go:embed index.html
var webPage []byte

// This is our HTTP hander. It handles ALL incoming requests. Path routing is left
// as an excercise to the reader.
func HTTPHandler(respWriter io.Writer, resp *httpx.ResponseHeader, req *httpx.RequestHeader) {
	uri := string(req.RequestURI())
	if uri != "/" {
		return // Ignore all requests that are not for the root path.
	}
	println("got request:", string(req.Method()), "@", uri)
	resp.SetConnectionClose()
	resp.SetContentType("text/html")
	resp.SetContentLength(len(webPage))
	respWriter.Write(resp.Header())
	respWriter.Write(webPage)
}

func main() {
	stack, err := setupDHCPStack(hostname, netip.AddrFrom4([4]byte{192, 168, 1, 4}))
	// Start TCP server.
	const listenPort = 1234
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
		HTTPHandler(conn, &resp, &req)
		time.Sleep(100 * time.Millisecond)
		conn.Close()
	}
}
