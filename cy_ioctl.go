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

	"github.com/soypat/cyw43439/whd"
)

func (d *Device) LED() Pin {
	const RaspberryPiPicoWOnboardLED = 0
	return Pin{
		pin: RaspberryPiPicoWOnboardLED,
		d:   d,
	}
}

type Pin struct {
	d   *Device
	pin uint8
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
	// a =
	// sdpcmHeaderSize = 12 //unsafe.Sizeof(sdpcmHeader{})
	ioctlHeaderSize = 16 // unsafe.Sizeof(ioctlHeader{})
)

func (d *Device) GPIOSet(wlGPIO uint8, value bool) (err error) {
	Debug("gpioset", int(wlGPIO), "value=", value)
	if wlGPIO >= 3 {
		panic("GPIO out of range 0..2")
	}
	val := uint32(1 << wlGPIO)
	if value {
		err = d.WriteIOVar2("gpioout", whd.WWD_STA_INTERFACE, val, val)
	} else {
		err = d.WriteIOVar2("gpioout", whd.WWD_STA_INTERFACE, val, 0)
	}
	return err
}

// Returns a safe to use buffer outside of the bounds of buffers used by Ioctl calls.
func (d *Device) offbuf() []byte { return d.auxbuf[:] }

// reference: cyw43_write_iovar_u32
func (d *Device) WriteIOVar(VAR string, iface whd.IoctlInterface, val uint32) error {
	Debug("WriteIOVar var=", VAR, "ioctl=", uint8(iface), "val=", val)
	buf := d.offbuf()
	length := copy(buf, VAR)
	buf[length] = 0 // Null terminate the string
	length++
	binary.LittleEndian.PutUint32(buf[length:], val)
	return d.doIoctl(whd.SDPCM_SET, iface, whd.WLC_SET_VAR, buf[:length+4])
}

// reference: cyw43_write_iovar_u32_u32 (const char *var, uint32_t val0, uint32_t val1, uint32_t iface)
func (d *Device) WriteIOVar2(VAR string, iface whd.IoctlInterface, val0, val1 uint32) error {
	Debug("WriteIOVar2 var=", VAR, "ioctl=", uint8(iface), "val1=", val0, "val2=", val1)
	buf := d.offbuf()
	length := copy(buf, VAR)
	buf[length] = 0 // Null terminate the string
	length++
	binary.LittleEndian.PutUint32(buf[length:], val0)
	binary.LittleEndian.PutUint32(buf[length+4:], val1)
	return d.doIoctl(whd.SDPCM_SET, iface, whd.WLC_SET_VAR, buf[:length+8])
}

// reference: cyw43_write_iovar_n
func (d *Device) WriteIOVarN(VAR string, iface whd.IoctlInterface, src []byte) error {
	Debug("WriteIOVarN var=", VAR, "ioctl=", uint8(iface), "len=", len(src))
	iobuf := d.offbuf()
	if len(VAR)+len(src)+1 > len(iobuf) {
		return errors.New("buffer too short for IOVarN call")
	}
	// Must do some buffer juggling so that
	// if offbuf is passed in src we do not overwrite the data.
	length := copy(iobuf[len(VAR)+1:], src)
	length += copy(iobuf, VAR)
	iobuf[len(VAR)] = 0 // Null terminate the string
	length++
	return d.doIoctl(whd.SDPCM_SET, iface, whd.WLC_SET_VAR, iobuf[:length])
}

// reference: cyw43_set_ioctl_u32 (uint32_t cmd, uint32_t val, uint32_t iface)
func (d *Device) SetIoctl32(iface whd.IoctlInterface, cmd whd.SDPCMCommand, val uint32) error {
	Debug("SetIoctl32")
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], val)
	return d.doIoctl(whd.SDPCM_SET, iface, cmd, buf[:])
}

// reference: cyw43_get_ioctl_u32
func (d *Device) GetIoctl32(iface whd.IoctlInterface, cmd whd.SDPCMCommand) (uint32, error) {
	Debug("GetIoctl32")
	var buf [4]byte
	err := d.doIoctl(whd.SDPCM_GET, iface, cmd, buf[:])
	return binary.LittleEndian.Uint32(buf[:4]), err
}

// reference: cyw43_read_iovar_u32
func (d *Device) ReadIOVar(VAR string, iface whd.IoctlInterface) (uint32, error) {
	Debug("ReadIOVar var=", VAR, "ioctl=", uint8(iface))
	buf := d.offbuf()
	length := copy(buf, VAR)
	buf[length] = 0 // Null terminate the string
	length++
	binary.LittleEndian.PutUint32(buf[length:], 0)
	err := d.doIoctl(whd.SDPCM_GET, iface, whd.WLC_GET_VAR, buf[:length+4])
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf[:4]), nil
}

// reference: cyw43_ll_ioctl
func (d *Device) ioctl(cmd whd.SDPCMCommand, iface whd.IoctlInterface, w []byte) error {
	kind := whd.SDPCM_GET
	if cmd&1 != 0 {
		kind = whd.SDPCM_SET
	}
	return d.doIoctl(uint32(kind), iface, cmd>>1, w)
}

// doIoctl uses Device's primary buffer to perform ioctl call. Use [Dev.offbuff] for
// allocations that are passed into doIoctl.
//
//	reference: cyw43_do_ioctl(uint32_t kind, uint32_t cmd, size_t len, uint8_t *buf, uint32_t iface)
func (d *Device) doIoctl(kind uint32, iface whd.IoctlInterface, cmd whd.SDPCMCommand, buf []byte) error {
	// TODO: once testing is done these checks may be removed.
	if !iface.IsValid() {
		return errors.New("invalid ioctl interface")
	} else if !cmd.IsValid() {
		return errors.New("invalid ioctl command")
	} else if kind != whd.SDPCM_GET && kind != whd.SDPCM_SET {
		return errors.New("invalid ioctl kind")
	}

	err := d.sendIoctl(kind, iface, cmd, buf)
	if err != nil {
		return err
	}
	start := time.Now()
	const ioctlTimeout = 50 * time.Millisecond
	const maxRetries = 3
	for retries := 0; time.Since(start) < ioctlTimeout || retries < maxRetries; retries++ {
		payloadOffset, plen, header, err := d.sdpcmPoll(d.buf[:])
		Debug("doIoctl:sdpcmPoll conclude payloadoffset=", int(payloadOffset), "plen=", int(plen), "header=", header.String(), err)
		// The "wrong payload type" appears to happen during startup. See sdpcmProcessRxPacket
		if err != nil && !errors.Is(err, err9WrongPayloadType) {
			continue
		}
		payload := d.buf[payloadOffset : payloadOffset+plen]
		switch header {
		case whd.CONTROL_HEADER:
			n := copy(buf[:], payload)
			if uint32(n) != plen {
				return errors.New("not enough space on ioctl buffer for control header copy")
			}
			return nil

		case whd.ASYNCEVENT_HEADER:
			// TODO(soypat): Must handle this for wifi to work. cyw43_cb_process_async_event
			Debug("ASYNCEVENT not handled")
		case whd.DATA_HEADER:
			// TODO(soypat): Implement ethernet interface. cyw43_cb_process_ethernet
			Debug("DATA_HEADER not implemented yet")
		default:
			Debug("doIoctl got unexpected packet", header)
		}
		time.Sleep(time.Millisecond)
	}
	Debug("todo")
	return errDoioctlTimeout
}

var errDoioctlTimeout = errors.New("doIoctl time out waiting for data")

// reference: cyw43_send_ioctl
func (d *Device) sendIoctl(kind uint32, iface whd.IoctlInterface, cmd whd.SDPCMCommand, w []byte) error {
	Debug("sendIoctl")
	length := uint32(len(w))
	if uint32(len(d.buf)) < whd.SDPCM_HEADER_LEN+whd.IOCTL_HEADER_LEN+length {
		return errors.New("ioctl buffer too large for sending")
	}
	d.sdpcmRequestedIoctlID++
	id := uint32(d.sdpcmRequestedIoctlID)
	flags := (id<<16)&0xffff_0000 | uint32(kind) | uint32(iface)<<12 // look for CDCF_IOC* identifiers in pico-sdk
	header := whd.IoctlHeader{
		Cmd:   cmd,
		Len:   length & 0xffff,
		Flags: flags,
	}
	header.Put(d.buf[whd.SDPCM_HEADER_LEN:])
	copy(d.buf[whd.SDPCM_HEADER_LEN+whd.IOCTL_HEADER_LEN:], w)
	Debug("sendIoctl cmd=", uint32(header.Cmd), " len=", header.Len, " flags=", header.Flags, "status=", header.Status)

	return d.sendSDPCMCommon(whd.CONTROL_HEADER, d.buf[:whd.SDPCM_HEADER_LEN+whd.IOCTL_HEADER_LEN+length])
}

// sdpcmPoll reads next packet from WLAN into buf and returns the offset of the
// payload, length of the payload and the header type. Is cyw43_ll_sdpcm_poll_device in reference.
func (d *Device) sdpcmPoll(buf []byte) (payloadOffset, plen uint32, header whd.SDPCMHeaderType, err error) {
	const badResult = 0
	noPacketSuccess := !d.hadSuccesfulPacket
	if noPacketSuccess && !d.wlRegOn.Get() {
		return 0, 0, badResult, errors.New("sdpcmPoll yield fault")
	}
	err = d.busSleep(false)
	if err != nil {
		return 0, 0, badResult, err
	}
	if noPacketSuccess {
		lastInt := d.lastInt
		intStat, err := d.GetInterrupts()
		if err != nil || lastInt != uint16(intStat) && intStat.IsBusOverflowedOrUnderflowed() {
			Debug("bus error condition detected =", uint16(intStat), err)
		}
		if intStat != 0 {
			d.Write16(FuncBus, whd.SPI_INTERRUPT_REGISTER, uint16(intStat))
		}
		if !intStat.IsF2Available() {
			return 0, 0, badResult, errors.New("sdpcmPoll: F2 unavailable")
		}
	}
	const (
		payloadMTU         = 1500
		linkHeader         = 30
		ethernetSize       = 14
		linkMTU            = payloadMTU + linkHeader + ethernetSize
		gspiPacketOverhead = 8
	)
	var status Status = 0xFFFFFFFF
	for i := 0; i < 1000 && status == 0xFFFFFFFF; i++ {
		status, err = d.GetStatus()
		if err != nil {
			break
		}
	}
	if status == 0xFFFFFFFF || err != nil {
		return 0, 0, badResult, fmt.Errorf("bad status get in sdpcmPoll: %w", err)
	}
	if !status.GSPIPacketAvailable() {
		Debug("no packet")
		d.hadSuccesfulPacket = false
		return 0, 0, badResult, errors.New("sdpcmPoll: no packet")
	}
	bytesPending := status.F2PacketLength()
	if bytesPending == 0 || bytesPending > linkMTU-gspiPacketOverhead || status.IsUnderflow() {
		Debug("SPI invalid bytes pending", bytesPending)
		d.Write8(FuncBackplane, whd.SPI_FRAME_CONTROL, 1)
		d.hadSuccesfulPacket = false
		return 0, 0, badResult, errors.New("sdpcmPoll: invalid bytes pending")
	}
	err = d.ReadBytes(FuncWLAN, 0, buf[:bytesPending])
	if err != nil {
		return 0, 0, badResult, err
	}
	hdr0 := binary.LittleEndian.Uint16(buf[:])
	hdr1 := binary.LittleEndian.Uint16(buf[2:])
	if hdr0 == 0 && hdr1 == 0 {
		// no packets.
		Debug("no packet:zero size header")
		d.hadSuccesfulPacket = false
		return 0, 0, badResult, errors.New("sdpcmPoll:zero header")
	}
	d.hadSuccesfulPacket = true
	if hdr0^hdr1 != 0xffff {
		Debug("header xor mismatch h[0]=", hdr0, "h[1]=", hdr1)
		return 0, 0, badResult, errors.New("sdpcmPoll:header mismatch")
	}
	payloadOffset, plen, header, err = d.sdpcmProcessRxPacket(buf)
	return payloadOffset, plen, header, err
}

// sendSDPCMCommon Total IO performed is WriteBytes, which may call GetStatus if packet is WLAN.
//
//	reference: cyw43_sdpcm_send_common
func (d *Device) sendSDPCMCommon(kind whd.SDPCMHeaderType, w []byte) error {
	Debug("sendSDPCMCommon len=", len(w))
	if kind != whd.CONTROL_HEADER && kind != whd.DATA_HEADER {
		return errors.New("unexpected SDPCM kind")
	}
	err := d.busSleep(false)
	if err != nil {
		return err
	}
	headerLength := uint8(whd.SDPCM_HEADER_LEN)
	if kind == whd.DATA_HEADER {
		headerLength += 2
	}
	size := uint16(len(w))        //+ uint16(hdlen)
	paddedSize := (size + 3) &^ 3 // If not using gSPI then this should be padded to 64bytes
	if uint16(cap(w)) < paddedSize {
		return errors.New("buffer too small to be SDPCM padded")
	}
	//w = w[0:paddedSize] // padded in WriteBytes.
	header := whd.SDPCMHeader{
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

// reference: disable_device_core
func (d *Device) disableDeviceCore(coreID uint8, coreHalt bool) error {
	base := coreaddress(coreID)
	Debug("disable core", coreID, base)
	d.ReadBackplane(base+whd.AI_RESETCTRL_OFFSET, 1)
	reg, err := d.ReadBackplane(base+whd.AI_RESETCTRL_OFFSET, 1)
	if err != nil {
		return err
	}
	if reg&whd.AIRC_RESET != 0 {
		return nil
	}
	Debug("core not in reset", reg)
	return errors.New("core not in reset")
}

// reference: reset_device_core
func (d *Device) resetDeviceCore(coreID uint8, coreHalt bool) error {
	err := d.disableDeviceCore(coreID, coreHalt)
	if err != nil {
		return err
	}
	var cpuhaltFlag uint32
	if coreHalt {
		cpuhaltFlag = whd.SICF_CPUHALT
	}
	base := coreaddress(coreID)
	const addr = 0x18103000 + whd.AI_IOCTRL_OFFSET
	Debug("begin reset process coreid=", coreID)
	d.WriteBackplane(base+whd.AI_IOCTRL_OFFSET, 1, whd.SICF_FGC|whd.SICF_CLOCK_EN|cpuhaltFlag)
	d.ReadBackplane(base+whd.AI_IOCTRL_OFFSET, 1)
	d.WriteBackplane(base+whd.AI_RESETCTRL_OFFSET, 1, 0)
	time.Sleep(time.Millisecond)
	d.WriteBackplane(base+whd.AI_IOCTRL_OFFSET, 1, whd.SICF_CLOCK_EN|cpuhaltFlag)
	d.ReadBackplane(base+whd.AI_IOCTRL_OFFSET, 1)
	time.Sleep(time.Millisecond)
	Debug("end reset process coreid=", coreID)
	return nil
}

// CoreIsActive returns if the specified core is not in reset.
// Can be called with CORE_WLAN_ARM and CORE_SOCRAM global constants.
// It returns true if communications are down (WL_REG_ON at low).
//
//	reference: device_core_is_up
func (d *Device) CoreIsActive(coreID uint8) bool {
	base := coreaddress(coreID)
	reg, _ := d.ReadBackplane(base+whd.AI_IOCTRL_OFFSET, 1)
	if reg&(whd.SICF_FGC|whd.SICF_CLOCK_EN) != whd.SICF_CLOCK_EN {
		return false
	}
	reg, _ = d.ReadBackplane(base+whd.AI_RESETCTRL_OFFSET, 1)
	return reg&whd.AIRC_RESET == 0
}

// coreaddress returns either WLAN=0x18103000  or  SOCRAM=0x18104000
//
//	reference: get_core_address
func coreaddress(coreID uint8) (v uint32) {
	switch coreID {
	case whd.CORE_WLAN_ARM:
		v = whd.WRAPPER_REGISTER_OFFSET + whd.WLAN_ARMCM3_BASE_ADDRESS
	case whd.CORE_SOCRAM:
		v = whd.WRAPPER_REGISTER_OFFSET + whd.SOCSRAM_BASE_ADDRESS
	default:
		panic("bad core id")
	}
	return v
}

// reference: cyw43_read_backplane
func (d *Device) ReadBackplane(addr uint32, size uint32) (uint32, error) {
	Debug("read backplane", addr, size)
	err := d.setBackplaneWindow(addr)
	if err != nil {
		return 0, err
	}
	addr &= whd.BACKPLANE_ADDR_MASK
	// addr &=  whd.BACKPLANE_ADDR_MASK
	if size == 4 {
		addr |= whd.SBSDIO_SB_ACCESS_2_4B_FLAG
	}

	reg, err := d.rr(FuncBackplane, addr, size)
	if err != nil {
		return 0, err
	}
	err = d.setBackplaneWindow(whd.CHIPCOMMON_BASE_ADDRESS)
	return reg, err
}

// reference: cyw43_write_backplane
func (d *Device) WriteBackplane(addr, size, value uint32) error {
	Debug("write backplane", addr, "=", value, "size=", int(size))
	err := d.setBackplaneWindow(addr)
	if err != nil {
		return err
	}
	addr &= whd.BACKPLANE_ADDR_MASK
	if size == 4 {
		addr |= whd.SBSDIO_SB_ACCESS_2_4B_FLAG
	}
	err = d.wr(FuncBackplane, addr, size, value)
	if err != nil {
		return err
	}

	return d.setBackplaneWindow(whd.CHIPCOMMON_BASE_ADDRESS)
}

// reference: cyw43_set_backplane_window
func (d *Device) setBackplaneWindow(addr uint32) (err error) {
	const (
		SDIO_BACKPLANE_ADDRESS_HIGH = 0x1000c
		SDIO_BACKPLANE_ADDRESS_MID  = 0x1000b
		SDIO_BACKPLANE_ADDRESS_LOW  = 0x1000a
	)
	currentWindow := d.currentBackplaneWindow
	addr = addr &^ whd.BACKPLANE_ADDR_MASK
	if addr == currentWindow {
		return nil
	}

	if (addr & 0xff000000) != currentWindow&0xff000000 {
		err = d.wr(FuncBackplane, SDIO_BACKPLANE_ADDRESS_HIGH, 1, addr>>24)
	}
	if err == nil && (addr&0x00ff0000) != currentWindow&0x00ff0000 {
		err = d.wr(FuncBackplane, SDIO_BACKPLANE_ADDRESS_MID, 1, addr>>16)
	}
	if err == nil && (addr&0x0000ff00) != currentWindow&0x0000ff00 {
		err = d.wr(FuncBackplane, SDIO_BACKPLANE_ADDRESS_LOW, 1, addr>>8)
	}
	if err != nil {
		d.currentBackplaneWindow = 0
		return err
	}
	d.currentBackplaneWindow = addr
	return nil
}

// reference: cyw43_download_resource
func (d *Device) downloadResource(addr uint32, src []byte) error {
	Debug("download resource addr=", addr, "len=", len(src))
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
		if dstAddr&whd.BACKPLANE_ADDR_MASK+uint32(sz) > whd.BACKPLANE_ADDR_MASK+1 {
			panic("invalid dstAddr:" + strconv.Itoa(int(dstAddr)))
		}

		err := d.setBackplaneWindow(dstAddr)
		if err != nil {
			return err
		}
		if offset+sz > len(src) {
			srcPtr = src[:cap(src)][offset:]
		} else {
			srcPtr = src[offset:]
		}

		err = d.WriteBytes(FuncBackplane, dstAddr&whd.BACKPLANE_ADDR_MASK, srcPtr[:sz])
		if err != nil {
			return err
		}
	}
	if !validateDownloads {
		return nil
	}
	Debug("download finished, validate data")
	// Finished writing firmware... should be ready for use. We choose to validate it though.
	for offset := 0; offset < rlen; offset += BLOCKSIZE {
		sz := BLOCKSIZE
		if offset+sz > rlen {
			sz = rlen - offset
		}
		dstAddr := addr + uint32(offset)
		// Debug("dstAddr", dstAddr, "addr=", addr, "offset=", offset, "sz=", sz)
		if dstAddr&whd.BACKPLANE_ADDR_MASK+uint32(sz) > whd.BACKPLANE_ADDR_MASK+1 {
			panic("invalid dstAddr:" + strconv.Itoa(int(dstAddr)))
		}
		err := d.setBackplaneWindow(dstAddr)
		if err != nil {
			return err
		}

		err = d.ReadBytes(FuncBackplane, dstAddr&whd.BACKPLANE_ADDR_MASK, buf[:sz])
		if err != nil {
			return err
		}
		if offset+sz > len(src) {
			srcPtr = src[:cap(src)][offset:]
		} else {
			srcPtr = src[offset:]
		}
		if !bytes.Equal(buf[:sz], srcPtr[:sz]) {
			err = fmt.Errorf("%w at addr=%#x: expected:%q\ngot: %q", errFirmwareValidationFailed, dstAddr, srcPtr[:sz], buf[:sz])
			return err
		}
	}
	Debug("firmware validation success")
	return nil
}

// reference: cyw43_ll_bus_sleep and cyw43_ll_bus_sleep_helper
func (d *Device) busSleep(canSleep bool) (err error) {
	if d.busIsUp != canSleep {
		return nil // Already at desired state.
	}
	err = d.ksoSet(!canSleep) // We use KSO on pico, so no need to do SDIO chip stuff.
	if err == nil {
		d.busIsUp = !canSleep
	}
	return err
}

// ksoSet enable KSO mode (keep SDIO on)
//
//	reference: cyw43_kso_set
func (d *Device) ksoSet(value bool) error {
	Debug("ksoSet enable=", value)
	var writeVal uint8
	if value {
		writeVal = whd.SBSDIO_SLPCSR_KEEP_SDIO_ON
	}
	// These can fail and it's still ok.
	d.Write8(FuncBackplane, whd.SDIO_SLEEP_CSR, writeVal)
	d.Write8(FuncBackplane, whd.SDIO_SLEEP_CSR, writeVal)
	// Put device to sleep, turn off KSO if value == 0 and
	// check for bit0 only, bit1(devon status) may not get cleared right away
	var compareValue uint8
	var bmask uint8 = whd.SBSDIO_SLPCSR_KEEP_SDIO_ON
	if value {
		// device WAKEUP through KSO:
		// write bit 0 & read back until
		// both bits 0(kso bit) & 1 (dev on status) are set
		compareValue = whd.SBSDIO_SLPCSR_KEEP_SDIO_ON | whd.SBSDIO_SLPCSR_DEVICE_ON
		bmask = compareValue
	}
	for i := 0; i < 64; i++ {
		// Reliable KSO bit set/clr:
		// Sdiod sleep write access appears to be in sync with PMU 32khz clk
		// just one write attempt may fail, (same is with read ?)
		// in any case, read it back until it matches written value
		// this can fail and it's still ok
		readValue, err := d.Read8(FuncBackplane, whd.SDIO_SLEEP_CSR)
		if err == nil && readValue&bmask == compareValue && readValue != 0xff {
			return nil // success
		}
		time.Sleep(time.Millisecond)
		d.Write8(FuncBackplane, whd.SDIO_SLEEP_CSR, writeVal)
	}
	return errors.New("kso set failed")
}

// reference: cyw43_clm_load
func (d *Device) clmLoad(clm []byte) error {
	const CLM_CHUNK_LEN = 1024
	buf := d.buf[whd.SDPCM_HEADER_LEN+16:]
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
		// endian
		binary.LittleEndian.PutUint16(buf[8:], flag)
		binary.LittleEndian.PutUint16(buf[10:], 2)
		binary.LittleEndian.PutUint32(buf[12:], uint32(ln))
		binary.LittleEndian.PutUint32(buf[16:], 0)
		n := copy(buf[20:], clm[off:off+ln])
		if uint16(n) != ln {
			return errors.New("CLM download failed due to small buffer")
		}
		// Send data aligned to 8 bytes. We do end up sending scratch data
		// at end of buffer that has not been set here.
		Debug("clm data send off+len=", int(off+ln), "clmlen=", int(clmLen))
		err := d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_VAR, buf[:align32(20+uint32(ln), 8)])
		if err != nil {
			return err
		}
	}
	Debug("clm data send done")
	// Check status of the download.
	const clmStatString = "clmload_status\x00\x00\x00\x00\x00"
	const clmStatLen = len(clmStatString)
	buf = d.auxbuf[:]
	copy(buf[:clmStatLen], clmStatString)
	err := d.doIoctl(whd.SDPCM_GET, whd.WWD_STA_INTERFACE, whd.WLC_GET_VAR, buf[:clmStatLen])
	if err != nil {
		return err
	}
	status := binary.LittleEndian.Uint32(buf[:])
	if status != 0 {
		return errors.New("CLM load failed due to bad status return")
	}
	return nil
}

// putMAC gets current MAC address from CYW43439.
//
//	reference: cy43_ll_bus_init (end)
func (d *Device) putMAC(dst []byte) error {
	if len(dst) < 6 {
		panic("dst too short to put MAC")
	}
	const sdpcmHeaderLen = unsafe.Sizeof(whd.SDPCMHeader{})
	buf := d.auxbuf[sdpcmHeaderLen+16:]
	const varMAC = "cur_etheraddr\x00\x00\x00\x00\x00\x00\x00"
	copy(buf[:len(varMAC)], varMAC)
	err := d.doIoctl(whd.SDPCM_GET, whd.WWD_STA_INTERFACE, whd.WLC_GET_VAR, buf[:len(varMAC)])
	if err == nil {
		copy(dst[:6], buf)
	}
	return err
}

var (
	err2InvalidPacket             = errors.New("invalid packet")
	err3PacketTooSmall            = errors.New("packet too small")
	err4IgnoreControlPacket       = errors.New("ignore flow ctl packet")
	err5IgnoreSmallControlPacket  = errors.New("ignore too small flow ctl packet")
	err6IgnoreWrongIDPacket       = errors.New("ignore packet with wrong id")
	err7IgnoreSmallDataPacket     = errors.New("ignore too small data packet")
	err8IgnoreTooSmallAsyncPacket = errors.New("ignore too small async packet")
	err9WrongPayloadType          = errors.New("wrong payload type")
	err10IncorrectOUI             = errors.New("incorrect oui")
	err11UnknownHeader            = errors.New("unknown header")
)

// sdpcmProcessRxPacket finds payload in WLAN RxPacket and returns the kind of packet.
// is sdpcm_process_rx_packet in reference.
//
//	reference: sdpcm_process_rx_packet
func (d *Device) sdpcmProcessRxPacket(buf []byte) (payloadOffset, plen uint32, flag whd.SDPCMHeaderType, err error) {
	const badFlag = 0
	const sdpcmOffset = 0
	hdr := whd.DecodeSDPCMHeader(buf[sdpcmOffset:])
	switch {
	case hdr.Size != ^hdr.SizeCom&0xffff:
		return 0, 0, badFlag, err2InvalidPacket
	case hdr.Size < whd.SDPCM_HEADER_LEN:
		return 0, 0, badFlag, err3PacketTooSmall
	}
	if d.wlanFlowCtl != hdr.WirelessFlowCtl {
		Debug("WLAN: changed flow control", d.wlanFlowCtl, hdr.WirelessFlowCtl)
	}
	d.wlanFlowCtl = hdr.WirelessFlowCtl

	if hdr.ChanAndFlags&0xf < 3 {
		// A valid header, check the bus data credit.
		credit := hdr.BusDataCredit - d.sdpcmLastBusCredit
		if credit <= 20 {
			d.sdpcmLastBusCredit = hdr.BusDataCredit
		}
	}
	if hdr.Size == whd.SDPCM_HEADER_LEN {
		return 0, 0, badFlag, err4IgnoreControlPacket // Flow ctl packet with no data.
	}

	payloadOffset = uint32(hdr.HeaderLength)
	headerFlag := hdr.Type()
	switch headerFlag {
	case whd.CONTROL_HEADER:
		const totalHeaderSize = whd.SDPCM_HEADER_LEN + whd.IOCTL_HEADER_LEN
		if hdr.Size < totalHeaderSize {
			return 0, 0, badFlag, err5IgnoreSmallControlPacket
		}
		ioctlHeader := whd.DecodeIoctlHeader(buf[payloadOffset:])
		id := ioctlHeader.ID()
		if id != d.sdpcmRequestedIoctlID {
			return 0, 0, badFlag, err6IgnoreWrongIDPacket
		}
		payloadOffset += whd.IOCTL_HEADER_LEN
		plen = uint32(hdr.Size) - payloadOffset
		Debug("ioctl response id=", id, "len=", plen)

	case whd.DATA_HEADER:
		const totalHeaderSize = whd.SDPCM_HEADER_LEN + whd.BDC_HEADER_LEN
		if hdr.Size <= totalHeaderSize {
			return 0, 0, badFlag, err7IgnoreSmallDataPacket
		}

		bdcHeader := whd.DecodeBDCHeader(buf[payloadOffset:])
		itf := bdcHeader.Flags2 // Get interface number.
		payloadOffset += whd.BDC_HEADER_LEN + uint32(bdcHeader.DataOffset)<<2
		plen = (uint32(hdr.Size) - payloadOffset) | uint32(itf)<<31

	case whd.ASYNCEVENT_HEADER:
		const totalHeaderSize = whd.SDPCM_HEADER_LEN + whd.BDC_HEADER_LEN
		if hdr.Size <= totalHeaderSize {
			return 0, 0, badFlag, err8IgnoreTooSmallAsyncPacket
		}
		bdcHeader := whd.DecodeBDCHeader(buf[payloadOffset:])
		payloadOffset += whd.BDC_HEADER_LEN + uint32(bdcHeader.DataOffset)<<2
		plen = uint32(hdr.Size) - payloadOffset
		payload := buf[payloadOffset:]
		// payload is actually an ethernet packet with type 0x886c.
		if !(payload[12] == 0x88 && payload[13] == 0x6c) {
			// ethernet packet doesn't have the correct type.
			// Note - this happens during startup but appears to be expected
			// return 0, 0, badFlag, Err9WrongPayloadType
			err = err9WrongPayloadType
		}
		// Check the Broadcom OUI.
		if !(payload[19] == 0x00 && payload[20] == 0x10 && payload[21] == 0x18) {
			return 0, 0, badFlag, err10IncorrectOUI
		}
		plen = plen - 24
		payloadOffset = payloadOffset + 24
	default:
		// Unknown Header.
		return 0, 0, badFlag, err11UnknownHeader
	}
	return payloadOffset, plen, headerFlag, err
}
