package cyw43439

import (
	"encoding/binary"
	"errors"
	"unsafe"
)

func (d *Dev) LED() Pin {
	const RaspberryPiPicoWOnboardLED = 0
	return Pin{
		pin: RaspberryPiPicoWOnboardLED,
		d:   d,
	}
}

type Pin struct {
	pin uint8
	d   *Dev
}

func (p Pin) Set(b bool) error {
	return p.d.GPIOSet(p.pin, b)
}

func (p Pin) High() error { return p.Set(true) }
func (p Pin) Low() error  { return p.Set(false) }

// cy_ioctl.go contains multi-word control IO functions for controlling
// the CYW43439's inputs and outputs including Wifi, Bluetooth and GPIOs.
// Most of these were inspired by cyw43-driver/src/cyw43_ll.c contents.

const (
	sdpcmCTLHEADER      = 0
	sdpcmASYNCEVTHEADER = 1
	sdpcmDATAHEADER     = 2
)

// Wifi command for sdpcm send common.
const (
	wlc_UP            = 2
	wlc_SET_INFRA     = 20
	wlc_SET_AUTH      = 22
	wlc_GET_SSID      = 25
	wlc_SET_SSID      = 26
	wlc_SET_CHANNEL   = 30
	wlc_DISASSOC      = 52
	wlc_GET_ANTDIV    = 63
	wlc_SET_ANTDIV    = 64
	wlc_SET_DTIMPRD   = 78
	wlc_SET_PM        = 86
	wlc_SET_GMODE     = 110
	wlc_SET_WSEC      = 134
	wlc_SET_BAND      = 142
	wlc_GET_ASSOCLIST = 159
	wlc_SET_WPA_AUTH  = 165
	wlc_SET_VAR       = 263
	wlc_GET_VAR       = 262
	wlc_SET_WSEC_PMK  = 268
)

// Interface.
const (
	wwd_STA_INTERFACE = 0
	wwd_AP_INTERFACE  = 1
	wwd_P2P_INTERFACE = 2
)

type sdpcmHeader struct {
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

type ioctlHeader struct {
	Cmd    uint32
	Len    uint32
	Flags  uint32
	Status uint32
}

// PaddedSize rounds size up to multiple of 64.
func (s *sdpcmHeader) PaddedSize() uint32 {
	a := uint32(s.Size)
	return (a + 63) &^ 63
}

func (s *sdpcmHeader) ArrayPtr() *[12]byte {
	const mustBe12 = unsafe.Sizeof(sdpcmHeader{}) // Will fail to compile when size changes.
	return (*[mustBe12]byte)(unsafe.Pointer(s))
}

// Put puts all 12 bytes of sdpcmHeader in dst. Panics if dst is shorter than 12 bytes in length.
func (s *sdpcmHeader) Put(dst []byte) {
	_ = dst[11]
	ptr := s.ArrayPtr()[:]
	copy(dst, ptr)
}

func (io *ioctlHeader) ArrayPtr() *[16]byte {
	const mustBe16 = unsafe.Sizeof(ioctlHeader{}) // Will fail to compile when size changes.
	return (*[mustBe16]byte)(unsafe.Pointer(io))
}

// Put puts all 16 bytes of ioctlHeader in dst. Panics if dst is shorter than 16 bytes in length.
func (io *ioctlHeader) Put(dst []byte) {
	_ = dst[15]
	ptr := io.ArrayPtr()[:]
	copy(dst, ptr)
}

func (d *Dev) GPIOSet(wlGPIO uint8, value bool) (err error) {
	if wlGPIO >= 3 {
		panic("GPIO out of range 0..2")
	}
	val := uint32(1 << wlGPIO)
	if value {
		err = d.Write2IOVar("gpioout", wwd_STA_INTERFACE, val, val)
	} else {
		err = d.Write2IOVar("gpioout", wwd_STA_INTERFACE, val, 0)
	}
	return err
}

func (d *Dev) Write2IOVar(VAR string, iface, val0, val1 uint32) error {
	println("write2iovar")
	buf := d.buf[1024:]
	length := copy(buf, VAR)

	binary.BigEndian.PutUint32(buf[length:], val0)
	binary.BigEndian.PutUint32(buf[length+4:], val1)
	return d.doIoctl(2, iface, wlc_SET_VAR, buf[:length+8])
}

func (d *Dev) DoIoctl32(kind, iface, cmd, val uint32) error {
	println("DoIoctl32")
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], val)
	return d.doIoctl(kind, iface, cmd, buf[:])
}

func (d *Dev) ioctl(cmd, iface uint32, w []byte) error {
	kind := uint32(0)
	if cmd&1 != 0 {
		kind = 2
	}
	return d.doIoctl(kind, iface, cmd>>1, w)
}

func (d *Dev) doIoctl(kind, iface, cmd uint32, w []byte) error {
	// TODO do we have to add all that polling?
	return d.sendIoctl(kind, iface, cmd, w)
}

// sendIoctl is cyw43_send_ioctl in pico-sdk (actually contained in cy43-driver)
func (d *Dev) sendIoctl(kind, iface, cmd uint32, w []byte) error {
	println("sendIoctl")
	length := uint32(len(w))
	const sdpcmSize = uint32(unsafe.Sizeof(sdpcmHeader{}))
	const ioctlSize = uint32(unsafe.Sizeof(ioctlHeader{}))
	if uint32(len(d.buf)) < sdpcmSize+ioctlSize+length {
		return errors.New("ioctl buffer too large for sending")
	}
	d.sdpcmRequestedIoctlID++
	id := uint32(d.sdpcmRequestedIoctlID)
	flags := (id<<16)&0xffff_0000 | kind | iface<<12 // look for CDCF_IOC* identifiers in pico-sdk
	header := ioctlHeader{
		Cmd:   cmd,
		Len:   length & 0xffff,
		Flags: flags,
	}

	header.Put(d.buf[sdpcmSize:])
	copy(d.buf[sdpcmSize+ioctlSize:], w)
	return d.sendSDPCMCommon(sdpcmCTLHEADER, cmd, d.buf[:sdpcmSize+ioctlSize+length])
}

// sendSDPCMCommon is cyw43_sdpcm_send_common in pico-sdk (actually contained in cy43-driver)
func (d *Dev) sendSDPCMCommon(kind, cmd uint32, w []byte) error {
	println("sendSDPCMCommon")
	if kind != sdpcmCTLHEADER && kind != sdpcmDATAHEADER {
		return errors.New("unexpected SDPCM kind")
	}
	headerLength := uint8(unsafe.Sizeof(sdpcmHeader{}))
	if kind == 2 {
		headerLength += 2
	}
	size := uint16(len(w)) + uint16(headerLength)
	paddedSize := (size + 63) &^ 63
	if uint16(cap(w)) < paddedSize {
		return errors.New("buffer too small to be SDPCM padded")
	}
	w = w[:paddedSize]
	header := sdpcmHeader{
		Size:         size,
		SizeCom:      ^size & 0xffff,
		Seq:          d.sdpcmTxSequence,
		ChanAndFlags: uint8(kind),
		HeaderLength: headerLength,
	}
	header.Put(w)
	d.sdpcmTxSequence++
	return d.WriteBytes(FuncWLAN, 0, w)
}

// WriteBytes is cyw43_write_bytes
func (d *Dev) WriteBytes(fn Function, addr uint32, src []byte) error {
	println("writeBytes")
	length := uint32(len(src))
	alignedLength := (length + 3) &^ 3
	if length != alignedLength {
		return errors.New("buffer length must be length multiple of 4")
	}
	if fn == FuncBackplane || !(length <= 64 && (addr+length) <= 0x8000) {
		panic("bad argument to WriteBytes")
	}
	if fn == FuncWLAN {
		readyAttempts := 1000
		for ; readyAttempts > 0; readyAttempts-- {
			status, err := d.GetStatus()
			if err != nil {
				return err
			}
			if status.F2RxReady() {
				break
			}
		}
		if readyAttempts <= 0 {
			return errors.New("F2 not ready")
		}
	}
	cmd := make_cmd(true, true, fn, addr, length)
	return d.SPIWrite(cmd, src)
}

func (d *Dev) ReadBytes(fn Function, addr uint32, src []byte) error {
	const maxReadPacket = 2040
	length := uint32(len(src))
	alignedLength := (length + 3) &^ 3
	if length != alignedLength {
		return errors.New("buffer length must be length multiple of 4")
	}
	assert := fn == FuncBackplane || (length <= 64 && (addr+length) <= 0x8000)
	assert = assert && alignedLength > 0 && alignedLength < maxReadPacket
	if !assert {
		panic("bad argument to WriteBytes")
	}
	padding := uint32(0)
	if fn == FuncBackplane {
		padding = 4
		if cap(src) < len(src)+4 {
			return errors.New("ReadBytes src arg requires more capacity for byte padding")
		}
		src = src[:len(src)+4]
	}
	// TODO: Use DelayResponse to simulate padding effect.
	cmd := make_cmd(false, true, fn, addr, length+padding)
	return d.SPIRead(cmd, src)
}
