package main

import (
	"machine"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/cyw43439/examples/cywnet/credentials"
)

// Setup Wifi Password and SSID in common/secrets.go
var (
	stack       cywnet.StackAsync
	requestedIP = [4]byte{192, 168, 1, 99}
)

func main() {

	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	err := stack.Reset(cywnet.StackConfig{
		Hostname: "DHCP-pico",
		RandSeed: uint32(time.Now().UnixMicro()), // Not terribly random.
		MTU:      cyw43439.MTU,
	})
	if err != nil {
		panic(err)
	}
	println("starting program")

	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	dev, err := stack.SetupPicoWifi(credentials.SSID(), credentials.Password(), devcfg)
	if err != nil {
		panic(err)
	}
	go loopForeverStack(dev, &stack)
	const (
		timeout = 6 * time.Second
		retries = 3
	)
	rstack := stack.StackRetrying()
	results, err := rstack.DoDHCPv4(requestedIP, timeout, retries)
	if err != nil {
		panic(err)
	}
	logger.Info("DHCP complete",
		slog.String("hostname", stack.Hostname()),
		slog.String("ourIP", results.AssignedAddr.String()),
		slog.String("subnet", results.Subnet.String()),
		slog.String("router", results.Router.String()),
		slog.String("server", results.ServerAddr.String()),
		slog.String("broadcast", results.BroadcastAddr.String()),
		slog.String("gateway", results.Gateway.String()),
		slog.Uint64("lease[seconds]", uint64(results.TLease)),
		slog.Uint64("rebind[seconds]", uint64(results.TRebind)),
		slog.Uint64("renew[seconds]", uint64(results.TRenewal)),
		slog.Any("DNS-servers", results.DNSServers),
	)
}

func loopForeverStack(dev *cyw43439.Device, stack *cywnet.StackAsync) {
	for {
		send, recv, _ := stack.RecvAndSend(dev, nil)
		if send == 0 && recv == 0 {
			time.Sleep(5 * time.Millisecond) // No data to send or receive, sleep for a bit.
		}
	}
}
