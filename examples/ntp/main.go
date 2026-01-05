package main

import (
	"log/slog"
	"machine"
	"net/netip"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/cyw43439/examples/cywnet/credentials"
	"github.com/soypat/lneto/ntp"
)

// Setup Wifi Password and SSID by creating ssid.text and password.text files in
// ../cywnet/credentials/ directory. Credentials are used for examples in this repo.

const hostname = "ntp-pico"

var requestedIP = [4]byte{192, 168, 1, 145}

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelDebug,
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
	logger.Info("DHCP complete", slog.String("addr", dhcpResults.AssignedAddr.String()))

	stack := cystack.LnetoStack()
	const pollTime = 5 * time.Millisecond
	rstack := stack.StackRetrying(pollTime)

	// DNS lookup for NTP server.
	const ntpHost = "pool.ntp.org"
	logger.Info("resolving NTP host", slog.String("host", ntpHost))
	addrs, err := rstack.DoLookupIP(ntpHost, 5*time.Second, 3)
	if err != nil {
		panic("DNS lookup failed:" + err.Error())
	}
	logger.Info("DNS resolved", slog.String("addr", addrs[0].String()))

	// Perform NTP request.
	logger.Info("starting NTP request")
	offset, err := rstack.DoNTP(addrs[0], 5*time.Second, 3)
	if err != nil {
		panic("NTP failed:" + err.Error())
	}

	// Calculate corrected time.
	// Using ntp.BaseTime().Add(offset) for Y2K36 compatibility on systems without RTC.
	now := ntp.BaseTime().Add(offset)
	logger.Info("NTP complete",
		slog.String("time", now.String()),
		slog.Duration("offset", offset),
	)
	println("ntp done newtime=", now.String())
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}
