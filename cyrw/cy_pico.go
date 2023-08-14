//go:build pico

package cyrw

import (
	"encoding/binary"
	"machine"
)

const (
	sharedDATA = true
	sdPin      = machine.GP27
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
	binary.BigEndian.PutUint32(buf[:], swap32(cmd))
	err := d.spi.Tx(buf[:], nil)
	if err != nil {
		d.csEnable(false)
		return err
	}
	binary.BigEndian.PutUint32(buf[:], swap32(val))
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
	binary.BigEndian.PutUint32(buf[:], swap32(cmd))
	d.spi.Tx(buf[:], nil)
	if sharedDATA {
		sdPin.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	err := d.spi.Tx(nil, buf[:])
	result := swap32(binary.BigEndian.Uint32(buf[:]))
	d.csEnable(false)
	return result, err
}
