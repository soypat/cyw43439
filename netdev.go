// Netdev implmentation of cyw43439

package cyw43439

import (
	"net"
	"time"

	"tinygo.org/x/drivers"
)

func (d *Device) GetHostByName(name string) (net.IP, error) {
	return net.IP{}, drivers.ErrNotSupported
}

func (d *Device) Socket(domain int, stype int, protocol int) (int, error) {
	return -1, drivers.ErrNotSupported
}

func (d *Device) Bind(sockfd int, ip net.IP, port int) error {
	return drivers.ErrNotSupported
}

func (d *Device) Connect(sockfd int, host string, ip net.IP, port int) error {
	return drivers.ErrNotSupported
}

func (d *Device) Listen(sockfd int, backlog int) error {
	return drivers.ErrNotSupported
}

func (d *Device) Accept(sockfd int, ip net.IP, port int) (int, error) {
	return -1, drivers.ErrNotSupported
}

func (d *Device) Send(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	return 0, drivers.ErrNotSupported
}

func (d *Device) Recv(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	return 0, drivers.ErrNotSupported
}

func (d *Device) Close(sockfd int) error {
	return drivers.ErrNotSupported
}

func (d *Device) SetSockOpt(sockfd int, level int, opt int, value interface{}) error {
	return drivers.ErrNotSupported
}
