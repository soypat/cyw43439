package cyw43439

import (
	"machine"
	"time"

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

const (
	responseDelay                 = 20 * time.Microsecond
	backplaneFunction             = 0
	whdBusSPIBackplaneReadPadding = 4
)

func (d *Dev) ReadReg(fn, reg uint32, size int) (uint32, error) {
	var padding uint32
	if fn == backplaneFunction {
		padding = whdBusSPIBackplaneReadPadding
	}
	cmd := make_cmd(false, true, fn, reg, uint32(size)+padding)
	var buf [4]byte

}

func (d *Dev) SPIWriteRead(command uint32, r []byte) error {
	d.cs.Low()
	err := d.spiWrite(command, nil)
	if err != nil {
		return err
	}
	d.responseDelay()
	err = d.spi.Tx(nil, r)
	d.cs.High()
	return err
}

func (d *Dev) SPIRead(command uint32, r []byte) error {
	d.cs.Low()
	err := d.spiWrite(command, nil)
	d.cs.High()
	if err != nil {
		return err
	}
	d.cs.Low()
	d.responseDelay()
	err = d.spi.Tx(nil, r)
	d.cs.High()
	return err
}

func (d *Dev) SPIWrite(command uint32, w []byte) error {
	d.cs.Low()
	err := d.spiWrite(command, w)
	d.cs.High()
	return err
}

func (d *Dev) spiWrite(command uint32, w []byte) error {
	d.spi.Transfer(byte(command >> (32 - 8)))
	d.spi.Transfer(byte(command >> (32 - 16)))
	d.spi.Transfer(byte(command >> (32 - 24)))
	_, err := d.spi.Transfer(byte(command))
	if len(w) == 0 || err != nil {
		return err
	}
	err = d.spi.Tx(w, nil)
	return err
}

func (d *Dev) responseDelay() {
	// Wait for response.
	waitStart := time.Now()
	for time.Since(waitStart) < responseDelay {
		d.spi.Transfer(0)
	}
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
