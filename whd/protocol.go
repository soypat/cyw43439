package whd

import (
	"encoding/binary"
	"errors"
	"unsafe"
)

type SDPCMHeader struct {
	Size            uint16
	SizeCom         uint16 // complement of size, so ^Size.
	Seq             uint8
	ChanAndFlags    uint8 // Channel types: Control=0; Event=1; Data=2.
	NextLength      uint8
	HeaderLength    uint8
	WirelessFlowCtl uint8
	BusDataCredit   uint8
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

type IoctlHeader struct {
	Cmd    SDPCMCommand
	Len    uint32
	Flags  uint32
	Status uint32
}

func (io *IoctlHeader) ID() uint16 {
	return uint16((io.Flags & CDCF_IOC_ID_MASK) >> CDCF_IOC_ID_SHIFT)
}

// DecodeIoctlHeader c-ref:LittleEndian
func DecodeIoctlHeader(order binary.ByteOrder, b []byte) (hdr IoctlHeader) {
	_ = b[IOCTL_HEADER_LEN-1]
	hdr.Cmd = SDPCMCommand(order.Uint32(b))
	hdr.Len = order.Uint32(b[4:])
	hdr.Flags = order.Uint32(b[8:])
	hdr.Status = order.Uint32(b[12:])
	return hdr
}

// Put puts all 16 bytes of ioctlHeader in dst. Panics if dst is shorter than 16 bytes in length.
func (io *IoctlHeader) Put(order binary.ByteOrder, dst []byte) {
	_ = dst[15]
	order.PutUint32(dst, uint32(io.Cmd))
	order.PutUint32(dst[4:], io.Len)
	order.PutUint32(dst[8:], io.Flags)
	order.PutUint32(dst[12:], io.Status)
}

type BDCHeader struct {
	Flags      uint8
	Priority   uint8
	Flags2     uint8
	DataOffset uint8
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

type CDCHeader struct {
	Cmd    SDPCMCommand
	Length uint32
	Flags  uint16
	ID     uint16
	Status uint32
}

// DecodeCDCHeader c-ref:LittleEndian
func DecodeCDCHeader(order binary.ByteOrder, b []byte) (hdr CDCHeader) {
	_ = b[CDC_HEADER_LEN-1]
	hdr.Cmd = SDPCMCommand(order.Uint32(b))
	hdr.Length = order.Uint32(b[4:])
	hdr.Flags = order.Uint16(b[8:])
	hdr.ID = order.Uint16(b[10:])
	hdr.Status = order.Uint32(b[12:])
	return hdr
}

// Put c-ref:LittleEndian
func (cdc *CDCHeader) Put(order binary.ByteOrder, b []byte) {
	_ = b[15]
	order.PutUint32(b, uint32(cdc.Cmd))
	order.PutUint32(b[4:], cdc.Length)
	order.PutUint16(b[8:], cdc.Flags)
	order.PutUint16(b[10:], cdc.ID)
	order.PutUint32(b[12:], cdc.Status)
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
		return ev, errors.New("buffer too small to parse async event")
	}
	ev.Flags = order.Uint16(buf[2:])
	ev.EventType = AsyncEventType(order.Uint32(buf[4:]))
	ev.Status = order.Uint32(buf[8:])
	ev.Reason = order.Uint32(buf[12:])
	const ifaceOffset = 12 + 4 + 30
	ev.Interface = buf[ifaceOffset]
	if ev.EventType == CYW43_EV_ESCAN_RESULT && ev.Status == CYW43_STATUS_PARTIAL {
		if len(buf) < int(unsafe.Sizeof(ev)) {
			return ev, errors.New("buffer too small to parse scan results")
		}
		ev.u, err = ParseScanResult(order, buf[48:])
	}
	return ev, err
}

func (ev *AsyncEvent) EventScanResult() *EventScanResult {
	return &ev.u
}

type evscanresult struct {
	Version      uint32  // 1
	Length       uint32  // 2
	BSSID        [6]byte // 3.5
	BeaconPeriod uint16  // 4
	Capability   uint16  // 4.5
	SSIDLength   uint8   // 4.25
	SSID         [32]byte
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
		return sr, errors.New("buffer to small for scanresult")
	}

	println("prep deref")
	ptr := unsafe.Pointer(&buf[0])
	if uintptr(ptr)%4 != 0 {
		return sr, errors.New("buffer not aligned to 4 bytes")
	}
	scan := (*scanresult)(ptr)
	if uint32(scan.bss.IEOffset)+scan.bss.IELength > scan.bss.Length {
		return sr, errors.New("IE end exceeds bss length")
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
