//go:build tinygo
package main

import (
	"device"
	"encoding/binary"
	"errors"
	"machine"
	"unsafe"
)

var order = binary.LittleEndian

// CmdRead(cmd uint32, buf []uint32) error
// CmdWrite(cmd uint32, buf []uint32) error
// LastStatus() uint32

// SPIbb is a dumb bit-bang implementation of SPI protocol that is hardcoded
// to mode 0.
type SPIbb struct {
	SCK   machine.Pin
	SDI   machine.Pin
	SDO   machine.Pin
	Delay uint32
	// If MockTo is not nil then clock, SDI and SDO writes/reads are duplicated to it.
	MockTo *SPIbb
	buf    [4]byte
	status uint32
}

// Configure sets up the SCK and SDO pins as outputs and sets them low
func (s *SPIbb) Configure() {
	s.SCK.Configure(machine.PinConfig{Mode: machine.PinOutput})
	s.SDO.Configure(machine.PinConfig{Mode: machine.PinOutput})
	if s.SDI != s.SDO {
		// Shared pin configurations.
		s.SDI.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	s.SCK.Low()
	s.SDO.Low()
	if s.Delay == 0 {
		s.Delay = 1
	}
	if s.MockTo != nil {
		s.MockTo.Configure()
		// We take inputs on our SDI and send them to MockTo.SDI as outputs.
		s.MockTo.SDI.Configure(machine.PinConfig{Mode: machine.PinOutput})
		s.MockTo.SDI.Low()
	}
}

func (s *SPIbb) CmdRead(cmd uint32, buf []uint32) error {
	order.PutUint32(s.buf[:], cmd)
	mock := s.MockTo != nil
	for _, b := range s.buf[:4] {
		s.transfer(b, mock)
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), 4*len(buf))
	for i := range data {
		data[i] = byte(s.transfer(0, mock))
	}
	s.readStatus(mock)
	return nil
}

func (s *SPIbb) CmdWrite(cmd uint32, buf []uint32) error {
	order.PutUint32(s.buf[:], cmd)
	mock := s.MockTo != nil
	for _, b := range s.buf[:4] {
		s.transfer(b, mock)
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), 4*len(buf))
	for i := range data {
		s.transfer(data[i], mock)
	}
	s.readStatus(mock)
	return nil
}

func (s *SPIbb) LastStatus() uint32 {
	return s.status
}

func (s *SPIbb) readStatus(mock bool) {
	for i := 0; i < 4; i++ {
		s.buf[i] = s.transfer(0, mock)
	}
	s.status = order.Uint32(s.buf[:])
}

// Tx matches signature of machine.SPI.Tx() and is used to send multiple bytes.
// The r slice is ignored and no error will ever be returned.
func (s *SPIbb) Tx(w []byte, r []byte) (err error) {
	aux := s.buf[:1]
	mocking := s.MockTo != nil
	if len(w) != 0 {
		if len(r) == 0 {
			r = aux[:]
		}
		r[0] = s.firstTransfer(w[0], mocking)
		w = w[1:]
		r = r[1:]
	}
	switch {
	case len(r) == len(w):
		for i, b := range w {
			r[i] = s.transfer(b, mocking)
		}
	case len(w) != 0:
		for _, b := range w {
			s.transfer(b, mocking)
		}
	case len(r) != 0:
		for i := range r {
			r[i] = s.transfer(0, mocking)
		}
	default:
		err = errors.New("unhandled SPI buffer length mismatch case")
	}
	return err
}

// Transfer matches signature of machine.SPI.Transfer() and is used to send a
// single byte. The received data is ignored and no error will ever be returned.
func (s *SPIbb) Transfer(b byte) (out byte, _ error) {
	return s.transfer(b, s.MockTo != nil), nil
}

//go:inline
func (s *SPIbb) transfer(b byte, mocking bool) (out byte) {
	out |= b2u8(s.bitTransfer(b&(1<<7) != 0, mocking)) << 7
	out |= b2u8(s.bitTransfer(b&(1<<6) != 0, mocking)) << 6
	out |= b2u8(s.bitTransfer(b&(1<<5) != 0, mocking)) << 5
	out |= b2u8(s.bitTransfer(b&(1<<4) != 0, mocking)) << 4
	out |= b2u8(s.bitTransfer(b&(1<<3) != 0, mocking)) << 3
	out |= b2u8(s.bitTransfer(b&(1<<2) != 0, mocking)) << 2
	out |= b2u8(s.bitTransfer(b&(1<<1) != 0, mocking)) << 1
	out |= b2u8(s.bitTransfer(b&(1<<0) != 0, mocking))
	return out
}

//go:inline
func (s *SPIbb) bitTransfer(b, mocking bool) bool {
	s.SDOSet(b, mocking)
	s.delay(mocking)
	inputBit := s.SDI.Get()
	s.SCKSet(true, mocking)
	s.delay(mocking)
	s.delay(mocking)
	s.SCKSet(false, mocking)
	s.delay(mocking)
	return inputBit
}

// Only used for first write byte. Not for reads
//
//go:inline
func (s *SPIbb) firstTransfer(b byte, mocking bool) (out byte) {
	out |= b2u8(s.firstBitTransfer(b&(1<<7) != 0, mocking)) << 7
	out |= b2u8(s.bitTransfer(b&(1<<6) != 0, mocking)) << 6
	out |= b2u8(s.bitTransfer(b&(1<<5) != 0, mocking)) << 5
	out |= b2u8(s.bitTransfer(b&(1<<4) != 0, mocking)) << 4
	out |= b2u8(s.bitTransfer(b&(1<<3) != 0, mocking)) << 3
	out |= b2u8(s.bitTransfer(b&(1<<2) != 0, mocking)) << 2
	out |= b2u8(s.bitTransfer(b&(1<<1) != 0, mocking)) << 1
	out |= b2u8(s.bitTransfer(b&1 != 0, mocking))
	return out
}

//go:inline
func (s *SPIbb) firstBitTransfer(b bool, mocking bool) bool {
	//The host puts the first bit of the data onto the bus half a clock-cycle
	// before the first active edge following the CS going low. T
	s.SDOSet(b, mocking)
	s.delay(mocking)
	s.delay(mocking)
	inputBit := s.SDI.Get()
	s.SCKSet(true, mocking)
	s.delay(mocking)
	s.SCKSet(false, mocking)
	s.delay(mocking)
	return inputBit
}

// delay represents a quarter of the clock cycle
//
//go:inline
func (s *SPIbb) delay(mocking bool) {
	if mocking {
		s.MockTo.SDI.Set(s.SDI.Get())
	}
	for i := uint32(0); i < s.Delay; i++ {
		device.Asm("nop")
		if mocking {
			s.MockTo.SDI.Set(s.SDI.Get())
		}
	}
}

func (s *SPIbb) SDOSet(b, mocking bool) {
	s.SDO.Set(b)
	if mocking {
		s.MockTo.SDO.Set(b)
	}
}

func (s *SPIbb) SCKSet(b, mocking bool) {
	s.SCK.Set(b)
	if mocking {
		s.MockTo.SCK.Set(b)
	}
}

//go:inline
func b2u8(b bool) byte {
	if b {
		return 1
	}
	return 0
}
