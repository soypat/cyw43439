// Netdev implmentation of cyw43439

package cyw43439

import (
	"net"
	"time"

	"tinygo.org/x/drivers"
)

func (d *Dev) GetHostByName(name string) (net.IP, error) {
	return net.IP{}, drivers.ErrNotSupported
}

func (d *Dev) Socket(domain int, stype int, protocol int) (int, error) {
	return -1, drivers.ErrNotSupported
}

func (d *Dev) Bind(sockfd int, ip net.IP, port int) error {
	return drivers.ErrNotSupported
}

func (d *Dev) Connect(sockfd int, host string, ip net.IP, port int) error {
	return drivers.ErrNotSupported
}

func (d *Dev) Listen(sockfd int, backlog int) error {
	return drivers.ErrNotSupported
}

func (d *Dev) Accept(sockfd int, ip net.IP, port int) (int, error) {
	return -1, drivers.ErrNotSupported
}

func (d *Dev) Send(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	return 0, drivers.ErrNotSupported
}

func (d *Dev) Recv(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	return 0, drivers.ErrNotSupported
}

func (d *Dev) Close(sockfd int) error {
	return drivers.ErrNotSupported
}

func (d *Dev) SetSockOpt(sockfd int, level int, opt int, value interface{}) error {
	return drivers.ErrNotSupported
}
