package main

// WARNING: default -scheduler=cores unsupported, compile with -scheduler=tasks set!

import (
	"log/slog"
	"machine"
	"net"
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

const connTimeout = 3 * time.Second
const numListeners = 1 // Just one listener.
const maxConns = 3     // Max amount of concurrent connections.
const tcpbufsize = 512 // MTU - ethhdr - iphdr - tcphdr
const hostname = "http-pico"
const listenPort = 80                  // HTTP server port.
const loopSleep = 5 * time.Millisecond // Sleep between polls of network.
const httpBuf = 512

// Setup Wifi Password and SSID by creating ssid.text and password.text files in
// ../cywnet/credentials/ directory. Credentials are used for examples in this repo.
// When building your own application use local storage to store wifi credentials securely.
var (
	// We embed the html file in the binary so that we can edit
	// index.html with pretty syntax highlighting.
	//
	//go:embed index.html
	webPage      []byte
	lastLedState bool
	requestedIP  = [4]byte{192, 168, 1, 99}
	cystack      *cywnet.Stack
)

func main() {
	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	println("starting HTTP server example")
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	var err error
	cystack, err = cywnet.NewConfiguredPicoWithStack(credentials.SSID(), credentials.Password(), devcfg, cywnet.StackConfig{
		Hostname:              hostname,
		MaxTCPPorts:           numListeners,
		EnableRxPacketCapture: true,
	})
	if err != nil {
		panic("setup failed:" + err.Error())
	}

	// Goroutine loop needed to use the cywnet.StackBlocking implementation.
	// To avoid goroutines use StackAsync. This however means much more effort and boilerplate done by the user.
	go loopForeverStack(cystack)

	dhcpResults, err := cystack.SetupWithDHCP(cywnet.DHCPConfig{
		RequestedAddr: netip.AddrFrom4(requestedIP),
	})
	if err != nil {
		panic("DHCP failed:" + err.Error())
	}
	// tracelog can log very verbose output to debug low level bugs in lneto.
	// traceLog := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
	// 	Level: slog.LevelDebug - 2,
	// }))
	tcpPool, err := xnet.NewTCPPool(xnet.TCPPoolConfig{
		PoolSize:           maxConns,
		QueueSize:          3,
		TxBufSize:          len(webPage) + 128,
		RxBufSize:          256,
		EstablishedTimeout: 5 * time.Second,
		ClosingTimeout:     5 * time.Second,
		NewUserData: func() any {
			var hdr httpraw.Header
			buf := make([]byte, httpBuf)
			hdr.Reset(buf)
			return &hdr
		},
		// Logger:             traceLog.WithGroup("tcppool"),
		// ConnLogger:         traceLog,
	})
	if err != nil {
		panic("tcppool create:" + err.Error())
	}

	stack := cystack.LnetoStack()
	listenAddr := netip.AddrPortFrom(dhcpResults.AssignedAddr, listenPort)

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

	logger.Info("listening",
		slog.String("addr", "http://"+listenAddr.String()),
	)

	for {
		if listener.NumberOfReadyToAccept() == 0 {
			time.Sleep(5 * time.Millisecond)
			tcpPool.CheckTimeouts()
			continue
		}

		conn, httpBuf, err := listener.TryAccept()
		if err != nil {
			logger.Error("listener accept:", slog.String("err", err.Error()))
			time.Sleep(time.Second)
			continue
		}
		go handleConn(conn, httpBuf.(*httpraw.Header))
	}
}

type page uint8

const (
	pageNotExits  page = iota
	pageLanding        // /
	pageToggleLED      // /toggle-led
)

func handleConn(conn *tcp.Conn, hdr *httpraw.Header) {
	defer conn.Close()
	const AsRequest = false
	var buf [64]byte
	hdr.Reset(nil)

	remoteAddr, _ := netip.AddrFromSlice(conn.RemoteAddr())
	println("incoming connection:", remoteAddr.String(), "from port", conn.RemotePort())

	for {
		n, err := conn.Read(buf[:])
		if n > 0 {
			hdr.ReadFromBytes(buf[:n])
			needMoreData, err := hdr.TryParse(AsRequest)
			if err != nil && !needMoreData {
				println("parsing HTTP request:", err.Error())
				return
			}
			if !needMoreData {
				break
			}
		}
		closed := err == net.ErrClosed || conn.State() != tcp.StateEstablished
		if closed {
			break
		} else if hdr.BufferReceived() >= httpBuf {
			println("too much HTTP data")
			return
		}
	}
	// Check requested requestedPage URI.
	var requestedPage page
	uri := hdr.RequestURI()
	switch string(uri) {
	case "/":
		println("Got webpage request!")
		requestedPage = pageLanding
	case "/toggle-led":
		println("got toggle led request")
		requestedPage = pageToggleLED
		lastLedState = !lastLedState
		cystack.Device().GPIOSet(0, lastLedState)
	}

	// Prepare response with same buffer.
	hdr.Reset(nil)
	hdr.SetProtocol("HTTP/1.1")
	if requestedPage == pageNotExits {
		hdr.SetStatus("404", "Not Found")
	} else {
		hdr.SetStatus("200", "OK")
	}
	var body []byte
	switch requestedPage {
	case pageLanding:
		body = webPage
		hdr.Set("Content-Type", "text/html")
	}
	if len(body) > 0 {
		hdr.Set("Content-Length", strconv.Itoa(len(body)))
	}
	responseHeader, err := hdr.AppendResponse(buf[:0])
	if err != nil {
		println("error appending:", err.Error())
	}
	conn.Write(responseHeader)
	if len(body) > 0 {
		_, err := conn.Write(body)
		if err != nil {
			println("writing body:", err.Error())
		}
		time.Sleep(loopSleep)
	}
	// connection closed automatically by defer.
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(loopSleep)
		}
	}
}
