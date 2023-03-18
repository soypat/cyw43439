//go:build tinygo

package cyw43439

import (
	"encoding/binary"
	"errors"
	"machine"
)

// gSPI transaction endianess.
var endian binary.ByteOrder = binary.LittleEndian

// cy_io.go contains low level functions for reading and writing to the
// CYW43439's gSPI interface. These map to functions readily found in the datasheet.

var ErrDataNotAvailable = errors.New("requested data not available")

func (d *Dev) Write32(fn Function, addr, val uint32) error {
	err := d.wr(fn, addr, 4, uint32(val))
	Debug("cyw43_write_reg_u32", fn.String(), addr, "=", val, err)
	return err
}

func (d *Dev) Write16(fn Function, addr uint32, val uint16) error {
	err := d.wr(fn, addr, 2, uint32(val))
	Debug("cyw43_write_reg_u16", fn.String(), addr, "=", val, err)
	return err
}

func (d *Dev) Write8(fn Function, addr uint32, val uint8) error {
	err := d.wr(fn, addr, 1, uint32(val))
	Debug("cyw43_write_reg_u8", fn.String(), addr, "=", val, err)
	return err
}

func (d *Dev) wr(fn Function, addr, size, val uint32) error {
	var buf [4]byte
	cmd := make_cmd(true, true, fn, addr, size)
	if fn == FuncBackplane {
		d.lastSize = 8
		d.lastHeader[0] = cmd
		d.lastHeader[1] = uint32(val)
		d.lastBackplaneWindow = d.currentBackplaneWindow
	}
	switch size {
	case 4:
		endian.PutUint32(buf[:], val)
	case 2:
		endian.PutUint16(buf[:2], uint16(val)) // !LE
	case 1:
		buf[0] = byte(val)      // !LE
		buf[1] = byte(val >> 8) // TODO: original driver has this behaviour. is it necessary?
	default:
		panic("misuse of general write register")
	}
	d.csLow()
	err := d.spiWrite(cmd, buf[:4])
	d.csHigh()
	return err
}

// WriteBytes is cyw43_write_bytes
func (d *Dev) WriteBytes(fn Function, addr uint32, src []byte) error {
	// println("writeBytes")
	length := uint32(len(src))
	alignedLength := (length + 3) &^ 3
	if length != alignedLength {
		return errors.New("buffer length must be length multiple of 4")
	}
	if !(fn != FuncBackplane || (length <= 64 && (addr+length) <= 0x8000)) {
		panic("bad argument to WriteBytes")
	}
	if fn == FuncWLAN {
		readyAttempts := 1000
		for ; readyAttempts > 0; readyAttempts-- {
			status, err := d.GetStatus() // TODO: We're getting Status not ready here.
			if err != nil {
				return err
			}
			if status.F2RxReady() {
				break
			}
		}
		if readyAttempts <= 0 {
			return errors.New("F2 not ready")
		}
	}
	cmd := make_cmd(true, true, fn, addr, length)
	d.csLow()
	err := d.spiWrite(cmd, src)
	d.csHigh()
	return err
}

// spiWrite performs the gSPI Write action. Does not control CS pin.
func (d *Dev) spiWrite(cmd uint32, w []byte) error {
	var buf [4]byte
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	endian.PutUint32(buf[:], cmd) // !LE
	err := d.spi.Tx(buf[:], nil)
	if err != nil {
		return err
	}
	err = d.spi.Tx(w, nil)
	if err != nil || !d.enableStatusWord {
		return err
	}
	// Read Status.
	buf = [4]byte{}
	d.spi.Tx(buf[:], buf[:])
	status := Status(swap32(endian.Uint32(buf[:]))) // !LE
	if status.DataUnavailable() {
		println("got status:", status)
		return ErrDataNotAvailable
	}
	return nil
}

func (d *Dev) Read32(fn Function, addr uint32) (uint32, error) {
	v, err := d.rr(fn, addr, 4)
	Debug("cyw43_read_reg_u32", fn.String(), addr, "=", uint32(v), err)
	return v, err
}

func (d *Dev) Read16(fn Function, addr uint32) (uint16, error) {
	v, err := d.rr(fn, addr, 2)
	Debug("cyw43_read_reg_u16", fn.String(), addr, "=", uint16(v), err)
	return uint16(v), err
}

func (d *Dev) Read8(fn Function, addr uint32) (uint8, error) {
	v, err := d.rr(fn, addr, 1)
	Debug("cyw43_read_reg_u8", fn.String(), addr, "=", uint8(v), err)
	return uint8(v), err
}

// rr reads a register and returns the result and an error if there was one.
func (d *Dev) rr(fn Function, addr, size uint32) (uint32, error) {
	var padding uint32
	if fn == FuncBackplane {
		padding = whdBusSPIBackplaneReadPadding
	}
	cmd := make_cmd(false, true, fn, addr, size+padding)
	var buf [4 + whdBusSPIBackplaneReadPadding]byte
	d.csLow()
	err := d.spiRead(cmd, buf[:4+padding], 0)
	d.csHigh()
	result := endian.Uint32(buf[padding : padding+4]) // !LE
	return result, err
}

func (d *Dev) ReadBytes(fn Function, addr uint32, src []byte) error {
	const maxReadPacket = 2040
	length := uint32(len(src))
	alignedLength := (length + 3) &^ 3
	if length != alignedLength {
		return errors.New("buffer length must be length multiple of 4")
	}
	assert := fn == FuncBackplane || (length <= 64 && (addr+length) <= 0x8000)
	assert = assert && alignedLength > 0 && alignedLength < maxReadPacket
	if !assert {
		panic("bad argument to ReadBytes")
	}
	padding := uint8(0)
	if fn == FuncBackplane {
		padding = 4
		if cap(src) < len(src)+4 {
			return errors.New("ReadBytes src arg requires more capacity for byte padding")
		}
		src = src[:len(src)+4]
	}
	// TODO: Use DelayResponse to simulate padding effect.
	cmd := make_cmd(false, true, fn, addr, length+uint32(padding))
	d.csLow()
	err := d.spiRead(cmd, src, uint8(padding))
	d.csHigh()
	return err
}

// spiRead performs the gSPI Read action.
func (d *Dev) spiRead(cmd uint32, r []byte, padding uint8) error {
	var buf [4]byte
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	endian.PutUint32(buf[:], cmd) // !LE
	d.spi.Tx(buf[:], nil)
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	d.responseDelay(padding)
	err := d.spi.Tx(nil, r)
	if err != nil || !d.enableStatusWord {
		return err
	}
	// Read Status.
	buf = [4]byte{} // zero out buffer.
	err = d.spi.Tx(buf[:], buf[:])
	status := Status(endian.Uint32(buf[:])) // !LE
	if err == nil && status.DataUnavailable() {
		err = ErrDataNotAvailable
		println("got data unavailable status:", status)
	}
	return err
}

//go:inline
func (d *Dev) csHigh() {
	d.cs.High()
	machine.GPIO1.High()
}

//go:inline
func (d *Dev) csLow() {
	d.cs.Low()
	machine.GPIO1.Low()
}

//go:inline
func (d *Dev) responseDelay(padding uint8) {
	// Wait for response.
	for i := uint8(0); i < padding; i++ {
		d.spi.Transfer(0)
	}
}

//go:inline
func (d *Dev) csPeak() {
	// d.csHigh()
	// for i := 0; i < 40; i++ {
	// 	device.Asm("nop")
	// }
	// d.csLow()
}

//go:inline
func make_cmd(write, autoInc bool, fn Function, addr uint32, sz uint32) uint32 {
	return b2u32(write)<<31 | b2u32(autoInc)<<30 | uint32(fn)<<28 | (addr&0x1ffff)<<11 | sz
}

func funcFromCmd(cmd uint32) Function {
	return Function(cmd>>28) & 0b11
}

//go:inline
func b2u32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// swap32 swaps lowest 16 bits with highest 16 bits of a uint32.
func swap32(b uint32) uint32 {
	return (b >> 16) | (b << 16)
}

// Write32S writes register and swaps big-endian 16bit word length. Used only at initialization.
func (d *Dev) Write32S(fn Function, addr, val uint32) error {
	cmd := make_cmd(true, true, fn, addr, 4)
	var buf [4]byte
	d.csLow()
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	binary.BigEndian.PutUint32(buf[:], swap32(cmd))
	err := d.spi.Tx(buf[:], nil)
	if err != nil {
		d.csHigh()
		return err
	}
	binary.BigEndian.PutUint32(buf[:], swap32(val))
	err = d.spi.Tx(buf[:], nil)
	Debug("cyw43_write_reg_u32_swap", fn.String(), addr, "=", val, err)
	if err != nil || !d.enableStatusWord {
		d.csHigh()
		return err
	}
	// Read Status.
	buf = [4]byte{}
	d.spi.Tx(buf[:], buf[:])
	d.csHigh()
	status := Status(swap32(binary.BigEndian.Uint32(buf[:])))
	if status.DataUnavailable() {
		println("got status:", status)
		err = ErrDataNotAvailable
	}
	return err
}

// Read32S reads register and swaps big-endian 16bit word length. Used only at initialization.
func (d *Dev) Read32S(fn Function, addr uint32) (uint32, error) {
	if fn == FuncBackplane {
		panic("backplane not implemented for rrS")
	}
	cmd := make_cmd(false, true, fn, addr, 4)
	var buf [4]byte
	d.csLow()
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	binary.BigEndian.PutUint32(buf[:], swap32(cmd))
	d.spi.Tx(buf[:], nil)
	d.csPeak()
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	d.responseDelay(0)
	err := d.spi.Tx(nil, buf[:])
	result := swap32(binary.BigEndian.Uint32(buf[:]))
	Debug("cyw43_read_reg_u32_swap", fn.String(), addr, "=", result, err)
	if err != nil || !d.enableStatusWord {
		d.csHigh()
		return result, err
	}
	// Read Status.
	buf = [4]byte{} // zero out buffer.
	d.spi.Tx(buf[:], buf[:])
	d.csHigh()
	status := Status(swap32(binary.BigEndian.Uint32(buf[:])))
	if status.DataUnavailable() {
		err = ErrDataNotAvailable
		println("got data unavailable status:", status)
	}
	return result, err
}
