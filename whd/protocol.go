package whd

import (
	"encoding/binary"
	"errors"
	"io"
	"runtime"
	"unsafe"

	"github.com/soypat/seqs/eth"
)

var (
	errShortBufferCDC = errors.New("short CDC.Parse buffer")
	errBadSPCM        = errors.New("SDPCM size exceeds buffer")
)

// SDPCM header errors.
var (
	errSDPCMHeaderSizeComplementMismatch = errors.New("sdpcm hdr size complement mismatch")
	errSDPCMHeaderSizeMismatch           = errors.New("len from header doesn't match len from spi")
)

// ScanResult errors
var (
	errBufferUnaligned = errors.New("buffer not aligned to 4 bytes")
	errIEEndExceedsBSS = errors.New("IE end exceeds bss length")
)

// Common async event errors.
var (
	ErrInvalidEtherType   = errors.New("whd: invalid EtherType")
	errInvalidOUI         = errors.New("invalid oui; expected broadcom OUI")
	errInvalidSubtype     = errors.New("invalid subtype; expected BCMILCP_SUBTYPE_VENDOR_LONG=32769")
	errInvalidUserSubtype = errors.New("invalid user subtype; expected BCMILCP_BCM_SUBTYPE_EVENT=1")
)

type SDPCMHeader struct {
	Size         uint16
	SizeCom      uint16 // complement of size, so ^Size.
	Seq          uint8  // Rx/Tx sequence number
	ChanAndFlags uint8  // 4 MSB Channel number, 4 LSB arbitrary flag
	// channel types: Control=0; Event=1; Data=2.
	NextLength      uint8 // length of next data frame, reserved for Tx
	HeaderLength    uint8 // data offset
	WirelessFlowCtl uint8 // flow control bits, reserved for Tx
	BusDataCredit   uint8 // maximum Sequence number allowed by firmware for Tx
	Reserved        [2]uint8
}

func (s SDPCMHeader) Type() SDPCMHeaderType { return SDPCMHeaderType(s.ChanAndFlags & 0xf) }

// DecodeSDPCMHeader c-ref:LittleEndian
func DecodeSDPCMHeader(order binary.ByteOrder, b []byte) (hdr SDPCMHeader) {
	_ = b[SDPCM_HEADER_LEN-1]
	hdr.Size = order.Uint16(b)
	hdr.SizeCom = order.Uint16(b[2:])
	hdr.Seq = b[4]
	hdr.ChanAndFlags = b[5]
	hdr.NextLength = b[6]
	hdr.HeaderLength = b[7]
	hdr.WirelessFlowCtl = b[8]
	hdr.BusDataCredit = b[9]
	copy(hdr.Reserved[:], b[10:])
	return hdr
}

// Put puts all 12 bytes of sdpcmHeader in dst. Panics if dst is shorter than 12 bytes in length.
func (s *SDPCMHeader) Put(order binary.ByteOrder, dst []byte) {
	_ = dst[11]
	order.PutUint16(dst, s.Size)
	order.PutUint16(dst[2:], s.SizeCom)
	dst[4] = s.Seq
	dst[5] = s.ChanAndFlags
	dst[6] = s.NextLength
	dst[7] = s.HeaderLength
	dst[8] = s.WirelessFlowCtl
	dst[9] = s.BusDataCredit
	copy(dst[10:], s.Reserved[:])
}

func (s *SDPCMHeader) Parse(packet []byte) ([]byte, error) {
	if len(packet) < int(s.Size) {
		return nil, errBadSPCM
	}
	if s.Size != ^s.SizeCom {
		return nil, errSDPCMHeaderSizeComplementMismatch
	}
	if int(s.Size) != len(packet) {
		return nil, errSDPCMHeaderSizeMismatch
	}

	return packet[s.HeaderLength:], nil
}

type CDCHeader struct {
	Cmd    SDPCMCommand
	Length uint32
	Flags  uint16
	ID     uint16
	Status uint32
}

// DecodeCDCHeader c-ref:LittleEndian
func DecodeCDCHeader(order binary.ByteOrder, b []byte) (hdr CDCHeader) {
	// Reference: https://github.com/Infineon/wifi-host-driver/blob/ad3bad006082488163cfb5d5613dcd2bdaddca90/WiFi_Host_Driver/src/whd_cdc_bdc.c#L518-L519
	_ = b[CDC_HEADER_LEN-1]
	hdr.Cmd = SDPCMCommand(order.Uint32(b))
	hdr.Length = order.Uint32(b[4:])

	flags := dtoh32(order.Uint32(b[8:]))
	hdr.Flags = uint16(flags & 0xffff)
	hdr.ID = uint16((flags & CDCF_IOC_ID_MASK) >> CDCF_IOC_ID_SHIFT)

	hdr.Status = order.Uint32(b[12:])
	return hdr
}

// Put c-ref:LittleEndian
func (cdc *CDCHeader) Put(order binary.ByteOrder, b []byte) {
	_ = b[15]
	flags := uint32(cdc.ID)<<CDCF_IOC_ID_SHIFT | uint32(cdc.Flags)
	order.PutUint32(b, uint32(cdc.Cmd))
	order.PutUint32(b[4:], cdc.Length)
	order.PutUint32(b[8:], htod32(flags))
	order.PutUint32(b[12:], cdc.Status)
}

func (cdc *CDCHeader) Parse(packet []byte) (payload []byte, err error) {
	if len(packet) < CDC_HEADER_LEN+int(cdc.Length) {
		return nil, errShortBufferCDC
	}
	payload = packet[CDC_HEADER_LEN:]
	return
}

type BDCHeader struct {
	Flags      uint8
	Priority   uint8 // 802.1d Priority (low 3 bits)
	Flags2     uint8
	DataOffset uint8 // Offset from end of BDC header to packet data, in
	// 4-uint8_t words. Leaves room for optional headers.
}

func (bdc *BDCHeader) Put(b []byte) {
	_ = b[3]
	b[0] = bdc.Flags
	b[1] = bdc.Priority
	b[2] = bdc.Flags2
	b[3] = bdc.DataOffset
}

func DecodeBDCHeader(b []byte) (hdr BDCHeader) {
	_ = b[3]
	hdr.Flags = b[0]
	hdr.Priority = b[1]
	hdr.Flags2 = b[2]
	hdr.DataOffset = b[3]
	return hdr
}

type AsyncEvent struct {
	_         uint16
	Flags     uint16
	EventType AsyncEventType
	Status    uint32
	Reason    uint32
	_         [30]byte
	Interface uint8
	_         uint8
	u         EventScanResult
}

// ParseAsyncEvent c-ref:BigEndian
// reference: cyw43_ll_parse_async_event
func ParseAsyncEvent(order binary.ByteOrder, buf []byte) (ev AsyncEvent, err error) {
	if len(buf) < 48 {
		return ev, io.ErrShortBuffer
	}
	ev.Flags = order.Uint16(buf[2:])
	ev.EventType = AsyncEventType(order.Uint32(buf[4:]))
	ev.Status = order.Uint32(buf[8:])
	ev.Reason = order.Uint32(buf[12:])
	const ifaceOffset = 12 + 4 + 30
	ev.Interface = buf[ifaceOffset]
	if ev.EventType == CYW43_EV_ESCAN_RESULT && ev.Status == CYW43_STATUS_PARTIAL {
		const sizeStruct = unsafe.Sizeof(ev)
		if len(buf) < int(sizeStruct) {
			return ev, io.ErrShortBuffer
		}
		ev.u, err = ParseScanResult(order, buf[48:])
	}
	return ev, err
}

func (ev *AsyncEvent) EventScanResult() *EventScanResult {
	return &ev.u
}

type evscanresult struct {
	Version      uint32   // 0:4
	Length       uint32   // 4:8
	BSSID        [6]byte  // 8:14
	BeaconPeriod uint16   // 14:16
	SSIDLength   uint8    // 16:17
	Capability   uint16   // 17:19
	SSID         [32]byte //
	RatesetCount uint32
	RatesetRates [16]uint8
	ChanSpec     uint16
	AtimWindow   uint16
	DtimPeriod   uint8
	RSSI         int16
	PHYNoise     int8
	NCap         uint8
	NBSSCap      uint32
	CtlCh        uint8
	_            [1]uint32
	Flags        uint8
	_            [3]uint8
	BasicMCS     [16]uint8
	IEOffset     uint16
	IELength     uint32
	SNR          int16
}

// EventScanResult holds wifi scan results.
type EventScanResult struct {
	_ [5]uint32
	// Access point MAC address.
	BSSID [6]uint8
	_     [2]uint16
	// Length of access point name.
	SSIDLength uint8
	// WLAN access point name.
	SSID    [32]byte
	_       [5]uint32
	Channel uint16
	_       uint16
	// Wifi auth mode. See CYW43_AUTH_*.
	AuthMode uint8
	// Signal strength.
	RSSI int16
}

// reference: cyw43_ll_wifi_parse_scan_result
func ParseScanResult(order binary.ByteOrder, buf []byte) (sr EventScanResult, err error) {
	type scanresult struct {
		buflen   uint32
		version  uint32
		syncid   uint16
		bssCount uint16
		bss      evscanresult
	}
	if len(buf) > int(unsafe.Sizeof(scanresult{})) {
		return sr, io.ErrShortBuffer
	}
	ptr := unsafe.Pointer(&buf[0])
	if uintptr(ptr)%4 != 0 {
		return sr, errBufferUnaligned
	}
	scan := (*scanresult)(ptr)
	if uint32(scan.bss.IEOffset)+scan.bss.IELength > scan.bss.Length {
		return sr, errIEEndExceedsBSS
	}
	// TODO(soypat): lots of stuff missing here.
	return *(*EventScanResult)(unsafe.Pointer(&scan.bss)), nil
}

// ScanOptions are wifi scan options.
type ScanOptions struct {
	Version uint32
	Action  uint16
	_       uint16
	// 0=all
	SSIDLength uint32
	// SSID Name.
	SSID    [32]byte
	BSSID   [6]byte
	BSSType int8
	// Scan type. 0=active, 1=passive.
	ScanType    int8
	NProbes     int32
	ActiveTime  int32
	PassiveTime int32
	HomeTime    int32
	ChannelNum  int32
	ChannelList [1]uint16
}

type DownloadHeader struct {
	Flags uint16 // VER=0x1000, NO_CRC=0x1, BEGIN=0x2, END=0x4
	Type  uint16 // Download type.
	Len   uint32
	CRC   uint32
}

func (dh *DownloadHeader) Put(order binary.ByteOrder, b []byte) {
	order.PutUint16(b[0:2], dh.Flags)
	order.PutUint16(b[2:4], dh.Type)
	order.PutUint32(b[4:8], dh.Len)
	order.PutUint32(b[8:12], dh.CRC)
}

type EventPacket struct {
	EthHeader   eth.EthernetHeader // 0:14
	EventHeader EventHeader        // 14:24
	Message     EventMessage       // 24:72
}

type EventHeader struct {
	Subtype     uint16  // 0:2
	Length      uint16  // 2:4
	Version     uint8   // 4:5
	OUI         [3]byte // 5:8
	UserSubtype uint16  // 8:10
}

type EventMessage struct {
	Version   uint16         // 0:2
	Flags     uint16         // 2:4
	EventType AsyncEventType // 4:8
	Status    uint32         // 8:12
	Reason    uint32         // 12:16
	AuthType  uint32         // 16:20
	DataLen   uint32         // 20:24
	Addr      [6]byte        // 24:30
	IFName    [16]byte       // 30:46 Name of incoming packet interface.
	IFIdx     uint8          // 46:47
	BSSCfgIdx uint8          // 47:48
}

// DecodeEventPacket decodes an async event packet. Requires 72 byte buffer.
func DecodeEventPacket(order binary.ByteOrder, buf []byte) (ev EventPacket, err error) {
	// https://github.com/embassy-rs/embassy/blob/26870082427b64d3ca42691c55a2cded5eadc548/cyw43/src/structs.rs#L234C18-L234C18
	const totalLen = 14 + 10 + 48
	if len(buf) < totalLen {
		return ev, io.ErrShortBuffer
	}
	ev.EthHeader = eth.DecodeEthernetHeader(buf[:14])
	if ev.EthHeader.AssertType() != 0x886c {
		return ev, ErrInvalidEtherType
	}
	ev.EventHeader = DecodeEventHeader(order, buf[14:24])
	const (
		BCMILCP_SUBTYPE_VENDOR_LONG = 32769
		BCMILCP_BCM_SUBTYPE_EVENT   = 1
	)
	switch {
	case ev.EventHeader.OUI != [3]byte{0x00, 0x10, 0x18}:
		return ev, errInvalidOUI
	case ev.EventHeader.Subtype != BCMILCP_SUBTYPE_VENDOR_LONG:
		return ev, errInvalidSubtype
	case ev.EventHeader.UserSubtype != BCMILCP_BCM_SUBTYPE_EVENT:
		return ev, errInvalidUserSubtype
	}
	ev.Message = DecodeEventMessage(order, buf[24:totalLen])
	return ev, nil
}

func DecodeEventHeader(order binary.ByteOrder, buf []byte) (ev EventHeader) {
	_ = buf[9]
	ev.Subtype = order.Uint16(buf)
	ev.Length = order.Uint16(buf[2:])
	ev.Version = buf[4]
	copy(ev.OUI[:], buf[5:8])
	ev.UserSubtype = order.Uint16(buf[8:])
	return ev
}

func DecodeEventMessage(order binary.ByteOrder, buf []byte) (ev EventMessage) {
	_ = buf[47]
	ev.Version = order.Uint16(buf)
	ev.Flags = order.Uint16(buf[2:])
	ev.EventType = AsyncEventType(order.Uint32(buf[4:]))
	ev.Status = order.Uint32(buf[8:])
	ev.Reason = order.Uint32(buf[12:])
	ev.AuthType = order.Uint32(buf[16:])
	ev.DataLen = order.Uint32(buf[20:])
	copy(ev.Addr[:], buf[24:30])
	copy(ev.IFName[:], buf[30:46])
	ev.IFIdx = buf[46]
	ev.BSSCfgIdx = buf[47]
	return ev
}

// dtoh32 device to host 32 bit.
func dtoh32(v uint32) uint32 {
	switch runtime.GOARCH {
	// Little endian architecture.
	case "arm", "amd64":
		return v

	// Big endian architecture.
	case "XXX":
		return swap32(v)
	}
	panic("please define your architecture in whd/protocol.go")
}

// dtoh32 host to device 32 bit.
func htod32(v uint32) uint32 {
	switch runtime.GOARCH {
	// Little endian architecture.
	case "arm", "amd64":
		return v

	// Big endian architecture.
	case "XXX":
		return swap32(v)
	}
	panic("please define your architecture in whd/protocol.go")
}

func swap32(v uint32) uint32 {
	return (v&0xff)<<24 | (v&0xff00)<<8 | (v&0xff0000)>>8 | (v&0xff000000)>>24
}
func swap16(v uint16) uint16 { return (v&0xff)<<8 | (v&0xff00)>>8 }
