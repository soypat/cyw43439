package main

import (
	"log/slog"
	"machine"
	"net/http"
	"net/netip"
	"time"

	_ "embed"

	"github.com/soypat/cyw43439/examples/common"
	"github.com/soypat/seqs/stacks"
)

const connTimeout = 3 * time.Second
const maxconns = 3
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr
const hostname = "TCP-pico"

// We embed the html file in the binary so that we can edit
// index.html with pretty syntax highlighting.
//
//go:embed index.html
var webPage []byte

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelDebug - 2,
	}))
	time.Sleep(time.Second)
	_, stack, dev, err := common.SetupWithDHCP(common.SetupConfig{
		Hostname: hostname,
		Logger:   logger,
		TCPPorts: 1,
	})
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
	logger.Info("listening", slog.String("addr", "http://"+listenAddr.String()))

	var lastLedState bool
	http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Connection", "close")
		switch r.URL.Path {
		case "/":
			logger.Info("Got webpage request!")
			header.Set("Content-Type", "text/html")
			w.Write(webPage)

		case "/toggle-led":
			logger.Info("Got toggle led request!")
			lastLedState = !lastLedState
			dev.GPIOSet(0, lastLedState)

		default:
			logger.Info("Path not found", slog.String("path", r.URL.Path))
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}
