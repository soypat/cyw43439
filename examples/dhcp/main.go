package main

// WARNING: default -scheduler=cores unsupported, compile with -scheduler=tasks set!

import (
	"machine"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/cyw43439/examples/cywnet/credentials"
	"github.com/soypat/lneto/ethernet"
	"github.com/soypat/lneto/ipv4"
)

// Setup Wifi Password and SSID by creating ssid.text and password.text files in
// ../cywnet/credentials/ directory. Credentials are used for examples in this repo.
// When building your own application use local storage to store wifi credentials securely.
var (
	requestedIP = [4]byte{192, 168, 1, 99}
)

func main() {
	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	println("starting program")
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	stack, err := cywnet.NewConfiguredPicoWithStack(credentials.SSID(), credentials.Password(), devcfg, cywnet.StackConfig{
		Hostname: "DHCP-pico",
	})
	if err != nil {
		panic(err)
	}
	// Goroutine loop needed to use the cywnet.StackBlocking implementation.
	// To avoid goroutines use StackAsync. This however means much more effort and boilerplate done by the user.
	go loopForeverStack(stack)

	const (
		timeout = 6 * time.Second
		retries = 3
	)
	llstack := stack.LnetoStack()
	rstack := llstack.StackRetrying(cywnet.DefaultStackBackoff)
	results, err := rstack.DoDHCPv4(requestedIP, timeout, retries)
	if err != nil {
		panic(err)
	}
	// Apply the DHCP-assigned address, subnet and DNS server to the stack.
	// Without this the IP/ARP layers keep their zero address and the device
	// won't answer ARP or ICMP for the assigned IP.
	err = llstack.AssimilateDHCPResults(results)
	if err != nil {
		panic(err)
	}
	gatewayHW, err := rstack.DoResolveHardwareAddress6(results.Router, 500*time.Millisecond, 4)
	if err != nil {
		panic(err)
	}
	llstack.SetGatewayHardwareAddr(gatewayHW)
	logger.Info("DHCP complete",
		slog.String("hostname", stack.Hostname()),
		slog.String("ourIP", ipv4.String(results.AssignedAddr4)),
		slog.String("subnet", results.Subnet.String()),
		slog.String("router", results.Router.String()),
		slog.String("server", results.ServerAddr.String()),
		slog.String("broadcast", results.BroadcastAddr.String()),
		slog.String("gateway", results.Gateway.String()),
		slog.String("gatewayhw", ethernet.String(gatewayHW)),
		slog.Uint64("lease[seconds]", uint64(results.TLease)),
		slog.Uint64("rebind[seconds]", uint64(results.TRebind)),
		slog.Uint64("renew[seconds]", uint64(results.TRenewal)),
		slog.Any("DNS-servers", results.DNSServers),
	)
	stack.Device().GPIOSet(0, true) // LED on pico.
	// Keep main alive. If main returns the program halts. You can reach device via ping.
	select {}
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(5 * time.Millisecond) // No data to send or receive, sleep for a bit.
		}
	}
}
