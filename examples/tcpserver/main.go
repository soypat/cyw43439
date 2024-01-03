package main

import (
	"errors"
	"machine"
	"net"
	"net/netip"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439/examples/common"
	"github.com/soypat/seqs"
	"github.com/soypat/seqs/stacks"
)

var (
	logger *slog.Logger
)

func main() {
	logger = slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Go lower (Debug-1) to see more verbosity on wifi device.
	}))

	time.Sleep(2 * time.Second)
	println("starting program")
	_, stack, _, err := common.SetupWithDHCP(common.SetupConfig{
		Hostname: "TCP-pico",
		Logger:   logger,
		TCPPorts: 1,
	})
	if err != nil {
		panic("in dhcp setup:" + err.Error())
	}
	// Start TCP server.
	const socketBuf = 256
	const listenPort = 1234
	listenAddr := netip.AddrPortFrom(stack.Addr(), listenPort)
	socket, err := stacks.NewTCPConn(stack, stacks.TCPConnConfig{TxBufSize: socketBuf, RxBufSize: socketBuf})
	if err != nil {
		panic("socket create:" + err.Error())
	}
	println("start listening on:", listenAddr.String())
	err = ForeverTCPListenEcho(socket, listenAddr)
	if err != nil {
		panic("socket listen:" + err.Error())
	}
}

func ForeverTCPListenEcho(socket *stacks.TCPConn, addr netip.AddrPort) error {
	var iss seqs.Value = 100
	var buf [512]byte
	for {
		iss += 200
		err := socket.OpenListenTCP(addr.Port(), iss)
		if err != nil {
			return err
		}
		for socket.State().IsPreestablished() {
			time.Sleep(5 * time.Millisecond)
		}
		for {
			n, err := socket.Read(buf[:])
			if errors.Is(err, net.ErrClosed) {
				break
			}
			if n == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			_, err = socket.Write(buf[:n])
			if err != nil {
				return err
			}
		}
		socket.Close()
		socket.FlushOutputBuffer()
		time.Sleep(time.Second)
	}
}
