package tcpctl

import (
	"net/netip"

	"github.com/soypat/cyw43439/internal/tcpctl/eth"
	"github.com/soypat/cyw43439/internal/tcpctl/eth/seqs"
)

// tcpController specifies TCP connection state logic to interact with incoming packets
// and send correctly marshalled outgoing packets.
type tcpController struct {
	cs      seqs.CtlBlock
	ourPort uint16
	us      netip.AddrPort
	them    netip.AddrPort
}

// handleTCP handles an incoming TCP packet and modifies the corresponding internal state.
// If the output number of bytes written of handleTCP is non-zero then
// a control packet has been written to dst.
// packet is modified to be the data of the packet being sent.
func (c *tcpController) handleTCP(dst []byte, packet *TCPPacket) (n int, err error) {
	const (
		payloadOffset = eth.SizeEthernetHeader + eth.SizeIPv4Header + eth.SizeTCPHeader
	)
	thdr := &packet.TCP
	payload := packet.Payload()

	plen := len(payload)
	seg := seqs.Segment{
		SEQ:     thdr.Seq,
		ACK:     thdr.Ack,
		WND:     thdr.WindowSize(),
		DATALEN: seqs.Size(plen),
		Flags:   thdr.Flags(),
	}
	err = c.cs.Rcv(seg)
	if err != nil {
		return 0, err
	}
	seg = c.cs.PendingSegment(plen)
	if seg.Flags != 0 {
		// There is a pending CTL packet to send, we make use of this moment to
		// write our CTL response.
		c.setResponseTCP(packet, seg, nil)
		packet.PutHeaders(dst)
		n = payloadOffset
	}

	return n, nil
}

func (c *tcpController) setResponseTCP(packet *TCPPacket, seg seqs.Segment, payload []byte) {
	const ipLenInWords = 5
	// Ethernet frame.
	for i := 0; i < 6; i++ {
		// TODO: use actual MAC addresses.
		packet.Eth.Destination[i], packet.Eth.Source[i] = packet.Eth.Source[i], packet.Eth.Destination[i]
	}
	packet.Eth.SizeOrEtherType = uint16(eth.EtherTypeIPv4)

	// IPv4 frame.
	themIP := c.them.Addr().As4()
	usIP := c.us.Addr().As4()
	copy(packet.IP.Destination[:], themIP[:])
	copy(packet.IP.Source[:], usIP[:])
	packet.IP.Protocol = 6 // TCP.
	packet.IP.TTL = 64
	packet.IP.ID = prand16(packet.IP.ID)
	packet.IP.VersionAndIHL = ipLenInWords // Sets IHL: No IP options. Version set automatically.
	packet.IP.TotalLength = 4*ipLenInWords + eth.SizeTCPHeader + uint16(len(payload))
	packet.IP.Checksum = packet.IP.CalculateChecksum()
	// TODO(soypat): Document how to handle ToS. For now just use ToS used by other side.
	// packet.IP.ToS = 0
	packet.IP.Flags = 0
	// TCP frame.
	const offset = 5
	packet.TCP = eth.TCPHeader{
		SourcePort:      c.us.Port(),
		DestinationPort: c.them.Port(),
		Seq:             seg.SEQ,
		Ack:             seg.ACK,
		OffsetAndFlags:  [1]uint16{uint16(offset) << 12},
		WindowSizeRaw:   uint16(seg.WND),
		UrgentPtr:       0, // We do not implement urgent pointer.
	}
	packet.TCP.SetFlags(seg.Flags)
	packet.TCP.Checksum = packet.TCP.CalculateChecksumIPv4(&packet.IP, nil, payload)
}

// prand16 generates a pseudo random number from a seed.
func prand16(seed uint16) uint16 {
	// 16bit Xorshift  https://en.wikipedia.org/wiki/Xorshift
	seed ^= seed << 7
	seed ^= seed >> 9
	seed ^= seed << 8
	return seed
}
