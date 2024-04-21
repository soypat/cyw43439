package main

import (
	"log/slog"
	"machine"
	"time"

	"github.com/soypat/cyw43439/examples/common"
	"github.com/soypat/netif"
	"github.com/soypat/seqs/eth/dns"
)

const (
	domain      = "google.com"
	dnsTimeout  = 5 * time.Second
	dhcpTimeout = 5 * time.Second
)

func main() {
	println("program start")
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelDebug - 2, // Go lower (Debug-2) to see more verbosity on wifi device + network stack and network engine.
	}))
	println("program start2")
	engine, err := common.SetupWithEngine(netif.EngineConfig{
		Hostname:        "tinygo-netengine-http",
		AddrMethod:      netif.AddrMethodDHCP,
		MaxOpenPortsUDP: 1,
		MaxOpenPortsTCP: 1,
		Logger:          logger,
	})

	if err != nil {
		panic("setup Engine:" + err.Error())
	}

	err = engine.WaitDHCP(dhcpTimeout)
	if err != nil {
		panic("waiting DHCP:" + err.Error())
	}

	resolver := engine.NewResolver(dns.ClientPort, dnsTimeout)
	addrs, err := resolver.LookupNetIP(domain)
	if err != nil {
		panic("resolving google.com:" + err.Error())
	}
	logger.Info("resolved", slog.String("domain", domain), slog.String("addr", addrs[0].String()))

	// a

}
