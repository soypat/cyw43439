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
	err = DoDHCP(stack, dev)

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
	var ehdr eth.EthernetHeader
	var ihdr eth.IPv4Header
	var uhdr eth.UDPHeader

	copy(ehdr.Destination[:], eth.BroadcastHW())
	copy(ehdr.Source[:], dev.MAC())
	ehdr.SizeOrEtherType = uint16(eth.EtherTypeIPv4)

	copy(ihdr.Destination[:], eth.BroadcastHW())
	ihdr.Protocol = 17
	ihdr.TTL = 2
	ihdr.TotalLength = eth.SizeUDPHeader + 11*4 + 192 + 21
	ihdr.ID = 12345
	ihdr.VersionAndIHL = 5 // Sets IHL: No IP options. Version set automatically.

	uhdr.DestinationPort = 67
	uhdr.SourcePort = 68
	uhdr.Length = ihdr.TotalLength - eth.SizeIPv4Header
	ehdr.Put(txbuf[:])
	ihdr.Put(txbuf[eth.SizeEthernetHeader:])
	uhdr.Put(txbuf[eth.SizeEthernetHeader+4*ihdr.IHL():])
	dhcppayload := txbuf[eth.SizeEthernetHeader+4*ihdr.IHL()+eth.SizeUDPHeader:]

	dev.SendEth(txbuf[:])
	for retry := 0; retry < 20 && state == none; retry++ {
		time.Sleep(50 * time.Millisecond)
		// We should see received packets received on callback passed into OpenUDP.
	}
	if state == 0 {
		return errors.New("DoDHCP failed")
	}
	return nil
}

// DHCPHeader specifies the first 44 bytes of a DHCP packet payload
// not including BOOTP, magic cookie and options.
type DHCPHeader struct {
	OP     byte        // 0:1
	HType  byte        // 1:2
	HLen   byte        // 2:3
	HOps   byte        // 3:4
	Xid    uint32      // 4:8
	Secs   uint16      // 8:10
	Flags  uint16      // 10:12
	CIAddr [4]byte     // 12:16
	YIAddr [4]byte     // 16:20
	SIAddr [4]byte     // 20:24
	GIAddr [4]byte     // 24:28
	CHAddr [4 * 4]byte // 28:44
	// BOOTP, Magic Cookie, and DHCP Options not included.
}

func (d *DHCPHeader) Put(dst []byte) {
	_ = dst[43]
	dst[0] = d.OP
	dst[1] = d.HType
	dst[2] = d.HLen
	dst[3] = d.HOps
	binary.BigEndian.PutUint32(dst[4:8], d.Xid)
	binary.BigEndian.PutUint16(dst[8:10], d.Secs)
	binary.BigEndian.PutUint16(dst[10:12], d.Flags)
	copy(dst[12:16], d.CIAddr[:])
	copy(dst[16:20], d.YIAddr[:])
	copy(dst[20:24], d.SIAddr[:])
	copy(dst[24:28], d.GIAddr[:])
	copy(dst[28:44], d.CHAddr[:])
}

func DecodeDHCPHeader(src []byte) (d DHCPHeader) {
	_ = src[43]
	d.OP = src[0]
	d.HType = src[1]
	d.HLen = src[2]
	d.HOps = src[3]
	d.Xid = binary.BigEndian.Uint32(src[4:8])
	d.Secs = binary.BigEndian.Uint16(src[8:10])
	d.Flags = binary.BigEndian.Uint16(src[10:12])
	copy(d.CIAddr[:], src[12:16])
	copy(d.YIAddr[:], src[16:20])
	copy(d.SIAddr[:], src[20:24])
	copy(d.GIAddr[:], src[24:28])
	copy(d.CHAddr[:], src[28:44])
	return d
}
