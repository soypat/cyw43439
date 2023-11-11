package tcpctl

import (
	"errors"
	"io"
	"math"
	"net/netip"

	"github.com/soypat/cyw43439/internal/tcpctl/eth"
	"github.com/soypat/cyw43439/internal/tcpctl/eth/seqs"
)

// tcpController specifies TCP connection state logic to interact with incoming packets
// and send correctly marshalled outgoing packets.
type tcpController struct {
	cs      connState
	ourPort uint16
	us      netip.AddrPort
	them    netip.AddrPort
}

// connState contains the state of a TCP connection likely to change throughout
// the connection's lifetime. This is so mutable state can be kept in one place
// and wrapped in a mutex for safe concurrent access.
type connState struct {
	// # Send Sequence Space
	//
	//	1         2          3          4
	//	----------|----------|----------|----------
	//		   SND.UNA    SND.NXT    SND.UNA
	//								+SND.WND
	//	1. old sequence numbers which have been acknowledged
	//	2. sequence numbers of unacknowledged data
	//	3. sequence numbers allowed for new data transmission
	//	4. future sequence numbers which are not yet allowed
	snd sendSpace
	// # Receive Sequence Space
	//
	//		1          2          3
	//	----------|----------|----------
	//		   RCV.NXT    RCV.NXT
	//					 +RCV.WND
	//	1 - old sequence numbers which have been acknowledged
	//	2 - sequence numbers allowed for new reception
	//	3 - future sequence numbers which are not yet allowed
	rcv             rcvSpace
	pendingCtlFrame eth.TCPFlags
	state           State
}

// sendSpace contains Send Sequence Space data.
type sendSpace struct {
	iss seqs.Value // initial send sequence number, defined on our side on connection start
	UNA seqs.Value // send unacknowledged
	NXT seqs.Value // send next
	WL1 uint32     // segment sequence number used for last window update
	WL2 uint32     // segment acknowledgment number used for last window update
	WND seqs.Size  // send window
	UP  bool       // send urgent pointer (deprecated)
}

// rcvSpace contains Receive Sequence Space data.
type rcvSpace struct {
	irs seqs.Value // initial receive sequence number, defined in SYN segment received
	NXT seqs.Value // receive next
	WND seqs.Size  // receive window
	UP  bool       // receive urgent pointer (deprecated)
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
	err = c.cs.validateHeader(thdr, plen)
	if err != nil {
		return 0, err
	}

	switch c.cs.state {
	case StateListen:
		err = c.cs.rcvListen(thdr, plen)
	case StateSynRcvd:
		err = c.cs.rcvSynRcvd(thdr, plen)
	}
	if err != nil {
		return 0, err
	}

	if c.cs.pendingCtlFrame != 0 {
		// There is a pending CTL packet to send, we make use of this moment to
		// write our CTL response.
		c.setResponseTCP(packet, nil)
		packet.PutHeaders(dst)
		n = payloadOffset
	}

	return n, nil
}

func (cs *connState) validateHeader(hdr *eth.TCPHeader, plen int) (err error) {
	switch {
	case cs.state == StateClosed:
		err = io.ErrClosedPipe
	case hdr.Ack <= cs.snd.UNA:
		err = errors.New("seg.ack > snd.UNA")
	case hdr.Ack > cs.snd.NXT:
		err = errors.New("seg.ack <= snd.NXT")
	}
	return err
}

func (cs *connState) rcvSynRcvd(hdr *eth.TCPHeader, plen int) (err error) {
	cs.snd.UNA = hdr.Ack
	return nil
}

func (cs *connState) rcvListen(hdr *eth.TCPHeader, plen int) (err error) {
	switch {
	case hdr.Flags() != eth.FlagTCP_SYN:
		err = errors.New("expected SYN")
	}
	if err != nil {
		return err
	}

	var iss seqs.Value = 0 // TODO: use random start sequence when done debugging.
	// Initialize connection state:
	cs.snd = sendSpace{
		iss: iss,
		UNA: iss,
		NXT: iss,
		WND: 10,
		// UP, WL1, WL2 defaults to zero values.
	}
	cs.rcv = rcvSpace{
		irs: hdr.Seq,
		NXT: hdr.Seq + 1,
		WND: hdr.WindowSize(),
	}
	// We must respond with SYN|ACK frame after receiving SYN in listen state.
	cs.pendingCtlFrame = eth.FlagTCP_ACK | eth.FlagTCP_SYN
	return nil
}

func (c *tcpController) setResponseTCP(packet *TCPPacket, payload []byte) {
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
	if c.cs.rcv.WND > math.MaxUint16 {
		panic("window overflow")
	}
	// TCP frame.
	const offset = 5
	packet.TCP = eth.TCPHeader{
		SourcePort:      c.us.Port(),
		DestinationPort: c.them.Port(),
		Seq:             c.cs.snd.NXT,
		Ack:             c.cs.rcv.NXT,
		OffsetAndFlags:  [1]uint16{uint16(c.cs.pendingCtlFrame) | uint16(offset)<<12},
		WindowSizeRaw:   uint16(c.cs.rcv.WND),
		UrgentPtr:       0, // We do not implement urgent pointer.
	}
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
