package main

// WARNING: default -scheduler=cores unsupported, compile with -scheduler=tasks set!

import (
	"log/slog"
	"machine"
	"net/netip"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/cyw43439/examples/cywnet/credentials"
	"github.com/soypat/lneto/dns"
	"github.com/soypat/lneto/mdns"
)

// Setup Wifi Password and SSID by creating ssid.text and password.text files in
// ../cywnet/credentials/ directory. Credentials are used for examples in this repo.

const hostname = "mdns-pico"
const remoteHostname = "blade"
const pollTime = 5 * time.Millisecond

var requestedIP = [4]byte{192, 168, 1, 145}

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	println("starting MDNS example")

	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	cystack, err := cywnet.NewConfiguredPicoWithStack(credentials.SSID(), credentials.Password(), devcfg, cywnet.StackConfig{
		Hostname:              hostname,
		MaxUDPPorts:           1,    // MDNS client is external to Stack.
		AcceptMulticast:       true, // Required for MDNS to work.
		EnableRxPacketCapture: true,
		EnableTxPacketCapture: true,
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
	addr := stack.Addr()
	// MDNS for locating local domains.
	var mdnsclient mdns.Client
	multicast := [4]byte{224, 0, 0, 251}
	err = mdnsclient.Configure(mdns.ClientConfig{
		LocalPort:     mdns.Port,
		MulticastAddr: multicast[:],
		Services: []mdns.Service{
			{
				Host: dns.MustNewName(hostname + ".local"),
				Addr: addr.AsSlice(),
			},
		},
	})
	if err != nil {
		panic("MDNS config failed: " + err.Error())
	}
	err = stack.RegisterUDP(&mdnsclient, nil, mdns.Port)
	if err != nil {
		panic("MDNS register failed: " + err.Error())
	}
	err = mdnsclient.StartResolve(mdns.ResolveConfig{
		MaxResponseAnswers: 1,
		Questions: []dns.Question{
			{
				Name:  dns.MustNewName(remoteHostname + ".local"),
				Type:  dns.TypeA,
				Class: dns.ClassINET,
			},
		},
	})
	if err != nil {
		panic("mdns start resolve failed:" + err.Error())
	}
	deadline := time.Now().Add(4 * time.Second)
	var answer [1]dns.Resource
	for {
		n, done, err := mdnsclient.AnswersCopyTo(answer[:])
		if done {
			if err != nil && n == 0 {
				panic("MDNS failed: " + err.Error())
			}
			break
		} else if time.Since(deadline) > 0 {
			panic("MDNS time out")
		}
		time.Sleep(pollTime)
	}

	addr, ok := netip.AddrFromSlice(answer[0].RawData())
	if !ok {
		panic("invalid MDNS address answer")
	}
	println("Address discovered for", remoteHostname, addr.String())
	select {} // keep stack running serving MDNS hostname.
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(pollTime)
		}
	}
}
