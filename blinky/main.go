package main

import (
	"time"

	cyw43439 "github.com/soypat/cyw43439"
)

func main() {
	const spiNOPs = 0
	spi, cs, WL_REG_ON, irq := cyw43439.PicoWSpi(spiNOPs)

	dev := cyw43439.NewDevice(spi, cs, WL_REG_ON, irq, irq)
	err := dev.Init(cyw43439.DefaultConfig(false))
	if err != nil {
		panic(err.Error())
	}
	led := dev.LED()
	for {
		println("LED high")
		led.High()
		time.Sleep(time.Second)
		println("LED low")
		led.Low()
		time.Sleep(time.Second)
	}
}
