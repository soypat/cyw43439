package main

import (
	"time"

	blink "github.com/soypat/cyw43439/examples/pico-blink"
)

func main() {
	led := blink.LED
	led.Configure()
	for {
		println("turn LED on")
		led.High()
		time.Sleep(time.Second)
		println("turn LED off")
		led.Low()
		time.Sleep(time.Second)
	}
}
