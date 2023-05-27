// Netlink implmentation of cyw43439

package cyw43439

import (
	"net"

	"tinygo.org/x/drivers"
)

func (d *Device) NetConnect() error {
	return drivers.ErrNotSupported
}

func (d *Device) NetDisconnect() {
}

func (d *Device) NetNotify(cb func(drivers.NetlinkEvent)) {
}

func (d *Device) GetHardwareAddr() (net.HardwareAddr, error) {
	return net.HardwareAddr{}, drivers.ErrNotSupported
}

func (d *Device) GetIPAddr() (net.IP, error) {
	return net.IP{}, drivers.ErrNotSupported
}
