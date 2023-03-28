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

	"github.com/soypat/cy43439/whd"
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

	sdpcmHeaderSize = 12 //unsafe.Sizeof(sdpcmHeader{})
	ioctlHeaderSize = 16 // unsafe.Sizeof(ioctlHeader{})
)

func (d *Dev) GPIOSet(wlGPIO uint8, value bool) (err error) {
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

func (d *Dev) WriteIOVar(VAR string, iface whd.IoctlInterface, val uint32) error {
	Debug("WriteIOVar var=", VAR, "ioctl=", iface, "val=", val)
	buf := d.buf[1024:]
	length := copy(buf, VAR)
	buf[length] = 0 // Null terminate the string
	length++
	binary.BigEndian.PutUint32(buf[length:], val)
	return d.doIoctl(whd.SDPCM_SET, iface, whd.WLC_SET_VAR, buf[:length+4])
}

func (d *Dev) WriteIOVar2(VAR string, iface whd.IoctlInterface, val0, val1 uint32) error {
	buf := d.buf[1024:]
	length := copy(buf, VAR)
	buf[length] = 0 // Null terminate the string
	length++
	binary.BigEndian.PutUint32(buf[length:], val0)
	binary.BigEndian.PutUint32(buf[length+4:], val1)
	return d.doIoctl(whd.SDPCM_SET, iface, whd.WLC_SET_VAR, buf[:length+8])
}

func (d *Dev) WriteIOVarN(VAR string, iface whd.IoctlInterface, src []byte) error {
	iobuf := d.buf[1024:]
	if len(VAR)+len(src)+1 > len(iobuf) {
		return errors.New("buffer too short for IOVarN call")
	}
	length := copy(iobuf, VAR)
	iobuf[length] = 0 // Null terminate the string
	length++
	length += copy(iobuf[length:], src)
	return d.doIoctl(whd.SDPCM_SET, iface, whd.WLC_SET_VAR, iobuf[:length])
}

func (d *Dev) DoIoctl32(kind uint32, iface whd.IoctlInterface, cmd whd.SDPCMCommand, val uint32) error {
	println("DoIoctl32")
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], val)
	return d.doIoctl(kind, iface, cmd, buf[:])
}

func (d *Dev) ioctl(cmd whd.SDPCMCommand, iface whd.IoctlInterface, w []byte) error {
	kind := uint32(0)
	if cmd&1 != 0 {
		kind = 2
	}
	return d.doIoctl(kind, iface, cmd>>1, w)
}

func (d *Dev) doIoctl(kind uint32, iface whd.IoctlInterface, cmd whd.SDPCMCommand, buf []byte) error {
	// TODO do we have to add all that polling?
	err := d.sendIoctl(kind, iface, cmd, buf)
	if err != nil {
		return err
	}
	start := time.Now()
	const ioctlTimeout = 50 * time.Millisecond
	for time.Since(start) < ioctlTimeout {
		payloadOffset, plen, header, err := d.sdpcmPoll(d.buf[:])
		Debug("doIoctl:sdpcmPoll conclude poff=", payloadOffset, "plen=", plen, "header=", header, err)
		if err != nil {
			return err
		}

		switch header {
		case whd.CONTROL_HEADER:
			n := copy(buf[:], d.buf[:plen])
			if uint32(n) != plen {
				return errors.New("not enough space on ioctl buffer for control header copy")
			}
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	Debug("todo")
	return nil
}

func (d *Dev) sdpcmPoll(buf []byte) (payloadOffset, plen uint32, res uint8, err error) {
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
			d.Write16(FuncBus, AddrInterrupt, uint16(intStat))
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
	}
	if status == 0xFFFFFFFF || err != nil {
		return 0, 0, badResult, fmt.Errorf("bad status get in sdpcmPoll: %w", err)
	}
	if !status.F2PacketAvailable() {
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
	err = d.ReadBytes(FuncWLAN, 0, d.buf[:bytesPending])
	if err != nil {
		return 0, 0, badResult, err
	}
	hdr0 := binary.LittleEndian.Uint16(d.buf[:])
	hdr1 := binary.LittleEndian.Uint16(d.buf[2:])
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
	return d.sdpcmProcessRxPacket(d.buf[:])
}

var (
	Err2InvalidPacket             = errors.New("invalid packet")
	Err3PacketTooSmall            = errors.New("packet too small")
	Err4IgnoreControlPacket       = errors.New("ignore flow ctl packet")
	Err5IgnoreSmallControlPacket  = errors.New("ignore too small flow ctl packet")
	Err6IgnoreWrongIDPacket       = errors.New("ignore packet with wrong id")
	Err7IgnoreSmallDataPacket     = errors.New("ignore too small data packet")
	Err8IgnoreTooSmallAsyncPacket = errors.New("ignore too small async packet")
	Err9WrongPayloadType          = errors.New("wrong payload type")
	Err10IncorrectOUI             = errors.New("incorrect oui")
	Err11UnknownHeader            = errors.New("unknown header")
)

func (d *Dev) sdpcmProcessRxPacket(buf []byte) (payloadOffset, plen uint32, flag uint8, err error) {
	const badFlag = 0
	const sdpcmOffset = 0
	hdr := whd.DecodeSDPCMHeader(buf[sdpcmOffset:])
	switch {
	case hdr.Size != ^hdr.SizeCom&0xffff:
		return 0, 0, badFlag, Err2InvalidPacket
	case hdr.Size < sdpcmHeaderSize:
		return 0, 0, badFlag, Err3PacketTooSmall
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
		return 0, 0, badFlag, Err4IgnoreControlPacket // Flow ctl packet with no data.
	}

	payloadOffset = uint32(hdr.HeaderLength)
	headerFlag := hdr.ChanAndFlags & 0xf
	switch headerFlag {
	case whd.CONTROL_HEADER:
		const totalHeaderSize = whd.SDPCM_HEADER_LEN + whd.IOCTL_HEADER_LEN
		if hdr.Size < totalHeaderSize {
			return 0, 0, badFlag, Err5IgnoreSmallControlPacket
		}
		ioctlHeader := whd.DecodeIoctlHeader(buf[payloadOffset:])
		id := ioctlHeader.ID()
		if id != d.sdpcmRequestedIoctlID {
			return 0, 0, badFlag, Err6IgnoreWrongIDPacket
		}
		payloadOffset += whd.IOCTL_HEADER_LEN
		plen = uint32(hdr.Size) - payloadOffset

	case whd.DATA_HEADER:
		const totalHeaderSize = whd.SDPCM_HEADER_LEN + whd.BDC_HEADER_LEN
		if hdr.Size <= totalHeaderSize {
			return 0, 0, badFlag, Err7IgnoreSmallDataPacket
		}

		bdcHeader := whd.DecodeBDCHeader(buf[payloadOffset:])
		itf := bdcHeader.Flags2 // Get interface number.
		payloadOffset += whd.BDC_HEADER_LEN + uint32(bdcHeader.DataOffset)<<2
		plen = (uint32(hdr.Size) - payloadOffset) | uint32(itf)<<31

	case whd.ASYNCEVENT_HEADER:
		const totalHeaderSize = whd.SDPCM_HEADER_LEN + whd.BDC_HEADER_LEN
		if hdr.Size <= totalHeaderSize {
			return 0, 0, badFlag, Err8IgnoreTooSmallAsyncPacket
		}
		bdcHeader := whd.DecodeBDCHeader(buf[payloadOffset:])
		payloadOffset += whd.BDC_HEADER_LEN + uint32(bdcHeader.DataOffset)<<2
		plen = uint32(hdr.Size) - payloadOffset
		payload := buf[payloadOffset:]
		// payload is actually an ethernet packet with type 0x886c.
		if !(payload[12] == 0x88 && payload[13] == 0x6c) {
			// ethernet packet doesn't have the correct type.
			// Note - this happens during startup but appears to be expected
			return 0, 0, badFlag, Err9WrongPayloadType
		}
		// Check the Broadcom OUI.
		if !(payload[19] == 0x00 && payload[20] == 0x10 && payload[21] == 0x18) {
			return 0, 0, badFlag, Err10IncorrectOUI
		}
		plen = plen - 24
		payloadOffset = payloadOffset + 24
	default:
		// Unknown Header.
		return 0, 0, badFlag, Err11UnknownHeader
	}
	return payloadOffset, plen, headerFlag, nil
}

// sendIoctl is cyw43_send_ioctl in pico-sdk (actually contained in cy43-driver)
func (d *Dev) sendIoctl(kind uint32, iface whd.IoctlInterface, cmd whd.SDPCMCommand, w []byte) error {
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
	return d.sendSDPCMCommon(sdpcmCTLHEADER, d.buf[:whd.SDPCM_HEADER_LEN+whd.IOCTL_HEADER_LEN+length])
}

// sendSDPCMCommon is cyw43_sdpcm_send_common in pico-sdk (actually contained in cy43-driver)
func (d *Dev) sendSDPCMCommon(kind uint32, w []byte) error {
	// TODO where is cmd used here????
	Debug("sendSDPCMCommon")
	if kind != sdpcmCTLHEADER && kind != sdpcmDATAHEADER {
		return errors.New("unexpected SDPCM kind")
	}
	err := d.busSleep(false)
	if err != nil {
		return err
	}
	headerLength := uint8(whd.SDPCM_HEADER_LEN)
	if kind == sdpcmDATAHEADER {
		headerLength += 2
	}
	size := uint16(len(w))        //+ uint16(hdlen)
	paddedSize := (size + 3) &^ 3 // If not using gSPI then this should be padded to 64bytes
	if uint16(cap(w)) < paddedSize {
		return errors.New("buffer too small to be SDPCM padded")
	}
	w = w[0:paddedSize]
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
	const addr = 0x18103000 + AI_IOCTRL_OFFSET
	Debug("begin reset process coreid=", coreID)
	d.WriteBackplane(base+AI_IOCTRL_OFFSET, 1, SICF_FGC|SICF_CLOCK_EN|cpuhaltFlag)
	d.ReadBackplane(base+AI_IOCTRL_OFFSET, 1)
	d.WriteBackplane(base+AI_RESETCTRL_OFFSET, 1, 0)
	time.Sleep(time.Millisecond)
	d.WriteBackplane(base+AI_IOCTRL_OFFSET, 1, SICF_CLOCK_EN|cpuhaltFlag)
	d.ReadBackplane(base+AI_IOCTRL_OFFSET, 1)
	time.Sleep(time.Millisecond)
	Debug("end reset process coreid=", coreID)
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

// coreaddress returns either WLAN=0x18103000  or  SOCRAM=0x18104000
func coreaddress(coreID uint8) (v uint32) {
	switch coreID {
	case CORE_WLAN_ARM:
		v = WRAPPER_REGISTER_OFFSET + WLAN_ARMCM3_BASE_ADDRESS
	case CORE_SOCRAM:
		v = WRAPPER_REGISTER_OFFSET + SOCSRAM_BASE_ADDRESS
	default:
		panic("bad core id")
	}
	return v
}

func (d *Dev) ReadBackplane(addr uint32, size uint32) (uint32, error) {
	Debug("read backplane", addr, size)
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

func (d *Dev) WriteBackplane(addr, size, value uint32) error {
	Debug("write backplane", addr, "=", value, "size=", int(size))
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
	currentWindow := d.currentBackplaneWindow
	addr = addr &^ backplaneAddrMask
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

func (d *Dev) downloadResource(addr uint32, src []byte) error {
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
		if offset+sz > len(src) {
			// fmt.Println("ALLOCA", sz)
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
func (d *Dev) ksoSet(value bool) error {
	Debug("ksoSet enable=", value)
	var writeVal uint8
	if value {
		writeVal = SBSDIO_SLPCSR_KEEP_SDIO_ON
	}
	// These can fail and it's still ok.
	d.Write8(FuncBackplane, SDIO_SLEEP_CSR, writeVal)
	d.Write8(FuncBackplane, SDIO_SLEEP_CSR, writeVal)
	// Put device to sleep, turn off KSO if value == 0 and
	// check for bit0 only, bit1(devon status) may not get cleared right away
	var compareValue uint8
	var bmask uint8 = SBSDIO_SLPCSR_KEEP_SDIO_ON
	if value {
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
		if err == nil && readValue&bmask == compareValue && readValue != 0xff {
			return nil // success
		}
		time.Sleep(time.Millisecond)
		d.Write8(FuncBackplane, SDIO_SLEEP_CSR, writeVal)
	}
	return errors.New("kso set failed")
}

func (d *Dev) clmLoad(clm []byte) error {
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
		Debug("clm data send off+len=", off+ln, "clmlen=", clmLen)
		err := d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_VAR, buf[:align32(20+uint32(ln), 8)])
		if err != nil {
			return err
		}
	}
	// CLM data send done.
	const clmStatString = "clmload_status\x00\x00\x00\x00\x00"
	copy(buf[:len(clmStatString)], clmStatString)
	err := d.doIoctl(whd.SDPCM_GET, whd.WWD_STA_INTERFACE, whd.WLC_GET_VAR, buf[:len(clmStatString)])
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
	const sdpcmHeaderLen = unsafe.Sizeof(whd.SDPCMHeader{})
	buf := d.buf[sdpcmHeaderLen+16:]
	const varMAC = "cur_etheraddr\x00\x00\x00\x00\x00\x00\x00"
	copy(buf[:len(varMAC)], varMAC)
	err := d.doIoctl(SDPCM_GET, whd.WWD_STA_INTERFACE, whd.WLC_GET_VAR, buf[:len(varMAC)])
	if err == nil {
		copy(dst[:6], buf)
	}
	return err
}
