//go:build cy43nopio

package main

// NOTE: This program must be compiled with build flag: cy43nopio
// tinygo flash -target=pico -tags=cy43nopio examples/cyw43439-signal-analyze

import (
	"machine"
	"time"

	"github.com/soypat/cyw43439"
)

// This program declares the SPI bus as a software bit-bang implementation
// which also broadcasts the CYW43439 responses to the the mock output pins
// so they can be analyzed by a logic analyzer, such as a Saleae or Digital Analog Discovery devices.

// Raspberry Pi Pico W pin definitions for the CY43439.
const (
	// IRQ       = machine.GPIO24 // AKA WL_HOST_WAKE
	WL_REG_ON = machine.GPIO23
	DATA_OUT  = machine.GPIO24
	DATA_IN   = DATA_OUT
	CLK       = machine.GPIO29
	CS        = machine.GPIO25

	// Mock pins can be any not shared by original implementation.
	// Attach your logic analyzer to these.
	MOCK_DAT = machine.GPIO4
	MOCK_CLK = machine.GPIO5
	MOCK_CS  = machine.GPIO6
)

var bus = &SPIbb{
	SCK:   CLK,
	SDI:   DATA_IN,
	SDO:   DATA_OUT,
	Delay: 1,
	MockTo: &SPIbb{
		SCK: MOCK_CLK,
		SDI: MOCK_DAT,
		SDO: MOCK_DAT,
	},
}

func main() {
	bus.Configure()
	WL_REG_ON.Configure(machine.PinConfig{Mode: machine.PinOutput})
	CS.Configure(machine.PinConfig{Mode: machine.PinOutput})
	cs := func(b bool) {
		CS.Set(b)
		MOCK_CS.Set(b)
	}
	dev := cyw43439.New(WL_REG_ON.Set, cs, bus)
	err := dev.Init(cyw43439.DefaultWifiConfig())
	if err != nil {
		panic("cyw43 init error: " + err.Error())
	}

	// Frequency of LED in Hertz.
	const ledFreq = 2
	for {
		dev.GPIOSet(0, true)
		time.Sleep(time.Second / ledFreq)
		dev.GPIOSet(0, false)
		time.Sleep(time.Second / ledFreq)
	}
}
