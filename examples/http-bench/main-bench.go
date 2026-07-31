package main

// WARNING: default -scheduler=cores unsupported, compile with -scheduler=tasks set!

import (
	"fmt"
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
	"github.com/soypat/lneto/ethernet"
	"github.com/soypat/lneto/http/httphi"
	"github.com/soypat/lneto/ipv4"
	"github.com/soypat/lneto/tcp"
	"github.com/soypat/lneto/x/xnet"
)

const numListeners = 1 // Just one listener.
const maxConns = 3     // Max amount of concurrent connections.
const hostname = "bench-pico"
const listenPort = 80                  // HTTP server port.
const loopSleep = 5 * time.Millisecond // Sleep between polls of network.
const maxTCPReadWrite = ethernet.MaxMTU - 20 - 20

// httphi.Router memory. All of it is allocated during Configure, serving
// requests afterwards allocates nothing.
const (
	httpRequestBuf  = 1024 // Chrome tends to send ~700 bytes on a landing page request.
	httpResponseBuf = 128  // Shares leftover request memory, so not a hard limit.
	httpNumHeaderKV = httpRequestBuf / 32
	uploadBufSize   = 512 // Discard buffer the upload benchmark reads into.
)

// Setup Wifi Password and SSID by creating ssid.text and password.text files in
// ../cywnet/credentials/ directory. Credentials are used for examples in this repo.
// When building your own application use local storage to store wifi credentials securely.
var (
	//go:embed index-bench.html
	webPage []byte
	//go:embed lorem.ipsum
	ipsum        []byte
	lastLedState bool
	requestedIP  = [4]byte{192, 168, 1, 99}
	cystack      *cywnet.Stack
)

// uploadBufs hands out the upload handler's discard buffers, one per router
// goroutine. Filled once at startup so a request never allocates.
var uploadBufs = make(chan []byte, maxConns)

func init() {
	for range maxConns {
		uploadBufs <- make([]byte, uploadBufSize)
	}
}

func main() {
	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	println("starting HTTP benchmark server")
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	var err error
	cystack, err = cywnet.NewConfiguredPicoWithStack(credentials.SSID(), credentials.Password(), devcfg, cywnet.StackConfig{
		Hostname:          hostname,
		MaxActiveTCPPorts: numListeners,
	})
	if err != nil {
		panic("setup failed:" + err.Error())
	}

	go loopForeverStack(cystack)

	dhcpResults, err := cystack.SetupWithDHCP(cywnet.DHCPConfig{
		RequestedAddr: netip.AddrFrom4(requestedIP),
	})
	if err != nil {
		panic("DHCP failed:" + err.Error())
	}

	tcpPool, err := xnet.NewTCPPool(xnet.TCPPoolConfig{
		PoolSize:  maxConns,
		QueueSize: 10,
		// Increasing buffers above x3 maxTCPReadWrite has diminishing returns.
		TxBufSize:          3 * maxTCPReadWrite,
		RxBufSize:          3 * maxTCPReadWrite,
		EstablishedTimeout: 10 * time.Second,
		ClosingTimeout:     5 * time.Second,
		NewBackoff:         func() lneto.BackoffStrategy { return backoff },
	})
	if err != nil {
		panic("tcppool create:" + err.Error())
	}

	// Routes are registered before Configure: the router sizes its path value
	// storage from the mux.
	var mux httphi.MuxSlice
	mux.Handle("GET /", handleLanding)
	mux.Handle("GET /toggle-led", handleToggleLED)
	mux.Handle("GET /download", handleDownload)
	mux.Handle("POST /upload", handleUpload)

	var router httphi.Router
	err = router.Configure(httphi.RouterConfig{
		FixedNumGoroutines:          maxConns, // Workers and exchanges allocated here and never again.
		RequestHeaderBufferSize:     httpRequestBuf,
		ResponseHeaderMinBufferSize: httpResponseBuf,
		RequestNumHeaderKVCap:       httpNumHeaderKV,
		NormalizeOutgoingKeys:       true,
		Mux:                         &mux,
		Logger:                      logger,
	})
	if err != nil {
		panic("router configure:" + err.Error())
	}

	stack := cystack.LnetoStack()

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
			tcpPool.CheckTimeouts()
			time.Sleep(loopSleep)
			continue
		}

		conn, _, err := listener.TryAccept()
		if err != nil {
			logger.Error("listener accept:", slog.String("err", err.Error()))
			time.Sleep(time.Second)
			continue
		}
		err = router.Handle(conn)
		if err != nil {
			// No exchange free: refusing is how fixed memory applies backpressure.
			logger.Error("router refused connection:", slog.String("err", err.Error()))
			conn.Close()
		}
	}
}

func handleLanding(ex *httphi.Exchange) {
	ex.Respond(httphi.StatusOK, "text/html", webPage)
}

func handleToggleLED(ex *httphi.Exchange) {
	lastLedState = !lastLedState
	cystack.Device().GPIOSet(0, lastLedState)
	ex.Respond(httphi.StatusOK, "", nil)
}

// handleDownload sends "size" query worth of bytes and reports the throughput
// it managed over serial. size=small|medium|large.
func handleDownload(ex *httphi.Exchange) {
	sizeArg, _ := ex.RequestQueryValue("size")
	totalSize, label := downloadSize(string(sizeArg))
	println("download benchmark [", label, "]: sending", totalSize, "bytes")

	ex.StageHeader("Content-Type", "application/octet-stream")
	ex.StageHeaderInt("Content-Length", int64(totalSize))
	ex.StageHeader("Connection", "close")
	ex.StageStatus(httphi.StatusOK)

	start := time.Now()
	remaining := totalSize
	for remaining > 0 {
		chunk := ipsum
		if remaining < len(chunk) {
			chunk = chunk[:remaining]
		}
		n, err := ex.WriteBody(chunk)
		remaining -= n
		if err != nil {
			println("download write error:", err.Error())
			break
		}
	}
	printThroughput("DOWNLOAD "+label, totalSize-remaining, time.Since(start))
}

// handleUpload drains the request body, discarding it, and answers with the
// throughput measured server side.
func handleUpload(ex *httphi.Exchange) {
	contentLength, _, err := ex.RequestContentLength()
	if err != nil {
		ex.RespondString(httphi.StatusBadRequest, "text/plain", "bad Content-Length")
		return
	}
	println("upload benchmark: expecting", int(contentLength), "bytes")

	buf := <-uploadBufs
	defer func() { uploadBufs <- buf }()

	start := time.Now()
	var totalRecv int64
	for totalRecv < contentLength {
		// ReadBody starts with the body bytes that arrived in the same read as
		// the header, then continues from the connection.
		n, err := ex.ReadBody(buf)
		totalRecv += int64(n)
		if err != nil {
			break
		}
	}
	elapsed := time.Since(start)
	printThroughput("UPLOAD", int(totalRecv), elapsed)

	body := "received " + strconv.FormatInt(totalRecv, 10) + " bytes in " + elapsed.String()
	ex.RespondString(httphi.StatusOK, "text/plain", body)
}

// downloadSize returns the total number of bytes to send based on the
// "size" query parameter: small=256B, medium=~358KB, large=~2MB.
func downloadSize(size string) (totalBytes int, label string) {
	switch size {
	case "small":
		return 256, "small(256B)"
	case "large":
		return 2 * 1024 * 1024, "large(2MB)"
	default:
		return len(ipsum) * 256, "medium(358KB)"
	}
}

func printThroughput(label string, bytes int, elapsed time.Duration) {
	ms := elapsed.Milliseconds()
	if ms == 0 {
		ms = 1
	}
	Mbps := float32(bytes) * 8.0 / float32(ms) / 1000.0
	kBps := 1000 * Mbps / 8
	fmt.Fprintf(machine.Serial, "[BENCH] %s: %.2fMb/s = %.2fkBps, %db in %dms\n", label, Mbps, kBps, bytes, ms)
}

func loopForeverStack(stack *cywnet.Stack) {
	var backoffs uint
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			sleep := backoff(backoffs)
			time.Sleep(sleep)
			backoffs++
		} else {
			backoffs = 0
		}
	}
}

// ConnRWBackoff implements exponential backoff suitable for TCP connection
// read/write polling. It starts at 1us and caps at 5ms, doubling on each consecutive backoff.
func backoff(consecutiveBackoffs uint) time.Duration {
	const (
		minWait        = uint32(time.Microsecond)
		maxWait        = 5 * uint32(time.Millisecond)
		maxShift       = 22
		_overflowCheck = minWait << maxShift
	)
	wait := minWait << min(consecutiveBackoffs, maxShift)
	if wait > maxWait {
		wait = maxWait
	}
	return time.Duration(wait)
}
