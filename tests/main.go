package main

import (
	"machine"
	"time"
)

func main() {
	// Give time for monitor to hook up to USB.
	time.Sleep(time.Second)
	const (
		mockSDI = machine.GPIO0
		mockCS  = machine.GPIO1
		mockCLK = machine.GPIO2
		mockSDO = machine.GPIO3
	)
	TestMockCY43439(mockSDO, mockSDO, mockCS, mockCLK)
	// TestCy43439RegistersOnPicoW()
}
