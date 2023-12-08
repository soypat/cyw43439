//go:build tinygo

package cyw43439

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"time"
	"unsafe"

	"log/slog"

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
	d.info("GPIOSet", slog.Uint64("wlGPIO", uint64(wlGPIO)), slog.Bool("value", value))
	if wlGPIO >= 3 {
		panic("GPIO out of range 0..2")
	}
	val := uint32(1 << wlGPIO)
	if value {
		err = d.WriteIOVar2("gpioout", whd.IF_STA, val, val)
	} else {
		err = d.WriteIOVar2("gpioout", whd.IF_STA, val, 0)
	}
	return err
}

// Returns a safe to use buffer outside of the bounds of buffers used by Ioctl calls.
func (d *Device) offbuf() []byte { return d.auxbuf[:] }

// reference: cyw43_write_iovar_u32
func (d *Device) WriteIOVar(VAR string, iface whd.IoctlInterface, val uint32) error {
	d.debug("WriteIOVar", slog.String("var", VAR), slog.String("iface", iface.String()), slog.Uint64("val", uint64(val)))
	// Debug("WriteIOVar var=", VAR, "ioctl=", uint8(iface), "val=", val)
	buf := d.offbuf()
	length := copy(buf, VAR)
	buf[length] = 0 // Null terminate the string
	length++
	binary.LittleEndian.PutUint32(buf[length:], val)
	return d.doIoctl(whd.SDPCM_SET, iface, whd.WLC_SET_VAR, buf[:length+4])
}

// reference: cyw43_write_iovar_u32_u32 (const char *var, uint32_t val0, uint32_t val1, uint32_t iface)
func (d *Device) WriteIOVar2(VAR string, iface whd.IoctlInterface, val0, val1 uint32) error {
	d.debug("WriteIOVar2", slog.String("var", VAR), slog.String("iface", iface.String()), slog.Uint64("val0", uint64(val0)), slog.Uint64("val1", uint64(val1)))
	// Debug("WriteIOVar2 var=", VAR, "ioctl=", uint8(iface), "val1=", val0, "val2=", val1)
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
	d.debug("WriteIOVarN", slog.String("var", VAR), slog.String("iface", iface.String()), slog.Int("datalen", len(src)))
	// Debug("WriteIOVarN var=", VAR, "ioctl=", uint8(iface), "len=", len(src))
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
	d.debug("SetIoctl32", slog.String("iface", iface.String()), slog.String("cmd", cmd.String()), slog.Uint64("val", uint64(val)))
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], val)
	return d.doIoctl(whd.SDPCM_SET, iface, cmd, buf[:])
}

// reference: cyw43_get_ioctl_u32
func (d *Device) GetIoctl32(iface whd.IoctlInterface, cmd whd.SDPCMCommand) (uint32, error) {
	d.debug("GetIoctl32", slog.String("iface", iface.String()), slog.String("cmd", cmd.String()))
	var buf [4]byte
	err := d.doIoctl(whd.SDPCM_GET, iface, cmd, buf[:])
	return binary.LittleEndian.Uint32(buf[:4]), err
}

// reference: cyw43_read_iovar_u32
func (d *Device) ReadIOVar(VAR string, iface whd.IoctlInterface) (uint32, error) {
	d.debug("ReadIOVar", slog.String("var", VAR), slog.String("iface", iface.String()))
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

// reference: cyw43_ioctl/cyw43_ll_ioctl
func (d *Device) ioctl(cmd whd.SDPCMCommand, iface whd.IoctlInterface, w []byte) error {
	d.lock()
	defer d.unlock()

	kind := whd.SDPCM_GET
	if cmd&1 != 0 {
		kind = whd.SDPCM_SET
	}
	err := d.doIoctl(uint32(kind), iface, cmd>>1, w)
	if err != nil {
		d.logError("doIoctl fail", slog.Any("err", err))
	}
	return err
}

// doIoctl uses Device's primary buffer to perform ioctl call. Use [Dev.offbuff] for
// allocations that are passed into doIoctl. Does not require offsetting buf for
// the SDPCM header.
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
	timeout := 500 * time.Millisecond

	for time.Since(start) < timeout {
		payloadOffset, plen, header, err := d.sdpcmPoll(d.buf[:])
		d.debug("doIoctl:sdpcmPoll(done)",
			slog.Int("payloadoffset", int(payloadOffset)),
			slog.Int("plen", int(plen)),
			slog.String("header", header.String()),
		)
		payload := d.buf[payloadOffset : payloadOffset+plen]
		switch {
		case err != nil:
			d.logError("doIoctl:sdpcmPoll", slog.String("err", err.Error()))
			break
		case header == whd.CONTROL_HEADER:
			n := copy(buf, payload)
			if uint32(n) != plen {
				return errDoIoctlNoSpace
			}
			return nil
		case header == whd.ASYNCEVENT_HEADER:
			d.handleAsyncEvent(payload)
		case header == whd.DATA_HEADER:
			d.processEthernet(payload)
		default:
			d.logError("got unexpected packet", slog.Uint64("header", uint64(header)))
		}
		time.Sleep(time.Millisecond)
	}

	return errDoIoctlTimeout
}

var errDoIoctlNoSpace = errors.New("not enough space on ioctl buffer for control header copy")
var errDoIoctlTimeout = errors.New("doIoctl time out waiting for data")

// reference: cyw43_send_ioctl
func (d *Device) sendIoctl(kind uint32, iface whd.IoctlInterface, cmd whd.SDPCMCommand, w []byte) error {
	d.debug("sendIoctl")

	length := uint32(len(w))
	if uint32(len(d.buf)) < whd.SDPCM_HEADER_LEN+whd.IOCTL_HEADER_LEN+length {
		return errors.New("ioctl buffer too large for sending")
	}
	d.sdpcmRequestedIoctlID++
	id := uint32(d.sdpcmRequestedIoctlID)
	// flags := (id<<16)&0xffff_0000 | uint32(kind) | uint32(iface)<<12 // look for CDCF_IOC* identifiers in pico-sdk
	header := whd.CDCHeader{
		Cmd:    cmd,
		Length: length & 0xffff,
		ID:     uint16(id),
		Flags:  uint16(kind) | uint16(iface)<<12,
	}
	header.Put(binary.LittleEndian, d.buf[whd.SDPCM_HEADER_LEN:])
	copy(d.buf[whd.SDPCM_HEADER_LEN+whd.IOCTL_HEADER_LEN:], w)
	d.debug("sendIoctl", slog.String("hdr.Cmd", header.Cmd.String()), slog.Uint64("hdr.Len", uint64(header.Length)), slog.Uint64("hdr.Flags", uint64(header.Flags)), slog.Uint64("hdr.Status", uint64(header.Status)))
	return d.sendSDPCMCommon(whd.CONTROL_HEADER, d.buf[:whd.SDPCM_HEADER_LEN+whd.IOCTL_HEADER_LEN+length])
}

// sdpcmPoll reads next packet from WLAN into buf and returns the offset of the
// payload, length of the payload and the header type.
// reference: cyw43_ll_sdpcm_poll_device(cyw43_int_t *self, size_t *len, uint8_t **buf)
func (d *Device) sdpcmPoll(buf []byte) (payloadOffset, plen uint32, header whd.SDPCMHeaderType, err error) {
	d.debug("sdpcmPoll", slog.Int("len", len(buf)))
	// First check the SDIO interrupt line to see if the WLAN notified us
	const badHeader = whd.UNKNOWN_HEADER
	noPacketSuccess := !d.hadSuccesfulPacket
	// TODO(soypat): Adding this causes a timeout in the ioctl test "doIoctl time out waiting for data"
	if noPacketSuccess && !d.hasWork() {
		return 0, 0, badHeader, errors.New("sdpcm:no work")
	}
	err = d.busSleep(false)
	if err != nil {
		return 0, 0, badHeader, err
	}
	if noPacketSuccess {
		// Clear interrupt status so that HOST_WAKE/SDIO line is cleared
		lastInt := d.lastInt
		intStat, err := d.GetInterrupts()
		if err != nil || lastInt != intStat && intStat.IsBusOverflowedOrUnderflowed() {
			d.logError("bus error condition detected", slog.Uint64("intstat", uint64(intStat)), slog.Any("err", err))
		}
		d.lastInt = intStat
		if intStat != 0 {
			d.Write16(FuncBus, whd.SPI_INTERRUPT_REGISTER, uint16(intStat))
		}
		if !intStat.IsF2Available() {
			return 0, 0, badHeader, errors.New("sdpcmPoll: F2 unavailable")
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
		return 0, 0, badHeader, fmt.Errorf("bad status get in sdpcmPoll: %w", err)
	}
	if !status.GSPIPacketAvailable() {
		d.logError("no packet")
		d.hadSuccesfulPacket = false
		return 0, 0, badHeader, errors.New("sdpcmPoll: no packet")
	}
	bytesPending := status.F2PacketLength()
	if bytesPending > uint16(len(buf)) {
		d.logError("bytes pending too large", slog.Uint64("bytesPending", uint64(bytesPending)), slog.Int("len(buf)", len(buf)))
		return 0, 0, badHeader, errors.New("INVALID bytes pending")
	}
	if bytesPending == 0 || bytesPending > linkMTU-gspiPacketOverhead || status.IsUnderflow() {
		d.logError("SPI invalid bytes pending", slog.Uint64("bytesPending", uint64(bytesPending)))
		d.Write8(FuncBackplane, whd.SPI_FRAME_CONTROL, 1)
		d.hadSuccesfulPacket = false
		return 0, 0, badHeader, errors.New("sdpcmPoll: invalid bytes pending")
	}
	err = d.ReadBytes(FuncWLAN, 0, buf[:bytesPending])
	if err != nil {
		return 0, 0, badHeader, err
	}
	hdr0 := binary.LittleEndian.Uint16(buf[:])
	hdr1 := binary.LittleEndian.Uint16(buf[2:])
	if hdr0 == 0 && hdr1 == 0 {
		// no packets.
		d.logError("no packet:zero size header")
		d.hadSuccesfulPacket = false
		return 0, 0, badHeader, errors.New("sdpcmPoll:zero header")
	}
	d.hadSuccesfulPacket = true
	if hdr0^hdr1 != 0xffff {
		d.logError("header xor mismatch", slog.Uint64("hdr[0]", uint64(hdr0)), slog.Uint64("hdr[1]", uint64(hdr1)))
		return 0, 0, badHeader, errors.New("sdpcmPoll:header mismatch")
	}
	return d.sdpcmProcessRxPacket(buf[:bytesPending])
}

var errSendSDPCMTimeout = errors.New("sendSDPCMCommon time out waiting for data")

// sendSDPCMCommon Total IO performed is WriteBytes, which may call GetStatus if packet is WLAN.
// sendSDPCMCommon requires a buffer with 12 bytes of free space at the beginning,
// which are used for the SDPCM header. This means the actual data is at 12 bytes offset!
// Note: sendSDPCMCommon is called from SendEthernet and sendIoctl. Latter uses d.buf.
//
//	reference: cyw43_sdpcm_send_common
func (d *Device) sendSDPCMCommon(kind whd.SDPCMHeaderType, bufWithFreeFirst12Bytes []byte) error {
	// Developer beware: bufWithFreeFirst12Bytes is most likely d.buf
	w := bufWithFreeFirst12Bytes
	d.debug("sendSDPCMCommon", slog.Int("len", len(w)), slog.String("kind", kind.String()))
	if kind != whd.CONTROL_HEADER && kind != whd.DATA_HEADER {
		return errors.New("unexpected SDPCM kind")
	}
	err := d.busSleep(false)
	if err != nil {
		return err
	}

	// soypat: careful- this call is using d.buf! When you call d.sdcpmPoll on d.buf
	// you are overwriting the data in the argument buffer.

	// TODO: I've coded this up, but it is causing a timeout so something is
	// TODO: wrong or got lost in translation...needs investigation.

	// Wait until we are allowed to send Credits which are 8-bit unsigned
	// integers that roll over, so we are stalled while they are equal

	start := time.Now()
	timeout := 1000 * time.Millisecond
	auxbuf := d.offbuf()
	for d.wlanFlowCtl != 0 || d.sdpcmLastBusCredit == d.sdpcmTxSequence {
		d.debug("wait for credits", slog.Int("wlanFlowCtl", int(d.wlanFlowCtl)))
		if time.Since(start) > timeout {
			return errSendSDPCMTimeout
		}
		payloadOffset, plen, header, err := d.sdpcmPoll(auxbuf[:])
		payload := auxbuf[payloadOffset : payloadOffset+plen]
		switch {
		case err != nil:
			d.logError("sendSDPCMCommon:sdpcmPoll", slog.String("err", err.Error()))
			break
		case header == whd.ASYNCEVENT_HEADER:
			d.handleAsyncEvent(payload)
		case header == whd.DATA_HEADER:
			// Don't proccess it due to possible reentrancy
			// issues (eg sending another ETH as part of
			// the reception)
		default:
			d.debug("got unexpected packet")
		}
		time.Sleep(time.Millisecond)
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
	header.Put(binary.LittleEndian, w)
	d.sdpcmTxSequence++
	return d.WriteBytes(FuncWLAN, 0, w)
}

// reference: disable_device_core
func (d *Device) disableDeviceCore(coreID uint8, coreHalt bool) error {
	base := coreaddress(coreID)
	d.debug("disableDeviceCore", slog.Int("coreid", int(coreID)))
	d.ReadBackplane(base+whd.AI_RESETCTRL_OFFSET, 1)
	reg, err := d.ReadBackplane(base+whd.AI_RESETCTRL_OFFSET, 1)
	if err != nil {
		return err
	}
	if reg&whd.AIRC_RESET != 0 {
		return nil
	}
	d.logError("core not in reset")
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
	d.WriteBackplane(base+whd.AI_IOCTRL_OFFSET, 1, whd.SICF_FGC|whd.SICF_CLOCK_EN|cpuhaltFlag)
	d.ReadBackplane(base+whd.AI_IOCTRL_OFFSET, 1)
	d.WriteBackplane(base+whd.AI_RESETCTRL_OFFSET, 1, 0)
	time.Sleep(time.Millisecond)
	d.WriteBackplane(base+whd.AI_IOCTRL_OFFSET, 1, whd.SICF_CLOCK_EN|cpuhaltFlag)
	d.ReadBackplane(base+whd.AI_IOCTRL_OFFSET, 1)
	time.Sleep(time.Millisecond)
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
	d.debugIO("ReadBackplane", slog.Uint64("addr", uint64(addr)), slog.Uint64("size", uint64(size)))
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
	d.debugIO("WriteBackplane", slog.Uint64("addr", uint64(addr)), slog.Uint64("size", uint64(size)), slog.Uint64("value", uint64(value)))
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
func (d *Device) downloadResource(addr uint32, src string) error {
	d.debug("download resource", slog.Uint64("addr", uint64(addr)), slog.Int("len", len(src)))
	// round up length to simplify download.
	rlen := (len(src) + 255) &^ 255
	// if len(src) < rlen {
	// 	return errors.New("firmware slice capacity needs extra 255 padding over it's length for transfer")
	// }
	const BLOCKSIZE = 64
	var srcPtr string
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
		var n int
		if offset+sz > len(src) {
			n = sz
			buf = [len(buf)]byte{}
			if offset < len(src) {
				copy(buf[:sz], src[offset:])
			}
		} else {
			srcPtr = src[offset:]
			n = copy(buf[:sz], srcPtr)
		}

		err = d.WriteBytes(FuncBackplane, dstAddr&whd.BACKPLANE_ADDR_MASK, buf[:n])
		if err != nil {
			return err
		}
	}
	if !validateDownloads {
		return nil
	}
	d.debug("download finished, validate data")
	// Finished writing firmware... should be ready for use. We choose to validate it though.
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

		err = d.ReadBytes(FuncBackplane, dstAddr&whd.BACKPLANE_ADDR_MASK, buf[:sz])
		if err != nil {
			return err
		}
		if offset+sz > len(src) {
			srcPtr = src[offset:]
			sz = len(srcPtr)
		} else {
			srcPtr = src[offset:]
		}
		if string(buf[:sz]) != srcPtr[:sz] {
			err = fmt.Errorf("%w at addr=%#x: expected:%q\ngot: %q", errFirmwareValidationFailed, dstAddr, srcPtr[:sz], buf[:sz])
			return err
		}
	}
	d.debug("firmware validation success")
	return nil
}

// reference: cyw43_ll_bus_sleep and cyw43_ll_bus_sleep_helper
func (d *Device) busSleep(canSleep bool) (err error) {
	d.debug("busSleep", slog.Bool("canSleep", canSleep), slog.Bool("busIsUp", d.busIsUp))
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
	d.debug("ksoSet", slog.Bool("value", value))
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
		d.debug("clm data send", slog.Int("off+ln", int(off+ln)), slog.Int("clmlen", int(clmLen)))
		err := d.doIoctl(whd.SDPCM_SET, whd.IF_STA, whd.WLC_SET_VAR, buf[:align32(20+uint32(ln), 8)])
		if err != nil {
			return err
		}
	}
	d.debug("clm data send done")
	// Check status of the download.
	const clmStatString = "clmload_status\x00\x00\x00\x00\x00"
	const clmStatLen = len(clmStatString)
	buf = d.auxbuf[:]
	copy(buf[:clmStatLen], clmStatString)
	err := d.doIoctl(whd.SDPCM_GET, whd.IF_STA, whd.WLC_GET_VAR, buf[:clmStatLen])
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
	err := d.doIoctl(whd.SDPCM_GET, whd.IF_STA, whd.WLC_GET_VAR, buf[:len(varMAC)])
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
func (d *Device) sdpcmProcessRxPacket(buf []byte) (payloadOffset, plen uint32, header whd.SDPCMHeaderType, err error) {
	d.debug("sdpcmProcessRxPacket", slog.Int("len", len(buf)))
	const badHeader = whd.UNKNOWN_HEADER
	hdr := whd.DecodeSDPCMHeader(binary.LittleEndian, buf)
	switch {
	case hdr.Size != ^hdr.SizeCom&0xffff:
		return 0, 0, badHeader, err2InvalidPacket
	case hdr.Size < whd.SDPCM_HEADER_LEN:
		return 0, 0, badHeader, err3PacketTooSmall
	}
	if d.wlanFlowCtl != hdr.WirelessFlowCtl {
		d.debug("sdpcmProcessRxPacket:WLANFLOWCTL_CHANGE", slog.Uint64("old", uint64(d.wlanFlowCtl)), slog.Uint64("new", uint64(hdr.WirelessFlowCtl)))
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
		return 0, 0, badHeader, err4IgnoreControlPacket // Flow ctl packet with no data.
	}

	payloadOffset = uint32(hdr.HeaderLength)
	headerType := hdr.Type()
	switch headerType {
	case whd.CONTROL_HEADER:
		d.debug("sdpcmProcessRxPacket:CONTROL_HEADER")
		const totalHeaderSize = whd.SDPCM_HEADER_LEN + whd.IOCTL_HEADER_LEN
		if hdr.Size < totalHeaderSize {
			return 0, 0, badHeader, err5IgnoreSmallControlPacket
		}
		if payloadOffset+whd.IOCTL_HEADER_LEN > uint32(len(buf)) {
			// TODO(soypat): This error case is not specified in the reference.
			return 0, 0, badHeader, errors.New("undefined control packet error, size too large")
		}
		ioctlHeader := whd.DecodeCDCHeader(binary.LittleEndian, buf[payloadOffset:])
		// ioctlHeader := whd.DecodeIoctlHeader(binary.LittleEndian, buf[payloadOffset:])
		id := ioctlHeader.ID
		if id != d.sdpcmRequestedIoctlID {
			return 0, 0, badHeader, err6IgnoreWrongIDPacket
		}
		payloadOffset += whd.IOCTL_HEADER_LEN
		plen = uint32(hdr.Size) - payloadOffset
		d.debug("sdpcmProcessRxPacket:CONTROL_HEADER", slog.Int("id", int(id)), slog.Int("len", int(plen)))

	case whd.DATA_HEADER:
		d.debug("sdpcmProcessRxPacket:DATA_HEADER")
		const totalHeaderSize = whd.SDPCM_HEADER_LEN + whd.BDC_HEADER_LEN
		if hdr.Size <= totalHeaderSize {
			return 0, 0, badHeader, err7IgnoreSmallDataPacket
		}

		bdcHeader := whd.DecodeBDCHeader(buf[payloadOffset:])
		itf := bdcHeader.Flags2 // Get interface number.
		payloadOffset += whd.BDC_HEADER_LEN + uint32(bdcHeader.DataOffset)<<2
		plen = (uint32(hdr.Size) - payloadOffset) | uint32(itf)<<31

	case whd.ASYNCEVENT_HEADER:
		d.debug("sdpcmProcessRxPacket:ASYNC_HEADER")
		const totalHeaderSize = whd.SDPCM_HEADER_LEN + whd.BDC_HEADER_LEN
		if hdr.Size <= totalHeaderSize {
			return 0, 0, badHeader, err8IgnoreTooSmallAsyncPacket
		}
		bdcHeader := whd.DecodeBDCHeader(buf[payloadOffset:])
		payloadOffset += whd.BDC_HEADER_LEN + (uint32(bdcHeader.DataOffset) << 2)
		// headerPtr := uint32(uintptr(unsafe.Pointer(&buf[0])))
		// payloadPtr := uint32(uintptr(unsafe.Pointer(&buf[payloadOffset])))
		// plen = uint32(hdr.Size) - (payloadPtr - headerPtr)
		// Reference does some gnarly pointer arithmetic here. This is the safe equivalent to above.
		plen = uint32(hdr.Size) - payloadOffset
		payload := buf[payloadOffset:]
		// Check payload is actually an ethernet packet with type 0x886c.
		if !(payload[12] == 0x88 && payload[13] == 0x6c) {
			// ethernet packet doesn't have the correct type.
			// Note - this happens during startup but appears to be expected
			return 0, 0, badHeader, err9WrongPayloadType
		}
		// Check the Broadcom OUI.
		if !(payload[19] == 0x00 && payload[20] == 0x10 && payload[21] == 0x18) {
			return 0, 0, badHeader, err10IncorrectOUI
		}
		// Apparently we skip over ethernet header (14 bytes) and the event header (10 bytes):
		plen = plen - 24
		payloadOffset = payloadOffset + 24
	default:
		// Unknown Header.
		return 0, 0, badHeader, err11UnknownHeader
	}
	return payloadOffset, plen, headerType, err
}
