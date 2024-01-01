package main

import (
	"net/netip"
	"time"

	_ "embed"

	"github.com/soypat/seqs/eth/ntp"
	"github.com/soypat/seqs/stacks"
)

const tcpbufsize = 1024 // MTU - ethhdr - iphdr - tcphdr
const hostname = "ntp-pico"

// Run `dig pool.ntp.org` to get a list of NTP servers.
var ntpAddr = netip.AddrFrom4([4]byte{200, 11, 116, 10})

func main() {
	stack, err := setupDHCPStack(hostname, netip.AddrFrom4([4]byte{192, 168, 1, 4}))
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
