package whd

import (
	"encoding/binary"
	"errors"
	"unsafe"
)

type SDPCMHeader struct {
	Size            uint16
	SizeCom         uint16 // complement of size, so ^Size.
	Seq             uint8  // Rx/Tx sequence number
	ChanAndFlags    uint8  // 4 MSB Channel number, 4 LSB arbitrary flag
			       // channel types: Control=0; Event=1; Data=2.
	NextLength      uint8  // length of next data frame, reserved for Tx
	HeaderLength    uint8  // data offset
	WirelessFlowCtl uint8  // flow control bits, reserved for Tx
	BusDataCredit   uint8  // maximum Sequence number allowed by firmware for Tx
	Reserved        [2]uint8
}

func (s SDPCMHeader) Type() SDPCMHeaderType { return SDPCMHeaderType(s.ChanAndFlags & 0xf) }

func DecodeSDPCMHeader(b []byte) (hdr SDPCMHeader) {
	_ = b[SDPCM_HEADER_LEN-1]
	hdr.Size = binary.LittleEndian.Uint16(b)
	hdr.SizeCom = binary.LittleEndian.Uint16(b[2:])
	hdr.Seq = b[4]
	hdr.ChanAndFlags = b[5]
	hdr.NextLength = b[6]
	hdr.HeaderLength = b[7]
	hdr.WirelessFlowCtl = b[8]
	hdr.BusDataCredit = b[9]
	copy(hdr.Reserved[:], b[10:])
	return hdr
}

func (s *SDPCMHeader) arrayPtr() *[12]byte {
	const mustBe12 = unsafe.Sizeof(SDPCMHeader{}) // Will fail to compile when size changes.
	return (*[mustBe12]byte)(unsafe.Pointer(s))
}

// Put puts all 12 bytes of sdpcmHeader in dst. Panics if dst is shorter than 12 bytes in length.
func (s *SDPCMHeader) Put(dst []byte) {
	_ = dst[SDPCM_HEADER_LEN-1]
	ptr := s.arrayPtr()[:]
	copy(dst, ptr)
}

func (s SDPCMHeader) Parse(packet []byte) (payload []byte, err error) {
	if len(packet) < SDPCM_HEADER_LEN {
		err = errors.New("packet shorter than sdpcm hdr, len=", strconv.Itoa(len(packet)))
		return
	}

	if s.Size != !s.SizeCom {
		err = errors.New("sdpcm hdr size complement mismatch")
		return
	}

	if s.Size != len(packet) {
		err = errors.New("sdpcm hdr size doesn't match packet length from SPI")
		return
	}

	payload = packet[SDPCM_HEADER_LEN:]
	return
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

func DecodeIoctlHeader(b []byte) (hdr IoctlHeader) {
	_ = b[IOCTL_HEADER_LEN-1]
	hdr.Cmd = SDPCMCommand(binary.LittleEndian.Uint32(b))
	hdr.Len = binary.LittleEndian.Uint32(b[4:])
	hdr.Flags = binary.LittleEndian.Uint32(b[8:])
	hdr.Status = binary.LittleEndian.Uint32(b[12:])
	return hdr
}

func (io *IoctlHeader) arrayPtr() *[16]byte {
	const mustBe16 = unsafe.Sizeof(IoctlHeader{}) // Will fail to compile when size changes.
	return (*[mustBe16]byte)(unsafe.Pointer(io))
}

// Put puts all 16 bytes of ioctlHeader in dst. Panics if dst is shorter than 16 bytes in length.
func (io *IoctlHeader) Put(dst []byte) {
	_ = dst[15]
	ptr := io.arrayPtr()[:]
	copy(dst, ptr)
}

type BDCHeader struct {
	Flags      uint8
	Priority   uint8
	Flags2     uint8
	DataOffset uint8
}

func (bdc *BDCHeader) arrayptr() *[4]byte {
	return (*[4]byte)(unsafe.Pointer(bdc))
}

func (bdc *BDCHeader) ptr() *uint32 {
	return (*uint32)(unsafe.Pointer(bdc))
}

func (bdc *BDCHeader) Put(b []byte) {
	_ = b[3]
	// ptr := bdc.uint32ptr()
	copy(b, bdc.arrayptr()[:])
}

func DecodeBDCHeader(b []byte) (hdr BDCHeader) {
	_ = b[3]
	copy(hdr.arrayptr()[:], b)
	return hdr
}

type CDCHeader struct {
	Cmd    uint32
	Length uint32
	Flags  uint16
	ID     uint16
	Status uint32
}

func DecodeCDCHeader(b []byte) (hdr CDCHeader) {
	_ = b[CDC_HEADER_LEN-1]
	hdr.Cmd = binary.LittleEndian.Uint32(b)
	hdr.Length = binary.LittleEndian.Uint32(b[4:])
	hdr.Flags = binary.LittleEndian.Uint16(b[8:])
	hdr.ID = binary.LittleEndian.Uint16(b[10:])
	hdr.Status = binary.LittleEndian.Uint32(b[12:])
	return hdr
}

func (cdc *CDCHeader) Put(b []byte) {
	_ = b[15]
	binary.LittleEndian.PutUint32(b, cdc.Cmd)
	binary.LittleEndian.PutUint32(b[4:], cdc.Length)
	binary.LittleEndian.PutUint16(b[8:], cdc.Flags)
	binary.LittleEndian.PutUint16(b[10:], cdc.ID)
	binary.LittleEndian.PutUint32(b[12:], cdc.Status)
}

func (cdc CDCHeader) Parse(packet []byte) (payload []byte, err error) {
	if len(packet) < CDC_HEADER_LEN {
		err = errors.New("packet shorter than cdc hdr, len=", strconv.Itoa(len(packet)))
		return
	}
	payload = packet[CDC_HEADER_LEN:]
	return
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

// reference: cyw43_ll_parse_async_event
func ParseAsyncEvent(buf []byte) (ev AsyncEvent, err error) {
	if len(buf) < 48 {
		return ev, errors.New("buffer too small to parse async event")
	}
	ev.Flags = binary.BigEndian.Uint16(buf[2:])
	ev.EventType = AsyncEventType(binary.BigEndian.Uint32(buf[4:]))
	ev.Status = binary.BigEndian.Uint32(buf[8:])
	ev.Reason = binary.BigEndian.Uint32(buf[12:])
	const ifaceOffset = 12 + 4 + 30
	ev.Interface = buf[ifaceOffset]
	if ev.EventType == CYW43_EV_ESCAN_RESULT && ev.Status == CYW43_STATUS_PARTIAL {
		if len(buf) < int(unsafe.Sizeof(ev)) {
			return ev, errors.New("buffer too small to parse scan results")
		}
		ev.u, err = ParseScanResult(buf[48:])
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
func ParseScanResult(buf []byte) (sr EventScanResult, err error) {
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
