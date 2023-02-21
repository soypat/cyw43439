package cyw43439

import (
	"device"
	"errors"
	"machine"
)

// SPIbb is a dumb bit-bang implementation of SPI protocol that is hardcoded
// to mode 0.
type SPIbb struct {
	SCK   machine.Pin
	SDI   machine.Pin
	SDO   machine.Pin
	Delay uint32
}

// Configure sets up the SCK and SDO pins as outputs and sets them low
func (s *SPIbb) Configure() {
	s.SCK.Configure(machine.PinConfig{Mode: machine.PinOutput})
	s.SDO.Configure(machine.PinConfig{Mode: machine.PinOutput})
	if s.SDI != s.SDO {
		// Shared pin configurations.
		s.SDI.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
		s.SDI.Low()
	}
	s.SCK.Low()
	s.SDO.Low()
	if s.Delay == 0 {
		s.Delay = 1
	}
}

// Tx matches signature of machine.SPI.Tx() and is used to send multiple bytes.
// The r slice is ignored and no error will ever be returned.
func (s *SPIbb) Tx(w []byte, r []byte) error {
	if len(w) != 0 {
		r[0] = s.firstTransfer(w[0])
		w = w[1:]
		r = r[1:]
	}
	switch {
	case len(r) == len(w):
		for i, b := range w {
			r[i] = s.transfer(b)
		}
	case len(w) != 0:
		for _, b := range w {
			s.transfer(b)
		}
	case len(r) != 0:
		for i := range r {
			r[i] = s.transfer(0)
		}
	default:
		return errors.New("unhandled SPI buffer length mismatch case")
	}
	return nil
}

// Transfer matches signature of machine.SPI.Transfer() and is used to send a
// single byte. The received data is ignored and no error will ever be returned.
func (s *SPIbb) Transfer(b byte) (out byte, _ error) {
	return s.transfer(b), nil
}

//go:inline
func (s *SPIbb) transfer(b byte) (out byte) {
	out |= b2u8(s.bitTransfer(b&(1<<7) != 0)) << 7
	out |= b2u8(s.bitTransfer(b&(1<<6) != 0)) << 6
	out |= b2u8(s.bitTransfer(b&(1<<5) != 0)) << 5
	out |= b2u8(s.bitTransfer(b&(1<<4) != 0)) << 4
	out |= b2u8(s.bitTransfer(b&(1<<3) != 0)) << 3
	out |= b2u8(s.bitTransfer(b&(1<<2) != 0)) << 2
	out |= b2u8(s.bitTransfer(b&(1<<1) != 0)) << 1
	out |= b2u8(s.bitTransfer(b&1 != 0))
	return out
}

//go:inline
func (s *SPIbb) bitTransfer(b bool) bool {
	s.SDO.Set(b) // The host
	s.delay()
	s.SCK.High()
	s.delay()
	inputBit := s.SDI.Get()
	s.delay()
	s.SCK.Low()
	s.delay()
	return inputBit
}

// Only used for first write byte. Not for reads
//
//go:inline
func (s *SPIbb) firstTransfer(b byte) (out byte) {
	out |= b2u8(s.firstBitTransfer(b&(1<<7) != 0)) << 7
	out |= b2u8(s.bitTransfer(b&(1<<6) != 0)) << 6
	out |= b2u8(s.bitTransfer(b&(1<<5) != 0)) << 5
	out |= b2u8(s.bitTransfer(b&(1<<4) != 0)) << 4
	out |= b2u8(s.bitTransfer(b&(1<<3) != 0)) << 3
	out |= b2u8(s.bitTransfer(b&(1<<2) != 0)) << 2
	out |= b2u8(s.bitTransfer(b&(1<<1) != 0)) << 1
	out |= b2u8(s.bitTransfer(b&1 != 0))
	return out
}

//go:inline
func (s *SPIbb) firstBitTransfer(b bool) bool {
	//The host puts the first bit of the data onto the bus half a clock-cycle
	// before the first active edge following the CS going low. T
	s.SDO.Set(b)
	s.delay()
	s.delay()
	s.SCK.High()
	s.delay()
	inputBit := s.SDI.Get()
	s.SCK.Low()
	s.delay()
	return inputBit
}

// delay represents a quarter of the clock cycle
//
//go:inline
func (s *SPIbb) delay() {
	for i := uint32(0); i < s.Delay; i++ {
		device.Asm("nop")
	}
}

//go:inline
func b2u8(b bool) byte {
	if b {
		return 1
	}
	return 0
}
