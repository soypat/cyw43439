package main

import (
	"time"

	cyw43439 "github.com/soypat/cyw43439"
)

func main() {
	dev := cyw43439.NewPicoWDevice()
	cfg := cyw43439.DefaultWifiConfig()
	// cfg.Logger = logger // Uncomment to see in depth info on wifi device functioning.
	err := dev.Init(cfg)
	if err != nil {
		panic(err)
	}
	for {
		dev.GPIOSet(0, true)
		time.Sleep(500 * time.Millisecond)
		dev.GPIOSet(0, false)
		time.Sleep(500 * time.Millisecond)
	}
}
