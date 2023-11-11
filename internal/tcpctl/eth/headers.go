/*
package eth implements Ethernet, ARP, IP, TCP among other datagram and
protocol frame processing and manipulation tools.

# ARP Frame (Address resolution protocol)

see https://www.youtube.com/watch?v=aamG4-tH_m8

Legend:
  - HW:    Hardware
  - AT:    Address type
  - AL:    Address Length
  - AoS:   Address of sender
  - AoT:   Address of Target
  - Proto: Protocol (below is ipv4 example)

Below is the byte schema for an ARP header:

	0      2          4       5          6         8       14          18       24          28
	| HW AT | Proto AT | HW AL | Proto AL | OP Code | HW AoS | Proto AoS | HW AoT | Proto AoT |
	|  2B   |  2B      |  1B   |  1B      | 2B      |   6B   |    4B     |  6B    |   4B
	| ethern| IP       |macaddr|          |ask|reply|                    |for op=1|
	| = 1   |=0x0800   |=6     |=4        | 1 | 2   |       known        |=0      |

See https://hpd.gasmi.net/ to decode Hex Frames.

TODO Handle IGMP
Frame example: 01 00 5E 00 00 FB 28 D2 44 9A 2F F3 08 00 46 C0 00 20 00 00 40 00 01 02 41 04 C0 A8 01 70 E0 00 00 FB 94 04 00 00 16 00 09 04 E0 00 00 FB 00 00 00 00 00 00 00 00 00 00 00 00 00

TODO Handle LLC Logical Link Control
Frame example: 05 62 70 73 D7 10 80 04 6C 00 02 00 00 04 00 00 10 20 41 70 00 00 00 0E 00 00 00 19 40 40 00 01 16 4E E9 B0 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
*/
package eth

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"

	"github.com/soypat/cyw43439/internal/tcpctl/eth/seqs"
)

// EthernetHeader is a 14 byte ethernet header representation with no VLAN support on its own.
type EthernetHeader struct {
	Destination     [6]byte // 0:6
	Source          [6]byte // 6:12
	SizeOrEtherType uint16  // 12:14
}

// ARPv4Header is the Address Resolution Protocol header for IPv4 address resolution
// and 6 byte hardware addresses. 28 bytes in size.
type ARPv4Header struct {
	// This field specifies the network link protocol type. Example: Ethernet is 1.
	HardwareType uint16 // 0:2
	// This field specifies the internetwork protocol for which the ARP request is
	// intended. For IPv4, this has the value 0x0800. The permitted PTYPE
	// values share a numbering space with those for EtherType.
	ProtoType uint16 // 2:4
	// Length (in octets) of a hardware address. Ethernet address length is 6.
	HardwareLength uint8 // 4:5
	// Length (in octets) of internetwork addresses. The internetwork protocol
	// is specified in PTYPE. Example: IPv4 address length is 4.
	ProtoLength uint8 // 5:6
	// Specifies the operation that the sender is performing: 1 for request, 2 for reply.
	Operation uint16 // 6:8
	// Media address of the sender. In an ARP request this field is used to indicate
	// the address of the host sending the request. In an ARP reply this field is
	// used to indicate the address of the host that the request was looking for.
	HardwareSender [6]byte // 8:14
	// Internetwork address of the sender.
	ProtoSender [4]byte // 14:18
	// Media address of the intended receiver. In an ARP request this field is ignored.
	// In an ARP reply this field is used to indicate the address of the host that originated the ARP request.
	HardwareTarget [6]byte // 18:24
	// Internetwork address of the intended receiver.
	ProtoTarget [4]byte // 24:28
}

// IPv4Header is the Internet Protocol header. 20 bytes in size. Does not include options.
type IPv4Header struct {
	// VersionAndIHL contains union of both IP Version and IHL data.
	//
	// Version must be 4 for IPv4. It is force-set to its valid value in a call to Put.
	//
	// Internet Header Length (IHL) The IPv4 header is variable in size due to the
	// optional 14th field (options). The IHL field contains the size of the IPv4 header;
	// it has 4 bits that specify the number of 32-bit words in the header.
	// The minimum value for this field is 5, which indicates a length of
	// 5 × 32 bits = 160 bits = 20 bytes. As a 4-bit field, the maximum value is 15;
	// this means that the maximum size of the IPv4 header is 15 × 32 bits = 480 bits = 60 bytes.
	VersionAndIHL uint8 // 0:1 (first 4 bits are version, last 4 bits are IHL)

	// Type of Service contains Differential Services Code Point (DSCP) and
	// Explicit Congestion Notification (ECN) union data.
	//
	// DSCP originally defined as the type of service (ToS), this field specifies
	// differentiated services (DiffServ) per RFC 2474. Real-time data streaming
	// makes use of the DSCP field. An example is Voice over IP (VoIP), which is
	// used for interactive voice services.
	//
	// ECN is defined in RFC 3168 and allows end-to-end notification of
	// network congestion without dropping packets. ECN is an optional feature available
	// when both endpoints support it and effective when also supported by the underlying network.
	ToS uint8 // 1:2 (first 6 bits are DSCP, last 2 bits are ECN)

	// This 16-bit field defines the entire packet size in bytes, including header and data.
	// The minimum size is 20 bytes (header without data) and the maximum is 65,535 bytes.
	// All hosts are required to be able to reassemble datagrams of size up to 576 bytes,
	// but most modern hosts handle much larger packets.
	//
	// Links may impose further restrictions on the packet size, in which case datagrams
	// must be fragmented. Fragmentation in IPv4 is performed in either the
	// sending host or in routers. Reassembly is performed at the receiving host.
	TotalLength uint16 // 2:4

	// This field is an identification field and is primarily used for uniquely
	// identifying the group of fragments of a single IP datagram.
	ID uint16 // 4:6

	// A three-bit field follows and is used to control or identify fragments.
	//  - If the DF flag is set (bit 1), and fragmentation is required to route the packet, then the packet is dropped.
	//  - For fragmented packets, all fragments except the last have the MF flag set (bit 2).
	//  - Bit 0 is reserved and must be set to zero.
	Flags IPFlags // 6:8

	// An eight-bit time to live field limits a datagram's lifetime to prevent
	// network failure in the event of a routing loop. In practice, the field
	// is used as a hop count—when the datagram arrives at a router,
	// the router decrements the TTL field by one. When the TTL field hits zero,
	// the router discards the packet and typically sends an ICMP time exceeded message to the sender.
	TTL uint8 // 8:9

	// This field defines the protocol used in the data portion of the IP datagram. TCP is 6, UDP is 17.
	Protocol    uint8   // 9:10
	Checksum    uint16  // 10:12
	Source      [4]byte // 12:16
	Destination [4]byte // 16:20
}

// TCPHeader are the first 20 bytes of a TCP header. Does not include options.
type TCPHeader struct {
	SourcePort      uint16 // 0:2
	DestinationPort uint16 // 2:4
	// The sequence number of the first data octet in this segment (except when SYN present)
	// If SYN present this is the Initial Sequence Number (ISN) and the first data octet would be ISN+1.
	Seq seqs.Value // 4:8
	// Value of the next sequence number (Seq field) the sender is expecting to receive (when ACK is present).
	// In other words an Ack of X indicates all octets up to but not including X have been received.
	// Once a connection is established the ACK flag should always be set.
	Ack seqs.Value // 8:12
	// Contains 4 bit TCP offset (in 32bit words), the 6 bit TCP flags field and a 6 bit reserved field.
	OffsetAndFlags [1]uint16 // 12:14 bitfield
	WindowSizeRaw  uint16    // 14:16
	Checksum       uint16    // 16:18
	UrgentPtr      uint16    // 18:20
}

// UDPHeader represents a UDP header. 8 bytes in size. UDP is protocol 17.
type UDPHeader struct {
	SourcePort      uint16 // 0:2
	DestinationPort uint16 // 2:4
	// Length specifies length in bytes of UDP header and UDP payload. The minimum length
	// is 8 bytes (UDP header length). This field should match the result of the IP header
	// TotalLength field minus the IP header size: udp.Length == ip.TotalLength - 4*ip.IHL
	Length   uint16 // 4:6
	Checksum uint16 // 6:8
}

// DHCPHeader specifies the first 44 bytes of a DHCP packet payload. It does
// not include BOOTP, magic cookie and options.
// Reference: https://lists.gnu.org/archive/html/lwip-users/2012-12/msg00016.html
type DHCPHeader struct {
	OP    byte   // 0:1
	HType byte   // 1:2
	HLen  byte   // 2:3
	HOps  byte   // 3:4
	Xid   uint32 // 4:8
	Secs  uint16 // 8:10
	Flags uint16 // 10:12
	// CIAddr is the client IP address. If the client has not obtained an IP
	// address yet, this field is set to 0.
	CIAddr [4]byte // 12:16
	YIAddr [4]byte // 16:20
	SIAddr [4]byte // 20:24
	GIAddr [4]byte // 24:28
	// CHAddr is the client hardware address. Can be up to 16 bytes in length but
	// is usually 6 bytes for Ethernet.
	CHAddr [16]byte // 28:44
	// BOOTP, Magic Cookie, and DHCP Options not included.
	// LegacyBOOTP [192]byte
	// Magic       [4]byte // 0x63,0x82,0x53,0x63
	// Options     [275...]byte // as of RFC2131 it is variable length
}

// There are 9 flags, bits 100 thru 103 are reserved
const (
	// TCP words are 4 octals, or uint32s
	tcpWordlen         = 4
	tcpFlagmask uint16 = 0x01ff
)

// These are minimum sizes that do not take into consideration the presence of
// options or special tags (i.e: VLAN, IP/TCP Options).
const (
	SizeEthernetHeader = 14
	SizeIPv4Header     = 20
	SizeUDPHeader      = 8
	SizeARPv4Header    = 28
	SizeTCPHeader      = 20
	SizeDHCPHeader     = 44
	ipflagDontFrag     = 0x4000
	ipFlagMoreFrag     = 0x8000
	ipVersion4         = 0x45
	ipProtocolTCP      = 6
	ipProtocolUDP      = 17
)

func IsBroadcastHW(hwaddr net.HardwareAddr) bool {
	// This comparison should be optimized by compiler to not allocate.
	// See bytes.Equal.
	return string(hwaddr) == broadcast
}

func BroadcastHW() net.HardwareAddr { return net.HardwareAddr(broadcast) }

// Broadcast is a special hardware address which indicates a Frame should
// be sent to every device on a given LAN segment.
const broadcast = "\xff\xff\xff" + "\xff\xff\xff"

var (
	none = net.HardwareAddr{0, 0, 0, 0, 0, 0}
)

type EtherType uint16

// AssertType returns the Size or EtherType field of the Ethernet frame as EtherType.
func (e EthernetHeader) AssertType() EtherType { return EtherType(e.SizeOrEtherType) }

// DecodeEthernetHeader decodes an ethernet frame from the first 14 bytes of buf.
// It does not handle 802.1Q VLAN situation where at least 4 more bytes must be decoded from wire.
func DecodeEthernetHeader(b []byte) (ethdr EthernetHeader) {
	_ = b[13]
	copy(ethdr.Destination[0:], b[0:])
	copy(ethdr.Source[0:], b[6:])
	ethdr.SizeOrEtherType = binary.BigEndian.Uint16(b[12:14])
	return ethdr
}

// IsVLAN returns true if the SizeOrEtherType is set to the VLAN tag 0x8100. This
// indicates the EthernetHeader is invalid as-is and instead of EtherType the field
// contains the first two octets of a 4 octet 802.1Q VLAN tag. In this case 4 more bytes
// must be read from the wire, of which the last 2 of these bytes contain the actual
// SizeOrEtherType field, which needs to be validated yet again in case the packet is
// a VLAN double-tap packet.
func (ethdr *EthernetHeader) IsVLAN() bool { return ethdr.SizeOrEtherType == uint16(EtherTypeVLAN) }

// Put marshals the ethernet frame onto buf. buf needs to be 14 bytes in length or Put panics.
func (ethdr *EthernetHeader) Put(buf []byte) {
	_ = buf[13]
	copy(buf[0:], ethdr.Destination[0:])
	copy(buf[6:], ethdr.Source[0:])
	binary.BigEndian.PutUint16(buf[12:14], ethdr.SizeOrEtherType)
}

// String returns a human readable representation of the Ethernet frame.
func (f *EthernetHeader) String() string {
	var vlanstr string
	if f.IsVLAN() {
		vlanstr = "(VLAN)"
	}
	// Default case for most common IPv4 traffic.
	ethertpStr := "IPv4"
	ethertp := f.AssertType()
	if ethertp != EtherTypeIPv4 {
		var ok bool
		ethertpStr, ok = _EtherType_map[EtherType(ethertp)]
		if !ok {
			ethertpStr = strconv.Itoa(int(ethertp))
		}
	}
	return strcat("dst: ", net.HardwareAddr(f.Destination[:]).String(), ", ",
		"src: ", net.HardwareAddr(f.Source[:]).String(), ", ",
		"etype: ", ethertpStr, vlanstr)
}

// IHL returns the internet header length in 32bit words and is guaranteed to be within 0..15.
// Valid values for IHL are 5..15. When multiplied by 4 this yields number of bytes of the header, 20..60.
func (iphdr *IPv4Header) IHL() uint8     { return iphdr.VersionAndIHL & 0xf }
func (iphdr *IPv4Header) Version() uint8 { return iphdr.VersionAndIHL >> 4 }
func (iphdr *IPv4Header) DSCP() uint8    { return iphdr.ToS >> 2 }
func (iphdr *IPv4Header) ECN() uint8     { return iphdr.ToS & 0b11 }

func (iphdr *IPv4Header) FrameLength() int {
	return int(iphdr.TotalLength)
}

func (iphdr *IPv4Header) PayloadLength() int {
	return int(iphdr.TotalLength) - 20
}

func (ip *IPv4Header) String() string {
	return strcat("IPv4 ", net.IP(ip.Source[:]).String(), " -> ",
		net.IP(ip.Destination[:]).String(), " proto=", strconv.Itoa(int(ip.Protocol)),
		" len=", strconv.Itoa(int(ip.TotalLength)),
	)
}

// DecodeIPv4Header decodes a 20 byte IPv4 header from buf.
func DecodeIPv4Header(buf []byte) (iphdr IPv4Header) {
	_ = buf[19]
	iphdr.VersionAndIHL = buf[0]
	iphdr.ToS = buf[1]
	iphdr.TotalLength = binary.BigEndian.Uint16(buf[2:])
	iphdr.ID = binary.BigEndian.Uint16(buf[4:])
	iphdr.Flags = IPFlags(binary.BigEndian.Uint16(buf[6:]))
	iphdr.TTL = buf[8]
	iphdr.Protocol = buf[9]
	iphdr.Checksum = binary.BigEndian.Uint16(buf[10:])
	copy(iphdr.Source[:], buf[12:16])
	copy(iphdr.Destination[:], buf[16:20])
	return iphdr
}

// Put marshals the IPv4 frame onto buf. buf needs to be 20 bytes in length or Put panics.
func (iphdr *IPv4Header) Put(buf []byte) {
	_ = buf[19]
	buf[0] = (4 << 4) | (iphdr.VersionAndIHL & 0xf) // ignore set version.
	buf[1] = iphdr.ToS
	binary.BigEndian.PutUint16(buf[2:], iphdr.TotalLength)
	binary.BigEndian.PutUint16(buf[4:], iphdr.ID)
	binary.BigEndian.PutUint16(buf[6:], uint16(iphdr.Flags))
	buf[8] = iphdr.TTL
	buf[9] = iphdr.Protocol
	binary.BigEndian.PutUint16(buf[10:], iphdr.Checksum)
	copy(buf[12:16], iphdr.Source[:])
	copy(buf[16:20], iphdr.Destination[:])
}

// PutPseudo marshals the pseudo-header representation of IPv4 frame onto buf.
// buf needs to be 12 bytes in length or PutPseudo panics.
//
//	+--------+--------+--------+--------+
//	|           Source Address          |
//	+--------+--------+--------+--------+
//	|         Destination Address       |
//	+--------+--------+--------+--------+
//	|  zero  |  PTCL  |    TCP Length   |
//	+--------+--------+--------+--------+
//
// The TCP Length is the TCP header length plus the data length in octets
// (this is not an explicitly transmitted quantity, but is computed),
// and it does not count the 12 octets of the pseudo header (See [RFC 793])
//
// [RFC 793]: https://www.rfc-editor.org/rfc/rfc793
func (iphdr *IPv4Header) PutPseudo(buf []byte) {
	_ = buf[11]
	copy(buf[0:4], iphdr.Source[:])
	copy(buf[4:8], iphdr.Destination[:])
	buf[8] = 0
	buf[9] = iphdr.Protocol
	binary.BigEndian.PutUint16(buf[10:12], iphdr.TotalLength)
}

func (iphdr *IPv4Header) CalculateChecksum() uint16 {
	crc := CRC791{}
	var buf [SizeIPv4Header]byte
	iphdr.Put(buf[:])
	binary.BigEndian.PutUint16(buf[10:], 0) // Zero out checksum field.
	crc.Write(buf[:])
	return crc.Sum16()
}

type IPFlags uint16

func (f IPFlags) DontFragment() bool     { return f&ipflagDontFrag != 0 }
func (f IPFlags) MoreFragments() bool    { return f&ipFlagMoreFrag != 0 }
func (f IPFlags) FragmentOffset() uint16 { return uint16(f) & 0x1fff }

func DecodeARPv4Header(buf []byte) (arphdr ARPv4Header) {
	_ = buf[27]
	arphdr.HardwareType = binary.BigEndian.Uint16(buf[0:])
	arphdr.ProtoType = binary.BigEndian.Uint16(buf[2:])
	arphdr.HardwareLength = buf[4]
	arphdr.ProtoLength = buf[5]
	arphdr.Operation = binary.BigEndian.Uint16(buf[6:])
	copy(arphdr.HardwareSender[:], buf[8:14])
	copy(arphdr.ProtoSender[:], buf[14:18])
	copy(arphdr.HardwareTarget[:], buf[18:24])
	copy(arphdr.ProtoTarget[:], buf[24:28])
	return arphdr
}

// DecodeUDPHeader decodes a UDP header from buf. Panics if buf is less than 8 bytes in length.
func DecodeUDPHeader(buf []byte) (udp UDPHeader) {
	_ = buf[7]
	udp.SourcePort = binary.BigEndian.Uint16(buf[0:2])
	udp.DestinationPort = binary.BigEndian.Uint16(buf[2:4])
	udp.Length = binary.BigEndian.Uint16(buf[4:6])
	udp.Checksum = binary.BigEndian.Uint16(buf[6:8])
	return udp
}

// Put marshals the UDPHeader onto buf. If buf's length is less than 8 then Put panics.
func (udphdr *UDPHeader) Put(buf []byte) {
	_ = buf[7]
	binary.BigEndian.PutUint16(buf[0:2], udphdr.SourcePort)
	binary.BigEndian.PutUint16(buf[2:4], udphdr.DestinationPort)
	binary.BigEndian.PutUint16(buf[4:6], udphdr.Length)
	binary.BigEndian.PutUint16(buf[6:8], udphdr.Checksum)
}

// CalculateChecksumIPv4 calculates the checksum for a UDP packet over IPv4.
func (udphdr *UDPHeader) CalculateChecksumIPv4(pseudoHeader *IPv4Header, payload []byte) uint16 {
	var crc CRC791
	crc.Write(pseudoHeader.Source[:])
	crc.Write(pseudoHeader.Destination[:])
	crc.AddUint16(uint16(pseudoHeader.Protocol)) // Pads with 0.
	crc.AddUint16(udphdr.Length)                 // UDP length appears twice: https://stackoverflow.com/questions/45908909/my-udp-checksum-calculation-gives-wrong-results-every-time
	crc.AddUint16(udphdr.SourcePort)
	crc.AddUint16(udphdr.DestinationPort)
	crc.AddUint16(udphdr.Length)
	crc.Write(payload)
	return crc.Sum16()
}

func (udphdr *UDPHeader) String() string {
	return fmt.Sprintf("%d->%d len=%d", udphdr.SourcePort, udphdr.DestinationPort, udphdr.Length)
}

// Put marshals the ARP header onto buf. buf needs to be 28 bytes in length or Put panics.
func (arphdr *ARPv4Header) Put(buf []byte) {
	_ = buf[27]
	binary.BigEndian.PutUint16(buf[0:], arphdr.HardwareType)
	binary.BigEndian.PutUint16(buf[2:], arphdr.ProtoType)
	buf[4] = arphdr.HardwareLength
	buf[5] = arphdr.ProtoLength
	binary.BigEndian.PutUint16(buf[6:], arphdr.Operation)
	copy(buf[8:14], arphdr.HardwareSender[:])
	copy(buf[14:18], arphdr.ProtoSender[:])
	copy(buf[18:24], arphdr.HardwareTarget[:])
	copy(buf[24:28], arphdr.ProtoTarget[:])
}

func (a *ARPv4Header) String() string {
	if bytesAreAll(a.HardwareTarget[:], 0) {
		return strcat("ARP ", net.HardwareAddr(a.HardwareTarget[:]).String(), "->",
			"who has ", net.IP(a.ProtoTarget[:]).String(), "?", " Tell ", net.IP(a.ProtoSender[:]).String())
	}
	return strcat("ARP ", net.HardwareAddr(a.HardwareSender[:]).String(), "->",
		"I have ", net.IP(a.ProtoSender[:]).String(), "! Tell ", net.IP(a.ProtoTarget[:]).String(), ", aka ", net.HardwareAddr(a.HardwareTarget[:]).String())
}

func DecodeTCPHeader(buf []byte) (tcphdr TCPHeader) {
	_ = buf[19]
	tcphdr.SourcePort = binary.BigEndian.Uint16(buf[0:])
	tcphdr.DestinationPort = binary.BigEndian.Uint16(buf[2:])
	tcphdr.Seq = seqs.Value(binary.BigEndian.Uint32(buf[4:]))
	tcphdr.Ack = seqs.Value(binary.BigEndian.Uint32(buf[8:]))
	tcphdr.OffsetAndFlags[0] = binary.BigEndian.Uint16(buf[12:])
	tcphdr.WindowSizeRaw = binary.BigEndian.Uint16(buf[14:])
	tcphdr.Checksum = binary.BigEndian.Uint16(buf[16:])
	tcphdr.UrgentPtr = binary.BigEndian.Uint16(buf[18:])
	return tcphdr
}

// Put marshals the TCP frame onto buf. buf needs to be 20 bytes in length or Put panics.
func (tcphdr *TCPHeader) Put(buf []byte) {
	_ = buf[19]
	binary.BigEndian.PutUint16(buf[0:], tcphdr.SourcePort)
	binary.BigEndian.PutUint16(buf[2:], tcphdr.DestinationPort)
	binary.BigEndian.PutUint32(buf[4:], uint32(tcphdr.Seq))
	binary.BigEndian.PutUint32(buf[8:], uint32(tcphdr.Ack))
	binary.BigEndian.PutUint16(buf[12:], tcphdr.OffsetAndFlags[0])
	binary.BigEndian.PutUint16(buf[14:], tcphdr.WindowSizeRaw)
	binary.BigEndian.PutUint16(buf[16:], tcphdr.Checksum)
	binary.BigEndian.PutUint16(buf[18:], tcphdr.UrgentPtr)
}

// Offset specifies the size of the TCP header in 32-bit words. The minimum size
// header is 5 words and the maximum is 15 words thus giving the minimum size of
// 20 bytes and maximum of 60 bytes, allowing for up to 40 bytes of options in
// the header. This field gets its name from the fact that it is also the offset
// from the start of the TCP segment to the actual data.
func (tcphdr *TCPHeader) Offset() (tcpWords uint8) {
	return uint8(tcphdr.OffsetAndFlags[0] >> (8 + 4))
}

// OffsetInBytes returns the size of the TCP header in bytes, including options.
// See [TCPHeader.Offset] for more information.
func (tcphdr *TCPHeader) OffsetInBytes() uint8 {
	return tcphdr.Offset() * tcpWordlen
}

func (tcphdr *TCPHeader) Flags() seqs.Flags {
	return seqs.Flags(tcphdr.OffsetAndFlags[0] & tcpFlagmask)
}

func (tcphdr *TCPHeader) SetFlags(v seqs.Flags) {
	onlyOffset := tcphdr.OffsetAndFlags[0] &^ tcpFlagmask
	tcphdr.OffsetAndFlags[0] = onlyOffset | uint16(v)&tcpFlagmask
}

func (tcphdr *TCPHeader) SetOffset(tcpWords uint8) {
	if tcpWords > 0b1111 {
		panic("attempted to set an offset too large")
	}
	onlyFlags := tcphdr.OffsetAndFlags[0] & tcpFlagmask
	tcphdr.OffsetAndFlags[0] = onlyFlags | (uint16(tcpWords) << 12)
}

// WindowSize is a convenience method for obtaining a seqs.Size from the TCP header internal WindowSize 16bit field.
func (tcphdr *TCPHeader) WindowSize() seqs.Size {
	return seqs.Size(tcphdr.WindowSizeRaw)
}

// CalculateChecksumIPv4 calculates the checksum of the TCP header, options and payload.
func (tcphdr *TCPHeader) CalculateChecksumIPv4(pseudoHeader *IPv4Header, tcpOptions, payload []byte) uint16 {
	const sizePseudo = 12
	var crc CRC791
	var buf [sizePseudo + 20]byte
	pseudoHeader.PutPseudo(buf[:sizePseudo])
	tcphdr.Put(buf[sizePseudo:])
	// Zero out checksum field.
	binary.BigEndian.PutUint16(buf[sizePseudo+16:sizePseudo+18], 0)
	crc.Write(buf[:])
	crc.Write(tcpOptions)
	crc.Write(payload)
	return crc.Sum16()
}

func (tcp *TCPHeader) String() string {
	return strcat("TCP port ", u32toa(uint32(tcp.SourcePort)), "->", u32toa(uint32(tcp.DestinationPort)),
		tcp.Flags().String(), "seq ", u32toa(uint32(tcp.Seq)), " ack ", u32toa(uint32(tcp.Ack)))
}

func u32toa(u uint32) string {
	return strconv.FormatUint(uint64(u), 10)
}

// bytesAreAll returns true if b is composed of only unit bytes
func bytesAreAll(b []byte, unit byte) bool {
	for i := range b {
		if b[i] != unit {
			return false
		}
	}
	return true
}

func strcat(strs ...string) (s string) {
	for i := range strs {
		s += strs[i]
	}
	return s
}

func hexascii(b byte) [2]byte {
	const hexstr = "0123456789abcdef"
	return [2]byte{hexstr[b>>4], hexstr[b&0b1111]}
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

func (d *DHCPHeader) String() (s string) {
	s = "DHCP op=" + strconv.Itoa(int(d.OP)) + " "
	if d.CIAddr != [4]byte{} {
		s += "ciaddr=" + net.IP(d.CIAddr[:]).String() + " "
	}
	if d.YIAddr != [4]byte{} {
		s += "yiaddr=" + net.IP(d.YIAddr[:]).String() + " "
	}
	if d.SIAddr != [4]byte{} {
		s += "siaddr=" + net.IP(d.SIAddr[:]).String() + " "
	}
	if d.GIAddr != [4]byte{} {
		s += "giaddr=" + net.IP(d.GIAddr[:]).String() + " "
	}
	if d.CHAddr != [16]byte{} && d.HLen < 16 && d.HLen > 0 {
		s += "chaddr=" + net.HardwareAddr(d.CHAddr[:d.HLen]).String() + " "
	}
	return s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b byte) byte {
	if a < b {
		return a
	}
	return b
}
