package main

import (
	"time"

	"github.com/soypat/cyw43439/cyrw"
)

func main() {
	time.Sleep(2 * time.Second)
	println("starting program")
	dev := cyrw.DefaultNew()

	err := dev.Init(cyrw.Config{})
	if err != nil {
		panic(err)
	}
	println("finished program OK")
}
