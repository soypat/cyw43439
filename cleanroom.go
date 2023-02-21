package cyw43439

import (
	"device"
	"encoding/binary"
	"errors"
	"machine"
)

var ErrDataNotAvailable = errors.New("requested data not available")

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
	return d.SPIWriteV2(swap32(cmd), buf[:])
}

//go:inline
func (d *Dev) SPIWriteV2(cmd uint32, w []byte) error {
	d.cs.Low()
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
	d.cs.High()
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

// rrS reads register.
func (d *Dev) rr(fn Function, addr, size uint32) (uint32, error) {
	var padding uint32
	if fn == FuncBackplane {
		padding = whdBusSPIBackplaneReadPadding
	}
	cmd := make_cmd(false, true, fn, addr, size+padding)
	var buf [4 + whdBusSPIBackplaneReadPadding]byte
	d.SPIReadV2(cmd, buf[:4+padding])
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
	d.SPIReadV2(swap32(cmd), buf[:4+padding])
	return swap32(binary.BigEndian.Uint32(buf[:4])), nil
}

func (d *Dev) SPIReadV2(cmd uint32, r []byte) {
	d.cs.Low()
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
	d.cs.High()
	status := Status(swap32(uint32(b0)<<24 | uint32(b1)<<16 | uint32(b2)<<8 | uint32(b3)))
	if !status.IsDataAvailable() {
		println("got data unavailable status:", status)
	}
}

//go:inline
func (d *Dev) csPeak() {
	d.cs.High()
	for i := 0; i < 40; i++ {
		device.Asm("nop")
	}
	d.cs.Low()
}
