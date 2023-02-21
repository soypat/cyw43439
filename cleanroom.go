package cyw43439

import "device"

func (d *Dev) ReadRegister16Swap(fn Function, addr uint32) (uint16, error) {
	cmd := make_cmd(false, true, fn, addr, 2)
	d.cs.Low()
	d.spi.Transfer(byte(cmd >> 8))
	d.spi.Transfer(byte(cmd))

	d.spi.Transfer(byte(cmd >> 24))
	d.spi.Transfer(byte(cmd >> 16))

	d.csPeak()
	d.responseDelay()
	b1, _ := d.spi.Transfer(0)
	b2, _ := d.spi.Transfer(0)
	d.cs.High()
	return (uint16(b1) << 8) | uint16(b2), nil
}

func (d *Dev) ReadRegister16(fn Function, addr uint32) (uint16, error) {
	cmd := make_cmd(false, true, fn, addr, 2)
	d.cs.Low()
	d.spi.Transfer(byte(cmd >> 24))
	d.spi.Transfer(byte(cmd >> 16))
	d.spi.Transfer(byte(cmd >> 8))
	d.spi.Transfer(byte(cmd))
	d.csPeak()
	d.responseDelay()
	b1, _ := d.spi.Transfer(0)
	b2, _ := d.spi.Transfer(0)
	d.cs.High()
	return (uint16(b1) << 8) | uint16(b2), nil
}

//go:inline
func (d *Dev) csPeak() {
	d.cs.High()
	for i := 0; i < 10; i++ {
		device.Asm("nop")
	}
	d.cs.Low()
}
