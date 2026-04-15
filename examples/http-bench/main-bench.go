package main

// WARNING: default -scheduler=cores unsupported, compile with -scheduler=tasks set!

import (
	"fmt"
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
	"github.com/soypat/lneto"
	"github.com/soypat/lneto/ethernet"
	"github.com/soypat/lneto/http/httpraw"
	"github.com/soypat/lneto/tcp"
	"github.com/soypat/lneto/x/xnet"
)

const connTimeout = 3 * time.Second
const numListeners = 1 // Just one listener.
const maxConns = 3     // Max amount of concurrent connections.
const tcpbufsize = 512 // MTU - ethhdr - iphdr - tcphdr
const hostname = "bench-pico"
const listenPort = 80                  // HTTP server port.
const loopSleep = 5 * time.Millisecond // Sleep between polls of network.
const httpBuf = 512
const maxTCPReadWrite = ethernet.MaxMTU - 20 - 20

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
		NewUserData: func() any {
			var hdr httpraw.Header
			buf := make([]byte, httpBuf)
			hdr.Reset(buf)
			return &hdr
		},
		NewBackoff: func() lneto.BackoffStrategy {
			return backoff
		},
	})
	if err != nil {
		panic("tcppool create:" + err.Error())
	}

	stack := cystack.LnetoStack()
	listenAddr := netip.AddrPortFrom(dhcpResults.AssignedAddr, listenPort)

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
	pageNotExists page = iota
	pageLanding        // /
	pageToggleLED      // /toggle-led
	pageDownload       // /download?size=small|medium|large
	pageUpload         // /upload
)

func handleConn(conn *tcp.Conn, hdr *httpraw.Header) {
	defer conn.Close()
	const AsRequest = false
	var buf [512]byte
	hdr.Reset(nil)

	remoteAddr, _ := netip.AddrFromSlice(conn.RemoteAddr())
	println("incoming connection:", remoteAddr.String(), "from port", conn.RemotePort())

	// Read HTTP request headers.
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

	uri := hdr.RequestURI()
	method := hdr.Method()
	var requestedPage page
	// Parse URI path and query string. We look for "/download" prefix
	// since the URI may include "?size=..." query parameters.
	uriStr := string(uri)
	switch {
	case uriStr == "/":
		requestedPage = pageLanding
	case uriStr == "/toggle-led":
		requestedPage = pageToggleLED
		lastLedState = !lastLedState
		cystack.Device().GPIOSet(0, lastLedState)
	case hasPrefix(uriStr, "/download"):
		requestedPage = pageDownload
	case uriStr == "/upload" && string(method) == "POST":
		requestedPage = pageUpload
	}

	switch requestedPage {
	case pageDownload:
		handleDownload(conn, hdr, buf[:], uriStr)
		return
	case pageUpload:
		handleUpload(conn, hdr, buf[:])
		return
	}

	// Serve normal pages.
	hdr.Reset(nil)
	hdr.SetProtocol("HTTP/1.1")
	if requestedPage == pageNotExists {
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
		conn.Write(body)
		time.Sleep(loopSleep)
	}
}

// downloadSize returns the total number of bytes to send based on the
// "size" query parameter: small=256B, medium=~358KB, large=~2MB.
func downloadSize(uri string) (totalBytes int, label string) {
	q := queryValue(uri, "size")
	switch q {
	case "small":
		return 256, "small(256B)"
	case "large":
		return 2 * 1024 * 1024, "large(2MB)"
	default:
		return len(ipsum) * 256, "medium(358KB)"
	}
}

func handleDownload(conn *tcp.Conn, hdr *httpraw.Header, buf []byte, uri string) {
	totalSize, label := downloadSize(uri)
	println("download benchmark [", label, "]: sending", totalSize, "bytes")

	hdr.Reset(nil)
	hdr.SetProtocol("HTTP/1.1")
	hdr.SetStatus("200", "OK")
	hdr.Set("Content-Type", "application/octet-stream")
	hdr.Set("Content-Length", strconv.Itoa(totalSize))
	responseHeader, err := hdr.AppendResponse(buf[:0])
	if err != nil {
		println("error building download response:", err.Error())
		return
	}
	_, err = conn.Write(responseHeader)
	if err != nil {
		println("error writing download header:", err.Error())
		return
	}

	start := time.Now()
	remaining := totalSize
	for remaining > 0 {
		chunk := ipsum
		if remaining < len(chunk) {
			chunk = chunk[:remaining]
		}
		n, err := conn.Write(chunk)
		remaining -= n
		if err != nil {
			println("download write error:", err.Error())
			break
		}
	}
	elapsed := time.Since(start)
	sent := totalSize - remaining

	printThroughput("DOWNLOAD "+label, sent, elapsed)
}

func handleUpload(conn *tcp.Conn, hdr *httpraw.Header, buf []byte) {
	clStr := hdr.Get("Content-Length")
	contentLength := 0
	if len(clStr) > 0 {
		contentLength, _ = strconv.Atoi(string(clStr))
	}
	println("upload benchmark: expecting", contentLength, "bytes")

	// Body bytes that arrived in the same segment as the headers
	// are already consumed by the parser — retrieve them before reading more.
	bodyPrefix, _ := hdr.Body()
	totalRecv := len(bodyPrefix)

	start := time.Now()
	for totalRecv < contentLength {
		n, err := conn.Read(buf)
		totalRecv += n
		if err != nil || conn.State() != tcp.StateEstablished {
			break
		}
	}
	elapsed := time.Since(start)

	printThroughput("UPLOAD", totalRecv, elapsed)

	// Send response with server-side measurement.
	hdr.Reset(nil)
	hdr.SetProtocol("HTTP/1.1")
	hdr.SetStatus("200", "OK")
	hdr.Set("Content-Type", "text/plain")

	body := "received " + strconv.Itoa(totalRecv) + " bytes in " + elapsed.String()
	hdr.Set("Content-Length", strconv.Itoa(len(body)))
	responseHeader, err := hdr.AppendResponse(buf[:0])
	if err != nil {
		println("error building upload response:", err.Error())
		return
	}
	conn.Write(responseHeader)
	conn.Write([]byte(body))
	time.Sleep(loopSleep)
}

func printThroughput(label string, bytes int, elapsed time.Duration) {
	ms := elapsed.Milliseconds()
	if ms == 0 {
		ms = 1
	}
	Mbps := float32(bytes) * 8.0 / float32(ms) / 1000.0
	kBps := 1000 * Mbps / 8
	fmt.Fprintf(machine.Serial, "[BENCH] %s: %.2fMb/s = %.2fkBps, %db in %dms\n", label, Mbps, kBps, bytes, ms)
	// println("[BENCH]", label, ":", bytes, "bytes in", ms, "ms ->", Mbps, "Mbps =", kBps, "kBps")
}

// queryValue extracts the value of a query parameter from a URI string.
// Returns empty string if not found.
func queryValue(uri, key string) string {
	qIdx := -1
	for i := 0; i < len(uri); i++ {
		if uri[i] == '?' {
			qIdx = i
			break
		}
	}
	if qIdx < 0 {
		return ""
	}
	query := uri[qIdx+1:]
	for len(query) > 0 {
		k := query
		eqIdx := -1
		ampIdx := -1
		for i := 0; i < len(query); i++ {
			if query[i] == '=' && eqIdx < 0 {
				eqIdx = i
			}
			if query[i] == '&' {
				ampIdx = i
				break
			}
		}
		if ampIdx >= 0 {
			k = query[:ampIdx]
			query = query[ampIdx+1:]
		} else {
			query = ""
		}
		if eqIdx >= 0 && k[:eqIdx] == key {
			return k[eqIdx+1:]
		}
	}
	return ""
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
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
