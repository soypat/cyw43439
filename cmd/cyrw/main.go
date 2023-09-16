package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
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
	// stack.GlobalHandler = func(b []byte) {
	// 	println("NEW payload:\n", hex.Dump(b))
	// }
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
		offer
		ack
		myXid = 0x12345678

		sizeSName     = 64  // Server name, part of BOOTP too.
		sizeFILE      = 128 // Boot file name, Legacy.
		sizeOptions   = 312
		dhcpOffset    = eth.SizeEthernetHeader + eth.SizeIPv4Header + eth.SizeUDPHeader
		optionsStart  = dhcpOffset + eth.SizeDHCPHeader + sizeSName + sizeFILE
		sizeDHCPTotal = eth.SizeDHCPHeader + sizeSName + sizeFILE + sizeOptions
	)
	state := none

	var dhdr eth.DHCPHeader
	var ehdr eth.EthernetHeader
	var ihdr eth.IPv4Header
	var uhdr eth.UDPHeader
	mac := dev.MAC()
	err := s.OpenUDP(68, func(u *tcpctl.UDPPacket, response []byte) (n int, _ error) {
		payload := u.Payload()
		if payload == nil || len(payload) < eth.SizeDHCPHeader {
			fmt.Printf("\n%+v\n%+v\n", u.IP, u.UDP)
			return 0, errors.New("nil payload")
		}

		dhdr = eth.DecodeDHCPHeader(payload)

		println(time.Since(lastTx).String(), dhdr.String())
		switch {
		case state == none && dhdr.OP == 2 && bytes.Equal(mac, dhdr.CHAddr[:6]) && dhdr.Xid == myXid:
			state = offer
			dhdr.OP = 1
			dhdr.YIAddr, dhdr.CIAddr = [4]byte{}, [4]byte{}
			dhdr.Put(response[dhcpOffset:])
			ptr := optionsStart
			binary.BigEndian.PutUint32(response[ptr:], 0x63825363) // Magic cookie.
			ptr += 4
			// DHCP options.
			ptr += encodeDHCPOption(response[ptr:], 53, []byte{3}) // DHCP Message Type: Request
			ptr += encodeDHCPOption(response[ptr:], 50, dhdr.YIAddr[:])
			ptr += encodeDHCPOption(response[ptr:], 54, dhdr.SIAddr[:])
			response[ptr] = 0xff // endmark
			ptr++

			// IPv4 header remains the same.
			uhdr.Checksum = uhdr.CalculateChecksumIPv4(&ihdr, response[dhcpOffset:])
			uhdr.Put(response[eth.SizeEthernetHeader+eth.SizeIPv4Header:])
			ihdr.Checksum = ihdr.CalculateChecksum()
			ihdr.Put(response[eth.SizeEthernetHeader:])
			ehdr.Put(response[:])
			for i := dhcpOffset + eth.SizeDHCPHeader; i < len(response); i++ {
				response[i] = 0
			}
			binary.BigEndian.PutUint32(response[dhcpOffset+eth.SizeDHCPHeader+4:], 0x63825363) // Magic cookie.
			n = sizeDHCPTotal

		case state == offer && dhdr.OP == 2 && bytes.Equal(mac, dhdr.CHAddr[:6]) && dhdr.Xid == myXid:
			state = ack
			println("\nACK received")
		default:
			return 0, errors.New("unexpected DHCP packet")
		}
		println("\nSENDING ", n, "BYTES\n\n")
		lastTx = time.Now()
		return n, nil
	})
	if err != nil {
		return err
	}
	defer s.CloseUDP(68)

	copy(ehdr.Destination[:], eth.BroadcastHW())
	copy(ehdr.Source[:], dev.MAC())
	ehdr.SizeOrEtherType = uint16(eth.EtherTypeIPv4)

	copy(ihdr.Destination[:], eth.BroadcastHW())
	ihdr.Protocol = 17
	ihdr.TTL = 64

	ihdr.TotalLength = uint16(4*ihdr.IHL()) + eth.SizeUDPHeader + sizeDHCPTotal
	ihdr.ID = 12345
	ihdr.VersionAndIHL = 5 // Sets IHL: No IP options. Version set automatically.

	uhdr.DestinationPort = 67
	uhdr.SourcePort = 68
	uhdr.Length = ihdr.TotalLength - eth.SizeIPv4Header

	dhcppayload := txbuf[eth.SizeEthernetHeader+4*ihdr.IHL()+eth.SizeUDPHeader:]

	dhdr.OP = 1
	dhdr.HType = 1
	dhdr.HLen = 6
	dhdr.Xid = myXid

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

	totalSize := eth.SizeEthernetHeader + int(4*ihdr.IHL()) + eth.SizeUDPHeader + sizeDHCPTotal
	// Calculate Checksums:
	uhdr.CalculateChecksumIPv4(&ihdr, dhcppayload[:ptr])
	udpOffset := eth.SizeEthernetHeader + 4*ihdr.IHL()
	uhdr.Put(txbuf[udpOffset:])
	ihdr.Checksum = ihdr.CalculateChecksum()
	ihdr.Put(txbuf[eth.SizeEthernetHeader:])
	ehdr.Put(txbuf[:])
	lastTx = time.Now()
	err = dev.SendEth(txbuf[:totalSize])
	if err != nil {
		return err
	}
	for retry := 0; retry < 20 && state < ack; retry++ {
		n, err := stack.HandleEth(txbuf[:])
		if err != nil {
			return err
		}
		if n == 0 {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		err = dev.SendEth(txbuf[:n])
		if err != nil {
			return err
		}
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
