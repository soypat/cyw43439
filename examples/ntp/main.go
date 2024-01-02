package main

import (
	"log/slog"
	"machine"
	"net/netip"
	"time"

	_ "embed"

	"github.com/soypat/cyw43439/examples/common"
	"github.com/soypat/seqs/eth/ntp"
	"github.com/soypat/seqs/stacks"
)

// Setup Wifi Password and SSID in common/secrets.go

const hostname = "ntp-pico"

// Run `dig pool.ntp.org` to get a list of NTP servers.
var ntpAddr = netip.AddrFrom4([4]byte{200, 11, 116, 10})

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	_, stack, _, err := common.SetupWithDHCP(common.Config{
		Hostname:    hostname,
		RequestedIP: "192.168.1.145",
		Logger:      logger,
		UDPPorts:    1,
	})
	if err != nil {
		panic("listener create:" + err.Error())
	}
	ntpc := stacks.NewNTPClient(stack, ntp.ClientPort)
	err = ntpc.BeginDefaultRequest(ntpAddr)
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
