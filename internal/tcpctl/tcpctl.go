package tcpctl

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net"

	"github.com/soypat/cyw43439/internal/tcpctl/eth"
)

// State enumerates states a TCP connection progresses through during its lifetime.
//
//go:generate stringer -type=State -trimprefix=State
type State uint8

const (
	// CLOSED - represents no connection state at all.
	StateClosed State = iota
	// LISTEN - represents waiting for a connection request from any remote TCP and port.
	StateListen
	// SYN-RECEIVED - represents waiting for a confirming connection request acknowledgment
	// after having both received and sent a connection request.
	StateSynRcvd
	// SYN-SENT - represents waiting for a matching connection request after having sent a connection request.
	StateSynSent
	// ESTABLISHED - represents an open connection, data received can be delivered
	// to the user.  The normal state for the data transfer phase of the connection.
	StateEstablished
	// FIN-WAIT-1 - represents waiting for a connection termination request
	// from the remote TCP, or an acknowledgment of the connection
	// termination request previously sent.
	StateFinWait1
	// FIN-WAIT-2 - represents waiting for a connection termination request
	// from the remote TCP.
	StateFinWait2
	// CLOSING - represents waiting for a connection termination request
	// acknowledgment from the remote TCP.
	StateClosing
	// TIME-WAIT - represents waiting for enough time to pass to be sure the remote
	// TCP received the acknowledgment of its connection termination request.
	StateTimeWait
	// CLOSE-WAIT - represents waiting for a connection termination request
	// from the local user.
	StateCloseWait
	// LAST-ACK - represents waiting for an acknowledgment of the
	// connection termination request previously sent to the remote TCP
	// (which includes an acknowledgment of its connection termination request).
	StateLastAck
)

type Socket struct {
	cs        connState
	us        net.TCPAddr
	them      net.TCPAddr
	staticBuf [1504]byte
}

func (s *Socket) Listen() {
	s.cs.SetState(StateListen)
}

func (s *Socket) RecvEthernet(buf []byte) (payloadStart, payloadEnd uint16, err error) {
	buflen := uint16(len(buf))
	switch {
	case len(buf) > math.MaxUint16:
		err = errors.New("buffer too long")
	case buflen < eth.SizeEthernetHeaderNoVLAN+eth.SizeIPv4Header+eth.SizeTCPHeaderNoOptions:
		err = errors.New("buffer too short to contain TCP")

	}
	if err != nil {
		return 0, 0, err
	}
	ethhdr := eth.DecodeEthernetHeader(buf)
	if ethhdr.IsVLAN() {
		return 0, 0, errors.New("VLAN not supported")
	}
	if ethhdr.SizeOrEtherType != uint16(eth.EtherTypeIPv4) {
		return 0, 0, errors.New("support only IPv4")
	}
	payloadStart, payloadEnd, err = s.RecvTCP(buf[eth.SizeEthernetHeaderNoVLAN:])
	if err != nil {
		return 0, 0, err
	}
	return payloadStart + eth.SizeEthernetHeaderNoVLAN, payloadEnd + eth.SizeEthernetHeaderNoVLAN, nil
}

func (s *Socket) RecvTCP(buf []byte) (payloadStart, payloadEnd uint16, err error) {
	buflen := uint16(len(buf))
	ip := eth.DecodeIPv4Header(buf[:])
	payloadEnd = ip.TotalLength
	if payloadEnd > buflen {
		return 0, 0, fmt.Errorf("IP.TotalLength exceeds buffer size %d/%d", payloadEnd, buflen)
	}
	if ip.Protocol != 6 { // Ensure TCP protocol.
		// fmt.Printf("%+v\n%s\n", ip, ip.String())
		return 0, 0, fmt.Errorf("expected TCP protocol (6) in IP.Proto field; got %d", ip.Protocol)
	}
	if ip.IHL != 0 {
		return 0, 0, errors.New("expected IP.IHL to be zero")
	}
	tcp := eth.DecodeTCPHeader(buf[eth.SizeIPv4Header:])
	nb := tcp.OffsetInBytes()
	if nb < 20 {
		return 0, 0, errors.New("garbage TCP.Offset")
	}
	payloadStart = nb + eth.SizeIPv4Header
	if payloadStart > buflen {
		return 0, 0, fmt.Errorf("malformed packet, got payload offset %d/%d", payloadStart, buflen)
	}
	err = s.rx(&tcp)
	if err != nil {
		return 0, 0, err
	}
	if s.cs.pendingCtlFrame == 0 {
		return payloadStart, payloadEnd, nil
	}
	tcpOptions := buf[eth.SizeIPv4Header+eth.SizeTCPHeaderNoOptions : payloadStart]
	gotSum := tcp.CalculateChecksumIPv4(&ip, tcpOptions, buf[payloadStart:payloadEnd])
	if gotSum != tcp.Checksum {
		fmt.Println("Checksum mismatch!")
	}
	n, err := s.writeTCPIPv4(s.staticBuf[:], nil, nil)
	if err != nil {
		return 0, 0, err
	}
	fmt.Printf("[success] Wrote %d bytes: %q\n\n", n, s.staticBuf[:n])
	return payloadStart, payloadEnd, err
}

func (s *Socket) rx(hdr *eth.TCPHeader) (err error) {
	s.cs.mu.Lock()
	defer s.cs.mu.Unlock()
	switch s.cs.state {
	case StateClosed:
		// Ignore packet.
		err = errors.New("connection closed")
	case StateListen:
		if hdr.Flags() != eth.FlagTCP_SYN {
			return //
		}
		var iss uint32 = 0 // TODO: use random start sequence when done debugging.
		fmt.Println("SYN received!")
		// Initialize connection state:
		s.cs.snd = sendSpace{
			iss: iss,
			UNA: iss,
			NXT: iss,
			WND: 10,
			// UP, WL1, WL2 defaults to zero values.
		}
		s.cs.rcv = rcvSpace{
			irs: hdr.Seq,
			NXT: hdr.Seq + 1,
			WND: hdr.WindowSize,
		}
		// We must respond with SYN|ACK frame after receiving SYN in listen state.
		s.cs.pendingCtlFrame = eth.FlagTCP_ACK | eth.FlagTCP_SYN

	case StateSynRcvd:
		// Handle SynRcvd state.
		s.cs.snd.UNA = hdr.Ack

	default:
		err = errors.New("[ERR] unhandled state transition:" + s.cs.state.String())
		fmt.Println("[ERR] unhandled state transition:" + s.cs.state.String())
	}
	return err
}

// writeTCPIPv4 writes a TCP+IPv4 packet to dst, returning the number of bytes written.
func (s *Socket) writeTCPIPv4(dst, tcpOpts, payload []byte) (n int, err error) {
	if len(dst) > math.MaxUint16 {
		return 0, errors.New("buffer too long for TCP/IP")
	}
	// Exclude Ethernet header and CRC in frame size.
	payloadOffset := len(tcpOpts) + eth.SizeIPv4Header + eth.SizeTCPHeaderNoOptions
	if len(dst) < payloadOffset+len(payload) {
		return 0, io.ErrShortBuffer
	}
	// Limit dst to the size of the frame.
	dst = dst[:payloadOffset+len(payload)]

	offsetBytes := len(tcpOpts) + eth.SizeTCPHeaderNoOptions
	offset := offsetBytes / 4
	if offsetBytes%4 != 0 {
		offset++
	}
	if offset > 0b1111 {
		return 0, errors.New("TCP options too large")
	}
	payloadOffset = eth.SizeIPv4Header + offset*4
	ip := eth.IPv4Header{
		Version:     4,
		IHL:         eth.SizeIPv4Header / 4,
		TotalLength: uint16(offsetBytes+len(payload)) + eth.SizeIPv4Header + eth.SizeTCPHeaderNoOptions,
		ID:          0,
		Flags:       0,
		TTL:         255,
		Protocol:    6, // 6 == TCP.
	}
	copy(ip.Destination[:], s.them.IP)
	copy(ip.Source[:], s.us.IP)
	tcp := eth.TCPHeader{
		SourcePort:      s.us.AddrPort().Port(),
		DestinationPort: s.them.AddrPort().Port(),
		Seq:             s.cs.snd.NXT,
		Ack:             s.cs.rcv.NXT,
		OffsetAndFlags:  [1]uint16{uint16(s.cs.pendingCtlFrame) | uint16(offset)<<12},
		WindowSize:      s.cs.rcv.WND,
		UrgentPtr:       0, // We do not implement urgent pointer.
	}
	// Calculate TCP checksum.
	tcp.Checksum = tcp.CalculateChecksumIPv4(&ip, tcpOpts, payload)
	// Copy TCP header+options and payload into buffer.
	tcp.Put(dst[eth.SizeIPv4Header:])
	nopt := copy(dst[eth.SizeIPv4Header+eth.SizeTCPHeaderNoOptions:payloadOffset], tcpOpts)
	if nopt != len(tcpOpts) {
		panic("tcp options copy failed")
	}
	copy(dst[payloadOffset:], payload)
	// Calculate IP checksum and copy IP header into buffer.
	crc := eth.CRC791{}
	crc.Write(dst[eth.SizeIPv4Header:]) // We limited dst size above.
	ip.Checksum = crc.Sum()
	ip.Put(dst)
	return len(dst), nil
}
