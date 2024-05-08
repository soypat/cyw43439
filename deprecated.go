package cyw43439

import "sync"

// Functions deprecated due to naming changes to adapt to netif
// package github.com/soypat/netif.

var deprecated sync.Once

func printDeprecationOnce() {
	deprecated.Do(func() {
		println("github.com/soypat/cyw43439: use of deprecated MACAs6 or TryPoll *Device method. Will be removed in future version. This message is only printed once")
	})
}

// MACAs6
//
// Deprecated: Use HardwareAddr6() instead.
func (d *Device) MACAs6() [6]byte {
	printDeprecationOnce()
	mac, _ := d.HardwareAddr6()
	return mac
}

// TryPoll
//
// Deprecated: Use PollOne instead.
func (d *Device) TryPoll() (gotPacket bool, err error) {
	printDeprecationOnce()
	return d.PollOne()
}
