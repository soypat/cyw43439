package main

import "machine"

func main() {
	const (
		mockSDO = machine.GPIO0
		mockSDI = machine.GPIO1
		mockCS  = machine.GPIO2
		mockCLK = machine.GPIO3
	)
	// TestMockCY43439(mockSDO, mockSDI, mockCS, mockCLK)
	TestCy43439RegistersOnPicoW()
}
