package cyw43439

import (
	"encoding/binary"
	"machine"

	"tinygo.org/x/drivers"
)

func PicoWSpi() (spi drivers.SPI, cs machine.Pin) {
	// Raspberry Pi Pico W pin definitions for the CY43439.
	const (
		WL_REG_ON = machine.GPIO23
		DATA_OUT  = machine.GPIO24
		DATA_IN   = machine.GPIO24
		IRQ       = machine.GPIO24
		CLK       = machine.GPIO29
		CS        = machine.GPIO25
	)
	// Need software spi implementation since Rx/Tx are on same pin.
	cs = CS
	cs.Configure(machine.PinConfig{Mode: machine.PinOutput})
	spi = &bbSPI{
		SCK:   CLK,
		SDI:   DATA_IN,
		SDO:   DATA_OUT,
		Delay: 1 << 10,
	}
	return spi, cs
}

type Dev struct {
	spi drivers.SPI
	// Chip select pin. Driven LOW during SPI transaction.
	cs machine.Pin
}

func (d *Dev) Init() error {
	d.cs.High()
	return nil
}

func (d *Dev) SPIExchange(w, r []byte) (int, error) {
	d.cs.Low()
	err := d.spi.Tx(w, nil)
	if err != nil {
		return 0, err
	}
}

func (d *Dev) ReadReg(fn, reg uint32, size int) (uint32, error) {
	var buf [4 * 2]byte
	binary.BigEndian.PutUint32(buf[:4], make_cmd(false, true, fn, reg, 4))

}

func make_cmd(write, inc bool, fn uint32, addr uint32, sz uint32) uint32 {
	return b2i(write)<<31 | b2i(inc)<<30 | fn<<28 | (addr&0x1ffff)<<11 | sz
}

func b2i(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}
