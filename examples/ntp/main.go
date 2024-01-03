package main

import (
	"log/slog"
	"machine"
	"time"

	_ "embed"

	"github.com/soypat/cyw43439/examples/common"
	"github.com/soypat/seqs/eth/ntp"
	"github.com/soypat/seqs/stacks"
)

// Setup Wifi Password and SSID in common/secrets.go

const hostname = "ntp-pico"

// Run `dig pool.ntp.org` to get a list of NTP servers.

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	time.Sleep(100 * time.Millisecond)
	dhcpc, stack, _, err := common.SetupWithDHCP(common.SetupConfig{
		Hostname:    hostname,
		RequestedIP: "192.168.1.145",
		Logger:      logger,
		UDPPorts:    2,
	})
	if err != nil {
		panic("setup failed:" + err.Error())
	}

	routerhw, err := common.ResolveHardwareAddr(stack, dhcpc.Router())
	if err != nil {
		panic("router hwaddr resolving:" + err.Error())
	}

	resolver, err := common.NewResolver(stack, dhcpc)
	if err != nil {
		panic("resolver create:" + err.Error())
	}
	addrs, err := resolver.LookupNetIP("pool.ntp.org")
	if err != nil {
		panic("DNS lookup failed:" + err.Error())
	}

	ntpaddr := addrs[0]
	ntpc := stacks.NewNTPClient(stack, ntp.ClientPort)
	err = ntpc.BeginDefaultRequest(routerhw, ntpaddr)
	if err != nil {
		panic("NTP create:" + err.Error())
	}
	for !ntpc.IsDone() {
		time.Sleep(time.Second)
		println("still ntping")
	}
	now := time.Now()
	print("ntp done oldtime=", now.String(), " newtime=", now.Add(ntpc.Offset()).String())
}
