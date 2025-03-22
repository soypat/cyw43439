//go:build !cy43nopio && (rp2040 || rp2350)

package cyw43439

import (
	"encoding/binary"
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

var _busOrder = binary.LittleEndian

type cmdBus struct {
	piolib.SPI3w
}

func NewPicoWCmdBus(baud uint32) (cmdBus, error) {
	const (
		DATA_OUT = machine.GPIO24
		DATA_IN  = DATA_OUT
		CLK      = machine.GPIO29
	)
	sm, err := pio.PIO0.ClaimStateMachine()
	if err != nil {
		panic(err.Error())
	}
	spi, err := piolib.NewSPI3w(sm, DATA_IN, CLK, baud)
	if err != nil {
		panic(err.Error())
	}
	spi.EnableStatus(true)
	err = spi.EnableDMA(true)
	if err != nil {
		panic(err.Error())
	}
	return cmdBus{*spi}, nil
}

func NewPicoWDevice() *Device {
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
	cmd, err := NewPicoWCmdBus(25000_000 - 1)
	if err != nil {
		panic(err)
	}
	return New(WL_REG_ON.Set, CS.Set, cmd)
}
