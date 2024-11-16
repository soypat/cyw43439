package main

import (
	"errors"
	"io"
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
	dhcpc, stack, _, err := common.SetupWithDHCP(common.SetupConfig{
		Hostname: "TCP-pico",
		Logger:   logger,
		TCPPorts: 1,
	})
	if err != nil {
		panic("in dhcp setup:" + err.Error())
	}
	// Instantiate TCP Client with a target address to reach.
	const socketBuf = 256
	const listenPort = 1234
	targetAddr := netip.AddrPortFrom(stack.Addr(), listenPort)
	socket, err := stacks.NewTCPConn(stack, stacks.TCPConnConfig{TxBufSize: socketBuf, RxBufSize: socketBuf})
	if err != nil {
		panic("socket create:" + err.Error())
	}

	// Get target MAC Address.
	err = stack.ARP().BeginResolve(dhcpc.Router())
	if err != nil {
		panic(err)
	}
	targetMAC, err := common.ResolveHardwareAddr(stack, dhcpc.Router())
	if err != nil {
		panic(err)
	}
	println("start connecting to:", targetAddr.String())
	err = ForeverTCPSend(socket, targetAddr, targetMAC)
	if err != nil {
		panic("socket listen:" + err.Error())
	}
}

func ForeverTCPSend(socket *stacks.TCPConn, remoteAddr netip.AddrPort, remoteMAC [6]byte) error {
	var iss seqs.Value = 100 // Starting TCP sequence number.
	var localPort uint16 = 1
	var buf [512]byte
	for {
		localPort += 1
		if localPort == 0 {
			localPort = 1 // Prevent crash on wraparound.
		}
		iss += 200
		err := socket.OpenDialTCP(localPort, remoteMAC, remoteAddr, iss) // Attempt to reach server.
		if err != nil {
			return err
		}
		for socket.State().IsPreestablished() {
			time.Sleep(5 * time.Millisecond)
		}
		for {
			n, err := socket.Read(buf[:])
			if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
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
