package main

import (
	"errors"
	"io"
	"machine"
	"net"
	"net/netip"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/cyw43439/examples/cywnet/credentials"
	"github.com/soypat/lneto/tcp"
)

// Note: Try running the dhcp example before this one!

// Setup Wifi Password and SSID by creating ssid.text and password.text files in
// ../cywnet/credentials/ directory. Credentials are used for examples in this repo.
// When building your own application use local storage to store wifi credentials securely.
var (
	requestedIP = [4]byte{192, 168, 1, 99}
	targetIP    = [4]byte{192, 168, 1, 53}
	targetPort  = uint16(8080)
)

const (
	tcpTimeout = 6 * time.Second
	tcpRetries = 2
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
		MaxTCPPorts: 1,
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
	var buf [512]byte
	var conn tcp.Conn
	err = conn.Configure(tcp.ConnConfig{
		RxBuf:             make([]byte, 512),
		TxBuf:             make([]byte, 512),
		TxPacketQueueSize: 3,
	})
	if err != nil {
		panic(err)
	}
	targetIPPort := netip.AddrPortFrom(netip.AddrFrom4(targetIP), targetPort)
	const pollTime = 5 * time.Millisecond
	rstack := stack.StackRetrying(pollTime)
	for {
		lport := uint16(stack.Prand32()>>17) + 1 // Ensure non-zero local port.
		println("attempting TCP connection with port", lport)
		err := rstack.DoDialTCP(&conn, lport, targetIPPort, tcpTimeout, tcpRetries)
		if err != nil {
			conn.Close()
			println("failed TCP:", err.Error())
			time.Sleep(2 * time.Second)
			conn.Abort()
			continue
		}

		for {
			n, err := conn.Read(buf[:])
			if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
				break
			} else if n == 0 {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			_, err = conn.Write(buf[:n])
			if err != nil {
				panic(err)
			}
		}
		err = conn.Close()
		if err != nil {
			println("close failed:", err.Error())
		}
		// Give some time for connection to perform TCP close actions.
		for i := 0; i < 20 && !conn.State().IsClosed(); i++ {
			time.Sleep(5 * time.Millisecond)
		}
		println("abort conn.")
		conn.Abort() // Completely annihilate connection state even if not done by now.
		time.Sleep(time.Second)
	}
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(5 * time.Millisecond) // No data to send or receive, sleep for a bit.
		}
	}
}
