package cyrw

import (
	"encoding/hex"
	"errors"
	"reflect"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/cyw43439/whd"
	"golang.org/x/exp/constraints"
	"tinygo.org/x/drivers"
)

type OutputPin func(bool)

func DefaultConfig() Config {
	return Config{
		Firmware: wifiFW2,
	}
}

// type OutputPin func(bool)
type Device struct {
	mu              sync.Mutex
	pwr             OutputPin
	lastStatusGet   time.Time
	spi             spibus
	log             logstate
	backplaneWindow uint32
	ioctlID         uint16
	sdpcmSeq        uint8
	// uint32 buffers to ensure alignment of buffers.
	rwBuf         [2]uint32        // rwBuf used for read* and write* functions.
	_sendIoctlBuf [2048 / 4]uint32 // _sendIoctlBuf used only in sendIoctl and tx.
	_iovarBuf     [2048 / 4]uint32
	// We define headers in the Device struct to alleviate stack growth. Also used along with _sendIoctlBuf
	auxSDPCMHeader whd.SDPCMHeader
	auxCDCHeader   whd.CDCHeader
	auxBDCHeader   whd.BDCHeader
}

func New(pwr, cs OutputPin, spi drivers.SPI) *Device {
	d := &Device{
		pwr: pwr,
		spi: spibus{
			spi: spi,
			cs:  cs,
		},
	}
	return d
}

type Config struct {
	Firmware string
}

func hex32(u uint32) string {
	return hex.EncodeToString([]byte{byte(u >> 24), byte(u >> 16), byte(u >> 8), byte(u)})
}

func (d *Device) Init(cfg Config) (err error) {
	// Reference: https://github.com/embassy-rs/embassy/blob/6babd5752e439b234151104d8d20bae32e41d714/cyw43/src/runner.rs#L76
	err = d.initBus()
	if err != nil {
		return errjoin(errors.New("failed to init bus"), err)
	}
	d.backplaneWindow = 0xaaaa_aaaa
	d.write8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR, 0x08) // BACKPLANE_ALP_AVAIL_REQ
	for {
		got, _ := d.read8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR)
		if got&0x40 != 0 {
			break // ALP available-> clock OK.
		}
	}

	chip_id, _ := d.bp_read16(0x1800_0000)

	// Upload firmware.
	err = d.core_disable(whd.CORE_WLAN_ARM)
	if err != nil {
		return err
	}
	err = d.core_reset(whd.CORE_SOCRAM, false)
	if err != nil {
		return err
	}
	d.bp_write32(whd.SOCSRAM_BASE_ADDRESS+0x10, 3)
	d.bp_write32(whd.SOCSRAM_BASE_ADDRESS+0x44, 0)

	d.debug("flashing firmware", slog.Uint64("chip_id", uint64(chip_id)), slog.Int("fwlen", len(cfg.Firmware)))
	var ramAddr uint32 // Start at ATCM_RAM_BASE_ADDRESS = 0.
	err = d.bp_writestring(ramAddr, cfg.Firmware)
	if err != nil {
		return err
	}

	// Load NVRAM
	const chipRAMSize = 512 * 1024
	nvramLen := align(uint32(len(nvram43439)), 4)
	d.debug("flashing nvram")
	err = d.bp_writestring(ramAddr+chipRAMSize-4-nvramLen, nvram43439)
	if err != nil {
		return err
	}
	nvramLenWords := nvramLen / 4
	nvramLenMagic := ((^nvramLenWords) << 16) | nvramLenWords
	d.bp_write32(ramAddr+chipRAMSize-4, nvramLenMagic)

	// Start core.
	err = d.core_reset(whd.CORE_WLAN_ARM, false)
	if err != nil {
		return err
	}
	if !d.core_is_up(whd.CORE_WLAN_ARM) {
		return errors.New("core not up after reset")
	}
	d.debug("core up")
	retries := 256
	for {
		got, _ := d.read8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR)
		if got&0x80 != 0 {
			break
		}
		if retries <= 0 {
			return errors.New("timeout waiting for chip clock")
		}
		retries--
	}

	// "Set up the interrupt mask and enable interrupts"
	d.write16(FuncBus, whd.SPI_INTERRUPT_ENABLE_REGISTER, whd.F2_PACKET_AVAILABLE)

	// ""Lower F2 Watermark to avoid DMA Hang in F2 when SD Clock is stopped.""
	// "Sounds scary..."
	// yea it does
	const REG_BACKPLANE_FUNCTION2_WATERMARK = 0x10008
	d.write8(FuncBackplane, REG_BACKPLANE_FUNCTION2_WATERMARK, 32)

	// Wait for wifi startup.
	retries = 1000
	for !d.status().F2RxReady() {
		retries--
		if retries <= 0 {
			return errors.New("wifi startup timeout")
		}
	}

	// Clear pulls.
	d.write8(FuncBackplane, whd.SDIO_PULL_UP, 0)
	d.read8(FuncBackplane, whd.SDIO_PULL_UP)

	d.debug("wifi init done")
	return nil
}

func (d *Device) GPIOSet(wlGPIO uint8, value bool) (err error) {
	d.info("GPIOSet", slog.Uint64("wlGPIO", uint64(wlGPIO)), slog.Bool("value", value))
	if wlGPIO >= 3 {
		return errors.New("gpio out of range")
	}
	val0 := uint32(1) << wlGPIO
	val1 := b2u32(value) << wlGPIO
	return d.set_iovar2("gpioout", whd.WWD_STA_INTERFACE, val0, val1)
}

// status gets gSPI last bus status or reads it from the device if it's stale, for some definition of stale.
func (d *Device) status() Status {
	sinceStat := time.Since(d.lastStatusGet)
	if sinceStat < 12*time.Microsecond {
		runtime.Gosched() // Probably in hot loop.
	} else {
		got, _ := d.read32(FuncBus, whd.SPI_STATUS_REGISTER) // Explicitly get Status.
		return Status(got)
	}
	return d.spi.Status()
}

func (d *Device) Reset() {
	d.pwr(false)
	time.Sleep(20 * time.Millisecond)
	d.pwr(true)
	time.Sleep(250 * time.Millisecond)
}

func (d *Device) wlan_read(buf []uint32, lenInBytes int) (err error) {
	cmd := cmd_word(false, true, FuncWLAN, 0, uint32(lenInBytes))
	lenU32 := (lenInBytes + 3) / 4
	_, err = d.spi.cmd_read(cmd, buf[:lenU32])
	d.lastStatusGet = time.Now()
	return err
}

func (d *Device) wlan_write(data []uint32) (err error) {
	var buf [513]uint32
	cmd := cmd_word(true, true, FuncWLAN, 0, uint32(len(data)*4))
	buf[0] = cmd
	copy(buf[1:], data)
	_, err = d.spi.cmd_write(buf[:len(data)+1])
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
		length := min(min(uint32(len(data)), maxTxSize), windowRemaining)

		err = d.backplane_setwindow(addr)
		if err != nil {
			return err
		}
		cmd := cmd_word(false, true, FuncBackplane, windowOffset, length)

		// round `buf` to word boundary, add one extra word for the response delay byte.
		_, err = d.spi.cmd_read(cmd, buf[:(length+3)/4+1])
		if err != nil {
			return err
		}
		// when writing out the data, we skip the response-delay byte.
		copy(data[:length], buf8[1:])
		addr += length
		data = data[length:]
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
		Cap:  align(hdr.Len, 4), // Round capacity up. Not used yet.
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
	buf8 := unsafeAsSlice[uint32, byte](buf[1:]) // Slice excluding first word reserved for command.
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
		buf[0] = cmd_word(true, true, FuncBackplane, windowOffset, length)

		_, err = d.spi.cmd_write(buf[:(length+3)/4+1])
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
	d.rwBuf = [2]uint32{cmd, val}
	_, err = d.spi.cmd_write(d.rwBuf[:2])
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
	d.rwBuf = [2]uint32{swap16(cmd), swap16(value)}
	d.spi.cmd_write(d.rwBuf[:2])
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

func (d *Device) info(msg string, attrs ...slog.Attr) {
	d.logattrs(slog.LevelInfo, msg, attrs...)
}

func (d *Device) debug(msg string, attrs ...slog.Attr) {
	d.logattrs(slog.LevelDebug, msg, attrs...)
}

func (d *Device) logattrs(level slog.Level, msg string, attrs ...slog.Attr) {
	print(msg)
	for _, a := range attrs {
		print(" ")
		print(a.Key)
		print("=")
		print(a.Value.String())
	}
	println()
	// slog.LogAttrs(context.Background(), slog.LevelDebug, msg, attrs...) // Is segfaulting.
}

//go:inline
func cmd_word(write, autoInc bool, fn Function, addr uint32, sz uint32) uint32 {
	return b2u32(write)<<31 | b2u32(autoInc)<<30 | uint32(fn)<<28 | (addr&0x1ffff)<<11 | sz
}

//go:inline
func b2u32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// swap16 swaps lowest 16 bits with highest 16 bits of a uint32.
//
//go:inline
func swap16(b uint32) uint32 {
	return (b >> 16) | (b << 16)
}

func swap16be(b uint32) uint32 {
	b = swap16(b)
	b0 := b & 0xff
	b1 := (b >> 8) & 0xff
	b2 := (b >> 16) & 0xff
	b3 := (b >> 24) & 0xff
	return b0<<24 | b1<<16 | b2<<8 | b3
}
