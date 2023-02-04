package cyw43439

import (
	"errors"
	"machine"
)

// bbSPI is a dumb bit-bang implementation of SPI protocol that is hardcoded
// to mode 0 and ignores trying to receive data. Just enough for the APA102.
// Note: making this unexported for now because it is probable not suitable
// most purposes other than the APA102 package. It might be desirable to make
// this more generic and include it in the TinyGo "machine" package instead.
type bbSPI struct {
	SCK   machine.Pin
	SDI   machine.Pin
	SDO   machine.Pin
	Delay uint32
}

// Configure sets up the SCK and SDO pins as outputs and sets them low
func (s *bbSPI) Configure() {
	s.SCK.Configure(machine.PinConfig{Mode: machine.PinOutput})
	s.SDO.Configure(machine.PinConfig{Mode: machine.PinOutput})
	s.SCK.Low()
	s.SDO.Low()
	if s.Delay == 0 {
		s.Delay = 1
	}
}

// Tx matches signature of machine.SPI.Tx() and is used to send multiple bytes.
// The r slice is ignored and no error will ever be returned.
func (s *bbSPI) Tx(w []byte, r []byte) error {
	s.Configure()
	switch {
	case len(r) == len(w):
		for i, b := range w {
			r[i], _ = s.Transfer(b)
		}
	case len(w) != 0:
		for _, b := range w {
			s.Transfer(b)
		}
	case len(r) != 0:
		for i := range r {
			r[i], _ = s.Transfer(0)
		}
	default:
		return errors.New("unhandled SPI buffer length mismatch case")
	}
	return nil
}

// delay represents a quarter of the clock cycle
func (s *bbSPI) delay() {
	for i := uint32(0); i < s.Delay; {
		i++
	}
}

// Transfer matches signature of machine.SPI.Transfer() and is used to send a
// single byte. The received data is ignored and no error will ever be returned.
func (s *bbSPI) Transfer(b byte) (out byte, _ error) {
	out |= b2u8(s.bitTransfer(b&(1<<7) != 0)) << 7
	out |= b2u8(s.bitTransfer(b&(1<<6) != 0)) << 6
	out |= b2u8(s.bitTransfer(b&(1<<5) != 0)) << 5
	out |= b2u8(s.bitTransfer(b&(1<<4) != 0)) << 4
	out |= b2u8(s.bitTransfer(b&(1<<3) != 0)) << 3
	out |= b2u8(s.bitTransfer(b&(1<<2) != 0)) << 2
	out |= b2u8(s.bitTransfer(b&(1<<1) != 0)) << 1
	out |= b2u8(s.bitTransfer(b&1 != 0))
	return out, nil
}

//go:inline
func (s *bbSPI) bitTransfer(b bool) bool {
	s.SDO.Set(b)
	s.SCK.High()
	b4 := s.SDI.Get()
	s.delay()
	s.SCK.Low()
	s.delay()
	return b4
}

//go:inline
func b2u8(b bool) byte {
	if b {
		return 1
	}
	return 0
}
