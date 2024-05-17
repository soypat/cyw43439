package main

import (
	"time"

	"github.com/soypat/cyw43439"
)

func main() {
	time.Sleep(time.Second)
	dev := cyw43439.NewPicoWDevice()
	err := dev.Init(cyw43439.DefaultBluetoothConfig())
	if err != nil {
		panic("dev Init:" + err.Error())
	}
	n, err := dev.WriteHCI([]byte{0x1, 0x2, 0x3, 0x4})
	if err != nil {
		panic("writeHCI:" + err.Error())
	}
	println("wrote", n, "bytes over HCI")
}
