package whd

import (
	"encoding/binary"
	"errors"
	"unsafe"
)

type SDPCMHeader struct {
	Size            uint16
	SizeCom         uint16
	Seq             uint8
	ChanAndFlags    uint8
	NextLength      uint8
	HeaderLength    uint8
	WirelessFlowCtl uint8
	BusDataCredit   uint8
	Reserved        [2]uint8
}

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
	_ = dst[11]
	ptr := s.arrayPtr()[:]
	copy(dst, ptr)
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

type AsyncEvent struct {
	_         uint16
	Flags     uint16
	EventType uint32
	Status    uint32
	Reason    uint32
	_         [30]byte
	Interface uint8
	u         EventScanResult
}

func ParseAsyncEvent(buf []byte) (as AsyncEvent, err error) {
	if len(buf) < int(unsafe.Sizeof(as)) {
		return as, errors.New("buffer too small to parse async event")
	}
	as.Flags = binary.BigEndian.Uint16(buf[2:])
	as.EventType = binary.BigEndian.Uint32(buf[4:])
	as.Status = binary.BigEndian.Uint32(buf[8:])
	as.Reason = binary.BigEndian.Uint32(buf[12:])
	const ifaceOffset = 12 + 4 + 30
	as.Interface = buf[ifaceOffset]
	if as.EventType == CYW43_EV_ESCAN_RESULT {
		as.u, err = ParseScanResult(buf[48:])
	}
	return as, err
}

func (aev *AsyncEvent) EventScanResult() *EventScanResult {
	return &aev.u
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
	BSSID [8]uint8
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
	scan := (*scanresult)(unsafe.Pointer(&buf[0]))
	if uint32(scan.bss.IEOffset)+scan.bss.IELength > scan.bss.Length {
		return sr, errors.New("IE end exceeds bss length")
	}
	// TODO lots of stuff missing here.
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
