package whd

import (
	"encoding/binary"
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
