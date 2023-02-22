package cyw43439

import (
	"device"
	"encoding/binary"
	"errors"
	"machine"
)

// cy_io.go contains low level functions for reading and writing to the
// CYW43439's gSPI interface. These map to functions readily found in the datasheet.

var ErrDataNotAvailable = errors.New("requested data not available")

// SPIWriteRead performs the gSPI Write-Read action.
func (d *Dev) SPIWriteRead(cmd uint32, w, r []byte) error {
	d.csLow()
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	d.spi.Transfer(byte(cmd >> 24))
	d.spi.Transfer(byte(cmd >> 16))
	d.spi.Transfer(byte(cmd >> 8))
	d.spi.Transfer(byte(cmd))
	for _, v := range w {
		d.spi.Transfer(v)
	}
	d.responseDelay()
	for i := range r {
		r[i], _ = d.spi.Transfer(0)
	}
	// Read Status.
	b0, _ := d.spi.Transfer(0)
	b1, _ := d.spi.Transfer(0)
	b2, _ := d.spi.Transfer(0)
	b3, _ := d.spi.Transfer(0)
	d.csHigh()
	status := Status(b0)<<24 | Status(b1)<<16 | Status(b2)<<8 | Status(b3)
	status = Status(swap32(uint32(status)))
	if !status.IsDataAvailable() {
		println("got status:", status)
		return ErrDataNotAvailable
	}
	return nil
}

func (d *Dev) Write32S(fn Function, addr, val uint32) error {
	cmd := make_cmd(true, true, fn, addr, 4)
	if fn == FuncBackplane {
		d.lastSize = 8
		d.lastHeader[0] = cmd
		d.lastHeader[1] = val
		d.lastBackplaneWindow = d.currentBackplaneWindow
	}
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], swap32(val))
	return d.SPIWrite(swap32(cmd), buf[:])
}

func (d *Dev) Write16S(fn Function, addr uint32, val uint16) error {
	cmd := make_cmd(true, true, fn, addr, 2)
	if fn == FuncBackplane {
		d.lastSize = 8
		d.lastHeader[0] = cmd
		d.lastHeader[1] = uint32(val)
		d.lastBackplaneWindow = d.currentBackplaneWindow
	}
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], val)
	return d.SPIWrite(swap32(cmd), buf[:])
}

func (d *Dev) Write8S(fn Function, addr uint32, val uint8) error {
	cmd := make_cmd(true, true, fn, addr, 1)
	if fn == FuncBackplane {
		d.lastSize = 8
		d.lastHeader[0] = cmd
		d.lastHeader[1] = uint32(val)
		d.lastBackplaneWindow = d.currentBackplaneWindow
	}
	var buf [2]byte
	buf[1] = val
	return d.SPIWrite(swap32(cmd), buf[:])
}

func (d *Dev) Write32(fn Function, addr, val uint32) error {
	cmd := make_cmd(true, true, fn, addr, 4)
	if fn == FuncBackplane {
		d.lastSize = 8
		d.lastHeader[0] = cmd
		d.lastHeader[1] = val
		d.lastBackplaneWindow = d.currentBackplaneWindow
	}
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], val)
	return d.SPIWrite(cmd, buf[:])
}

func (d *Dev) Write16(fn Function, addr uint32, val uint16) error {
	cmd := make_cmd(true, true, fn, addr, 2)
	if fn == FuncBackplane {
		d.lastSize = 8
		d.lastHeader[0] = cmd
		d.lastHeader[1] = uint32(val)
		d.lastBackplaneWindow = d.currentBackplaneWindow
	}
	var buf [4]byte
	binary.BigEndian.PutUint16(buf[2:], val)
	return d.SPIWrite(cmd, buf[:])
}

func (d *Dev) Write8(fn Function, addr uint32, val uint8) error {
	cmd := make_cmd(true, true, fn, addr, 1)
	if fn == FuncBackplane {
		d.lastSize = 8
		d.lastHeader[0] = cmd
		d.lastHeader[1] = uint32(val)
		d.lastBackplaneWindow = d.currentBackplaneWindow
	}
	var buf [4]byte
	buf[3] = val
	return d.SPIWrite(cmd, buf[:])
}

// SPIWrite performs the gSPI Write action.
func (d *Dev) SPIWrite(cmd uint32, w []byte) error {
	d.csLow()
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	d.spi.Transfer(byte(cmd >> 24))
	d.spi.Transfer(byte(cmd >> 16))
	d.spi.Transfer(byte(cmd >> 8))
	d.spi.Transfer(byte(cmd))
	for _, v := range w {
		d.spi.Transfer(v)
	}
	// Read Status.
	b0, _ := d.spi.Transfer(0)
	b1, _ := d.spi.Transfer(0)
	b2, _ := d.spi.Transfer(0)
	b3, _ := d.spi.Transfer(0)
	d.csHigh()
	status := Status(b0)<<24 | Status(b1)<<16 | Status(b2)<<8 | Status(b3)
	status = Status(swap32(uint32(status)))
	if !status.IsDataAvailable() {
		println("got status:", status)
		return ErrDataNotAvailable
	}
	return nil
}

func (d *Dev) Read32S(fn Function, addr uint32) (uint32, error) {
	v, err := d.rrS(fn, addr, 4)
	return v, err
}

func (d *Dev) Read32(fn Function, addr uint32) (uint32, error) {
	v, err := d.rr(fn, addr, 4)
	return v, err
}

func (d *Dev) Read16S(fn Function, addr uint32) (uint16, error) {
	v, err := d.rrS(fn, addr, 2)
	return uint16(v), err
}

func (d *Dev) Read16(fn Function, addr uint32) (uint16, error) {
	v, err := d.rr(fn, addr, 2)
	return uint16(v), err
}

func (d *Dev) Read8(fn Function, addr uint32) (uint8, error) {
	v, err := d.rr(fn, addr, 1)
	return uint8(v), err
}

// rrS reads register.
func (d *Dev) rr(fn Function, addr, size uint32) (uint32, error) {
	var padding uint32
	if fn == FuncBackplane {
		padding = whdBusSPIBackplaneReadPadding
	}
	cmd := make_cmd(false, true, fn, addr, size+padding)
	var buf [4 + whdBusSPIBackplaneReadPadding]byte
	d.SPIRead(cmd, buf[:4+padding])
	return binary.BigEndian.Uint32(buf[:4]), nil
}

// rrS reads register and swaps
func (d *Dev) rrS(fn Function, addr, size uint32) (uint32, error) {
	var padding uint32
	if fn == FuncBackplane {
		padding = whdBusSPIBackplaneReadPadding
	}
	cmd := make_cmd(false, true, fn, addr, size+padding)
	var buf [4 + whdBusSPIBackplaneReadPadding]byte
	d.SPIRead(swap32(cmd), buf[:4+padding])
	return swap32(binary.BigEndian.Uint32(buf[:4])), nil
}

// SPIRead performs the gSPI Read action.
func (d *Dev) SPIRead(cmd uint32, r []byte) error {
	d.csLow()
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	d.spi.Transfer(byte(cmd >> 24))
	d.spi.Transfer(byte(cmd >> 16))
	d.spi.Transfer(byte(cmd >> 8))
	d.spi.Transfer(byte(cmd))

	d.csPeak()
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	d.responseDelay()
	d.spi.Tx(nil, r)
	// Read Status.
	b0, _ := d.spi.Transfer(0)
	b1, _ := d.spi.Transfer(0)
	b2, _ := d.spi.Transfer(0)
	b3, _ := d.spi.Transfer(0)
	d.csHigh()
	status := Status(swap32(uint32(b0)<<24 | uint32(b1)<<16 | uint32(b2)<<8 | uint32(b3)))
	if !status.IsDataAvailable() {
		println("got data unavailable status:", status)
	}
	return nil
}

//go:inline
func (d *Dev) csHigh() {
	d.cs.High()
	machine.GPIO1.High()
}

//go:inline
func (d *Dev) csLow() {
	d.cs.Low()
	machine.GPIO1.Low()
}

//go:inline
func (d *Dev) responseDelay() {
	// Wait for response.
	for i := uint8(0); i < d.ResponseDelayByteCount; i++ {
		d.spi.Transfer(0)
	}
}

//go:inline
func (d *Dev) csPeak() {
	d.csHigh()
	for i := 0; i < 40; i++ {
		device.Asm("nop")
	}
	d.csLow()
}

//go:inline
func make_cmd(write, inc bool, fn Function, addr uint32, sz uint32) uint32 {
	return b2u32(write)<<31 | b2u32(inc)<<30 | uint32(fn)<<28 | (addr&0x1ffff)<<11 | sz
}

//go:inline
func b2u32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// swap32 swaps lowest 16 bits with highest 16 bits of a uint32.
func swap32(b uint32) uint32 {
	return (b >> 16) | (b << 16)
}
