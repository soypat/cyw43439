package tcpctl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/cyw43439/internal/tcpctl/eth"
)

const (
	_MTU = 1500
)

type StackConfig struct {
	MAC         net.HardwareAddr
	IP          net.IP
	MaxUDPConns int
}

// NewStack creates a ready to use TCP/UDP Stack instance.
func NewStack(cfg StackConfig) *Stack {
	var s Stack
	s.MAC = cfg.MAC
	s.IP = cfg.IP
	s.UDPv4 = make([]udpSocket, cfg.MaxUDPConns)
	return &s
}

// Stack is a TCP/UDP netlink implementation for muxing packets received into
// their respective sockets with [Stack.RcvEth].
type Stack struct {
	lastRx        time.Time
	lastRxSuccess time.Time
	MAC           net.HardwareAddr
	// Set IP to non-nil to ignore packets not meant for us.
	IP               net.IP
	UDPv4            []udpSocket
	TCPv4            []tcpSocket
	GlobalHandler    func([]byte)
	pendingUDPv4     uint32
	pendingTCPv4     uint32
	droppedPackets   uint32
	processedPackets uint32
}

// Common errors.
var (
	ErrDroppedPacket    = errors.New("dropped packet")
	errPacketExceedsMTU = errors.New("packet exceeds MTU")
	errNotIPv4          = errors.New("require IPv4")
	errPacketSmol       = errors.New("packet too small")
	errNoSocketAvail    = errors.New("no available socket")
	errTooShortTCPOrUDP = errors.New("packet too short to be TCP/UDP")
	errZeroPort         = errors.New("zero port in TCP/UDP")
	errBadTCPOffset     = errors.New("invalid TCP offset")
	errNilHandler       = errors.New("nil handler")
	errChecksumTCPorUDP = errors.New("invalid TCP/UDP checksum")
	errBadUDPLength     = errors.New("invalid UDP length")
	errInvalidIHL       = errors.New("invalid IP IHL")
	errIPVersion        = errors.New("IP version not supported")
)

// RecvEth validates an ethernet+ipv4 frame in payload. If it is OK then it
// defers response handling of the packets during a call to [Stack.HandleEth].
//
// If [Stack.HandleEth] is not called often enough prevent packet queue from
// filling up on a socket RecvEth will start to return [ErrDroppedPacket].
func (s *Stack) RecvEth(ethernetFrame []byte) (err error) {
	var ehdr eth.EthernetHeader
	var ihdr eth.IPv4Header
	defer func() {
		if err != nil {
			s.error("Stack.RecvEth", slog.String("err", err.Error()), slog.Any("IP", ihdr))
		} else {
			s.lastRxSuccess = s.lastRx
			s.GlobalHandler(ethernetFrame)
		}
	}()
	payload := ethernetFrame
	if len(payload) < eth.SizeEthernetHeader+eth.SizeIPv4Header {
		return errPacketSmol
	}
	s.debug("Stack.RecvEth:start", slog.Int("plen", len(payload)))
	s.lastRx = time.Now()

	// Ethernet parsing block
	ehdr = eth.DecodeEthernetHeader(payload)
	if s.MAC != nil && !eth.IsBroadcastHW(ehdr.Destination[:]) && !bytes.Equal(ehdr.Destination[:], s.MAC) {
		return nil // Ignore packet, is not for us.
	} else if ehdr.AssertType() != eth.EtherTypeIPv4 {
		return nil // Ignore Non-IPv4 packets.
	}

	// IP parsing block.
	ihdr = eth.DecodeIPv4Header(payload[eth.SizeEthernetHeader:])
	ihl := ihdr.IHL()
	offset := eth.SizeEthernetHeader + 4*ihl // Can be at most 14+60=74, so no overflow risk.
	end := eth.SizeEthernetHeader + ihdr.TotalLength
	switch {
	case ihdr.Version() != 4:
		return errIPVersion
	case ihl < 5:
		return errInvalidIHL
	case s.IP != nil && string(ihdr.Destination[:]) != string(s.IP):
		return nil // Not for us.
	case uint16(offset) > end || int(offset) > len(payload):
		return errors.New("bad IP TotalLength/IHL")
	case end > _MTU:
		return errPacketExceedsMTU
	}

	payload = payload[offset:end]
	switch ihdr.Protocol {
	case 17:
		// UDP (User Datagram Protocol).
		if len(s.UDPv4) == 0 {
			return nil // No sockets.
		} else if len(payload) < eth.SizeUDPHeader {
			return errTooShortTCPOrUDP
		}
		uhdr := eth.DecodeUDPHeader(payload)
		switch {
		case uhdr.DestinationPort == 0 || uhdr.SourcePort == 0:
			return errZeroPort
		case uhdr.Length < 8:
			return errBadUDPLength
		}

		payload = payload[eth.SizeUDPHeader:]
		gotsum := uhdr.CalculateChecksumIPv4(&ihdr, payload)
		if gotsum != uhdr.Checksum {
			return errChecksumTCPorUDP
		}

		socket := s.getUDP(uhdr.DestinationPort)
		if socket == nil {
			break // No socket listening on this port.
		} else if socket.NeedsHandling() {
			s.error("UDP packet dropped")
			s.droppedPackets++
			return ErrDroppedPacket // Our socket needs handling before admitting more packets.
		}
		// The packet is meant for us. We handle it.
		s.info("UDP packet stored", slog.Int("plen", len(payload)))
		// Flag packets as needing processing.
		s.pendingUDPv4++
		socket.LastRx = s.lastRx // set as unhandled here.

		socket.packets[0].Rx = s.lastRx
		socket.packets[0].Eth = ehdr
		socket.packets[0].IP = ihdr
		socket.packets[0].UDP = uhdr

		copy(socket.packets[0].payload[:], payload)

	case 6:
		s.info("TCP packet received", slog.Int("plen", len(payload)))
		// TCP (Transport Control Protocol).
		switch {
		case len(s.TCPv4) == 0:
			return nil
		case len(payload) < eth.SizeTCPHeader:
			return errTooShortTCPOrUDP
		}

		thdr := eth.DecodeTCPHeader(payload)
		offset := thdr.Offset()
		switch {
		case thdr.DestinationPort == 0 || thdr.SourcePort == 0:
			return errZeroPort
		case offset < 5 || int(offset*4) > len(payload):
			return errBadTCPOffset
		}
		options := payload[:offset*4]
		payload = payload[offset*4:]
		gotsum := thdr.CalculateChecksumIPv4(&ihdr, options, payload)
		if gotsum != thdr.Checksum {
			return errChecksumTCPorUDP
		}

		socket := s.getTCP(thdr.DestinationPort)
		if socket == nil {
			break // No socket listening on this port.
		} else if socket.NeedsHandling() {
			s.error("TCP packet dropped")
			s.droppedPackets++
			return ErrDroppedPacket // Our socket needs handling before admitting more packets.
		}
		s.info("TCP packet stored", slog.Int("plen", len(payload)))
		// Flag packets as needing processing.
		s.pendingTCPv4++
		socket.LastRx = s.lastRx // set as unhandled here.

		socket.packets[0].Rx = s.lastRx
		socket.packets[0].Eth = ehdr
		socket.packets[0].IP = ihdr
		socket.packets[0].TCP = thdr

		copy(socket.packets[0].payload[:], payload) // TODO: add options to payload.
	}
	return nil
}

// HandleEth searches for a socket with a pending packet and writes the response
// into the dst argument. The length written to dst is returned.
// [io.ErrNoProgress] can be returned by value by a handler to indicate the packet was
// not processed and that a future call to HandleEth is required to complete.
//
// If a handler returns any other error the port is closed.
func (s *Stack) HandleEth(dst []byte) (n int, err error) {
	switch {
	case len(dst) < _MTU:
		return 0, io.ErrShortBuffer
	case s.pendingUDPv4 == 0 && s.pendingTCPv4 == 0:
		return 0, nil // No packets to handle
	}

	s.info("HandleEth", slog.Int("dstlen", len(dst)))
	if s.pendingUDPv4 > 0 {
		for i := range s.UDPv4 {
			socket := &s.UDPv4[i]
			if !socket.IsPendingHandling() {
				return 0, nil
			}
			// Socket has an unhandled packet.
			n, err = socket.HandleEth(dst)
			if err == io.ErrNoProgress {
				n = 0
				err = nil
				continue
			}
			s.pendingUDPv4--
			if err != nil {
				socket.Close()
				return 0, err
			}
			if n == 0 {
				continue // Nothing done or io.ErrNoProgress flag.
			}
			break // If we got here our packet has been processed.
		}
	}

	if n == 0 && s.pendingTCPv4 > 0 {
		socketList := s.TCPv4
		for i := range socketList {
			socket := &socketList[i]
			if !socket.IsPendingHandling() {
				return 0, nil
			}
			// Socket has an unhandled packet.
			n, err = socket.HandleEth(dst)
			if err == io.ErrNoProgress {
				n = 0
				err = nil
				continue
			}
			s.pendingTCPv4--
			if err != nil {
				socket.Close()
				return 0, err
			}
			if n == 0 {
				continue
			}
			break // If we got here our packet has been processed.
		}
	}

	if n != 0 && err == nil {
		s.processedPackets++
	}
	return n, err
}

// OpenUDP opens a UDP port and sets the handler. If the port is already open
// or if there is no socket available it returns an error.
func (s *Stack) OpenUDP(port uint16, handler func([]byte, *UDPPacket) (int, error)) error {
	switch {
	case port == 0:
		return errZeroPort
	case handler == nil:
		return errNilHandler
	}
	availIdx := -1
	socketList := s.UDPv4
	for i := range socketList {
		socket := &socketList[i]
		if socket.Port == port {
			availIdx = -1
			break
		} else if availIdx == -1 && socket.Port == 0 {
			availIdx = i
		}
	}
	if availIdx == -1 {
		return errNoSocketAvail
	}
	socketList[availIdx].Open(port, handler)
	return nil
}

// FlagUDPPending flags the socket listening on a given port as having a pending
// packet. This is useful to force a response even if no packet has been received.
func (s *Stack) FlagUDPPending(port uint16) error {
	if port == 0 {
		return errZeroPort
	}
	socket := s.getUDP(port)
	if socket == nil {
		return errNoSocketAvail
	}
	if socket.forceResponse() {
		s.pendingUDPv4++
	}
	return nil
}

// CloseUDP closes a UDP socket.
func (s *Stack) CloseUDP(port uint16) error {
	if port == 0 {
		return errZeroPort
	}
	socket := s.getUDP(port)
	if socket == nil {
		return errNoSocketAvail
	}
	s.pendingUDPv4 -= uint32(socket.pending())
	socket.Close()
	return nil
}

func (s *Stack) getUDP(port uint16) *udpSocket {
	for i := range s.UDPv4 {
		socket := &s.UDPv4[i]
		if socket.Port == port {
			return socket
		}
	}
	return nil
}

// OpenTCP opens a TCP port and sets the handler. If the port is already open
// or if there is no socket available it returns an error.
func (s *Stack) OpenTCP(port uint16, handler func([]byte, *TCPPacket) (int, error)) error {
	switch {
	case port == 0:
		return errZeroPort
	case handler == nil:
		return errNilHandler
	}

	availIdx := -1
	socketList := s.TCPv4
	for i := range socketList {
		socket := &socketList[i]
		if socket.Port == port {
			availIdx = -1
			break
		} else if availIdx == -1 && socket.Port == 0 {
			availIdx = i
		}
	}
	if availIdx == -1 {
		return errNoSocketAvail
	}
	socketList[availIdx].Open(port, handler)
	return nil
}

// FlagTCPPending flags the socket listening on a given port as having a pending
// packet. This is useful to force a response even if no packet has been received.
func (s *Stack) FlagTCPPending(port uint16) error {
	if port == 0 {
		return errZeroPort
	}
	socket := s.getTCP(port)
	if socket == nil {
		return errNoSocketAvail
	}
	if socket.forceResponse() {
		s.pendingTCPv4++
	}
	return nil
}

// CloseTCP closes a TCP socket.
func (s *Stack) CloseTCP(port uint16) error {
	if port == 0 {
		return errZeroPort
	}
	socket := s.getTCP(port)
	if socket == nil {
		return errNoSocketAvail
	}
	s.pendingTCPv4 -= socket.pending()
	socket.Close()
	return nil
}

func (s *Stack) getTCP(port uint16) *tcpSocket {
	for i := range s.UDPv4 {
		socket := &s.TCPv4[i]
		if socket.Port == port {
			return socket
		}
	}
	return nil
}

func (s *Stack) info(msg string, attrs ...slog.Attr) {
	logAttrsPrint(slog.LevelInfo, msg, attrs...)
}

func (s *Stack) error(msg string, attrs ...slog.Attr) {
	logAttrsPrint(slog.LevelError, msg, attrs...)
}

func (s *Stack) debug(msg string, attrs ...slog.Attr) {
	logAttrsPrint(slog.LevelDebug, msg, attrs...)
}

func logAttrsPrint(level slog.Level, msg string, attrs ...slog.Attr) {
	var levelStr string = level.String()

	print(levelStr)
	print(" ")
	print(msg)

	for _, a := range attrs {
		print(" ")
		print(a.Key)
		print("=")
		if a.Value.Kind() == slog.KindAny {
			fmt.Printf("%+v", a.Value.Any())
		} else {
			print(a.Value.String())
		}
	}
	println()
}
