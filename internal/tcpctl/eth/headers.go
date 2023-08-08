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
	"net"
	"strconv"
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
	Version uint8 // 0:1
	// Internet Header Length (IHL) The IPv4 header is variable in size due to the
	// optional 14th field (options). The IHL field contains the size of the IPv4 header;
	// it has 4 bits that specify the number of 32-bit words in the header.
	//
	// The minimum value for this field is 5, which indicates a length of
	// 5 × 32 bits = 160 bits = 20 bytes. As a 4-bit field, the maximum value is 15;
	// this means that the maximum size of the IPv4 header is 15 × 32 bits = 480 bits = 60 bytes.
	IHL uint8 // 1:2
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
	// network failure in the event of a routing loop. When the datagram arrives
	// at a router, the router decrements the TTL field by one. It is specified
	// in seconds, but time intervals less than 1 second are rounded up to 1.
	TTL uint8 // 8:9
	// This field defines the protocol used in the data portion of the IP datagram. TCP is 6, UDP is 17.
	Protocol    uint8   // 9:10
	Checksum    uint16  // 10:12
	Source      [4]byte // 12:16
	Destination [4]byte // 16:20
}

// TCPHeader are the first 20 bytes of a TCP header. Does not include options.
type TCPHeader struct {
	SourcePort      uint16    // 0:2
	DestinationPort uint16    // 2:4
	Seq             uint32    // 4:8
	Ack             uint32    // 8:12
	OffsetAndFlags  [1]uint16 // 12:14 bitfield
	WindowSize      uint16    // 14:16
	Checksum        uint16    // 16:18
	UrgentPtr       uint16    // 18:20
}

// UDPHeader represents a UDP header. 8 bytes in size. UDP is protocol 17.
type UDPHeader struct {
	SourcePort      uint16 // 0:2
	DestinationPort uint16 // 2:4
	Length          uint16 // 4:6
	Checksum        uint16 // 6:8
}

// There are 9 flags, bits 100 thru 103 are reserved
const (
	// TCP words are 4 octals, or uint32s
	tcpWordlen         = 4
	tcpFlagmask uint16 = 0x01ff
)

const (
	FlagTCP_FIN TCPFlags = 1 << iota
	FlagTCP_SYN
	FlagTCP_RST
	FlagTCP_PSH
	FlagTCP_ACK
	FlagTCP_URG
	FlagTCP_ECE
	FlagTCP_CWR
	FlagTCP_NS
)

const (
	SizeEthernetHeaderNoVLAN = 14
	SizeIPv4Header           = 20
	SizeUDPHeader            = 8
	SizeARPv4Header          = 28
	SizeTCPHeaderNoOptions   = 20
	ipflagDontFrag           = 0x4000
	ipFlagMoreFrag           = 0x8000
	ipVersion4               = 0x45
	ipProtocolTCP            = 6
)

var (
	// Broadcast is a special hardware address which indicates a Frame should
	// be sent to every device on a given LAN segment.
	Broadcast = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	None      = net.HardwareAddr{0, 0, 0, 0, 0, 0}
)

type EtherType uint16

// Ethertype values. From: http://en.wikipedia.org/wiki/Ethertype
//
//go:generate stringer -type=EtherType -trimprefix=EtherType
const (
	EtherTypeIPv4                EtherType = 0x0800
	EtherTypeARP                 EtherType = 0x0806
	EtherTypeWakeOnLAN           EtherType = 0x0842
	EtherTypeTRILL               EtherType = 0x22F3
	EtherTypeDECnetPhase4        EtherType = 0x6003
	EtherTypeRARP                EtherType = 0x8035
	EtherTypeAppleTalk           EtherType = 0x809B
	EtherTypeAARP                EtherType = 0x80F3
	EtherTypeIPX1                EtherType = 0x8137
	EtherTypeIPX2                EtherType = 0x8138
	EtherTypeQNXQnet             EtherType = 0x8204
	EtherTypeIPv6                EtherType = 0x86DD
	EtherTypeEthernetFlowControl EtherType = 0x8808
	EtherTypeIEEE802_3           EtherType = 0x8809
	EtherTypeCobraNet            EtherType = 0x8819
	EtherTypeMPLSUnicast         EtherType = 0x8847
	EtherTypeMPLSMulticast       EtherType = 0x8848
	EtherTypePPPoEDiscovery      EtherType = 0x8863
	EtherTypePPPoESession        EtherType = 0x8864
	EtherTypeJumboFrames         EtherType = 0x8870
	EtherTypeHomePlug1_0MME      EtherType = 0x887B
	EtherTypeIEEE802_1X          EtherType = 0x888E
	EtherTypePROFINET            EtherType = 0x8892
	EtherTypeHyperSCSI           EtherType = 0x889A
	EtherTypeAoE                 EtherType = 0x88A2
	EtherTypeEtherCAT            EtherType = 0x88A4
	EtherTypeEthernetPowerlink   EtherType = 0x88AB
	EtherTypeLLDP                EtherType = 0x88CC
	EtherTypeSERCOS3             EtherType = 0x88CD
	EtherTypeHomePlugAVMME       EtherType = 0x88E1
	EtherTypeMRP                 EtherType = 0x88E3
	EtherTypeIEEE802_1AE         EtherType = 0x88E5
	EtherTypeIEEE1588            EtherType = 0x88F7
	EtherTypeIEEE802_1ag         EtherType = 0x8902
	EtherTypeFCoE                EtherType = 0x8906
	EtherTypeFCoEInit            EtherType = 0x8914
	EtherTypeRoCE                EtherType = 0x8915
	EtherTypeCTP                 EtherType = 0x9000
	EtherTypeVeritasLLT          EtherType = 0xCAFE
	EtherTypeVLAN                EtherType = 0x8100
	EtherTypeServiceVLAN         EtherType = 0x88a8
	// minEthPayload is the minimum payload size for an Ethernet frame, assuming
	// that no 802.1Q VLAN tags are present.
	minEthPayload = 46
)

// AssertType returns the Size or EtherType field of the Ethernet frame as EtherType.
func (e EthernetHeader) AssertType() EtherType { return EtherType(e.SizeOrEtherType) }

// DecodeEthernetHeader decodes an ethernet frame from buf. It does not
// handle 802.1Q VLAN situation where at least 4 more bytes must be decoded from wire.
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
	iphdr.Version = buf[0]
	iphdr.IHL = buf[1]
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
	buf[0] = iphdr.Version
	buf[1] = iphdr.IHL
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
func (iphdr *IPv4Header) PutPseudo(buf []byte) {
	// |8 TTL |9 Proto |10 Checksum |12  Source  |16  Destination |20
	// |set 0 |  nop   | set length | nop        | nop            |
	_ = buf[11]
	buf[0] = 0
	buf[1] = iphdr.Protocol
	binary.BigEndian.PutUint16(buf[2:], iphdr.TotalLength)
	copy(buf[4:8], iphdr.Source[:])
	copy(buf[8:12], iphdr.Destination[:])
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
	const sizePseudo = 12
	crc := CRC791{}
	crc.Write(pseudoHeader.Source[:])
	crc.Write(pseudoHeader.Destination[:])
	crc.AddUint16(uint16(pseudoHeader.Protocol)) // Pads with 0.
	crc.AddUint16(pseudoHeader.TotalLength)
	crc.AddUint16(udphdr.SourcePort)
	crc.AddUint16(udphdr.DestinationPort)
	crc.AddUint16(udphdr.Length)
	crc.Write(payload)
	return crc.Sum16()
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
	tcphdr.Seq = binary.BigEndian.Uint32(buf[4:])
	tcphdr.Ack = binary.BigEndian.Uint32(buf[8:])
	tcphdr.OffsetAndFlags[0] = binary.BigEndian.Uint16(buf[12:])
	tcphdr.WindowSize = binary.BigEndian.Uint16(buf[14:])
	tcphdr.Checksum = binary.BigEndian.Uint16(buf[16:])
	tcphdr.UrgentPtr = binary.BigEndian.Uint16(buf[18:])
	return tcphdr
}

// Put marshals the TCP frame onto buf. buf needs to be 20 bytes in length or Put panics.
func (tcphdr *TCPHeader) Put(buf []byte) {
	_ = buf[19]
	binary.BigEndian.PutUint16(buf[0:], tcphdr.SourcePort)
	binary.BigEndian.PutUint16(buf[2:], tcphdr.DestinationPort)
	binary.BigEndian.PutUint32(buf[4:], tcphdr.Seq)
	binary.BigEndian.PutUint32(buf[8:], tcphdr.Ack)
	binary.BigEndian.PutUint16(buf[12:], tcphdr.OffsetAndFlags[0])
	binary.BigEndian.PutUint16(buf[14:], tcphdr.WindowSize)
	binary.BigEndian.PutUint16(buf[16:], tcphdr.Checksum)
	binary.BigEndian.PutUint16(buf[18:], tcphdr.UrgentPtr)
}

func (tcphdr *TCPHeader) Offset() (tcpWords uint8) {
	return uint8(tcphdr.OffsetAndFlags[0] >> (8 + 4))
}

func (tcphdr *TCPHeader) OffsetInBytes() (offsetInBytes uint16) {
	return uint16(tcphdr.Offset()) * tcpWordlen
}

func (tcphdr *TCPHeader) Flags() TCPFlags {
	return TCPFlags(tcphdr.OffsetAndFlags[0] & tcpFlagmask)
}

func (tcphdr *TCPHeader) SetFlags(v TCPFlags) {
	onlyOffset := tcphdr.OffsetAndFlags[0] &^ tcpFlagmask
	tcphdr.OffsetAndFlags[0] = onlyOffset | uint16(v)&tcpFlagmask
}

func (tcphdr *TCPHeader) SetOffset(tcpWords uint8) {
	if tcpWords > 0b1111 {
		panic("attempted to set an offset too large")
	}
	onlyFlags := tcphdr.OffsetAndFlags[0] & tcpFlagmask
	tcphdr.OffsetAndFlags[0] |= onlyFlags | (uint16(tcpWords) << 12)
}

// FrameLength returns the size of the TCP frame as described by tcphdr and
// payloadLength, which is the size of the TCP payload not including the TCP options.
func (tcphdr *TCPHeader) FrameLength(payloadLength uint16) uint16 {
	return tcphdr.OffsetInBytes() + payloadLength
}

// OptionsLength returns the length of the options section
func (tcphdr *TCPHeader) OptionsLength() uint16 {
	return tcphdr.OffsetInBytes()*tcpWordlen - 20
}

// CalculateChecksumIPv4 calculates the checksum of the TCP header, options and payload.
func (tcphdr *TCPHeader) CalculateChecksumIPv4(pseudoHeader *IPv4Header, tcpOptions, payload []byte) uint16 {
	const sizePseudo = 12
	crc := CRC791{}
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
		tcp.Flags().String(), "seq ", u32toa(tcp.Seq), " ack ", u32toa(tcp.Ack))
}

type TCPFlags uint16

// HasFlags checks if argument bits are all set in the receiver flags.
func (flags TCPFlags) HasFlags(argument TCPFlags) bool { return argument&flags == argument }

// StringFlags returns human readable flag string. i.e:
//
//	"[SYN,ACK]"
//
// Flags are printed in order from LSB (FIN) to MSB (NS).
// All flags are printed with length of 3, so a NS flag will
// end with a space i.e. [ACK,NS ]
func (flags TCPFlags) String() string {
	// String Flag const
	const flaglen = 3
	var flagbuff [2 + (flaglen+1)*9]byte
	const strflags = "FINSYNRSTPSHACKURGECECWRNS "
	n := 0
	for i := 0; i*3 < len(strflags)-flaglen; i++ {
		if flags&(1<<i) != 0 {
			if n == 0 {
				flagbuff[0] = '['
				n++
			} else {
				flagbuff[n] = ','
				n++
			}
			copy(flagbuff[n:n+3], []byte(strflags[i*flaglen:i*flaglen+flaglen]))
			n += 3
		}
	}
	if n > 0 {
		flagbuff[n] = ']'
		n++
	}
	return string(flagbuff[:n])
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
