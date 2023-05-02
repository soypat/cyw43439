//go:build tinygo

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
	"encoding/binary"
	"errors"
	"fmt"
	"machine"
	"time"

	"github.com/soypat/cyw43439/whd"
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

// reference: cyw43_ll_t
type Dev struct {
	spi drivers.SPI
	// Chip select pin. Driven LOW during SPI transaction.

	// SPI chip select. Low means SPI ready to send/receive.
	cs machine.Pin
	// WL_REG_ON pin enables wifi interface.
	wlRegOn  machine.Pin
	irq      machine.Pin
	sharedSD machine.Pin
	busIsUp  bool
	//	 These values are fix for device F1 buffer overflow problem:
	lastSize               int
	lastHeader             [2]uint32
	currentBackplaneWindow uint32
	lastBackplaneWindow    uint32
	ResponseDelayByteCount uint8
	enableStatusWord       bool
	hadSuccesfulPacket     bool
	// Max packet size is 2048 bytes.
	sdpcmTxSequence       uint8
	sdpcmLastBusCredit    uint8
	wlanFlowCtl           uint8
	sdpcmRequestedIoctlID uint16
	lastInt               uint16
	buf                   [2048]byte
	auxbuf                [2048]byte
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
		enableStatusWord:       false,
		currentBackplaneWindow: 0,
		busIsUp:                false,
	}
}

// reference: int cyw43_ll_bus_init(cyw43_ll_t *self_in, const uint8_t *mac)
func (d *Dev) Init(cfg Config) (err error) {
	if cfg.MAC != nil && len(cfg.MAC) != 6 {
		return errors.New("bad MAC address")
	}
	err = validateFirmware(cfg.Firmware)
	if err != nil {
		return err
	}
	var reg8 uint8
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
	// After power-up, the gSPI host needs to wait 50 ms for the device to be out of reset.
	// time.Sleep(60 * time.Millisecond) // it's actually slightly more than 50ms, including VDDC and POR startup.
	// For this, the host needs to poll with a read command
	// to F0 address 0x14. Address 0x14 contains a predefined bit pattern.
	d.Reset()

	var got uint32
	// Little endian test address values.
	for i := 0; i < 10; i++ {
		time.Sleep(time.Millisecond)
		got, err = d.Read32S(FuncBus, whd.SPI_READ_TEST_REGISTER)
		if err != nil {
			return err
		}
		if got == whd.TEST_PATTERN {
			goto chipup
		}
	}
	return errors.New("poll failed")

chipup:
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
		setupValue = (1 << WordLengthPos) | (1 << HiSpeedModePos) | // This line OK.
			(1 << InterruptPolPos) | (1 << WakeUpPos) | (0x4 << ResponseDelayPos) |
			(1 << InterruptWithStatusPos) // | (1 << StatusEnablePos)
	)
	b := setupValue | (b2u32(endian == binary.LittleEndian) << EndianessBigPos)
	// Write wake-up bit, switch to 32 bit SPI, and keep default interrupt polarity.
	err = d.Write32S(FuncBus, whd.SPI_BUS_CONTROL, b) // Last use of a swap writer/reader.
	if err != nil {
		return err
	}
	got, err = d.Read32(FuncBus, whd.SPI_BUS_CONTROL) // print out data on register contents
	if err != nil {
		return err
	}
	if got != b&^(1<<10) {
		return fmt.Errorf("register write-readback failed on bus control. beware erratic behavior. got=%#x, expect:%#x", got, b&^(1<<10))
	}

	err = d.Write8(FuncBus, whd.SPI_RESP_DELAY_F1, whd.BUS_SPI_BACKPLANE_READ_PADD_SIZE)
	if err != nil {
		return err
	}
	if initReadback {
		d.Read8(FuncBus, whd.SPI_RESP_DELAY_F1)
	}
	// Make sure error interrupt bits are clear
	const (
		dataUnavailable = 0x1
		commandError    = 0x8
		dataError       = 0x10
		f1Overflow      = 0x80
		value           = dataUnavailable | commandError | dataError | f1Overflow
	)
	err = d.Write8(FuncBus, whd.SPI_INTERRUPT_REGISTER, value)
	if err != nil {
		return err
	}
	if initReadback {
		d.Read8(FuncBus, whd.SPI_INTERRUPT_REGISTER)
	}
	// Enable selection of interrupts:
	const wifiIntr = whd.F2_F3_FIFO_RD_UNDERFLOW | whd.F2_F3_FIFO_WR_OVERFLOW |
		whd.COMMAND_ERROR | whd.DATA_ERROR | whd.F2_PACKET_AVAILABLE | f1Overflow
	var intr uint16 = wifiIntr
	if cfg.EnableBluetooth {
		intr |= whd.F1_INTR
	}
	err = d.Write16(FuncBus, whd.SPI_INTERRUPT_ENABLE_REGISTER, intr)
	if err != nil {
		return err
	}
	Debug("backplane is ready")

	// Clear data unavailable error if there is any.
	// err = d.ClearInterrupts()
	// if err != nil {
	// 	return err
	// }
	d.Write8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR, whd.SBSDIO_ALP_AVAIL_REQ)
	for i := 0; i < 10; i++ {
		time.Sleep(time.Millisecond)
		reg8, err = d.Read8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR)
		if err != nil {
			return err
		}
		if reg8&whd.SBSDIO_ALP_AVAIL != 0 {
			goto alpset
		}
	}
	Debug("ALP not set: ", reg8)
	return errors.New("timeout waiting for ALP to be set")

alpset:
	Debug("ALP Set")
	// Clear request for ALP
	d.Write8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR, 0)
	if verbose_debug && validateDownloads {
		chipID, err := d.ReadBackplane(whd.CHIPCOMMON_BASE_ADDRESS, 2)
		if err != nil {
			return err
		}
		Debug("chip ID:", chipID)
	}

	if cfg.Firmware == "" {
		return nil
	} else if cfg.CLM == "" {
		return errors.New("CLM is nil but firmware not nil")
	}
	Debug("begin disabling cores")
	// Begin preparing for Firmware download.
	err = d.disableDeviceCore(whd.CORE_WLAN_ARM, false)
	if err != nil {
		return err
	}
	err = d.disableDeviceCore(whd.CORE_SOCRAM, false)
	if err != nil {
		return err
	}
	err = d.resetDeviceCore(whd.CORE_SOCRAM, false)
	if err != nil {
		return err
	}
	// 4343x specific stuff: disable remap for SRAM_3
	err = d.WriteBackplane(whd.SOCSRAM_BANKX_INDEX, 4, 0x3)
	if err != nil {
		return err
	}
	err = d.WriteBackplane(whd.SOCSRAM_BANKX_PDA, 4, 0)
	if err != nil {
		return err
	}
	Debug("Cores ready, start firmware download")

	err = d.downloadResource(0x0, cfg.Firmware)
	if err != nil {
		return err
	}

	const RamSize = (512 * 1024)
	wifinvramLen := align32(uint32(len(nvram43439)), 64)
	Debug("start nvram download")
	// var nvrambuf [1024]byte
	// copy(nvrambuf[:], nvram43439)
	err = d.downloadResource(RamSize-4-wifinvramLen, nvram43439)
	if err != nil {
		return err
	}
	sz := (^(wifinvramLen/4)&0xffff)<<16 | (wifinvramLen / 4)
	err = d.WriteBackplane(RamSize-4, 4, sz)
	if err != nil {
		return err
	}
	d.resetDeviceCore(whd.CORE_WLAN_ARM, false)
	if !d.CoreIsActive(whd.CORE_WLAN_ARM) {
		return errors.New("CORE_WLAN_ARM is not active after reset")
	}
	Debug("wlan core reset success")
	// Wait until HT clock is available.
	for i := 0; i < 1000; i++ {
		reg, _ := d.Read8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR)
		if reg&whd.SBSDIO_HT_AVAIL != 0 {
			goto htready
		}
		time.Sleep(time.Millisecond)
	}
	return errors.New("HT not ready")

htready:
	Debug("HT Ready")
	err = d.WriteBackplane(whd.SDIO_INT_HOST_MASK, 4, whd.I_HMB_SW_MASK)
	if err != nil {
		return err
	}

	// Lower F2 Watermark to avoid DMA Hang in F2 when SD Clock is stopped.
	err = d.Write8(FuncBackplane, whd.SDIO_FUNCTION2_WATERMARK, whd.SPI_F2_WATERMARK)
	if err != nil {
		return err
	}
	Debug("preparing F2")
	for i := 0; i < 1000; i++ {
		status, _ := d.GetStatus()
		if status.F2PacketAvailable() {
			goto f2ready
		}
		time.Sleep(time.Millisecond)
	}
	return errors.New("F2 not ready")

f2ready:
	// Use of KSO:
	Debug("preparing KSO")
	reg8, err = d.Read8(FuncBackplane, whd.SDIO_WAKEUP_CTRL)
	if err != nil {
		return err
	}
	reg8 |= (1 << 1) // SBSDIO_WCTRL_WAKE_TILL_HT_AVAIL
	d.Write8(FuncBackplane, whd.SDIO_WAKEUP_CTRL, reg8)
	d.Write8(FuncBus, whd.SDIOD_CCCR_BRCM_CARDCAP, whd.SDIOD_CCCR_BRCM_CARDCAP_CMD_NODEC)
	d.Write8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR, whd.SBSDIO_FORCE_HT)
	reg8, err = d.Read8(FuncBackplane, whd.SDIO_SLEEP_CSR) // read 0x03000000, reference reads 0x03800000
	if err != nil {
		return err
	}
	if reg8&whd.SBSDIO_SLPCSR_KEEP_SDIO_ON == 0 { // Does not execute.
		reg8 |= whd.SBSDIO_SLPCSR_KEEP_SDIO_ON
		d.Write8(FuncBackplane, whd.SDIO_SLEEP_CSR, reg8)
	}
	// Put SPI interface back to sleep.
	d.Write8(FuncBackplane, whd.SDIO_PULL_UP, 0xf)

	// Clear pad pulls
	err = d.Write8(FuncBackplane, whd.SDIO_PULL_UP, 0)
	if err != nil {
		return err
	}
	_, err = d.Read8(FuncBackplane, whd.SDIO_PULL_UP) // read 0x00008001, ref reads 0x0000c001
	if err != nil {
		return err
	}
	// Clear data unavailable error if there is any.
	Debug("clear interrupts")
	err = d.ClearInterrupts()
	if err != nil {
		return err
	}
	if verbose_debug {
		// This will be a non-zero value if save/restore is enabled
		d.ReadBackplane(whd.CHIPCOMMON_BASE_ADDRESS+0x508, 4)
	}

	Debug("prep bus wake")
	err = d.busSleep(false)
	if err != nil {
		return err
	}

	// Load CLM data. It's right after main firmware
	Debug("prepare to flash CLM")
	err = d.clmLoad([]byte(cfg.CLM))
	if err != nil {
		return err
	}
	Debug("final IOVar writes")
	err = d.WriteIOVar("bus:txglom", whd.WWD_STA_INTERFACE, 0)
	if err != nil {
		return err
	}
	err = d.WriteIOVar("apsta", whd.WWD_STA_INTERFACE, 1)
	if err != nil {
		return err
	}
	// var defaultMAC = [6]byte{0x00, 0xA0, 0x50, 0xb5, 0x59, 0x5e}
	if cfg.MAC == nil {
		// Do not check if MAC address is set in OTP.
		return nil
	}
	err = d.WriteIOVarN("cur_etheraddr", whd.WWD_STA_INTERFACE, cfg.MAC)
	return err
}

func (d *Dev) GetStatus() (Status, error) {
	busStatus, err := d.Read32(FuncBus, whd.SPI_STATUS_REGISTER)
	Debug("read SPI Bus status:", Status(busStatus).String())
	return Status(busStatus), err
}

func (d *Dev) ClearStatus() (Status, error) {
	busStatus, err := d.Read32(FuncBus, whd.SPI_STATUS_REGISTER)
	d.Write32(FuncBus, whd.SPI_STATUS_REGISTER, 0)
	Debug("read SPI Bus status:", Status(busStatus).String())
	return Status(busStatus), err
}

func (d *Dev) GetInterrupts() (Interrupts, error) {
	reg, err := d.Read16(FuncBus, whd.SPI_INTERRUPT_REGISTER)
	if err == nil {
		d.lastInt = reg
	}
	return Interrupts(reg), err
}

func (d *Dev) ClearInterrupts() error {
	const dataUnavail = 0x1
	spiIntStatus, err := d.Read16(FuncBus, whd.SPI_INTERRUPT_REGISTER)
	if err != nil || spiIntStatus&dataUnavail == 0 {
		return err // no flags to clear or error.
	}
	err = d.Write16(FuncBus, whd.SPI_INTERRUPT_REGISTER, dataUnavail)
	if err != nil {
		return err
	}
	spiIntStatus, err = d.Read16(FuncBus, whd.SPI_INTERRUPT_REGISTER)
	if err == nil && spiIntStatus&dataUnavail != 0 {
		err = errors.New("interrupt raised after clear or clear failed")
	}
	return err
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

func flushprint() {
	for machine.UART0.Bus.GetUARTFR_BUSY() != 0 {
	}
}

// reference: cyw43_ll_wifi_on
func (d *Dev) wifiOn(country uint32) error {
	buf := d.offbuf()
	copy(buf, "country\x00")
	binary.LittleEndian.PutUint32(buf[:8], country&0xff_ff)
	if country>>16 == 0 {
		binary.LittleEndian.PutUint32(buf[:12], 4294967295)
	} else {
		binary.LittleEndian.PutUint32(buf[:12], country>>16)
	}
	binary.LittleEndian.PutUint32(buf[:16], country&0xff_ff)
	err := d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_VAR, buf[:20])
	if err != nil {
		return err
	}
	time.Sleep(20 * time.Millisecond)

	// Set antenna to chip antenna
	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_ANTDIV, 0)
	if err != nil {
		return err
	}

	// Set some WiFi config
	err = d.WriteIOVar("bus:txglom", whd.WWD_STA_INTERFACE, 0) // Tx glomming off.
	if err != nil {
		return err
	}
	err = d.WriteIOVar("apsta", whd.WWD_STA_INTERFACE, 1) // apsta on.
	if err != nil {
		return err
	}
	err = d.WriteIOVar("ampdu_ba_wsize", whd.WWD_STA_INTERFACE, 8)
	if err != nil {
		return err
	}
	err = d.WriteIOVar("ampdu_mpdu", whd.WWD_STA_INTERFACE, 4)
	if err != nil {
		return err
	}
	err = d.WriteIOVar("ampdu_rx_factor", whd.WWD_STA_INTERFACE, 0)
	if err != nil {
		return err
	}

	// This delay is needed for the WLAN chip to do some processing, otherwise
	// SDIOIT/OOB WL_HOST_WAKE IRQs in bus-sleep mode do no work correctly.
	time.Sleep(150 * time.Millisecond) // TODO(soypat): Not critical: rewrite to only sleep if 150ms did not elapse since startup.
	const (
		msg    = "bsscfg:event_msgs\x00"
		msgLen = len(msg)
	)
	copy(buf, msg)
	for i := 0; i < 19; i++ {
		buf[22+i] = 0xff // Clear async events.
	}
	clrEv := func(buf []byte, i int) {
		buf[18+4+i/8] &= ^(1 << (i % 8))
	}
	clrEv(buf, 19)
	clrEv(buf, 20)
	clrEv(buf, 40)
	clrEv(buf, 44)
	clrEv(buf, 54)
	clrEv(buf, 71)

	err = d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_VAR, buf[:18+4+19])
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)

	// Set interface as "up".
	err = d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_UP, nil)
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return nil
}
