package main

import (
	"time"

	"github.com/soypat/cyw43439"
)

func main() {
	// Wait for USB to initialize:
	time.Sleep(time.Second)
	dev := cyw43439.NewPicoWDevice()
	cfg := cyw43439.DefaultWifiConfig()
	// cfg.Logger = logger // Uncomment to see in depth info on wifi device functioning.
	err := dev.Init(cfg)
	if err != nil {
		panic(err)
	}
	for {
		dev.GPIOSet(0, true)
		println("LED ON")
		time.Sleep(500 * time.Millisecond)
		dev.GPIOSet(0, false)
		println("LED OFF")
		time.Sleep(500 * time.Millisecond)
	}
}
