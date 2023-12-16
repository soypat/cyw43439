//go:build pico && !cy43nopio

package cyw43439

import (
	"encoding/binary"
	"log/slog"
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
	"golang.org/x/exp/constraints"
)

var _busOrder = binary.LittleEndian

type spibus struct {
	cs  outputPin
	spi piolib.SPI3w
}

func NewPicoWDevice(logger *slog.Logger) *Device {
	// Raspberry Pi Pico W pin definitions for the CY43439.
	const (
		// IRQ       = machine.GPIO24 // AKA WL_HOST_WAKE
		WL_REG_ON = machine.GPIO23
		DATA_OUT  = machine.GPIO24
		DATA_IN   = DATA_OUT
		CLK       = machine.GPIO29
		CS        = machine.GPIO25
	)
	WL_REG_ON.Configure(machine.PinConfig{Mode: machine.PinOutput})
	CS.Configure(machine.PinConfig{Mode: machine.PinOutput})
	CS.High()
	sm, err := pio.PIO0.ClaimStateMachine()
	if err != nil {
		panic(err.Error())
	}
	spi, err := piolib.NewSPI3w(sm, DATA_IN, CLK, 25000_000-1)
	if err != nil {
		panic(err.Error())
	}
	spi.EnableStatus(true)
	err = spi.EnableDMA(true)
	if err != nil {
		panic(err.Error())
	}
	return New(WL_REG_ON.Set, CS.Set, spibus{
		cs:  CS.Set,
		spi: *spi,
	}, logger)
}

func (d *spibus) cmd_read(cmd uint32, buf []uint32) (status uint32, err error) {
	d.csEnable(true)
	err = d.spi.CmdRead(cmd, buf)
	d.csEnable(false)
	return d.spi.LastStatus(), err
}

func (d *spibus) cmd_write(cmd uint32, buf []uint32) (status uint32, err error) {
	// TODO(soypat): add cmd as argument and remove copies elsewhere?
	d.csEnable(true)
	err = d.spi.CmdWrite(cmd, buf)
	d.csEnable(false)
	return d.spi.LastStatus(), err
}

func (d *spibus) csEnable(b bool) {
	d.cs(!b)
	// machine.GPIO1.Set(!b) // When mocking.
}

func (d *spibus) Status() Status {
	return Status(d.spi.LastStatus())
}

// align rounds `val` up to nearest multiple of `align`.
func align[T constraints.Unsigned](val, align T) T {
	return (val + align - 1) &^ (align - 1)
}
