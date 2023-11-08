//go:build pico && cy43nopio

package cyrw

import (
	"encoding/binary"
	"machine"

	"github.com/soypat/cyw43439"
	"tinygo.org/x/drivers"
)

type spibus struct {
	cs  OutputPin
	spi drivers.SPI
	// Low level SPI buffers for readn and writen.
	wbuf   [1]uint32
	rbuf   [1]uint32
	status uint32
}

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
	bus := spibus{
		cs:  CS.Set,
		spi: spi,
	}
	return New(WL_REG_ON.Set, CS.Set, bus)
}

const (
	sharedDATA = true
	sdPin      = machine.GPIO24
)

var _busOrder = binary.LittleEndian

func (d *spibus) cmd_read(cmd uint32, buf []uint32) (status uint32, err error) {
	d.csEnable(true)
	if sharedDATA {
		sdPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	d.transfer(cmd)
	if sharedDATA {
		sdPin.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	for i := range buf {
		buf[i], err = d.transfer(0)
	}
	d.status, _ = d.transfer(0)
	d.csEnable(false)
	printLowLevelTx(cmd, buf[:])
	return d.status, err
}

func (d *spibus) cmd_write(buf []uint32) (status uint32, err error) {
	// TODO(soypat): add cmd as argument and remove copies elsewhere?
	d.csEnable(true)
	if sharedDATA {
		sdPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	for i := range buf {
		d.transfer(buf[i])
	}
	if sharedDATA {
		sdPin.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	d.status, _ = d.transfer(0)
	d.csEnable(false)
	printLowLevelTx(buf[0], buf[1:])
	return d.status, err
}

func (d *spibus) transfer(c uint32) (uint32, error) {
	var busOrder = binary.BigEndian
	wbuf := u32AsU8(d.wbuf[:1])
	busOrder.PutUint32(wbuf[:4], c)
	rbuf := u32AsU8(d.rbuf[:1])
	for i := range wbuf {
		rbuf[i], _ = d.spi.Transfer(wbuf[i])
	}
	return busOrder.Uint32(rbuf[:]), nil
}

func (d *spibus) csEnable(b bool) {
	d.cs(!b)
	machine.GPIO1.Set(!b) // When mocking.
}

func (d *spibus) Status() Status {
	return Status(d.status)
}
