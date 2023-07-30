//go:build tinygo

package cyw43439

import (
	"encoding/binary"
	"errors"
	"machine"

	"github.com/soypat/cyw43439/whd"
	"golang.org/x/exp/slog"
)

// gSPI transaction endianess.
var endian binary.ByteOrder = binary.LittleEndian

// cy_io.go contains low level functions for reading and writing to the
// CYW43439's gSPI interface. These map to functions readily found in the datasheet.

var ErrDataNotAvailable = errors.New("requested data not available")

func (d *Device) Write32(fn Function, addr, val uint32) error {
	err := d.wr(fn, addr, 4, uint32(val))
	return err
}

// reference: cyw43_write_reg_u16
func (d *Device) Write16(fn Function, addr uint32, val uint16) error {
	err := d.wr(fn, addr, 2, uint32(val))
	return err
}

// reference: cyw43_write_reg_u8
func (d *Device) Write8(fn Function, addr uint32, val uint8) error {
	err := d.wr(fn, addr, 1, uint32(val))
	return err
}

func (d *Device) wr(fn Function, addr, size, val uint32) error {
	var buf [4]byte
	cmd := make_cmd(true, true, fn, addr, size)
	if fn == FuncBackplane {
		d.lastSize = 8
		d.lastHeader[0] = cmd
		d.lastHeader[1] = uint32(val)
		d.lastBackplaneWindow = d.currentBackplaneWindow
	}
	// TODO(soypat): It seems that this would work with one single case:
	// endian.PutUint32(buf[:], val). Leave as is until driver is complete and can be end-to-end tested.
	switch size {
	case 4:
		endian.PutUint32(buf[:], val)
	case 2:
		endian.PutUint16(buf[:2], uint16(val))
	case 1:

		buf[0] = byte(val)
		buf[1] = byte(val >> 8)
		buf[2] = byte(val >> 16)
		buf[3] = byte(val >> 24)
	default:
		panic("misuse of general write register")
	}
	d.csLow()
	err := d.spiWrite(cmd, buf[:4])
	d.csHigh()
	if verbose_debug {
		function := "Write<INVALID_SIZE>"
		switch size {
		case 1:
			function = "Write8"
		case 2:
			function = "Write16"
		case 4:
			function = "Write32"
		}
		d.debugIO(function, slog.String("fn", fn.String()), slog.Uint64("addr", uint64(addr)), slog.Uint64("val", uint64(val)))
	}
	return err
}

// reference: cyw43_write_bytes
func (d *Device) WriteBytes(fn Function, addr uint32, src []byte) error {
	// Debug("WriteBytes addr=", addr, "len=", len(src), "fn=", fn.String())
	length := uint32(len(src))
	alignedLength := (length + 3) &^ 3
	assert := alignedLength > 0 && alignedLength <= 2040
	if !assert {
		return errors.New("buffer length not in 1..2040")
	}
	if !(fn != FuncBackplane || (length <= 64 && (addr+length) <= 0x8000)) {
		panic("bad argument to WriteBytes")
	}
	if fn == FuncWLAN {
		if cap(src) < int(alignedLength) {
			return errors.New("buffer capacity too small for WLAN writeBytes to pad to 4 bytes")
		}
		readyAttempts := 1000
		for ; readyAttempts > 0; readyAttempts-- {
			status, err := d.GetStatus()
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
		src = src[:alignedLength]
	}
	cmd := make_cmd(true, true, fn, addr, length)
	d.csLow()
	err := d.spiWrite(cmd, src)
	d.csHigh()
	return err
}

// spiWrite performs the gSPI Write action. Does not control CS pin.
func (d *Device) spiWrite(cmd uint32, w []byte) error {
	buf := d.spibuf[:4]
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
	d.spibuf = [4]byte{}
	d.spi.Tx(buf[:], buf[:])
	status := Status(swap32(endian.Uint32(buf[:]))) // !LE
	if status.DataUnavailable() {
		// d.logError("data unavailable status", slog.Uint64("status", uint64(status)))
		return ErrDataNotAvailable
	}
	return nil
}

// reference: cyw43_read_reg_u32
func (d *Device) Read32(fn Function, addr uint32) (uint32, error) {
	v, err := d.rr(fn, addr, 4)
	return v, err
}

// reference: cyw43_read_reg_u16
func (d *Device) Read16(fn Function, addr uint32) (uint16, error) {
	v, err := d.rr(fn, addr, 2)
	return uint16(v), err
}

// reference: cyw43_read_reg_u8
func (d *Device) Read8(fn Function, addr uint32) (uint8, error) {
	v, err := d.rr(fn, addr, 1)
	return uint8(v), err
}

// rr reads a register and returns the result and an error if there was one.
func (d *Device) rr(fn Function, addr, size uint32) (uint32, error) {
	var padding uint32
	if fn == FuncBackplane {
		padding = whd.BUS_SPI_BACKPLANE_READ_PADD_SIZE
	}
	cmd := make_cmd(false, true, fn, addr, size+padding)
	buf := d.spibufrr[:4+whd.BUS_SPI_BACKPLANE_READ_PADD_SIZE]
	d.csLow()
	err := d.spiRead(cmd, buf[:4+padding], 0)
	d.csHigh()
	result := endian.Uint32(buf[padding : padding+4]) // !LE
	if verbose_debug {
		function := "Read<INVALID_SIZE>"
		switch size {
		case 1:
			function = "Read8"
		case 2:
			function = "Read16"
		case 4:
			function = "Read32"
		}
		d.debugIO(function, slog.String("fn", fn.String()), slog.Uint64("addr", uint64(addr)), slog.Uint64("result", uint64(result)))
	}
	return result, err
}

// reference: cyw43_read_bytes
func (d *Device) ReadBytes(fn Function, addr uint32, src []byte) error {
	d.debugIO("ReadBytes", slog.String("fn", fn.String()), slog.Uint64("addr", uint64(addr)), slog.Int("len", len(src)))
	const maxReadPacket = 2040
	length := uint32(len(src))
	alignedLength := (length + 3) &^ 3
	if alignedLength < 0 || alignedLength > maxReadPacket {
		return errors.New("buffer length must be length in 0..2040")
	}
	assert := fn != FuncBackplane || (length <= 64 && (addr+length) <= 0x8000)
	if !assert {
		return errors.New("bad argument to ReadBytes")
	}
	padding := uint8(0)
	if fn == FuncBackplane {
		padding = 4
		if cap(src) < len(src)+4 {
			return errors.New("ReadBytes src arg requires more capacity for byte padding")
		}
		src = src[:len(src)+4]
	}
	cmd := make_cmd(false, true, fn, addr, length+uint32(padding))
	d.csLow()
	err := d.spiRead(cmd, src, uint8(padding))
	d.csHigh()
	return err
}

// spiRead performs the gSPI Read action.
func (d *Device) spiRead(cmd uint32, r []byte, padding uint8) error {
	buf := d.spibuf[:4]
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
	d.spibuf = [len(d.spibuf)]byte{} // zero out buffer.
	err = d.spi.Tx(buf[:], buf[:])
	status := Status(endian.Uint32(buf[:])) // !LE
	if err == nil && status.DataUnavailable() {
		err = ErrDataNotAvailable
		println("got data unavailable status:", status)
	}
	return err
}

//go:inline
func (d *Device) csHigh() {
	d.cs.High()
	machine.GPIO1.High()
}

//go:inline
func (d *Device) csLow() {
	d.cs.Low()
	machine.GPIO1.Low()
}

//go:inline
func (d *Device) responseDelay(padding uint8) {
	// Wait for response.
	for i := uint8(0); i < padding; i++ {
		d.spi.Transfer(0)
	}
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
func (d *Device) Write32S(fn Function, addr, val uint32) error {
	cmd := make_cmd(true, true, fn, addr, 4)
	buf := d.spibuf[:4]
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
	if err != nil || !d.enableStatusWord {
		d.csHigh()
		return err
	}
	// Read Status.
	d.spibuf = [len(d.spibuf)]byte{}
	d.spi.Tx(buf[:], buf[:])
	d.csHigh()
	status := Status(swap32(binary.BigEndian.Uint32(buf[:])))
	if status.DataUnavailable() {
		err = ErrDataNotAvailable
	}
	return err
}

// Read32S reads register and swaps big-endian 16bit word length. Used only at initialization.
func (d *Device) Read32S(fn Function, addr uint32) (uint32, error) {
	if fn == FuncBackplane {
		panic("backplane not implemented for rrS")
	}
	cmd := make_cmd(false, true, fn, addr, 4)
	buf := d.spibuf[:4]
	d.csLow()
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	binary.BigEndian.PutUint32(buf[:], swap32(cmd))
	d.spi.Tx(buf[:], nil)
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	d.responseDelay(0)
	err := d.spi.Tx(nil, buf[:])
	result := swap32(binary.BigEndian.Uint32(buf[:]))
	if err != nil || !d.enableStatusWord {
		d.csHigh()
		return result, err
	}
	// Read Status.
	d.spibuf = [len(d.spibuf)]byte{} // zero out buffer.
	d.spi.Tx(buf[:], buf[:])
	d.csHigh()
	status := Status(swap32(binary.BigEndian.Uint32(buf[:])))
	if status.DataUnavailable() {
		err = ErrDataNotAvailable
		// println("got data unavailable status:", status)
	}
	return result, err
}
