package main

import (
	"encoding/binary"

	"github.com/soypat/cyw43439"
)

func main() {
	spi, cs, wl, irq := cyw43439.PicoWSpi(0)
	err := Write32S(spi, cyw43439.FuncBus, 0, 0)
	println(err.Error())
	// return // REMOVE RETURN TO OBSERVE BEHAVIOR.
	_, _, _ = cs, wl, irq
	dev := cyw43439.NewDevice(spi, cs, wl, irq, irq)
	dev.Write32S(cyw43439.FuncBus, 0, 0)
}

// Attempt at a MWE of the behavior seen in Write32S.
//
//go:noinline
func Write32S(spi *cyw43439.SPIbb, fn cyw43439.Function, addr, val uint32) error {
	cmd := uint32(fn)
	var buf [4]byte

	binary.BigEndian.PutUint32(buf[:], cmd)
	err := spi.Tx(buf[:], nil)
	if err != nil {
		return err
	}
	binary.BigEndian.PutUint32(buf[:], (val))
	err = spi.Tx(buf[:], nil)
	if err != nil {
		return err
	}
	// Read Status.
	buf = [4]byte{}
	spi.Tx(buf[:], buf[:])
	return err
}
