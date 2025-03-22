//go:build cy43nopio || !(rp2040 || rp2350)

package cyw43439

import (
	"encoding/binary"
)

var _busOrder = binary.LittleEndian

type cmdBus interface {
	CmdRead(cmd uint32, buf []uint32) error
	CmdWrite(cmd uint32, buf []uint32) error
	LastStatus() uint32
}
