package main

// WARNING: default -scheduler=cores unsupported, compile with -scheduler=tasks set!

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
	"github.com/soypat/lneto"
	"github.com/soypat/lneto/http/httphi"
	"github.com/soypat/lneto/ipv4"
	"github.com/soypat/lneto/tcp"
	"github.com/soypat/lneto/x/xnet"
)

const numListeners = 1 // Just one listener.
const hostname = "http-pico"
const listenPort = 80                  // HTTP server port.
const loopSleep = 5 * time.Millisecond // Sleep between polls of network.
const httpConns = 3                    // Amount of goroutines spawned to deal with connections concurrently.
const httpConnPerMemory = 2048         // Memory allocated on init per connection.

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
		MaxActiveTCPPorts:     numListeners,
		EnableRxPacketCapture: true,
		EnableTxPacketCapture: true,
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
		PoolSize:           httpConns,
		QueueSize:          3,
		TxBufSize:          len(webPage) + 128,
		RxBufSize:          256,
		EstablishedTimeout: 5 * time.Second,
		ClosingTimeout:     5 * time.Second,
		NewBackoff:         func() lneto.BackoffStrategy { return backoff },
		// Logger:             traceLog.WithGroup("tcppool"),
		// ConnLogger:         traceLog,
	})
	if err != nil {
		panic("tcppool create:" + err.Error())
	}

	// Routes are registered before Configure: the router sizes its path value
	// storage from the http.
	var http httphi.MuxSlice
	http.Handle("GET /", handleLanding)
	http.Handle("GET /toggle-led", handleToggleLED)
	cfg := httphi.DefaultRouterConfig(httpConns, httpConnPerMemory, http.MaxPathValues())
	var router httphi.Router
	err = router.Configure(&http, cfg)
	if err != nil {
		panic("router configure:" + err.Error())
	}

	stack := cystack.LnetoStack()

	// Create and register TCP listener.
	var listener tcp.Listener
	err = listener.Reset(listenPort, tcpPool)
	if err != nil {
		panic("listener reset:" + err.Error())
	}
	err = stack.RegisterListenerTCP(&listener)
	if err != nil {
		panic("listener register:" + err.Error())
	}

	addr := ipv4.String(dhcpResults.AssignedAddr4)
	logger.Info("listening",
		slog.String("addr", "http://"+addr+":"+strconv.Itoa(listenPort)),
	)

	for {
		if listener.NumberOfReadyToAccept() == 0 {
			time.Sleep(loopSleep)
			tcpPool.CheckTimeouts()
			continue
		}

		conn, _, err := listener.TryAccept()
		if err != nil {
			logger.Error("listener accept:", slog.String("err", err.Error()))
			time.Sleep(time.Second)
			continue
		}
		// Handle does not block: it hands the connection to a router goroutine.
		// The router closes the connection once the exchange is done.
		err = router.Handle(conn)
		if err != nil {
			// No exchange free: refusing is how fixed memory applies backpressure.
			logger.Error("router refused connection:", slog.String("err", err.Error()))
			conn.Close()
		}
	}
}

func handleLanding(ex *httphi.Exchange) {
	println("Got webpage request!")
	ex.Respond(httphi.StatusOK, "text/html", webPage)
}

func handleToggleLED(ex *httphi.Exchange) {
	println("got toggle led request")
	lastLedState = !lastLedState
	cystack.Device().GPIOSet(0, lastLedState)
	ex.Respond(httphi.StatusOK, "", nil)
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(loopSleep)
		}
	}
}

// backoff implements exponential backoff suitable for TCP connection read/write
// polling. It starts at 1us and caps at 5ms, doubling on each consecutive backoff.
func backoff(consecutiveBackoffs uint) time.Duration {
	const (
		minWait  = uint32(time.Microsecond)
		maxWait  = 5 * uint32(time.Millisecond)
		maxShift = 22
	)
	wait := minWait << min(consecutiveBackoffs, maxShift)
	if wait > maxWait {
		wait = maxWait
	}
	return time.Duration(wait)
}
