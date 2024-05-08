package cyw43439

// Functions deprecated due to naming changes to adapt to netif
// package github.com/soypat/netif.

// MACAs6
//
// Deprecated: Use HardwareAddr6() instead.
func (d *Device) MACAs6() [6]byte {
	mac, _ := d.HardwareAddr6()
	return mac
}

// TryPoll
//
// Deprecated: Use PollOne instead.
func (d *Device) TryPoll() (gotPacket bool, err error) {
	return d.PollOne()
}
