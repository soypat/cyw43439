//go:build tinygo

package cyw43439

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"time"
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

type sdpcmCmd uint32

// Wifi command for sdpcm send common.
const (
	wlc_UP            sdpcmCmd = 2
	wlc_SET_INFRA     sdpcmCmd = 20
	wlc_SET_AUTH      sdpcmCmd = 22
	wlc_GET_SSID      sdpcmCmd = 25
	wlc_SET_SSID      sdpcmCmd = 26
	wlc_SET_CHANNEL   sdpcmCmd = 30
	wlc_DISASSOC      sdpcmCmd = 52
	wlc_GET_ANTDIV    sdpcmCmd = 63
	wlc_SET_ANTDIV    sdpcmCmd = 64
	wlc_SET_DTIMPRD   sdpcmCmd = 78
	wlc_SET_PM        sdpcmCmd = 86
	wlc_SET_GMODE     sdpcmCmd = 110
	wlc_SET_WSEC      sdpcmCmd = 134
	wlc_SET_BAND      sdpcmCmd = 142
	wlc_GET_ASSOCLIST sdpcmCmd = 159
	wlc_SET_WPA_AUTH  sdpcmCmd = 165
	wlc_SET_VAR       sdpcmCmd = 263
	wlc_GET_VAR       sdpcmCmd = 262
	wlc_SET_WSEC_PMK  sdpcmCmd = 268
)

type ioctlInterface uint8

// Interface.
const (
	wwd_STA_INTERFACE ioctlInterface = 0
	wwd_AP_INTERFACE  ioctlInterface = 1
	wwd_P2P_INTERFACE ioctlInterface = 2
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
	Cmd    sdpcmCmd
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
		err = d.WriteIOVar2("gpioout", wwd_STA_INTERFACE, val, val)
	} else {
		err = d.WriteIOVar2("gpioout", wwd_STA_INTERFACE, val, 0)
	}
	return err
}

func (d *Dev) WriteIOVar(VAR string, iface ioctlInterface, val uint32) error {
	buf := d.buf[1024:]
	length := copy(buf, VAR)
	buf[length] = 0 // Null terminate the string
	length++
	binary.BigEndian.PutUint32(buf[length:], val)
	return d.doIoctl(SDPCM_SET, iface, wlc_SET_VAR, buf[:length+4])
}

func (d *Dev) WriteIOVar2(VAR string, iface ioctlInterface, val0, val1 uint32) error {
	buf := d.buf[1024:]
	length := copy(buf, VAR)
	buf[length] = 0 // Null terminate the string
	length++
	binary.BigEndian.PutUint32(buf[length:], val0)
	binary.BigEndian.PutUint32(buf[length+4:], val1)
	return d.doIoctl(SDPCM_SET, iface, wlc_SET_VAR, buf[:length+8])
}

func (d *Dev) WriteIOVarN(VAR string, iface ioctlInterface, src []byte) error {
	iobuf := d.buf[1024:]
	if len(VAR)+len(src)+1 > len(iobuf) {
		return errors.New("buffer too short for IOVarN call")
	}
	length := copy(iobuf, VAR)
	iobuf[length] = 0 // Null terminate the string
	length++
	length += copy(iobuf[length:], src)
	return d.doIoctl(SDPCM_SET, iface, wlc_SET_VAR, iobuf[:length])
}

func (d *Dev) DoIoctl32(kind uint32, iface ioctlInterface, cmd sdpcmCmd, val uint32) error {
	println("DoIoctl32")
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], val)
	return d.doIoctl(kind, iface, cmd, buf[:])
}

func (d *Dev) ioctl(cmd sdpcmCmd, iface ioctlInterface, w []byte) error {
	kind := uint32(0)
	if cmd&1 != 0 {
		kind = 2
	}
	return d.doIoctl(kind, iface, cmd>>1, w)
}

func (d *Dev) doIoctl(kind uint32, iface ioctlInterface, cmd sdpcmCmd, w []byte) error {
	// TODO do we have to add all that polling?
	return d.sendIoctl(kind, iface, cmd, w)
}

// sendIoctl is cyw43_send_ioctl in pico-sdk (actually contained in cy43-driver)
func (d *Dev) sendIoctl(kind uint32, iface ioctlInterface, cmd sdpcmCmd, w []byte) error {
	println("sendIoctl")
	length := uint32(len(w))
	const sdpcmSize = uint32(unsafe.Sizeof(sdpcmHeader{}))
	const ioctlSize = uint32(unsafe.Sizeof(ioctlHeader{}))
	if uint32(len(d.buf)) < sdpcmSize+ioctlSize+length {
		return errors.New("ioctl buffer too large for sending")
	}
	d.sdpcmRequestedIoctlID++
	id := uint32(d.sdpcmRequestedIoctlID)
	flags := (id<<16)&0xffff_0000 | uint32(kind) | uint32(iface)<<12 // look for CDCF_IOC* identifiers in pico-sdk
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
func (d *Dev) sendSDPCMCommon(kind uint32, cmd sdpcmCmd, w []byte) error {
	// TODO where is cmd used here????
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

func (d *Dev) disableDeviceCore(coreID uint8, coreHalt bool) error {
	base := coreaddress(coreID)
	Debug("disable core", coreID, base)
	d.ReadBackplane(base+AI_RESETCTRL_OFFSET, 1)
	reg, err := d.ReadBackplane(base+AI_RESETCTRL_OFFSET, 1)
	if err != nil {
		return err
	}
	if reg&AIRC_RESET != 0 {
		return nil
	}
	Debug("core not in reset", reg)
	// TODO
	// println("core not in reset:", reg)
	return errors.New("core not in reset")
}

func (d *Dev) resetDeviceCore(coreID uint8, coreHalt bool) error {
	err := d.disableDeviceCore(coreID, coreHalt)
	if err != nil {
		return err
	}
	var cpuhaltFlag uint32
	if coreHalt {
		cpuhaltFlag = SICF_CPUHALT
	}
	base := coreaddress(coreID)
	d.WriteBackplane(base+AI_IOCTRL_OFFSET, 1, SICF_FGC|SICF_CLOCK_EN|cpuhaltFlag)
	d.ReadBackplane(base+AI_IOCTRL_OFFSET, 1)
	d.WriteBackplane(base+AI_RESETCTRL_OFFSET, 1, 0)
	time.Sleep(time.Millisecond)
	d.WriteBackplane(base+AI_IOCTRL_OFFSET, 1, SICF_CLOCK_EN|cpuhaltFlag)
	d.ReadBackplane(base+AI_IOCTRL_OFFSET, 1)
	time.Sleep(time.Millisecond)
	return nil
}

// CoreIsActive returns if the specified core is not in reset.
// Can be called with CORE_WLAN_ARM and CORE_SOCRAM global constants.
// It returns true if communications are down (WL_REG_ON at low).
func (d *Dev) CoreIsActive(coreID uint8) bool {
	base := coreaddress(coreID)
	reg, _ := d.ReadBackplane(base+AI_IOCTRL_OFFSET, 1)
	if reg&(SICF_FGC|SICF_CLOCK_EN) != SICF_CLOCK_EN {
		return false
	}
	reg, _ = d.ReadBackplane(base+AI_RESETCTRL_OFFSET, 1)
	return reg&AIRC_RESET == 0
}

func coreaddress(coreID uint8) (v uint32) {
	switch coreID {
	case CORE_WLAN_ARM:
		v = WRAPPER_REGISTER_OFFSET + WLAN_ARMCM3_BASE_ADDRESS
	case CORE_SOCRAM:
		v = WRAPPER_REGISTER_OFFSET + SOCSRAM_BASE_ADDRESS
	default:
		panic("bad core address")
	}
	return v
}

func (d *Dev) ReadBackplane(addr uint32, size uint32) (uint32, error) {
	err := d.setBackplaneWindow(addr)
	if err != nil {
		return 0, err
	}
	addr &= backplaneAddrMask
	if size == 4 {
		addr |= SBSDIO_SB_ACCESS_2_4B_FLAG
	}
	reg, err := d.rr(FuncBackplane, addr, size)
	Debug("read backplane", addr, "=", reg, err)
	if err != nil {
		return 0, err
	}
	err = d.setBackplaneWindow(CHIPCOMMON_BASE_ADDRESS)
	return reg, err
}

func (d *Dev) WriteBackplane(addr, size, value uint32) error {
	err := d.setBackplaneWindow(addr)
	if err != nil {
		return err
	}
	addr &= backplaneAddrMask
	if size == 4 {
		addr |= SBSDIO_SB_ACCESS_2_4B_FLAG
	}
	err = d.wr(FuncBackplane, addr, size, value)
	Debug("write backplane", addr, "=", value, err)
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
	currentWindow := d.currentBackplaneWindow
	// Debug("setting backplane window with addr=", addr, "currentwindow=", currentWindow, "maskaddr=", addr&^backplaneAddrMask)
	const (
		addrtest = 0x18003000 + 0x10000 + 0x800
		addrneg  = addrtest &^ backplaneAddrMask
		hiset    = addrneg&0xff000000 != 0
		medset   = addrneg&0xff0000 != 0
		loset    = addrneg&0xff00 != 0
	)
	addr = addr &^ backplaneAddrMask
	// TODO(soypat): maybe these should be calls to rr so that they are inlined?
	if (addr & 0xff000000) != currentWindow&0xff000000 {
		Debug("setting backplane addr hi")
		err = d.Write8(FuncBackplane, SDIO_BACKPLANE_ADDRESS_HIGH, uint8(addr>>24))
	}
	if err == nil && (addr&0x00ff0000) != currentWindow&0x00ff0000 {
		Debug("setting backplane addr mid")
		err = d.Write8(FuncBackplane, SDIO_BACKPLANE_ADDRESS_MID, uint8(addr>>16))
	}
	if err == nil && (addr&0x0000ff00) != currentWindow&0x0000ff00 {
		Debug("setting backplane addr low")
		err = d.Write8(FuncBackplane, SDIO_BACKPLANE_ADDRESS_LOW, uint8(addr>>8))
	}
	if err != nil {
		return err
	}
	d.currentBackplaneWindow = addr
	return err
}

func (d *Dev) downloadResource(addr uint32, src []byte) error {
	// round up length to simplify download.
	rlen := (len(src) + 255) &^ 255
	if cap(src) < rlen {
		return errors.New("firmware slice capacity needs extra 255 padding over it's length for transfer")
	}
	const BLOCKSIZE = 64
	var srcPtr []byte
	var buf [BLOCKSIZE + 4]byte
	for offset := 0; offset < rlen; offset += BLOCKSIZE {
		sz := BLOCKSIZE
		if offset+sz > rlen {
			sz = rlen - offset
		}
		dstAddr := addr + uint32(offset)
		if dstAddr&backplaneAddrMask+uint32(sz) > backplaneAddrMask+1 {
			panic("invalid dstAddr:" + strconv.Itoa(int(dstAddr)))
		}
		// fmt.Println("set backplane window to ", dstAddr, offset)
		err := d.setBackplaneWindow(dstAddr)
		if err != nil {
			return err
		}
		if offset+sz > len(src) {
			// fmt.Println("ALLOCA", sz)
			srcPtr = src[:cap(src)][offset:]
		} else {
			srcPtr = src[offset:]
		}
		// fmt.Println("write bytes to addr ", dstAddr&backplaneAddrMask)
		err = d.WriteBytes(FuncBackplane, dstAddr&backplaneAddrMask, srcPtr[:sz])
		if err != nil {
			return err
		}
	}
	Debug("download finished, validate data")
	// Finished writing firmware... should be ready for use. We choose to validate it though.

	for offset := 0; offset < rlen; offset += BLOCKSIZE {
		sz := BLOCKSIZE
		if offset+sz > rlen {
			sz = rlen - offset
		}
		dstAddr := addr + uint32(offset)
		Debug("dstAddr", dstAddr, "addr=", addr, "offset=", offset, "sz=", sz)
		if dstAddr&backplaneAddrMask+uint32(sz) > backplaneAddrMask+1 {
			panic("invalid dstAddr:" + strconv.Itoa(int(dstAddr)))
		}
		// fmt.Println("set backplane window", dstAddr)
		err := d.setBackplaneWindow(dstAddr)
		if err != nil {
			return err
		}
		// fmt.Println("read back bytes into buf from ", dstAddr&backplaneAddrMask)
		err = d.ReadBytes(FuncBackplane, dstAddr&backplaneAddrMask, buf[:sz])
		if err != nil {
			return err
		}

		srcPtr = src[offset:]
		if !bytes.Equal(buf[:sz], srcPtr[:sz]) {
			err = fmt.Errorf("%w at addr=%#x: expected:%q\ngot: %q", errFirmwareValidationFailed, dstAddr, srcPtr[:sz], buf[:sz])
			return err
		}
	}
	return nil
}

func (d *Dev) busSleep(canSleep bool) (err error) {
	if d.busIsUp != canSleep {
		return nil // Already at desired state.
	}
	err = d.ksoSet(!canSleep)
	if err == nil {
		d.busIsUp = !canSleep
	}
	return err
}

// ksoSet enable KSO mode (keep SDIO on)
func (d *Dev) ksoSet(enable bool) error {
	var writeVal uint8
	if enable {
		writeVal = SBSDIO_SLPCSR_KEEP_SDIO_ON
	}
	// These can fail and it's still ok.
	d.Write8(FuncBackplane, SDIO_SLEEP_CSR, writeVal)
	d.Write8(FuncBackplane, SDIO_SLEEP_CSR, writeVal)
	// Put device to sleep, turn off KSO if value == 0 and
	// check for bit0 only, bit1(devon status) may not get cleared right away
	var compareValue uint8
	var bmask uint8 = SBSDIO_SLPCSR_KEEP_SDIO_ON
	if enable {
		// device WAKEUP through KSO:
		// write bit 0 & read back until
		// both bits 0(kso bit) & 1 (dev on status) are set
		compareValue = SBSDIO_SLPCSR_KEEP_SDIO_ON | SBSDIO_SLPCSR_DEVICE_ON
		bmask = compareValue
	}
	for i := 0; i < 64; i++ {
		// Reliable KSO bit set/clr:
		// Sdiod sleep write access appears to be in sync with PMU 32khz clk
		// just one write attempt may fail, (same is with read ?)
		// in any case, read it back until it matches written value
		// this can fail and it's still ok
		readValue, err := d.Read8(FuncBackplane, SDIO_SLEEP_CSR)
		if err != nil && readValue&bmask == compareValue && readValue != 0xff {
			return nil // success
		}
		time.Sleep(time.Millisecond)
		d.Write8(FuncBackplane, SDIO_SLEEP_CSR, writeVal)
	}
	return errors.New("kso set failed")
}

func (d *Dev) clmLoad(clm []byte) error {
	const sdpcmHeaderLen = unsafe.Sizeof(sdpcmHeader{})
	const CLM_CHUNK_LEN = 1024
	buf := d.buf[sdpcmHeaderLen+16:]
	clmLen := uint16(len(clm))
	const clmLoadString = "clmload\x00"
	for off := uint16(0); off < clmLen; off += CLM_CHUNK_LEN {
		ln := uint16(CLM_CHUNK_LEN)
		flag := uint16(1 << 12)
		if off == 0 {
			flag |= 2 // DL begin.
		}
		if off+ln >= clmLen {
			flag |= 4 // DL end.
			ln = clmLen - off
		}
		copy(buf[:len(clmLoadString)], clmLoadString)
		binary.BigEndian.PutUint16(buf[8:], flag)
		binary.BigEndian.PutUint16(buf[10:], 2)
		binary.BigEndian.PutUint32(buf[12:], uint32(ln))
		binary.BigEndian.PutUint32(buf[16:], 0)
		n := copy(buf[20:], clm[off:off+ln])
		if uint16(n) != ln {
			return errors.New("CLM download failed due to small buffer")
		}
		// Send data aligned to 8 bytes. We do end up sending scratch data
		// at end of buffer that has not been set here.
		err := d.doIoctl(SDPCM_SET, wwd_STA_INTERFACE, wlc_SET_VAR, buf[:align32(20+uint32(ln), 8)])
		if err != nil {
			return err
		}
	}
	// CLM data send done.
	const clmStatString = "clmload_status\x00\x00\x00\x00\x00"
	copy(buf[:len(clmStatString)], clmStatString)
	err := d.doIoctl(SDPCM_GET, wwd_STA_INTERFACE, wlc_GET_VAR, buf[:len(clmStatString)])
	if err != nil {
		return err
	}
	status := binary.BigEndian.Uint32(buf[:])
	if status != 0 {
		return errors.New("CLM load failed due to bad status return")
	}
	return nil
}

// putMAC gets current MAC address from CYW43439.
func (d *Dev) putMAC(dst []byte) error {
	if len(dst) < 6 {
		panic("dst too short to put MAC")
	}
	const sdpcmHeaderLen = unsafe.Sizeof(sdpcmHeader{})
	buf := d.buf[sdpcmHeaderLen+16:]
	const varMAC = "cur_etheraddr\x00\x00\x00\x00\x00\x00\x00"
	copy(buf[:len(varMAC)], varMAC)
	err := d.doIoctl(SDPCM_GET, wwd_STA_INTERFACE, wlc_GET_VAR, buf[:len(varMAC)])
	if err == nil {
		copy(dst[:6], buf)
	}
	return err
}
