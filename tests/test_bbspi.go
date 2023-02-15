package main

import (
	"machine"
	"time"

	cyw43439 "github.com/soypat/cy43439"
)

func TestMockCY43439(sdo, sdi, cs, clk machine.Pin) {
	const wlRegOn = machine.GPIO11
	print("starting TestBBSPI with SDO=")
	print(sdo)
	print(" SDI=")
	print(sdi)
	print(" CS=")
	print(cs)
	print(" CLK=")
	println(clk)
	data := []byte{0x0, 0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0xff}
	spi := &cyw43439.SPIbb{
		SCK:   clk,
		SDI:   sdi,
		SDO:   sdo,
		Delay: 20,
	}
	spi.Configure()
	println("creating dev")
	dev := cyw43439.NewDev(spi, cs, wlRegOn, sdo, sdo)
	dev.GPIOSetup()
	cs.Low()
	spi.Tx(data, data)
	cs.High()
	dev.ReadReg32Swap(cyw43439.FuncBus, cyw43439.AddrTest)
	dev.RegisterReadUint32(cyw43439.FuncBus, cyw43439.AddrTest)
	dev.RegisterWriteUint32(cyw43439.FuncBus, cyw43439.AddrTest, 0xfeedbead)
	println("done")
	return
	dev.Init()
	println("reading mock register 0x14")
	time.Sleep(1 * time.Millisecond)
	dev.RegisterReadUint32(0, 0x14)

	println("writing 0xFEEDBEAD to mock register 0x18")
	time.Sleep(1 * time.Millisecond)
	dev.RegisterWriteUint32(0, 0x18, 0xFEEDBEAD)

	println("writing data slice and reading same amount")
	time.Sleep(1 * time.Millisecond)
	cs.Low()
	spi.Tx(data, data)
	cs.High()
}
