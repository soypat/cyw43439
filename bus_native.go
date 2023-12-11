//go:build !pico

package cyw43439

import (
	"encoding/binary"

	"golang.org/x/exp/constraints"
)

var _busOrder = binary.LittleEndian

type spibus struct {
}

// align rounds `val` up to nearest multiple of `align`.
func align[T constraints.Integer](val, align T) T {
	return (val + align - 1) &^ (align - 1)
}

func (d *spibus) cmd_read(cmd uint32, buf []uint32) (status uint32, err error) {
	return 0, nil
}

func (d *spibus) cmd_write(cmd uint32, buf []uint32) (status uint32, err error) {
	return 0, nil
}

func (d *spibus) csEnable(b bool) {
}

func (d *spibus) Status() Status {
	return 0
}
