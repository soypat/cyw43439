package main

import (
	"time"

	cyw43439 "github.com/soypat/cy43439"
)

func TestCy43439RegistersOnPicoW() {
	println("starting TestCy43439RegistersOnPicoW")
	spi, cs, wl, irq := cyw43439.PicoWSpi()
	dev := cyw43439.NewDev(spi, cs, wl, irq, irq)
	err := dev.Init()
	if err != nil {
		panic(err)
	}
	dev.Reset()
	time.Sleep(50 * time.Millisecond)
	v, err := dev.RegisterReadUint32(cyw43439.FuncBus, cyw43439.AddrTest)
	if err != nil {
		panic(err.Error())
	}
	if v != cyw43439.AddrTest {
		print("[FAIL] unexpected value for test register. got:")
		print(v)
		print(", expected:")
		println(cyw43439.TestPattern)
	} else {
		println("[PASS] register read of 0xFEEDBEAD at 0x14 success!")
	}
}
