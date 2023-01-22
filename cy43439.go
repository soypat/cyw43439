package cyw43439

import (
	"machine"

	"tinygo.org/x/drivers"
)

func PicoWSpi() (spi *machine.SPI, cs machine.Pin) {
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

func (d *Dev) csSet(b bool) { d.cs.Set(b) }
