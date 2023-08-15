package cyrw

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/soypat/cyw43439/whd"
	"tinygo.org/x/drivers"
)

type OutputPin func(bool)

// type OutputPin func(bool)
type Device struct {
	mu              sync.Mutex
	pwr             OutputPin
	cs              OutputPin
	busStatus       uint32
	backplaneWindow uint32
	spi             drivers.SPI
	// Low level SPI buffers for readn and writen.
	spiBuf    [4 + whd.BUS_SPI_BACKPLANE_READ_PADD_SIZE]byte
	spicmdBuf [4]byte
}

func New(pwr, cs OutputPin, spi drivers.SPI) *Device {
	d := &Device{
		pwr: pwr,
		cs:  cs,
		spi: spi,
	}
	return d
}

type Config struct{}

func hex32(u uint32) string {
	return hex.EncodeToString([]byte{byte(u >> 24), byte(u >> 16), byte(u >> 8), byte(u)})
}

func (d *Device) Init(cfg Config) error {
	d.Reset()
	for {
		got := d.read32_swapped(whd.SPI_READ_TEST_REGISTER)
		if got == whd.TEST_PATTERN {
			break
		}
		println(got)
	}

	const spiRegTestRW = 0x18
	d.write32_swapped(spiRegTestRW, whd.TEST_PATTERN)
	got := d.read32_swapped(spiRegTestRW)
	if got != whd.TEST_PATTERN {
		return errors.New("spi test failed:" + hex32(got))
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
		setupValue = (1 << WordLengthPos) | (1 << HiSpeedModePos) | (1 << EndianessBigPos) |
			(1 << InterruptPolPos) | (1 << WakeUpPos) | (0x4 << ResponseDelayPos) |
			(1 << InterruptWithStatusPos) // | (1 << StatusEnablePos)
	)
	d.write32_swapped(whd.SPI_BUS_CONTROL, setupValue)
	got, err := d.read32(FuncBus, whd.SPI_READ_TEST_REGISTER)
	if err != nil || got != whd.TEST_PATTERN {
		return errors.Join(errors.New("spi RO test failed:"+hex32(got)), err)
	}

	d.write32(FuncBus, spiRegTestRW, ^uint32(whd.TEST_PATTERN))
	got, err = d.read32(FuncBus, spiRegTestRW)
	if err != nil || got != ^uint32(whd.TEST_PATTERN) {
		return errors.Join(errors.New("spi RW test failed:"+hex32(got)), err)
	}
	return nil
}

func (d *Device) Reset() {
	d.pwr(false)
	time.Sleep(20 * time.Millisecond)
	d.pwr(true)
	time.Sleep(250 * time.Millisecond)
}

func (d *Device) wlan_read(dst []byte) error {
	return d.readbytes(FuncWLAN, 0, dst)
}

func (d *Device) wlan_write(data []byte) error {
	return d.writebytes(FuncWLAN, 0, data)
}

func (d *Device) bp_read(addr uint32, dst []byte) error {
	return d.readbytes(FuncBackplane, addr, dst)
}

func (d *Device) bp_write(addr uint32, data []byte) error {
	return d.writebytes(FuncBackplane, addr, data)
}

func (d *Device) writebytes(fn Function, addr uint32, data []byte) error {
	length := uint32(len(data))
	alignedLength := align(length, 4)
	assert := alignedLength > 0 && alignedLength <= 2040 && (fn != FuncBackplane || (length <= 64 && (addr+length) <= 0x8000))
	if !assert {
		return errors.New("bad argument to writebytes")
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
		data = data[:alignedLength]
	}
	cmd := cmd_word(true, true, fn, addr, length)
	d.csEnable(true)
	err := d.spiWrite(cmd, data)
	d.csEnable(false)
	return err
}

func (d *Device) readbytes(fn Function, addr uint32, dst []byte) error {
	const maxReadPacket = 2040
	length := uint32(len(dst))
	alignedLen := align(length, 4)
	if alignedLen > maxReadPacket && alignedLen > 0 {
		return errors.New("buffer length must be length in 4..2040")
	}
	assert := fn != FuncBackplane || (length <= 64 && (addr+length) <= 0x8000)
	if !assert {
		return errors.New("bad argument to readbytes")
	}
	var padding uint8
	if fn == FuncBackplane {
		padding = 4
	}
	cmd := cmd_word(false, true, fn, addr, length+uint32(padding))
	d.csEnable(true)
	err := d.spiRead(cmd, dst, padding)
	d.csEnable(false)
	return err
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
		return nil // early return.
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
		d.backplaneWindow = 0
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
	d.csEnable(true)
	binary.LittleEndian.PutUint32(d.spiBuf[:], val)

	err = d.spiWrite(cmd, d.spiBuf[:size])

	d.csEnable(false)
	return err
}

// readn is primitive SPI read function for <= 4 byte reads.
func (d *Device) readn(fn Function, addr, size uint32) (result uint32, err error) {
	var padding uint32
	if fn == FuncBackplane {
		padding = whd.BUS_SPI_BACKPLANE_READ_PADD_SIZE
	}
	cmd := cmd_word(false, true, fn, addr, size+padding)
	d.csEnable(true)

	err = d.spiRead(cmd, d.spiBuf[:4+padding], 0)

	d.csEnable(false)
	return binary.LittleEndian.Uint32(d.spiBuf[padding : 4+padding]), err
}

//go:inline
func (d *Device) responseDelay(padding uint8) {
	// Wait for response.
	for i := uint8(0); i < padding; i++ {
		d.spi.Transfer(0)
	}
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

func (d *Device) read32_swapped(addr uint32) uint32 {
	cmd := cmd_word(false, true, FuncBus, addr, 4)
	cmd = swap16be(cmd)
	d.csEnable(true)
	d.spiRead(cmd, d.spiBuf[:4], 0)
	d.csEnable(false)
	return swap16(binary.BigEndian.Uint32(d.spiBuf[:4]))
}
func (d *Device) write32_swapped(addr uint32, value uint32) {
	cmd := cmd_word(true, true, FuncBus, addr, 4)
	cmd = swap16be(cmd)
	value = swap16(value)
	binary.BigEndian.PutUint32(d.spiBuf[:], value)
	d.csEnable(true)
	d.spiWrite(cmd, d.spiBuf[:4])
	d.csEnable(false)
}
