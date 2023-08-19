//go:build pico

package cyrw

import (
	"encoding/binary"
	"machine"

	"github.com/soypat/cyw43439"
)

func DefaultNew() *Device {
	// Raspberry Pi Pico W pin definitions for the CY43439.
	const (
		WL_REG_ON = machine.GPIO23
		DATA_OUT  = machine.GPIO24
		DATA_IN   = machine.GPIO24
		IRQ       = machine.GPIO24 // AKA WL_HOST_WAKE
		CLK       = machine.GPIO29
		CS        = machine.GPIO25
	)
	OUT := machine.PinConfig{Mode: machine.PinOutput}
	// IN := machine.PinConfig{Mode: machine.PinInputPulldown}
	WL_REG_ON.Configure(OUT)
	DATA_OUT.Configure(OUT)
	CLK.Configure(OUT)
	CS.Configure(OUT)
	CS.High()
	spi := &cyw43439.SPIbb{
		SCK: CLK,
		SDI: DATA_OUT,
		SDO: DATA_IN,
	}
	const (
		mockSDI = machine.GPIO4
		mockCS  = machine.GPIO1
		mockSCK = machine.GPIO2
		mockSDO = machine.GPIO3
	)
	spi.MockTo = &cyw43439.SPIbb{
		SCK: mockSCK,
		SDI: mockSDI,
		SDO: mockSDO,
	}
	mockCS.Configure(OUT)
	mockCS.High()
	spi.Configure()
	return New(WL_REG_ON.Set, CS.Set, spi)
}

const (
	sharedDATA = true
	sdPin      = machine.GPIO24
)

// spiRead performs the gSPI Read action.
func (d *Device) spiRead(cmd uint32, r []byte, padding uint8) error {
	buf := d.spicmdBuf[:4]
	if sharedDATA {
		sdPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	binary.LittleEndian.PutUint32(buf[:], cmd) // !LE
	d.spi.Tx(buf[:], nil)
	if sharedDATA {
		sdPin.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	d.responseDelay(padding) // Needed due to response delay.

	return d.spi.Tx(nil, r)
}

// spiWrite performs the gSPI Write action. Does not control CS pin.
func (d *Device) spiWrite(cmd uint32, w []byte) error {
	if sharedDATA {
		sdPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	buf := d.spicmdBuf[:4]
	binary.LittleEndian.PutUint32(buf[:], cmd) // !LE
	err := d.spi.Tx(buf[:], nil)
	if err != nil {
		return err
	}
	return d.spi.Tx(w, nil)
}

// Write32S writes register and swaps big-endian 16bit word length. Used only at initialization.
func (d *Device) Write32S(fn Function, addr, val uint32) error {
	if sharedDATA {
		sdPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	cmd := cmd_word(true, true, fn, addr, 4)
	buf := d.spicmdBuf[:4]
	d.csEnable(true)
	binary.BigEndian.PutUint32(buf[:], swap16(cmd))
	err := d.spi.Tx(buf[:], nil)
	if err != nil {
		d.csEnable(false)
		return err
	}
	binary.BigEndian.PutUint32(buf[:], swap16(val))
	err = d.spi.Tx(buf[:], nil)
	d.csEnable(false)
	return err
}

// Read32S reads register and swaps big-endian 16bit word length. Used only at initialization.
func (d *Device) Read32S(fn Function, addr uint32) (uint32, error) {
	if sharedDATA {
		sdPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}

	cmd := cmd_word(false, true, fn, addr, 4)
	buf := d.spicmdBuf[:4]
	d.csEnable(true)
	binary.BigEndian.PutUint32(buf[:], swap16(cmd))
	d.spi.Tx(buf[:], nil)
	if sharedDATA {
		sdPin.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	err := d.spi.Tx(nil, buf[:])
	result := swap16(binary.BigEndian.Uint32(buf[:]))
	d.csEnable(false)
	return result, err
}

func (d *Device) csEnable(b bool) {
	d.cs(!b)
	machine.GPIO1.Set(!b) // When mocking.
}
