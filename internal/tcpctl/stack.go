package tcpctl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/cyw43439/internal/tcpctl/eth"
)

const (
	_MTU = 1500
)

type socketEth interface {
	Close()
	IsPendingHandling() bool
	// HandleEth searches for a socket with a pending packet and writes the response
	// into the dst argument. The length written to dst is returned.
	// [io.ErrNoProgress] can be returned by a handler to indicate the packet was
	// not processed and that a future call to HandleEth is required to complete.
	//
	// If a handler returns any other error the port is closed.
	HandleEth(dst []byte) (int, error)
}

// NeedsHandling returns true if the socket needs handling before it can
// admit more pending packets.
func (u *udpSocket) NeedsHandling() bool {
	// As of now socket has space for 1 packet so if packet is pending, queue is full.
	// Compile time check to ensure this is fulfilled:
	_ = u.packets[1-len(u.packets)]
	return u.IsPendingHandling()
}

// IsPendingHandling returns true if there are packet(s) pending handling.
func (u *udpSocket) IsPendingHandling() bool {
	return u.Port != 0 && !u.packets[0].Rx.IsZero()
}

// HandleEth writes the socket's response into dst to be sent over an ethernet interface.
// HandleEth can return 0 bytes written and a nil error to indicate no action must be taken.
func (u *udpSocket) HandleEth(dst []byte) (int, error) {
	if u.handler == nil {
		panic("nil udp handler on port " + strconv.Itoa(int(u.Port)))
	}
	return u.handler(&u.packets[0], dst)
}

// Open sets the UDP handler and opens the port.
func (u *udpSocket) Open(port uint16, h func(*UDPPacket, []byte) (int, error)) {
	if port == 0 || h == nil {
		panic("invalid port or nil handler" + strconv.Itoa(int(u.Port)))
	}
	u.handler = h
	u.Port = port
}

func (u *udpSocket) Close() {
	u.Port = 0 // Port 0 flags the port is inactive.
	for i := range u.packets {
		u.packets[i].Rx = time.Time{} // Invalidate packets.
	}
}

type udpSocket struct {
	LastRx  time.Time
	handler func(self *UDPPacket, response []byte) (int, error)
	Port    uint16
	packets [1]UDPPacket
}

type UDPPacket struct {
	Rx      time.Time
	Eth     eth.EthernetHeader
	IP      eth.IPv4Header
	UDP     eth.UDPHeader
	payload [_MTU - eth.SizeEthernetHeader - eth.SizeIPv4Header - eth.SizeUDPHeader]byte
}

// Payload returns the UDP payload. If UDP or IPv4 header data is incorrect/bad it returns nil.
func (p *UDPPacket) Payload() []byte {
	ipLen := int(p.IP.TotalLength) - int(p.IP.IHL()*4) // Total length(including header) - header length = payload length
	uLen := int(p.UDP.Length) - eth.SizeUDPHeader
	if ipLen != uLen || uLen > len(p.payload) {
		return nil // Mismatching IP and UDP data or bad length.
	}
	return p.payload[:uLen]
}

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
	pendingUDPv4     uint32
	pendingTCPv4     uint32
	droppedPackets   uint32
	processedPackets uint32
}

func (s *Stack) OpenUDP(port uint16, handler func(*UDPPacket, []byte) (int, error)) error {
	availIdx := -1
	for i := range s.UDPv4 {
		socket := &s.UDPv4[i]
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
	s.UDPv4[availIdx].Open(port, handler)
	return nil
}

func (s *Stack) CloseUDP(port uint16) error {
	for i := range s.UDPv4 {
		socket := &s.UDPv4[i]
		if socket.Port == port {
			socket.Close()
			return nil
		}
	}
	return errNoSocketAvail
}

// Common errors.
var (
	ErrDroppedPacket = errors.New("dropped packet")
	errNotIPv4       = errors.New("require IPv4")
	errPacketSmol    = errors.New("packet too small")
	errNoSocketAvail = errors.New("no available socket")
)

// RecvEth validates an ethernet+ipv4 frame in payload. If it is OK then it
// defers response handling of the packets during a call to [Stack.HandleEth].
//
// If [Stack.HandleEth] is not called often enough prevent packet queue from
// filling up on a socket RecvEth will start to return [ErrDroppedPacket].
func (s *Stack) RecvEth(payload []byte) (err error) {
	var ehdr eth.EthernetHeader
	var ihdr eth.IPv4Header
	defer func() {
		if err != nil {
			s.error("Stack.RecvEth", slog.String("err", err.Error()), slog.Any("IP", ihdr))
		}
	}()
	if len(payload) < eth.SizeEthernetHeader+eth.SizeIPv4Header {
		return errPacketSmol
	}
	s.info("Stack.RecvEth:start", slog.Int("plen", len(payload)))
	s.lastRx = time.Now()

	// Ethernet parsing block
	ehdr = eth.DecodeEthernetHeader(payload)
	if s.MAC != nil && !eth.IsBroadcastHW(ehdr.Destination[:]) && !bytes.Equal(ehdr.Destination[:], s.MAC) {
		return nil // Ignore packet, is not for us.
	}
	if ehdr.AssertType() != eth.EtherTypeIPv4 {
		return errNotIPv4
	}

	// IP parsing block.
	ihdr = eth.DecodeIPv4Header(payload[eth.SizeEthernetHeader:])
	if ihdr.ToS != 0 {
		return errors.New("ToS not supported")
	} else if ihdr.Version() != 4 {
		return errors.New("IP version not supported")
	} else if ihdr.IHL() < 5 {
		return errors.New("bad IHL")
	} else if s.IP != nil && string(ihdr.Destination[:]) != string(s.IP) {
		return nil // Not for us.
	}

	// Handle UDP/TCP packets.
	offset := eth.SizeEthernetHeader + 4*ihdr.IHL() // Can be at most 14+60=74, so no overflow risk.
	end := eth.SizeEthernetHeader + ihdr.TotalLength
	if len(payload) < int(end) || end < uint16(offset) {
		return errors.New("short payload buffer or bad IP TotalLength")
	} else if end > _MTU {
		return errors.New("packet size exceeds MTU")
	}

	s.lastRxSuccess = s.lastRx
	payload = payload[offset:end]
	switch ihdr.Protocol {
	case 17:
		// UDP (User Datagram Protocol).
		if len(s.UDPv4) == 0 {
			println("no sockets")
			return nil // No sockets.
		}
		uhdr := eth.DecodeUDPHeader(payload)
		if uhdr.DestinationPort == 0 || uhdr.SourcePort == 0 {
			// Ignore port 0. Is invalid and we use it to flag a closed/inactive port.
			s.debug("UDP packet with 0 port", slog.Any("hdr", uhdr))
			return nil
		} else if uhdr.Length < 8 {
			return errors.New("bad UDP length field")
		}
		payload = payload[eth.SizeUDPHeader:]
		gotsum := uhdr.CalculateChecksumIPv4(&ihdr, payload)
		if gotsum != uhdr.Checksum {
			return errors.New("UDP checksum mismatch")
		}
		for i := range s.UDPv4 {
			socket := &s.UDPv4[i]
			if socket.Port != uhdr.DestinationPort {
				continue
			}

			// The packet is meant for us. We handle it.
			if socket.NeedsHandling() {
				s.error("UDP packet dropped")
				s.droppedPackets++
				return ErrDroppedPacket // Our socket needs handling before admitting more packets.
			}
			s.info("UDP packet stored", slog.Int("plen", len(payload)))
			// Flag packets as needing processing.
			s.pendingUDPv4++
			socket.LastRx = s.lastRxSuccess // set as unhandled here.

			socket.packets[0].Rx = s.lastRxSuccess
			socket.packets[0].Eth = ehdr
			socket.packets[0].IP = ihdr
			socket.packets[0].UDP = uhdr

			copy(socket.packets[0].payload[:], payload)
			break // Packet succesfully processed, missing
		}
	case 6:
		s.info("TCP packet received", slog.Int("plen", len(payload)))
		// TCP (Transport Control Protocol).
		// TODO
	}
	s.debug("Stack.RecvEth:success")
	return nil
}

// HandleEth searches for a socket with a pending packet and writes the response
// into the dst argument. The length written to dst is returned.
// [io.ErrNoProgress] can be returned by value by a handler to indicate the packet was
// not processed and that a future call to HandleEth is required to complete.
//
// If a handler returns any other error the port is closed.
func (s *Stack) HandleEth(dst []byte) (n int, err error) {
	if len(dst) < _MTU {
		return
	}
	s.info("HandleEth", slog.Int("dstlen", len(dst)))
	if s.pendingUDPv4 == 0 && s.pendingTCPv4 == 0 {
		return 0, nil // No packets to handle
	}
	if s.pendingUDPv4 > 0 {
		for i := range s.UDPv4 {
			socket := &s.UDPv4[i]
			n, err = tryHandleEth(socket, dst)
			if err != nil {
				return 0, err
			}
			if n == 0 {
				continue // Nothing done or io.ErrNoProgress flag.
			}
			break // If we got here our packet has been processed.
		}
	}
	if n == 0 && s.pendingTCPv4 > 0 {
		// TODO
	}

	if n != 0 && err == nil {
		s.processedPackets++
	}
	return n, err
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

func tryHandleEth(socket socketEth, dst []byte) (n int, err error) {
	if !socket.IsPendingHandling() {
		return 0, nil
	}
	// Socket has an unhandled packet.
	n, err = socket.HandleEth(dst)
	if err != nil {
		if err == io.ErrNoProgress {
			err = nil // Ignore NoProgress
		}
		return 0, err
	}
	return n, nil
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
