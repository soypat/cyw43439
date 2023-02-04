package main

import (
	"time"

	cyw43439 "github.com/soypat/cy43439"
)

func main() {
	time.Sleep(time.Second)
	dev := cyw43439.NewDev(cyw43439.PicoWSpi())

	err := dev.Init()
	if err != nil {
		panic(err)
	}
	dev.Reset()
	time.Sleep(50 * time.Millisecond)
	v, err := dev.RegisterReadUint32(0, cyw43439.TestRegisterAddr)
	if err != nil {
		panic(err.Error())
	}
	println("register read success")
	if v != cyw43439.TestRegisterExpectedValue {
		print("got unexpected value for test register:")
		print(v)
		print(" expected ")
		println(cyw43439.TestRegisterExpectedValue)
	}
	println("setup successfully done")
}
