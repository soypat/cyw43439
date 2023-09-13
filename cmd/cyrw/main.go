package main

import (
	"encoding/hex"
	"errors"
	"time"

	"github.com/soypat/cyw43439/cyrw"
	"github.com/soypat/cyw43439/internal/slog"

	"github.com/soypat/cyw43439/internal/tcpctl/eth"
)

var lastRx, lastTx time.Time

func main() {
	defer func() {
		println("program finished")
		if a := recover(); a != nil {
			println("panic:", a)
		}
	}()
	// handler := slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{Level: slog.LevelDebug})
	// slog.SetDefault(slog.New(handler))

	time.Sleep(2 * time.Second)
	println("starting program")
	slog.Debug("starting program")
	dev := cyrw.DefaultNew()

	err := dev.Init(cyrw.DefaultConfig())
	if err != nil {
		panic(err)
	}

	dev.RecvEthHandle(rcv)

	for {
		// Set ssid/pass in secrets.go
		err = dev.JoinWPA2(ssid, pass)
		if err == nil {
			break
		}
		println("wifi join failed:", err.Error())
		time.Sleep(5 * time.Second)
	}

	println("finished init OK")

	const refresh = 300 * time.Millisecond
	lastLED := false
	for {
		recentRx := time.Since(lastRx) < refresh*3/2
		recentTx := time.Since(lastTx) < refresh*3/2
		ledStatus := recentRx || recentTx
		if ledStatus != lastLED {
			dev.GPIOSet(0, ledStatus)
			lastLED = ledStatus
		}
		time.Sleep(refresh)
	}
}

var (
	errNotTCP     = errors.New("packet not TCP")
	errNotIPv4    = errors.New("packet not IPv4")
	errPacketSmol = errors.New("packet too small")
)

func rcv(pkt []byte) error {
	// Note: rcv is called from a locked Device context.
	// No calls to device I/O should be performed here.
	lastRx = time.Now()
	if len(pkt) < 14 {
		return errPacketSmol
	}
	ethHdr := eth.DecodeEthernetHeader(pkt)
	if ethHdr.AssertType() != eth.EtherTypeIPv4 {
		return errNotIPv4
	}
	ipHdr := eth.DecodeIPv4Header(pkt[eth.SizeEthernetHeaderNoVLAN:])
	println("ETH:", ethHdr.String())
	println("IPv4:", ipHdr.String())
	println("Rx:", len(pkt))
	println(hex.Dump(pkt))
	if ipHdr.Protocol == 17 {
		// We got an UDP packet and we validate it.
		udpHdr := eth.DecodeUDPHeader(pkt[eth.SizeEthernetHeaderNoVLAN+eth.SizeIPv4Header:])
		gotChecksum := udpHdr.CalculateChecksumIPv4(&ipHdr, pkt[eth.SizeEthernetHeaderNoVLAN+eth.SizeIPv4Header+eth.SizeUDPHeader:])
		println("UDP:", udpHdr.String())
		if gotChecksum == 0 || gotChecksum == udpHdr.Checksum {
			println("checksum match!")
		} else {
			println("checksum mismatch! Received ", udpHdr.Checksum, " but calculated ", gotChecksum)
		}
		return nil
	}
	if ipHdr.Protocol != 6 {
		return errNotTCP
	}
	tcpHdr := eth.DecodeTCPHeader(pkt[eth.SizeEthernetHeaderNoVLAN+eth.SizeIPv4Header:])
	println("TCP:", tcpHdr.String())

	return nil
}
