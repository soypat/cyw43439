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
	n, err := dev.WriteHCI([]byte{0x1, 0x2, 0x3, 0x4})
	if err != nil {
		panic("writeHCI:" + err.Error())
	}
	println("wrote", n, "bytes over HCI")
}
