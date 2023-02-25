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

const (
	CORE_WLAN_ARM              = 1
	WLAN_ARMCM3_BASE_ADDRESS   = 0x18003000
	WRAPPER_REGISTER_OFFSET    = 0x100000
	CORE_SOCRAM                = 2
	SOCSRAM_BASE_ADDRESS       = 0x18004000
	SBSDIO_SB_ACCESS_2_4B_FLAG = 0x08000
	CHIPCOMMON_BASE_ADDRESS    = 0x18000000
	backplaneAddrMask          = 0x7fff
)

func (d *Dev) DisableDeviceCore(coreID uint8, coreHalt bool) error {
	// base := coreaddress(coreID)
	// d.read
	return nil
}

func coreaddress(coreID uint8) (v uint32) {
	switch coreID {
	case CORE_WLAN_ARM:
		v = WLAN_ARMCM3_BASE_ADDRESS + WRAPPER_REGISTER_OFFSET
	case CORE_SOCRAM:
		v = SOCSRAM_BASE_ADDRESS + WRAPPER_REGISTER_OFFSET
	}
	return v
}

func (d *Dev) readBackplane(addr uint32, size uint32) (uint32, error) {
	err := d.setBackplaneWindow(addr)
	if err != nil {
		return 0, err
	}
	addr &= backplaneAddrMask
	if size == 4 {
		addr |= SBSDIO_SB_ACCESS_2_4B_FLAG
	}
	reg, err := d.rr(FuncBackplane, addr, size)
	if err != nil {
		return 0, err
	}
	err = d.setBackplaneWindow(CHIPCOMMON_BASE_ADDRESS)
	return reg, err
}

func (d *Dev) writeBackplane(addr, size, value uint32) error {
	err := d.setBackplaneWindow(addr)
	if err != nil {
		return err
	}
	addr &= backplaneAddrMask
	if size == 4 {
		addr |= SBSDIO_SB_ACCESS_2_4B_FLAG
	}
	err = d.wr(FuncBackplane, addr, size, value)
	if err != nil {
		return err
	}

	return d.setBackplaneWindow(CHIPCOMMON_BASE_ADDRESS)
}

func (d *Dev) setBackplaneWindow(addr uint32) (err error) {
	const (
		SDIO_BACKPLANE_ADDRESS_HIGH = 0x1000c
		SDIO_BACKPLANE_ADDRESS_MID  = 0x1000b
		SDIO_BACKPLANE_ADDRESS_LOW  = 0x1000a
	)
	addr = addr &^ backplaneAddrMask
	currentWindow := d.currentBackplaneWindow
	// TODO(soypat): maybe these should be calls to rr so that they are inlined?
	if (addr & 0xff000000) != currentWindow&0xff000000 {
		err = d.Write8(FuncBackplane, SDIO_BACKPLANE_ADDRESS_HIGH, uint8(addr>>24))
	}
	if err != nil && (addr&0x00ff0000) != currentWindow&0x00ff0000 {
		err = d.Write8(FuncBackplane, SDIO_BACKPLANE_ADDRESS_MID, uint8(addr>>16))
	}
	if err != nil && (addr&0x0000ff00) != currentWindow&0x0000ff00 {
		err = d.Write8(FuncBackplane, SDIO_BACKPLANE_ADDRESS_LOW, uint8(addr>>8))
	}
	if err != nil {
		return err
	}
	d.currentBackplaneWindow = addr
	return err
}

func (d *Dev) downloadResource(addr uint32, rawLen int) error {
	// round up length to simplify download.
	// rlen := (rawLen + 255) &^ 255
	// const maxBlockSize uint32 = 64
	// blockSize := maxBlockSize

	return nil
}
