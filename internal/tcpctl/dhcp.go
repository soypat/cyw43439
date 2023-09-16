package tcpctl

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"github.com/soypat/cyw43439/internal/tcpctl/eth"
)

type DHCPClient struct {
	ourHeader eth.DHCPHeader
	State     uint8
	MAC       [6]byte
	// The result IP of the DHCP transaction (our new IP).
	YourIP [4]byte
	// DHCP server IP
	ServerIP [4]byte
}

const (
	dhcpStateNone = iota
	dhcpStateWaitOffer
	dhcpStateWaitAck
	dhcpStateDone
)

func (d *DHCPClient) HandleUDP(resp []byte, packet *UDPPacket) (_ int, err error) {
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
		ptr := eth.SizeDHCPHeader + sizeSName + sizeFILE + 4
		for ptr+1 < len(incpayload) && int(incpayload[ptr+1]) < len(incpayload) {
			if incpayload[ptr] == 0xff {
				break
			}
			option := eth.DHCPOption(incpayload[ptr])
			optlen := incpayload[ptr+1]
			// optionData := incpayload[ptr+2 : ptr+2+int(optlen)]

			// print("DHCP Option received ", option.String())
			optionData := incpayload[ptr+2 : ptr+2+int(optlen)]
			if d.State == dhcpStateWaitAck && option == eth.DHCPMessageType && len(optionData) > 0 && optionData[0] == 5 {
				d.State = dhcpStateDone
				return 0, nil
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
	case !hasPacket && d.State == dhcpStateNone:
		d.initOurHeader(xid)
		// DHCP options.
		Options = []option{
			{53, []byte{1}},               // DHCP Message Type: Discover
			{50, []byte{192, 168, 1, 69}}, // Requested IP
			{55, []byte{1, 3, 15, 6}},     // Parameter request list
		}
		d.State = dhcpStateWaitOffer

	case hasPacket && d.State == dhcpStateWaitOffer:
		offer := net.IP(rcvHdr.YIAddr[:])
		Options = []option{
			{53, []byte{3}},        // DHCP Message Type: Request
			{50, offer},            // Requested IP
			{54, rcvHdr.SIAddr[:]}, // DHCP server IP
		}
		// Accept this server's offer.
		copy(d.ourHeader.SIAddr[:], rcvHdr.SIAddr[:])
		copy(d.YourIP[:], offer) // Store our new IP.
		d.State = dhcpStateWaitAck
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

func (d *DHCPClient) setResponseUDP(packet *UDPPacket, payload []byte) {
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
