//go:build tinygo && (rp2040 || rp2350)

// package blink is a convenience package for users
// who only want to blink the Raspberry Pi Pico LED
// without WIFI/bluetooth usage.
package blink

import (
	"sync"
	"time"

	"github.com/soypat/cyw43439"
)

type led struct {
	once sync.Once
	dev  *cyw43439.Device
}

var LED = new(led)

func (led *led) High() {
	led.Set(true)
}

func (led *led) Low() {
	led.Set(false)
}

func (led *led) Set(b bool) {
	led.once.Do(func() {
		led.dev = cyw43439.NewPicoWDevice()
		cfg := cyw43439.DefaultWifiConfig()
		// cfg.Logger = logger // Uncomment to see in depth info on wifi device functioning.
		err := led.dev.Init(cfg)
		if err != nil {
			for {
				println("LED initialization failed", err.Error())
				time.Sleep(time.Second)
			}
		}
	})
	err := led.dev.GPIOSet(0, b)
	if err != nil {
		println("failed setting LED:", err.Error())
	}
}
