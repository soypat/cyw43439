package main

import (
	"machine"
	"time"

	cyw43439 "github.com/soypat/cy43439"
)

const (
	mockSDI = machine.GPIO4
	mockCS  = machine.GPIO1
	mockSCK = machine.GPIO2
	mockSDO = machine.GPIO3
)

func main() {
	cyw43439.Debug("preparing to write...")
	time.Sleep(time.Second)
	spi, cs, wlreg, irq := cyw43439.PicoWSpi(0)
	spi.MockTo = &cyw43439.SPIbb{
		SCK:   mockSCK,
		SDI:   mockSDI,
		SDO:   mockSDO,
		Delay: 10,
	}
	spi.Configure()

	dev := cyw43439.NewDev(spi, cs, wlreg, irq, irq)
	dev.Init(cyw43439.DefaultConfig(false))
	time.Sleep(time.Second)
	dev.Write8(cyw43439.FuncBackplane, cyw43439.SDIO_CHIP_CLOCK_CSR, 0)

	cyw43439.Debug("done writing...")
	time.Sleep(time.Second)
}
