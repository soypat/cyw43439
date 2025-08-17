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

func main() {
	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	println("starting program")
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	cystack, err := cywnet.NewConfiguredPicoWithStack(credentials.SSID(), credentials.Password(), devcfg, cywnet.StackConfig{
		Hostname: "DHCP-pico",
	})
	if err != nil {
		panic(err)
	}

	// Goroutine loop needed to use the cywnet.StackBlocking implementation.
	// To avoid goroutines use StackAsync. This however means much more effort and boilerplate done by the user.
	go loopForeverStack(cystack)

	const (
		timeout = 6 * time.Second
		retries = 2
	)
	stack := cystack.LnetoStack()
	rstack := stack.StackRetrying()
	dhcpResults, err := rstack.DoDHCPv4(requestedIP, timeout, retries)
	if err != nil {
		panic(err)
	}
	err = stack.AssimilateDHCPResults(dhcpResults)
	if err != nil {
		panic(err)
	}
	println("dhcp addr:", dhcpResults.AssignedAddr.String())
	var buf [512]byte

	for {
		lport := uint16(stack.Prand32()>>17) + 1 // Ensure non-zero local port.
		println("attempting TCP connection with port", lport)
		conn, err := rstack.DoDialTCP(lport, netip.AddrPortFrom(netip.AddrFrom4(targetIP), targetPort), timeout, retries)
		if err != nil {
			println("failed TCP:", err.Error())
			time.Sleep(2 * time.Second)
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
		conn.Close()
		// Give some time for connection to perform TCP close actions.
		for i := 0; i < 20 && !conn.State().IsClosed(); i++ {
			time.Sleep(5 * time.Millisecond)
		}
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
