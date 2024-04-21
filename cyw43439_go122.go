//go:build go1.22

package cyw43439

import "net"

// NetFlags returns the net.Flags for the device, either net.FlagUp or net.FlagRunning.
func (d *Device) NetFlags() (flags net.Flags) {
	state := d.state
	if state == linkStateUp {
		flags |= net.FlagRunning
	}
	if state != linkStateDown {
		flags |= net.FlagUp
	}
	return flags
}
