package main

import (
	"log/slog"
	"machine"
	"net"
	"net/netip"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/cyw43439/examples/cywnet/credentials"
	"github.com/soypat/lneto/tcp"
	"github.com/soypat/lneto/x/xnet"
)

// Setup Wifi Password and SSID by creating ssid.text and password.text files in
// ../cywnet/credentials/ directory. Credentials are used for examples in this repo.
// When building your own application use local storage to store wifi credentials securely.
var (
	requestedIP = [4]byte{192, 168, 1, 99}
	ourPort     = uint16(1234)
)

const (
	tcpTimeout = 6 * time.Second
	tcpRetries = 2
	loopSleep  = 5 * time.Millisecond
)

func main() {
	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	println("starting program")
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	cystack, err := cywnet.NewConfiguredPicoWithStack(credentials.SSID(), credentials.Password(), devcfg, cywnet.StackConfig{
		Hostname:    "DHCP-pico",
		MaxTCPConns: 1,
	})
	if err != nil {
		panic(err)
	}

	// Goroutine loop needed to use the cywnet.StackBlocking implementation.
	// To avoid goroutines use StackAsync. This however means much more effort and boilerplate done by the user.
	go loopForeverStack(cystack)

	dhcpResults, err := cystack.SetupWithDHCP(cywnet.DHCPConfig{
		RequestedAddr: netip.AddrFrom4(requestedIP),
	})
	if err != nil {
		panic("while performing DHCP: " + err.Error())
	}
	stack := cystack.LnetoStack()
	gatewayHW := stack.Gateway6()
	println("dhcp addr:", dhcpResults.AssignedAddr.String(), "routerhw:", net.HardwareAddr(gatewayHW[:]).String())
	tcpPool, err := xnet.NewTCPPool(xnet.TCPPoolConfig{
		PoolSize:           4,
		QueueSize:          3,
		BufferSize:         512,
		EstablishedTimeout: 5 * time.Second,
		ClosingTimeout:     5 * time.Second,
	})
	if err != nil {
		panic(err)
	}
	var listener tcp.Listener
	err = listener.Reset(ourPort, tcpPool)
	if err != nil {
		panic(err)
	}
	// atttach listener to stack so as to begin receiving packets.
	err = stack.RegisterListener(&listener)
	if err != nil {
		panic(err)
	}
	println("listening on:", netip.AddrPortFrom(stack.Addr(), ourPort).String())
	for {
		if listener.NumberOfReadyToAccept() == 0 {
			time.Sleep(loopSleep)
			continue
		}
		conn, err := listener.TryAccept()
		if err != nil {
			panic(err)
		}
		go handleConn(conn)
	}
}

func handleConn(conn *tcp.Conn) {
	// Do simple echo of data received.
	var buf [64]byte
	ntot := 0
	addr, _ := netip.AddrFromSlice(conn.RemoteAddr())
	addrs := addr.String()
	println("conn address:", addrs, "port", conn.RemotePort())
	for {
		println("read port", conn.RemotePort())
		n, err := conn.Read(buf[:])
		ntot += n
		if err == net.ErrClosed {
			println("connection closed:", addrs, "written:", ntot)
			return
		} else if n == 0 {
			time.Sleep(loopSleep)
			continue
		}
		println("write port", conn.RemotePort(), "data", n)
		conn.Write(buf[:n])
	}
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(loopSleep) // No data to send or receive, sleep for a bit.
		}
	}
}
