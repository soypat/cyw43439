package main

// WARNING: default -scheduler=cores unsupported, compile with -scheduler=tasks set!

import (
	"log/slog"
	"machine"
	"net/netip"
	"runtime"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/cyw43439/examples/cywnet/credentials"
	"github.com/soypat/lneto/dns"
	"github.com/soypat/lneto/ipv4"
)

// Setup Wifi Password and SSID by creating ssid.text and password.text files in
// ../cywnet/credentials/ directory. Credentials are used for examples in this repo.

const hostname = "ntp-pico"
const ntpHost = "pool.ntp.org"
const pollTime = 5 * time.Millisecond

var ntpName = dns.MustNewName(ntpHost)
var requestedIP = [4]byte{192, 168, 1, 145}

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	println("starting NTP example")

	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	cystack, err := cywnet.NewConfiguredPicoWithStack(credentials.SSID(), credentials.Password(), devcfg, cywnet.StackConfig{
		Hostname: hostname,
	})
	if err != nil {
		panic("setup failed:" + err.Error())
	}

	// Background loop needed to process packets.
	go loopForeverStack(cystack)

	dhcpResults, err := cystack.SetupWithDHCP(cywnet.DHCPConfig{
		RequestedAddr: netip.AddrFrom4(requestedIP),
	})
	if err != nil {
		panic("DHCP failed:" + err.Error())
	}
	logger.Info("DHCP complete", slog.String("addr", ipv4.String(dhcpResults.AssignedAddr4)))

	stack := cystack.StackAsync()
	rstack := stack.StackRetrying(cywnet.DefaultStackBackoff)
	// DNS lookup for NTP server.
	logger.Info("resolving NTP host", slog.String("host", ntpHost), slog.Any("dnssv", dhcpResults.DNSServers))
	var addrs [1]netip.Addr // we only get first address.
	n, err := rstack.DoLookupIP(addrs[:], ntpName, 5*time.Second, 3)
	if err != nil && n == 0 { // Do not fail on address written! We only need 1 address.
		panic("DNS lookup failed:" + err.Error())
	}
	logger.Info("DNS resolved", slog.String("addr", addrs[0].String()))

	// Perform NTP request.
	logger.Info("starting NTP request")
	offset, err := rstack.DoNTP(addrs[0], 5*time.Second, 3)
	if err != nil {
		panic("NTP failed:" + err.Error())
	}

	// Calculate corrected time (server time).
	now := time.Now().Add(offset)
	logger.Info("NTP complete",
		slog.String("time", now.String()),
		slog.Duration("offset", offset),
	)
	println("ntp done newtime=", now.String())
	runtime.AdjustTimeOffset(int64(offset))
	logger.Info("time-update")
	const timeLayout = "Jan _2 15:04:05 MST" // Will be UTC.
	var timebuf [len(timeLayout)]byte
	for {
		nowUTC := time.Now()
		for _, zone := range zones {
			t := nowUTC.In(zone.Zone)
			buf := t.AppendFormat(timebuf[:], timeLayout)
			println(string(buf), zone.City)
		}
		println("------------\n")
		time.Sleep(5 * time.Second)
	}
}

var zones = []struct {
	City string
	Zone *time.Location
}{
	{City: "La Plata", Zone: time.FixedZone("ART", -3*3600)},
	{City: "Hanoi", Zone: time.FixedZone("ICT", 7*3600)},
	{City: "Oviedo", Zone: time.FixedZone("CET", 1*3600)},
	{City: "Ouagadougou", Zone: time.FixedZone("GMT", 0)},
	{City: "Seattle", Zone: time.FixedZone("PST", -8*3600)},
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(pollTime)
		}
	}
}
