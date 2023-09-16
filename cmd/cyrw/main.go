package main

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
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
		time.Sleep(8 * time.Second)
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

const (
	StateNone = iota
	StateWaitOffer
	StateWaitAck
	StateDone
)

func DoDHCP(s *tcpctl.Stack, dev *cyrw.Device) error {
	var dc DHCPClient
	copy(dc.MAC[:], dev.MAC())
	err := s.OpenUDP(68, dc.HandleUDP)
	if err != nil {
		return err
	}
	defer s.CloseUDP(68)
	err = s.FlagUDPPending(68) // Force a DHCP discovery.
	if err != nil {
		return err
	}
	for retry := 0; retry < 20 && dc.State < 3; retry++ {
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
	if dc.State != StateDone {
		return errors.New("DHCP did not complete, state=" + strconv.Itoa(int(dc.State)))
	}
	return nil
}

type DHCPClient struct {
	ourHeader eth.DHCPHeader
	State     uint8
	MAC       [6]byte
	// The result IP of the DHCP transaction (our new IP).
	YourIP [4]byte
	// DHCP server IP
	ServerIP [4]byte
}

func (d *DHCPClient) HandleUDP(resp []byte, packet *tcpctl.UDPPacket) (_ int, err error) {
	println("HandleUDP called", packet.HasPacket())
	const (
		xid = 0x12345678

		sizeSName     = 64  // Server name, part of BOOTP too.
		sizeFILE      = 128 // Boot file name, Legacy.
		sizeOptions   = 312
		dhcpOffset    = eth.SizeEthernetHeader + eth.SizeIPv4Header + eth.SizeUDPHeader
		optionsStart  = dhcpOffset + eth.SizeDHCPHeader + sizeSName + sizeFILE
		sizeDHCPTotal = eth.SizeDHCPHeader + sizeSName + sizeFILE + sizeOptions
	)
	// First action is used to send data without having received a packet
	// so hasPacket will be false.
	hasPacket := packet.HasPacket()
	incpayload := packet.Payload()
	switch {
	case len(resp) < sizeDHCPTotal:
		return 0, errors.New("short payload to marshall DHCP")
	case hasPacket && len(incpayload) < eth.SizeDHCPHeader:
		return 0, errors.New("short payload to parse DHCP")
	}

	var rcvHdr eth.DHCPHeader
	if hasPacket {
		rcvHdr = eth.DecodeDHCPHeader(incpayload)
		println("DHCP packet received", rcvHdr.String(), "\n", hex.Dump(incpayload))
		ptr := eth.SizeDHCPHeader + sizeSName + sizeFILE + 4
		for ptr+1 < len(incpayload) && int(incpayload[ptr+1]) < len(incpayload) {
			if incpayload[ptr] == 0xff {
				break
			}
			optlen := incpayload[ptr+1]
			// optionData := incpayload[ptr+2 : ptr+2+int(optlen)]
			print("DHCP Option received ", eth.DHCPOption(incpayload[ptr]).String())
			optPtr := ptr + 2
			for optPtr < ptr+2+int(optlen) {
				print(" ", incpayload[optPtr])
				optPtr++
			}
			println()
			ptr += int(optlen) + 2
		}
	}

	// Switch statement prepares DHCP response depending on whether we're waiting
	// for offer, ack or if we still need to send a discover (StateNone).
	type option struct {
		code byte
		data []byte
	}
	var Options []option
	switch {
	case !hasPacket && d.State == StateNone:
		println("sending discover")
		d.initOurHeader(xid)
		// DHCP options.
		Options = []option{
			{53, []byte{1}},               // DHCP Message Type: Discover
			{50, []byte{192, 168, 1, 69}}, // Requested IP
			{55, []byte{1, 3, 15, 6}},     // Parameter request list
		}
		d.State = StateWaitOffer

	case hasPacket && d.State == StateWaitOffer:
		offer := net.IP(rcvHdr.YIAddr[:])
		println("Possible offer received for", offer.String())
		Options = []option{
			{53, []byte{3}},        // DHCP Message Type: Request
			{50, offer},            // Requested IP
			{54, rcvHdr.SIAddr[:]}, // DHCP server IP
		}
		// Accept this server's offer.
		copy(d.ourHeader.SIAddr[:], rcvHdr.SIAddr[:])
		copy(d.YourIP[:], offer) // Store our new IP.
		d.State = StateWaitAck
	default:
		err = fmt.Errorf("UNHANDLED CASE %v %+v", hasPacket, d)
	}
	if err != nil {
		return 0, nil
	}
	for i := dhcpOffset + 14; i < len(resp); i++ {
		resp[i] = 0 // Zero out BOOTP and options fields.
	}
	// Encode DHCP header + options.
	d.ourHeader.Put(resp[dhcpOffset:])

	ptr := optionsStart
	binary.BigEndian.PutUint32(resp[ptr:], 0x63825363) // Magic cookie.
	ptr += 4
	for _, opt := range Options {
		ptr += encodeDHCPOption(resp[ptr:], opt.code, opt.data)
	}
	resp[ptr] = 0xff // endmark
	// Set Ethernet+IP+UDP headers.
	payload := resp[dhcpOffset : dhcpOffset+sizeDHCPTotal]
	d.setResponseUDP(packet, payload)
	packet.PutHeaders(resp)
	return dhcpOffset + sizeDHCPTotal, nil
}

// initOurHeader zero's out most of header and sets the xid and MAC address along with OP=1.
func (d *DHCPClient) initOurHeader(xid uint32) {
	dhdr := &d.ourHeader
	dhdr.OP = 1
	dhdr.HType = 1
	dhdr.HLen = 6
	dhdr.HOps = 0
	dhdr.Secs = 0
	dhdr.Flags = 0
	dhdr.Xid = xid
	dhdr.CIAddr = [4]byte{}
	dhdr.YIAddr = [4]byte{}
	dhdr.SIAddr = [4]byte{}
	dhdr.GIAddr = [4]byte{}
	copy(dhdr.CHAddr[:], d.MAC[:])
}

func (d *DHCPClient) setResponseUDP(packet *tcpctl.UDPPacket, payload []byte) {
	const ipWordLen = 5
	// Ethernet frame.
	copy(packet.Eth.Destination[:], eth.BroadcastHW())
	copy(packet.Eth.Source[:], d.MAC[:])
	packet.Eth.SizeOrEtherType = uint16(eth.EtherTypeIPv4)

	// IPv4 frame.
	copy(packet.IP.Destination[:], eth.BroadcastHW())
	packet.IP.Source = [4]byte{} // Source IP is always zeroed when client sends.
	packet.IP.Protocol = 17      // UDP
	packet.IP.TTL = 64
	// 16bit Xorshift for prandom IP packet ID. https://en.wikipedia.org/wiki/Xorshift
	packet.IP.ID ^= packet.IP.ID << 7
	packet.IP.ID ^= packet.IP.ID >> 9
	packet.IP.ID ^= packet.IP.ID << 8
	packet.IP.VersionAndIHL = ipWordLen // Sets IHL: No IP options. Version set automatically.
	packet.IP.TotalLength = 4*ipWordLen + eth.SizeUDPHeader + uint16(len(payload))
	packet.IP.Checksum = packet.IP.CalculateChecksum()

	// UDP frame.
	packet.UDP.DestinationPort = 67
	packet.UDP.SourcePort = 68
	packet.UDP.Length = packet.IP.TotalLength - 4*ipWordLen
	packet.UDP.Checksum = packet.UDP.CalculateChecksumIPv4(&packet.IP, payload)
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
