package main

import (
	"log/slog"
	"machine"
	"time"

	"github.com/soypat/cyw43439"
)

func main() {
	time.Sleep(time.Second)
	dev := cyw43439.NewPicoWDevice()
	cfg := cyw43439.DefaultBluetoothConfig()
	cfg.Logger = slog.New(slog.NewTextHandler(machine.USBCDC, &slog.HandlerOptions{
		Level: slog.LevelDebug - 2,
	}))
	err := dev.Init(cfg)
	if err != nil {
		panic("dev Init:" + err.Error())
	}
	const maxRead = 1024 * 4
	buf := make([]byte, maxRead)

	n, err := dev.WriteHCI(buf[:4])
	if err != nil {
		panic("writeHCI:" + err.Error())
	}
	println("wrote", n, "bytes over HCI")
	for {
		time.Sleep(700 * time.Millisecond)
		if dev.BufferedHCI() == 0 {
			println("no data buffered on HCI interface")
			time.Sleep(time.Second)
			continue
		}
		avail := dev.BufferedHCI()
		if avail > len(buf) {
			println("short buffer available=", avail, " buflen=", len(buf))
			time.Sleep(time.Second)
			continue
		}
		println("avail=", avail)
		n, err = dev.ReadHCI(buf[:roundBuflen(avail)])
		if err != nil {
			panic("readHCI:" + err.Error())
		}
		println("read", n, "bytes over HCI", string(buf[:n]))
	}
}

// CYW43439 read buffer must be of length multiple of 4.
func roundBuflen(len int) int {
	const alignment = 4
	return (len + alignment - 1) &^ (alignment - 1)
}
