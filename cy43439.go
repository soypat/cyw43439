package cyw43439

import (
	"encoding/binary"
	"machine"
	"time"

	"tinygo.org/x/drivers"
)

func PicoWSpi() (spi drivers.SPI, cs, wlRegOn, irq machine.Pin) {
	// Raspberry Pi Pico W pin definitions for the CY43439.
	const (
		WL_REG_ON = machine.GPIO23
		DATA_OUT  = machine.GPIO24
		DATA_IN   = machine.GPIO24
		IRQ       = machine.GPIO24
		CLK       = machine.GPIO29
		CS        = machine.GPIO25
	)
	// Need software spi implementation since Rx/Tx are on same pin.
	CS.Configure(machine.PinConfig{Mode: machine.PinOutput})
	CLK.Configure(machine.PinConfig{Mode: machine.PinOutput})
	// DATA_IN.Configure(machine.PinConfig{Mode: machine.PinInput})
	DATA_OUT.Configure(machine.PinConfig{Mode: machine.PinOutput})
	spi = &bbSPI{
		SCK:   CLK,
		SDI:   DATA_IN,
		SDO:   DATA_OUT,
		Delay: 1 << 10,
	}
	return spi, CS, WL_REG_ON, IRQ
}

type Dev struct {
	spi drivers.SPI
	// Chip select pin. Driven LOW during SPI transaction.
	lastSize               uint32
	lastHeader             [2]uint32
	currentBackplaneWindow uint32
	lastBackplaneWindow    uint32
	cs                     machine.Pin
	wlRegOn                machine.Pin
	irq                    machine.Pin
	sharedSD               machine.Pin
}

type Config struct {
}

func NewDev(spi drivers.SPI, cs, wlRegOn, irq machine.Pin) *Dev {
	SD := machine.NoPin
	if sharedDATA {
		SD = machine.GPIO24 // Pico W special case.
	}
	return &Dev{
		spi:      spi,
		cs:       cs,
		wlRegOn:  wlRegOn,
		sharedSD: SD,
	}
}

func (d *Dev) Init() error {
	d.gpioSetup()
	return nil
}

const (
	TestRegisterAddr              uint32        = 0x14
	TestRegisterExpectedValue     uint32        = 0xFEEDBEAD
	responseDelay                 time.Duration = 0 //20 * time.Microsecond
	backplaneFunction                           = 0
	whdBusSPIBackplaneReadPadding               = 4
	sharedDATA                                  = true
)

func (d *Dev) RegisterReadUint32(fn, reg uint32) (uint32, error) {
	val, err := d.readReg(fn, reg, 4)
	return uint32(val), err
}

func (d *Dev) RegisterReadUint16(fn, reg uint32) (uint16, error) {
	val, err := d.readReg(fn, reg, 2)
	return uint16(val), err
}

func (d *Dev) RegisterReadUint8(fn, reg uint32) (uint8, error) {
	val, err := d.readReg(fn, reg, 1)
	return uint8(val), err
}

func (d *Dev) readReg(fn, reg uint32, size int) (uint32, error) {
	var padding uint32
	if fn == backplaneFunction {
		padding = whdBusSPIBackplaneReadPadding
	}
	cmd := make_cmd(false, true, fn, reg, uint32(size)+padding)
	var buf [4 + whdBusSPIBackplaneReadPadding]byte
	err := d.SPIRead(cmd, buf[:4+padding])
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(buf[:4]), nil
}

func (d *Dev) RegisterWriteUint32(fn, reg, val uint32) error {
	return d.writeReg(fn, reg, val, 4)
}

func (d *Dev) RegisterWriteUint16(fn, reg uint32, val uint16) error {
	return d.writeReg(fn, reg, uint32(val), 2)
}

func (d *Dev) RegisterWriteUint8(fn, reg uint32, val uint8) error {
	return d.writeReg(fn, reg, uint32(val), 1)
}

func (d *Dev) writeReg(fn, reg, val, size uint32) error {
	var buf [4]byte
	cmd := make_cmd(true, true, fn, reg, size)
	binary.BigEndian.PutUint32(buf[:], val)
	if fn == backplaneFunction {
		d.lastSize = 8
		d.lastHeader[0] = cmd
		d.lastHeader[1] = val
		d.lastBackplaneWindow = d.currentBackplaneWindow
	}
	return d.SPIWrite(cmd, buf[:size])
}

func (d *Dev) SPIWriteRead(command uint32, r []byte) error {
	d.cs.Low()
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	err := d.spiWrite(command, nil)
	if err != nil {
		return err
	}
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinInput})
	}
	d.responseDelay()
	err = d.spi.Tx(nil, r)
	d.cs.High()
	return err
}

func (d *Dev) SPIRead(command uint32, r []byte) error {
	d.cs.Low()
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	err := d.spiWrite(command, nil)
	d.cs.High()
	if err != nil {
		return err
	}
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinInput})
	}
	d.cs.Low()
	d.responseDelay()
	err = d.spi.Tx(nil, r)
	d.cs.High()
	return err
}

func (d *Dev) SPIWrite(command uint32, w []byte) error {
	d.cs.Low()
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	err := d.spiWrite(command, w)
	d.cs.High()
	return err
}

//go:inline
func (d *Dev) spiWrite(command uint32, w []byte) error {
	d.spi.Transfer(byte(command >> (32 - 8)))
	d.spi.Transfer(byte(command >> (32 - 16)))
	d.spi.Transfer(byte(command >> (32 - 24)))
	_, err := d.spi.Transfer(byte(command))
	if len(w) == 0 || err != nil {
		return err
	}
	err = d.spi.Tx(w, nil)
	return err
}

//go:inline
func (d *Dev) responseDelay() {
	if responseDelay != 0 {
		// Wait for response.
		waitStart := time.Now()
		for time.Since(waitStart) < responseDelay {
			d.spi.Transfer(0)
		}
	}
}

func (d *Dev) Reset() {
	d.wlRegOn.Low()
	time.Sleep(20 * time.Millisecond)
	d.wlRegOn.High()
	time.Sleep(250 * time.Millisecond)
	// d.irq.Configure(machine.PinConfig{Mode: machine.PinInput})
}

func (d *Dev) gpioSetup() {
	d.wlRegOn.Configure(machine.PinConfig{Mode: machine.PinOutput})
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
		d.sharedSD.Low()
	}
	d.cs.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.cs.High()
}

func make_cmd(write, inc bool, fn uint32, addr uint32, sz uint32) uint32 {
	return b2i(write)<<31 | b2i(inc)<<30 | fn<<28 | (addr&0x1ffff)<<11 | sz
}

func b2i(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}
