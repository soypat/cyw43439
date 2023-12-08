package cyrw

// File based on mainly on bus.rs from the reference
// https://github.com/embassy-rs/embassy/blob/26870082427b64d3ca42691c55a2cded5eadc548/cyw43/src/bus.rs

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log/slog"
	"reflect"
	"time"
	"unsafe"

	"github.com/soypat/cyw43439/whd"
	"golang.org/x/exp/constraints"
)

func (d *Device) initBus() error {
	// https://github.com/embassy-rs/embassy/blob/26870082427b64d3ca42691c55a2cded5eadc548/cyw43/src/bus.rs#L51
	d.Reset()
	retries := 128
	for {
		got := d.read32_swapped(whd.SPI_READ_TEST_REGISTER)
		if got == whd.TEST_PATTERN {
			break
		} else if retries <= 0 {
			return errors.New("spi test failed:" + hex32(got))
		}
		retries--
	}
	const RWTestPattern = 0x12345678
	const spiRegTestRW = 0x18
	d.write32_swapped(spiRegTestRW, RWTestPattern)
	got := d.read32_swapped(spiRegTestRW)
	if got != RWTestPattern {
		return errors.New("spi test failed:" + hex32(got) + " wanted " + hex32(RWTestPattern))
	}

	// Address 0x0000 registers.
	const (
		// 0=16bit word, 1=32bit word transactions.
		WordLengthPos = 0
		// Set to 1 for big endian words.
		EndianessBigPos = 1 // 30
		HiSpeedModePos  = 4
		InterruptPolPos = 5
		WakeUpPos       = 7

		ResponseDelayPos       = 0x1*8 + 0
		StatusEnablePos        = 0x2*8 + 0
		InterruptWithStatusPos = 0x2*8 + 1
		// 132275 is Pico-sdk's default value.
		// NOTE: embassy uses little endian words and StatusEnablePos.
		setupValue = (1 << WordLengthPos) | (1 << HiSpeedModePos) | (0 << EndianessBigPos) |
			(1 << InterruptPolPos) | (1 << WakeUpPos) |
			(1 << InterruptWithStatusPos) | (1 << StatusEnablePos)
	)
	val := d.read32_swapped(0)

	d.write32_swapped(whd.SPI_BUS_CONTROL, setupValue)
	got8, _ := d.read8(FuncBus, whd.SPI_BUS_CONTROL)
	d.debug("read back bus ctl", slog.Uint64("got", uint64(got8)))

	got, err := d.read32(FuncBus, whd.SPI_READ_TEST_REGISTER)

	d.debug("current bus ctl" + hex32(val) + "writing:" + hex32(setupValue) + " got:" + hex32(got))
	if err != nil || got != whd.TEST_PATTERN {
		return errjoin(errors.New("spi RO test failed:"+hex32(got)), err)
	}

	got, err = d.read32(FuncBus, spiRegTestRW)
	if err != nil || got != RWTestPattern {
		return errjoin(errors.New("spi RW test failed:"+hex32(got)), err)
	}
	return nil
}

func (d *Device) core_disable(coreID uint8) error {
	base := coreaddress(coreID)

	// Check if not already in reset.
	d.bp_read8(base + whd.AI_RESETCTRL_OFFSET) // Dummy read.
	r, _ := d.bp_read8(base + whd.AI_RESETCTRL_OFFSET)
	if r&whd.AIRC_RESET != 0 {
		return nil
	}

	d.bp_write8(base+whd.AI_IOCTRL_OFFSET, 0)
	d.bp_read8(base + whd.AI_IOCTRL_OFFSET) // Another dummy read.
	time.Sleep(time.Millisecond)

	d.bp_write8(base+whd.AI_RESETCTRL_OFFSET, whd.AIRC_RESET)
	r, _ = d.bp_read8(base + whd.AI_RESETCTRL_OFFSET)
	if r&whd.AIRC_RESET != 0 {
		return nil
	}
	return errors.New("core disable failed")
}

func (d *Device) core_reset(coreID uint8, coreHalt bool) error {
	err := d.core_disable(coreID)
	if err != nil {
		return err
	}
	var cpuhaltFlag uint8
	if coreHalt {
		cpuhaltFlag = whd.SICF_CPUHALT
	}
	base := coreaddress(coreID)
	const addr = 0x18103000 + whd.AI_IOCTRL_OFFSET
	d.bp_write8(base+whd.AI_IOCTRL_OFFSET, whd.SICF_FGC|whd.SICF_CLOCK_EN|cpuhaltFlag)
	d.bp_read8(base + whd.AI_IOCTRL_OFFSET) // Dummy read.

	d.bp_write8(base+whd.AI_RESETCTRL_OFFSET, 0)
	time.Sleep(time.Millisecond)

	d.bp_write8(base+whd.AI_IOCTRL_OFFSET, whd.SICF_CLOCK_EN|cpuhaltFlag)
	d.bp_read8(base + whd.AI_IOCTRL_OFFSET) // Dummy read.
	time.Sleep(time.Millisecond)
	return nil
}

// CoreIsActive returns if the specified core is not in reset.
// Can be called with CORE_WLAN_ARM and CORE_SOCRAM global constants.
// It may return true if communications are down (WL_REG_ON at low).
//
//	reference: device_core_is_up
func (d *Device) core_is_up(coreID uint8) bool {
	base := coreaddress(coreID)
	reg, _ := d.bp_read8(base + whd.AI_IOCTRL_OFFSET)
	if reg&(whd.SICF_FGC|whd.SICF_CLOCK_EN) != whd.SICF_CLOCK_EN {
		return false
	}
	reg, _ = d.bp_read8(base + whd.AI_RESETCTRL_OFFSET)
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

type sharedMem struct {
	flags            uint32 // offset 0x00
	trap_addr        uint32 // offset 0x04
	assert_exp_addr  uint32 // offset 0x08
	assert_file_addr uint32 // offset 0x0c
	assert_line      uint32 // offset 0x10
	console_addr     uint32 // offset 0x14
	msgtrace_addr    uint32 // offset 0x18
	fwid             uint32 // offset 0x1c
}

func decodeSharedMem(order binary.ByteOrder, buf []byte) (s sharedMem) {
	s.flags = order.Uint32(buf[0:4])
	s.trap_addr = order.Uint32(buf[4:8])
	s.assert_exp_addr = order.Uint32(buf[8:12])
	s.assert_file_addr = order.Uint32(buf[12:16])
	s.assert_line = order.Uint32(buf[16:20])
	s.console_addr = order.Uint32(buf[20:24])
	s.msgtrace_addr = order.Uint32(buf[24:28])
	s.fwid = order.Uint32(buf[28:32])
	return s
}

type logstate struct {
	addr     uint32
	last_idx uint32
	buf      [256]byte
	bufcount uint32
}

// sharedMemLog has size 4*4=16
type sharedMemLog struct {
	buf     uint32
	bufSize uint32
	idx     uint32
	outIdx  uint32
}

func decodeSharedMemLog(order binary.ByteOrder, buf []byte) (s sharedMemLog) {
	s.buf = order.Uint32(buf[0:4])
	s.bufSize = order.Uint32(buf[4:8])
	s.idx = order.Uint32(buf[8:12])
	s.outIdx = order.Uint32(buf[12:16])
	return s
}

func (d *Device) wlan_read(buf []uint32, lenInBytes int) (err error) {
	cmd := cmd_word(false, true, FuncWLAN, 0, uint32(lenInBytes))
	lenU32 := (lenInBytes + 3) / 4
	_, err = d.spi.cmd_read(cmd, buf[:lenU32])
	d.lastStatusGet = time.Now()
	return err
}

func (d *Device) wlan_write(data []uint32, plen uint32) (err error) {
	cmd := cmd_word(true, true, FuncWLAN, 0, plen)
	_, err = d.spi.cmd_write(cmd, data)
	d.lastStatusGet = time.Now()
	return err
}

func (d *Device) bp_read(addr uint32, data []byte) (err error) {
	const maxTxSize = whd.BUS_SPI_MAX_BACKPLANE_TRANSFER_SIZE
	// var buf [maxTxSize]byte
	alignedLen := align(uint32(len(data)), 4)
	data = data[:alignedLen]
	var buf [maxTxSize/4 + 1]uint32
	buf8 := unsafeAsSlice[uint32, byte](buf[:])
	for len(data) > 0 {
		// Calculate address and length of next write.
		windowOffset := addr & whd.BACKPLANE_ADDR_MASK
		windowRemaining := 0x8000 - windowOffset // windowsize - windowoffset
		lenBytes := min(min(uint32(len(data)), maxTxSize), windowRemaining)

		err = d.backplane_setwindow(addr)
		if err != nil {
			return err
		}
		cmd := cmd_word(false, true, FuncBackplane, windowOffset, lenBytes)

		// round `buf` to word boundary, add one extra word for the response delay byte.
		_, err = d.spi.cmd_read(cmd, buf[:(lenBytes+3)/4+1])
		if err != nil {
			return err
		}
		// when writing out the data, we skip the response-delay *word* (4 bytes).
		copy(data[:lenBytes], buf8[4:4+lenBytes])
		addr += lenBytes
		data = data[lenBytes:]
	}
	d.lastStatusGet = time.Now()
	return err
}

// bp_writestring exists to leverage static string data which is always put in flash.
func (d *Device) bp_writestring(addr uint32, data string) error {
	hdr := (*reflect.StringHeader)(unsafe.Pointer(&data))
	sliceHdr := reflect.SliceHeader{
		Data: hdr.Data,
		Len:  hdr.Len,
		Cap:  align(hdr.Len, 4),
	}
	return d.bp_write(addr, *(*[]byte)(unsafe.Pointer(&sliceHdr)))
}

func (d *Device) bp_write(addr uint32, data []byte) (err error) {
	if addr%4 != 0 {
		return errors.New("addr must be 4-byte aligned")
	}
	const maxTxSize = whd.BUS_SPI_MAX_BACKPLANE_TRANSFER_SIZE
	// var buf [maxTxSize]byte
	alignedLen := align(uint32(len(data)), 4)
	data = data[:alignedLen]
	d.debug("bp_write",
		slog.Uint64("addr", uint64(addr)),
		slog.String("last16", hex.EncodeToString(data[max(0, len(data)-16):])), // mismatch with reference?
	)
	var buf [maxTxSize/4 + 1]uint32
	buf8 := unsafeAsSlice[uint32, byte](buf[:])
	for err == nil && len(data) > 0 {
		// Calculate address and length of next write to ensure transfer doesn't cross a window boundary.
		windowOffset := addr & whd.BACKPLANE_ADDR_MASK
		windowRemaining := 0x8000 - windowOffset // windowsize - windowoffset
		length := min(min(uint32(len(data)), maxTxSize), windowRemaining)
		copy(buf8[:length], data[:length])

		err = d.backplane_setwindow(addr)
		if err != nil {
			return err
		}
		cmd := cmd_word(true, true, FuncBackplane, windowOffset, length)

		_, err = d.spi.cmd_write(cmd, buf[:(length+3)/4+1])
		addr += length
		data = data[length:]
	}
	d.lastStatusGet = time.Now()
	d.debug("bp_write:done", slog.String("status", d.status().String()))
	return nil
}

func (d *Device) bp_read8(addr uint32) (uint8, error) {
	v, err := d.backplane_readn(addr, 1)
	return uint8(v), err
}
func (d *Device) bp_write8(addr uint32, val uint8) error {
	return d.backplane_writen(addr, uint32(val), 1)
}
func (d *Device) bp_read16(addr uint32) (uint16, error) {
	v, err := d.backplane_readn(addr, 2)
	return uint16(v), err
}
func (d *Device) bp_write16(addr uint32, val uint16) error {
	return d.backplane_writen(addr, uint32(val), 2)
}
func (d *Device) bp_read32(addr uint32) (uint32, error) {
	return d.backplane_readn(addr, 4)
}
func (d *Device) bp_write32(addr, val uint32) error {
	return d.backplane_writen(addr, val, 4)
}

func (d *Device) backplane_readn(addr, size uint32) (uint32, error) {
	err := d.backplane_setwindow(addr)
	if err != nil {
		return 0, err
	}
	addr &= whd.BACKPLANE_ADDR_MASK
	if size == 4 {
		addr |= 0x08000 // 32bit addr flag, a.k.a: whd.SBSDIO_SB_ACCESS_2_4B_FLAG
	}
	// cref: defer d.setBackplaneWindow(whd.CHIPCOMMON_BASE_ADDRESS)
	return d.readn(FuncBackplane, addr, size)
}

func (d *Device) backplane_writen(addr, val, size uint32) (err error) {
	err = d.backplane_setwindow(addr)
	if err != nil {
		return err
	}
	addr &= whd.BACKPLANE_ADDR_MASK
	if size == 4 {
		addr |= 0x08000 // 32bit addr flag, a.k.a: whd.SBSDIO_SB_ACCESS_2_4B_FLAG
	}
	// cref: defer d.setBackplaneWindow(whd.CHIPCOMMON_BASE_ADDRESS)
	return d.writen(FuncBackplane, addr, val, size)
}

func (d *Device) backplane_setwindow(addr uint32) (err error) {
	const (
		SDIO_BACKPLANE_ADDRESS_HIGH = 0x1000c
		SDIO_BACKPLANE_ADDRESS_MID  = 0x1000b
		SDIO_BACKPLANE_ADDRESS_LOW  = 0x1000a
	)
	currentWindow := d.backplaneWindow
	addr = addr &^ whd.BACKPLANE_ADDR_MASK
	if addr == currentWindow {
		d.backplaneWindow = addr // Does this line have effect?
		return nil
	}

	if (addr & 0xff000000) != currentWindow&0xff000000 {
		err = d.write8(FuncBackplane, SDIO_BACKPLANE_ADDRESS_HIGH, uint8(addr>>24))
	}
	if err == nil && (addr&0x00ff0000) != currentWindow&0x00ff0000 {
		err = d.write8(FuncBackplane, SDIO_BACKPLANE_ADDRESS_MID, uint8(addr>>16))
	}
	if err == nil && (addr&0x0000ff00) != currentWindow&0x0000ff00 {
		err = d.write8(FuncBackplane, SDIO_BACKPLANE_ADDRESS_LOW, uint8(addr>>8))
	}

	if err != nil {
		d.backplaneWindow = 0xaaaa_aaaa
		return err
	}
	d.backplaneWindow = addr
	return nil
}

func (d *Device) write32(fn Function, addr, val uint32) error {
	return d.writen(fn, addr, val, 4)
}
func (d *Device) read32(fn Function, addr uint32) (uint32, error) {
	return d.readn(fn, addr, 4)
}
func (d *Device) read16(fn Function, addr uint32) (uint16, error) {
	v, err := d.readn(fn, addr, 2)
	return uint16(v), err
}
func (d *Device) read8(fn Function, addr uint32) (uint8, error) {
	v, err := d.readn(fn, addr, 1)
	return uint8(v), err
}
func (d *Device) write16(fn Function, addr uint32, val uint16) error {
	return d.writen(fn, addr, uint32(val), 2)
}
func (d *Device) write8(fn Function, addr uint32, val uint8) error {
	return d.writen(fn, addr, uint32(val), 1)
}

// writen is primitive SPI write function for <= 4 byte writes.
func (d *Device) writen(fn Function, addr, val, size uint32) (err error) {
	cmd := cmd_word(true, true, fn, addr, size)
	d.rwBuf = [2]uint32{val, 0}
	_, err = d.spi.cmd_write(cmd, d.rwBuf[:1])
	d.lastStatusGet = time.Now()
	return err
}

// readn is primitive SPI read function for <= 4 byte reads.
func (d *Device) readn(fn Function, addr, size uint32) (result uint32, err error) {
	cmd := cmd_word(false, true, fn, addr, size)
	buf := d.rwBuf[:]
	var padding uint32
	if fn == FuncBackplane {
		padding = 1
	}
	_, err = d.spi.cmd_read(cmd, buf[:1+padding])
	d.lastStatusGet = time.Now()
	return buf[padding], err
}

func (d *Device) read32_swapped(addr uint32) uint32 {
	cmd := cmd_word(false, true, FuncBus, addr, 4)
	cmd = swap16(cmd)
	buf := d.rwBuf[:1]
	d.spi.cmd_read(cmd, buf)
	return swap16(buf[0])
}
func (d *Device) write32_swapped(addr uint32, value uint32) {
	cmd := cmd_word(true, true, FuncBus, addr, 4)
	d.rwBuf = [2]uint32{swap16(value), 0}
	d.spi.cmd_write(swap16(cmd), d.rwBuf[:1])
}

func u32AsU8(buf []uint32) []byte {
	return unsafeAsSlice[uint32, byte](buf)
}

func u32PtrTo4U8(buf *uint32) *[4]byte {
	return (*[4]byte)(unsafe.Pointer(buf))
}

func unsafeAs[F, T constraints.Unsigned](ptr *F) *T {
	if unsafe.Alignof(F(0)) < unsafe.Alignof(T(0)) {
		panic("unsafeAs: F alignment < T alignment")
	}
	return (*T)(unsafe.Pointer(ptr))
}

// unsafeAsSlice converts a slice of F to a slice of T.
func unsafeAsSlice[F, T constraints.Unsigned](buf []F) []T {
	fSize := unsafe.Sizeof(F(0))
	tSize := unsafe.Sizeof(T(0))
	ptr := unsafe.Pointer(&buf[0])
	if fSize > tSize {
		// Common case, i.e: uint32->byte
		return unsafe.Slice((*T)(ptr), len(buf)*int(fSize/tSize))
	}
	div := int(tSize / fSize)
	if uintptr(ptr)%tSize != 0 {
		panic("unaligned pointer")
	}
	// i.e: byte->uint32, expands slice.
	return unsafe.Slice((*T)(ptr), align(uint32(len(buf)/div), uint32(div)))
}

//go:inline
func cmd_word(write, autoInc bool, fn Function, addr uint32, sz uint32) uint32 {
	return b2u32(write)<<31 | b2u32(autoInc)<<30 | uint32(fn)<<28 | (addr&0x1ffff)<<11 | sz
}
