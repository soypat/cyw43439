package tcpctl

import (
	"strconv"
	"time"

	"github.com/soypat/cyw43439/internal/tcpctl/eth"
)

type tcpSocket struct {
	LastRx  time.Time
	handler func(response []byte, self *TCPPacket) (int, error)
	Port    uint16
	packets [1]TCPPacket
	ctl     tcpController
}

type TCPPacket struct {
	Rx  time.Time
	Eth eth.EthernetHeader
	IP  eth.IPv4Header
	TCP eth.TCPHeader
	// data contains TCP options and then the actual data.
	data [_MTU - eth.SizeEthernetHeader - eth.SizeIPv4Header - eth.SizeTCPHeader]byte
}

func (p *TCPPacket) String() string {
	return "TCP Packet: " + p.Eth.String() + p.IP.String() + p.TCP.String() + " payload:" + strconv.Quote(string(p.Payload()))
}

// NeedsHandling returns true if the socket needs handling before it can
// admit more pending packets.
func (u *tcpSocket) NeedsHandling() bool {
	// As of now socket has space for 1 packet so if packet is pending, queue is full.
	// Compile time check to ensure this is fulfilled:
	_ = u.packets[1-len(u.packets)]
	return u.IsPendingHandling()
}

// IsPendingHandling returns true if there are packet(s) pending handling.
func (u *tcpSocket) IsPendingHandling() bool {
	return u.Port != 0 && !u.packets[0].Rx.IsZero()
}

// HandleEth writes the socket's response into dst to be sent over an ethernet interface.
// HandleEth can return 0 bytes written and a nil error to indicate no action must be taken.
// If
func (u *tcpSocket) HandleEth(dst []byte) (n int, err error) {
	if u.handler == nil {
		panic("nil udp handler on port " + strconv.Itoa(int(u.Port)))
	}
	packet := &u.packets[0]
	if packet.HasPacket() {
		// The normal case, we've received a packet and need to process it
		// via TCP control logic. The TCP controller can choose to write a
		// control packet to dst or not. We'll know because the packet will
		// will be marked with PSH flag to mark it as non-control packet.
		n, err = u.ctl.handleTCP(dst, packet)
		if packet.TCP.Flags().HasFlags(eth.FlagTCP_PSH) {
			n, err = u.handler(dst, &u.packets[0]) // TODO: I'm not happy with this API.
		}
	} else {
		// If no packet is pending the user has likely flagged they want to send a packet
		n, err = u.handler(dst, &u.packets[0])
	}

	packet.Rx = time.Time{} // Invalidate packet. TODO(soypat): we'll often send more than a single packet...
	return n, err
}

// Open sets the UDP handler and opens the port.
func (u *tcpSocket) Open(port uint16, h func([]byte, *TCPPacket) (int, error)) {
	if port == 0 || h == nil {
		panic("invalid port or nil handler" + strconv.Itoa(int(u.Port)))
	}
	u.handler = h
	u.Port = port
	for i := range u.packets {
		u.packets[i].Rx = time.Time{} // Invalidate packets.
	}
}

func (s *tcpSocket) pending() (p uint32) {
	for i := range s.packets {
		if s.packets[i].HasPacket() {
			p++
		}
	}
	return p
}

func (u *tcpSocket) Close() {
	u.handler = nil
	u.Port = 0 // Port 0 flags the port is inactive.
}

func (u *tcpSocket) forceResponse() (added bool) {
	if !u.IsPendingHandling() {
		added = true
		u.packets[0].Rx = forcedTime
	}
	return added
}

func (u *TCPPacket) HasPacket() bool {
	return u.Rx != forcedTime && !u.Rx.IsZero()
}

// PutHeaders puts the Ethernet, IPv4 and TCP headers into b.
// b must be at least 54 bytes or else PutHeaders panics. No options are marshalled.
func (p *TCPPacket) PutHeaders(b []byte) {
	const minSize = eth.SizeEthernetHeader + eth.SizeIPv4Header + eth.SizeTCPHeader
	if len(b) < minSize {
		panic("short tcpPacket buffer")
	}
	p.Eth.Put(b)
	p.IP.Put(b[eth.SizeEthernetHeader:])
	p.TCP.Put(b[eth.SizeEthernetHeader+eth.SizeIPv4Header:])
}

func (p *TCPPacket) PutHeadersWithOptions(b []byte) error {
	const minSize = eth.SizeEthernetHeader + eth.SizeIPv4Header + eth.SizeTCPHeader
	if len(b) < minSize {
		panic("short tcpPacket buffer")
	}
	panic("PutHeadersWithOptions not implemented")
}

// Payload returns the UDP payload. If UDP or IPv4 header data is incorrect/bad it returns nil.
// If the response is "forced" then payload will be nil.
func (p *TCPPacket) Payload() []byte {
	if !p.HasPacket() {
		return nil
	}
	payloadStart, payloadEnd, _ := p.dataPtrs()
	if payloadStart < 0 {
		return nil // Bad header value
	}
	return p.data[payloadStart:payloadEnd]
}

// Options returns the TCP options in the packet.
func (p *TCPPacket) TCPOptions() []byte {
	if !p.HasPacket() {
		return nil
	}
	payloadStart, _, tcpOptStart := p.dataPtrs()
	if payloadStart < 0 {
		return nil // Bad header value
	}
	return p.data[tcpOptStart:payloadStart]
}

// Options returns the TCP options in the packet.
func (p *TCPPacket) IPOptions() []byte {
	if !p.HasPacket() {
		return nil
	}
	_, _, tcpOpts := p.dataPtrs()
	if tcpOpts < 0 {
		return nil // Bad header value
	}
	return p.data[:tcpOpts]
}

//go:inline
func (p *TCPPacket) dataPtrs() (payloadStart, payloadEnd, tcpOptStart int) {
	tcpOptStart = int(4*p.IP.IHL()) - eth.SizeIPv4Header
	payloadStart = tcpOptStart + int(p.TCP.OffsetInBytes()) - eth.SizeTCPHeader
	payloadEnd = int(p.IP.TotalLength) - tcpOptStart - eth.SizeTCPHeader - eth.SizeIPv4Header
	if payloadStart < 0 || payloadEnd < 0 || tcpOptStart < 0 || payloadStart > payloadEnd ||
		payloadEnd > len(p.data) || tcpOptStart > payloadStart {
		return -1, -1, -1
	}
	return payloadStart, payloadEnd, tcpOptStart
}
