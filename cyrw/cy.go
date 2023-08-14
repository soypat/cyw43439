package cyrw

import (
	"encoding/binary"
	"errors"
	"sync"

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

func (d *Device) csEnable(b bool) {
	d.cs(!b)
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

// swap32 swaps lowest 16 bits with highest 16 bits of a uint32.
//
//go:inline
func swap32(b uint32) uint32 {
	return (b >> 16) | (b << 16)
}
