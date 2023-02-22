/*
# Notes on Endianness.

Endianness is the order or sequence of bytes of a word of digital data in computer memory.

  - A big-endian system stores the most significant byte of a word at the
    smallest memory address and the least significant byte at the largest.
  - A little-endian system, in contrast, stores the least-significant byte
    at the smallest address.

Endianness may also be used to describe the order in which the bits are
transmitted over a communication channel

  - big-endian in a communications channel transmits the most significant bits first

When CY43439 boots it is in:
  - Little-Endian byte order
  - 16 bit word length mode
  - Big-Endian bit order (most common in SPI and other protocols)
*/
package cyw43439

import (
	"errors"
	"machine"
	"time"

	"tinygo.org/x/drivers"
)

func PicoWSpi(delay uint32) (spi *SPIbb, cs, wlRegOn, irq machine.Pin) {
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
	spi = &SPIbb{
		SCK:   CLK,
		SDI:   DATA_IN,
		SDO:   DATA_OUT,
		Delay: delay,
	}
	return spi, CS, WL_REG_ON, IRQ
}

type Dev struct {
	spi drivers.SPI
	// Chip select pin. Driven LOW during SPI transaction.

	// SPI chip select. Low means SPI ready to send/receive.
	cs machine.Pin
	// WL_REG_ON pin enables wifi interface.
	wlRegOn  machine.Pin
	irq      machine.Pin
	sharedSD machine.Pin
	//	 These values are fix for device F1 buffer overflow problem:
	lastSize               int
	lastHeader             [2]uint32
	currentBackplaneWindow uint32
	lastBackplaneWindow    uint32
	ResponseDelayByteCount uint8
	// Max packet size is 2048 bytes.
	sdpcmTxSequence       uint8
	sdpcmLastBusCredit    uint8
	wlanFlowCtl           uint8
	sdpcmRequestedIoctlID uint16
	buf                   [2048]byte
}

type Config struct {
}

func NewDev(spi drivers.SPI, cs, wlRegOn, irq, sharedSD machine.Pin) *Dev {
	SD := machine.NoPin
	if sharedDATA && sharedSD != machine.NoPin {
		SD = sharedSD // Pico W special case.
	}
	return &Dev{
		spi:                    spi,
		cs:                     cs,
		wlRegOn:                wlRegOn,
		sharedSD:               SD,
		ResponseDelayByteCount: 0,
	}
}

func (d *Dev) Init() (err error) {
	/*
		To initiate communication through the gSPI after power-up, the host
		needs to bring up the WLAN chip by writing to the wake-up WLAN
		register bit. Writing a 1 to this bit will start up the necessary
		crystals and PLLs so that the CYW43439 is ready for data transfer. The
		device can signal an interrupt to the host indicating that the device
		is awake and ready. This procedure also needs to be followed for
		waking up the device in sleep mode. The device can interrupt the host
		using the WLAN IRQ line whenever it has any information to
		pass to the host. On getting an interrupt, the host needs to read the
		interrupt and/or status register to determine the cause of the
		interrupt and then take necessary actions.
	*/
	d.GPIOSetup()

	d.wlRegOn.High() //
	// After power-up, the gSPI host needs to wait 50 ms for the device to be out of reset.
	// time.Sleep(60 * time.Millisecond) // it's actually slightly more than 50ms, including VDDC and POR startup.
	// For this, the host needs to poll with a read command
	// to F0 address 0x14. Address 0x14 contains a predefined bit pattern.
	startPoll := time.Now()
	time.Sleep(50 * time.Millisecond)
	var got uint32
	// Little endian test address values.
	for got != TestPattern {
		time.Sleep(time.Millisecond)
		got, err = d.Read32S(FuncBus, AddrTest)
		if err != nil {
			return err
		}
	}
	if got != TestPattern && time.Since(startPoll) > pollLimit {
		return errors.New("poll failed")
	}
	// Address 0x0000 registers.
	const (
		// 0=16bit word, 1=32bit word transactions.
		WordLengthPos = 0
		// Set to 1 for big endian words.
		EndianessBigPos = 1 // 30
		HiSpeedModePos  = 4
		InterruptPolPos = 5
		WakeUpPos       = 7

		ResponseDelayPos       = 0x1*8 + 0
		StatusEnablePos        = 0x2*8 + 0
		InterruptWithStatusPos = 0x2*8 + 1
		// 132275 is Pico-sdk's default value.
		setupValue = (1 << WordLengthPos) | (0 << EndianessBigPos) | (1 << HiSpeedModePos) |
			(1 << InterruptPolPos) | (1 << WakeUpPos) | (0x4 << ResponseDelayPos) |
			(1 << StatusEnablePos) | (1 << InterruptWithStatusPos)
	)
	// Write wake-up bit, switch to 32 bit SPI, and keep default interrupt polarity.
	err = d.Write32S(FuncBus, AddrBusControl, setupValue) // Last use of a swap writer/reader.
	// err = d.RegisterWriteUint32(FuncBus, 0x0, setupValue)
	if err != nil {
		return err
	}
	const WHD_BUS_SPI_BACKPLANE_READ_PADD_SIZE = 4
	err = d.Write8(FuncBus, AddrRespDelayF1, WHD_BUS_SPI_BACKPLANE_READ_PADD_SIZE)
	if err != nil {
		return err
	}
	// Make sure error interrupt bits are clear
	const (
		dataUnavailable = 0x1
		commandError    = 0x8
		dataError       = 0x10
		f1Overflow      = 0x80
		value           = dataUnavailable | commandError | dataError | f1Overflow
	)
	err = d.Write8(FuncBus, AddrInterrupt, value)
	if err != nil {
		return err
	}
	return nil

	// TODO: For when we are ready to download firmware.
	const (
		SDIO_CHIP_CLOCK_CSR  = 0x1000e
		SBSDIO_ALP_AVAIL_REQ = 0x8
		SBSDIO_ALP_AVAIL     = 0x40
	)
	d.Write8(FuncBackplane, SDIO_CHIP_CLOCK_CSR, SBSDIO_ALP_AVAIL_REQ)
	for i := 0; i < 10; i++ {
		reg, err := d.Read8(FuncBackplane, SDIO_CHIP_CLOCK_CSR)
		if err != nil {
			return err
		}
		if reg&SBSDIO_ALP_AVAIL != 0 {
			goto alpset
		}
		time.Sleep(time.Millisecond)
	}
	return errors.New("timeout waiting for ALP to be set")

alpset:
	d.Write8(FuncBackplane, SDIO_CHIP_CLOCK_CSR, 0)

	status, err := d.GetStatus()
	println("init status on end:", status)
	return err
}

func (d *Dev) GetStatus() (Status, error) {
	busStatus, err := d.Read32(FuncBus, AddrStatus)
	return Status(busStatus), err
}

func (d *Dev) Reset() {
	d.wlRegOn.Low()
	time.Sleep(20 * time.Millisecond)
	d.wlRegOn.High()
	time.Sleep(250 * time.Millisecond)
	// d.irq.Configure(machine.PinConfig{Mode: machine.PinInput})
}

//go:inline
func (d *Dev) GPIOSetup() {
	d.wlRegOn.Configure(machine.PinConfig{Mode: machine.PinOutput})
	if sharedDATA {
		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
		d.sharedSD.Low()
	}
	d.cs.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.GPIO1.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.csHigh()
}
