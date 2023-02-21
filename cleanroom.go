package cyw43439

import (
	"device"
	"encoding/binary"
	"machine"
)

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
	return binary.LittleEndian.Uint32(buf[:4]), nil
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
	return swap32(binary.LittleEndian.Uint32(buf[:4])), nil
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
	for i := range r {
		r[i], _ = d.spi.Transfer(0)
	}
	d.cs.High()
}

//go:inline
func (d *Dev) csPeak() {
	d.cs.High()
	for i := 0; i < 10; i++ {
		device.Asm("nop")
	}
	d.cs.Low()
}
