// Netlink implmentation of cyw43439

package cyw43439

import (
	"net"

	"tinygo.org/x/drivers"
)

func (d *Dev) NetConnect() error {
	return drivers.ErrNotSupported
}

func (d *Dev) NetDisconnect() {
}

func (d *Dev) NetNotify(cb func(drivers.NetlinkEvent)) {
}

func (d *Dev) GetHardwareAddr() (net.HardwareAddr, error) {
	return net.HardwareAddr{}, drivers.ErrNotSupported
}

func (d *Dev) GetIPAddr() (net.IP, error) {
	return net.IP{}, drivers.ErrNotSupported
}
