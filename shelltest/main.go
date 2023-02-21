package main

import (
	"machine"
	"time"
)

const (
	mockSDI = machine.GPIO4
	mockCS  = machine.GPIO1
	mockSCK = machine.GPIO2
	mockSDO = machine.GPIO3
)

func main() {
	// Give time for monitor to hook up to USB.
	time.Sleep(time.Second)
	TestShellmode()

	// TestMockCY43439(mockSDO, mockSDO, mockCS, mockCLK)
	// TestCy43439RegistersOnPicoW()
}
