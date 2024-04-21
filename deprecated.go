package cyw43439

// These functions have been deprecated due to new interface design.

var deprecatePrinted bool

// TryPoll
//
// Deprecated: Use PollOne instead.
func (d *Device) TryPoll() (gotPacket bool, err error) {
	printDeprecated()
	return d.PollOne()
}

// MACAs6
//
// Deprecated: use HardwareAddr6
func (d *Device) MACAs6() [6]byte {
	printDeprecated()
	mac, _ := d.HardwareAddr6()
	return mac
}

//go:inline
func printDeprecated() {
	if !deprecatePrinted {
		println("\n\n==\n\ncyw43439.MACAs6 deprecated, use HardwareAddr6\n\n==\n\n")
		println("\n\n==\n\ncyw43439.TryPoll deprecated, use PollOne\n\n==\n\n")
		deprecatePrinted = true
	}
}
