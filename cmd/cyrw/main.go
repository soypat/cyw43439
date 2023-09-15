package main

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"

	"github.com/soypat/cyw43439/cyrw"
	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/cyw43439/internal/tcpctl"

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

	for {
		// Set ssid/pass in secrets.go
		err = dev.JoinWPA2(ssid, pass)
		if err == nil {
			break
		}
		println("wifi join failed:", err.Error())
		time.Sleep(5 * time.Second)
	}
	mac := dev.MAC()
	println("\n\n\nMAC:", mac.String())
	stack = tcpctl.NewStack(tcpctl.StackConfig{
		MAC:         nil,
		MaxUDPConns: 2,
	})
	dev.RecvEthHandle(stack.RecvEth)
	for {
		println("Trying DoDHCP")
		err = DoDHCP(stack, dev)
		if err == nil {
			break
		}
		println(err.Error())
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
	stack *tcpctl.Stack
	txbuf [1500]byte
)

func DoDHCP(s *tcpctl.Stack, dev *cyrw.Device) error {
	// States
	const (
		none = iota
		discover
		offer
		request
		ack
	)
	state := none
	err := s.OpenUDP(68, func(u *tcpctl.UDPPacket, b []byte) (int, error) {
		println("UDP payload:", hex.Dump(u.Payload()))
		return 0, nil
	})
	if err != nil {
		return err
	}
	defer s.CloseUDP(68)
	var ehdr eth.EthernetHeader
	var ihdr eth.IPv4Header
	var uhdr eth.UDPHeader
	var dhdr eth.DHCPHeader

	copy(ehdr.Destination[:], eth.BroadcastHW())
	copy(ehdr.Source[:], dev.MAC())
	ehdr.SizeOrEtherType = uint16(eth.EtherTypeIPv4)

	copy(ihdr.Destination[:], eth.BroadcastHW())
	ihdr.Protocol = 17
	ihdr.TTL = 64
	const (
		sizeSName     = 64  // Server name, part of BOOTP too.
		sizeFILE      = 128 // Boot file name, Legacy.
		sizeOptions   = 312
		sizeDHCPTotal = eth.SizeDHCPHeader + sizeSName + sizeFILE + sizeOptions
	)
	ihdr.TotalLength = eth.SizeUDPHeader + sizeDHCPTotal
	ihdr.ID = 12345
	ihdr.VersionAndIHL = 5 // Sets IHL: No IP options. Version set automatically.

	uhdr.DestinationPort = 67
	uhdr.SourcePort = 68
	uhdr.Length = ihdr.TotalLength - eth.SizeIPv4Header
	ehdr.Put(txbuf[:])
	ihdr.Put(txbuf[eth.SizeEthernetHeader:])
	uhdr.Put(txbuf[eth.SizeEthernetHeader+4*ihdr.IHL():])

	dhcppayload := txbuf[eth.SizeEthernetHeader+4*ihdr.IHL()+eth.SizeUDPHeader:]

	dhdr.OP = 1
	dhdr.HType = 1
	dhdr.HLen = 6
	dhdr.Xid = 0x12345678
	mac := dev.MAC()
	copy(dhdr.CHAddr[:], mac[:])
	dhdr.Put(dhcppayload[:])

	// Encode DHCP options.
	for i := eth.SizeDHCPHeader; i < len(dhcppayload); i++ {
		dhcppayload[i] = 0 // Zero out BOOTP and options fields.
	}
	// Skip BOOTP fields.
	ptr := eth.SizeDHCPHeader + sizeSName + sizeFILE
	binary.BigEndian.PutUint32(dhcppayload[ptr:], 0x63825363) // Magic cookie.
	ptr += 4
	// DHCP options.
	ptr += encodeDHCPOption(dhcppayload[ptr:], 53, []byte{1})               // DHCP Message Type: Discover
	ptr += encodeDHCPOption(dhcppayload[ptr:], 50, []byte{192, 168, 1, 69}) // Requested IP
	ptr += encodeDHCPOption(dhcppayload[ptr:], 55, []byte{1, 3, 15, 6})     // Parameter request list
	dhcppayload[ptr] = 0xff                                                 // endmark
	ptr++

	const typicalSize = eth.SizeEthernetHeader + eth.SizeIPv4Header + eth.SizeUDPHeader + sizeDHCPTotal

	totalSize := eth.SizeEthernetHeader + int(4*ihdr.IHL()) + eth.SizeUDPHeader + sizeDHCPTotal
	err = dev.SendEth(txbuf[:totalSize])
	if err != nil {
		return err
	}
	for retry := 0; retry < 20 && state == none; retry++ {
		time.Sleep(50 * time.Millisecond)
		// We should see received packets received on callback passed into OpenUDP.
	}
	if state < ack {
		return errors.New("DoDHCP failed")
	}
	return nil
}

func encodeDHCPOption(dst []byte, code byte, data []byte) int {
	if len(data)+2 > len(dst) {
		panic("small dst size for DHCP encoding")
	}
	dst[0] = code
	dst[1] = byte(len(data))
	copy(dst[2:], data)
	return 2 + len(data)
}
