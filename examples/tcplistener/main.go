package main

import (
	"errors"
	"machine"
	"net/netip"
	"os"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439/examples/common"
	"github.com/soypat/seqs/stacks"
)

const (
	tcpbufsize  = 512 // MTU - ethhdr - iphdr - tcphdr
	connTimeout = 5 * time.Second
	// Can help prevent stalling connections from blocking control the more connections you have.
	maxconns = 3
)

var (
	lastRx, lastTx time.Time
	logger         *slog.Logger
)

func main() {
	logger = slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Go lower (Debug-1) to see more verbosity on wifi device.
	}))
	time.Sleep(2 * time.Second)
	logger.Info("starting program")
	_, stack, _, err := common.SetupWithDHCP(common.Config{
		Hostname: "TCP-pico",
		Logger:   logger,
		TCPPorts: 1,
	})
	if err != nil {
		panic("in dhcp setup:" + err.Error())
	}
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
	var buf [512]byte
	logger.Info("listening", slog.String("addr", listenAddr.String()))
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
			logger.Error("conn set deadline:", slog.String("err", err.Error()))
			continue
		}
		for {
			n, err := conn.Read(buf[:])
			if err != nil {
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					logger.Error("conn read:", slog.String("err", err.Error()))
				}
				break
			}
			_, err = conn.Write(buf[:n])
			if err != nil {
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					logger.Error("conn write:", slog.String("err", err.Error()))
				}
				break
			}
		}
		err = conn.Close()
		if err != nil {
			logger.Error("conn close:", slog.String("err", err.Error()))
		}
		time.Sleep(100 * time.Millisecond)
	}
}
