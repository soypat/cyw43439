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
	"unsafe"

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
	// The following variables are used to store the last SSID joined
	// first 4 bytes are length of SSID, stored in little endian.
	lastSSIDJoined [36]byte
	buf            [2048]byte
	auxbuf         [2048]byte
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

	if cfg.Firmware == nil {
		return nil
	} else if cfg.CLM == nil {
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
	var nvrambuf [1024]byte
	copy(nvrambuf[:], nvram43439)
	err = d.downloadResource(RamSize-4-wifinvramLen, nvrambuf[:len(nvram43439)])
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
	err = d.clmLoad(cfg.CLM)
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

	// Enable multicast ethernet frames on IPv4 mDNS MAC address
	// (01:00:5e:00:00:fb).
	// This is needed for mDNS to work.
	binary.LittleEndian.PutUint32(buf[:4], 1)
	buf[4] = 0x01
	buf[5] = 0x00
	buf[6] = 0x5e
	buf[7] = 0x00
	buf[8] = 0x00
	buf[9] = 0xfb
	for i := 0; i < 9*6; i++ {
		buf[10+i] = 0
	}
	err = d.WriteIOVarN("mcast_list", whd.WWD_STA_INTERFACE, buf[:4+10*6])
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

// reference: cyw43_ll_wifi_get_mac
func (d *Dev) GetMAC() (mac [6]byte, err error) {
	buf := d.offbuf()
	copy(buf, "cur_etheraddr\x00\x00\x00\x00\x00\x00\x00")
	err = d.doIoctl(whd.SDPCM_GET, whd.WWD_STA_INTERFACE, whd.WLC_GET_VAR, buf[:6+14])
	if err == nil {
		copy(mac[:], buf[:6])
	}
	return mac, nil
}

// reference: cyw43_ll_wifi_pm
func (d *Dev) wifiPM(pm, pm_sleep_ret, li_bcn, li_dtim, li_assoc uint32) error {
	// set some power saving parameters
	// PM1 is very aggressive in power saving and reduces wifi throughput
	// PM2 only saves power when there is no wifi activity for some time
	// Value passed to pm2_sleep_ret measured in ms, must be multiple of 10, between 10 and 2000

	if pm_sleep_ret < 1 {
		pm_sleep_ret = 1
	} else if pm_sleep_ret > 200 {
		pm_sleep_ret = 200
	}
	err := d.WriteIOVar("pm2_sleep_ret", whd.WWD_STA_INTERFACE, pm_sleep_ret*10)
	if err != nil {
		return err
	}

	// these parameters set beacon intervals and are used to reduce power consumption
	// while associated to an AP but not doing tx/rx
	// bcn_li_xxx is what the CYW43x will do; assoc_listen is what is sent to the AP
	// bcn_li_dtim==0 means use bcn_li_bcn
	err = d.WriteIOVar("bcn_li_bcn", whd.WWD_STA_INTERFACE, li_bcn)
	if err != nil {
		return err
	}
	err = d.WriteIOVar("bcn_li_dtim", whd.WWD_STA_INTERFACE, li_dtim)
	if err != nil {
		return err
	}
	err = d.WriteIOVar("assoc_listen", whd.WWD_STA_INTERFACE, li_assoc)
	if err != nil {
		return err
	}
	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_PM, pm)
	if err != nil {
		return err
	}

	// Set GMODE_AUTO
	buf := d.offbuf()
	binary.LittleEndian.PutUint32(buf[:4], 1)
	err = d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_GMODE, buf[:4])
	if err != nil {
		return err
	}
	binary.LittleEndian.PutUint32(buf[:4], 0) // any
	err = d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_BAND, buf[:4])
	return err
}

// reference: cyw43_ll_wifi_get_pm
func (d *Dev) wifiGetPM() (pm, pm_sleep_ret, li_bcn, li_dtim, li_assoc uint32, err error) {
	// TODO: implement
	pm_sleep_ret, err = d.ReadIOVar("pm2_sleep_ret", whd.WWD_STA_INTERFACE)
	if err != nil {
		goto reterr
	}
	li_bcn, err = d.ReadIOVar("bcn_li_bcn", whd.WWD_STA_INTERFACE)
	if err != nil {
		goto reterr
	}
	li_dtim, err = d.ReadIOVar("bcn_li_dtim", whd.WWD_STA_INTERFACE)
	if err != nil {
		goto reterr
	}
	li_assoc, err = d.ReadIOVar("assoc_listen", whd.WWD_STA_INTERFACE)
	if err != nil {
		goto reterr
	}
	pm, err = d.GetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_GET_PM)
	if err != nil {
		goto reterr
	}
	return pm, pm_sleep_ret, li_bcn, li_dtim, li_assoc, nil
reterr:
	return 0, 0, 0, 0, 0, err
}

// reference: cyw43_ll_wifi_scan
func (d *Dev) wifiScan(opts *whd.ScanOptions) error {
	opts.Version = 1 // ESCAN_REQ_VERSION
	opts.Action = 1  // WL_SCAN_ACTION_START
	for i := 0; i < len(opts.BSSID); i++ {
		opts.BSSID[i] = 0xff
	}
	opts.BSSType = 2 // WICED_BSS_TYPE_ANY
	opts.NProbes = -1
	opts.ActiveTime = -1
	opts.PassiveTime = -1
	opts.HomeTime = -1
	opts.ChannelNum = 0
	opts.ChannelList[0] = 0
	unsafePtr := unsafe.Pointer(opts)
	if uintptr(unsafePtr)&0x3 != 0 {
		return errors.New("opts not aligned to 4 bytes")
	}
	buf := (*[unsafe.Sizeof(*opts)]byte)(unsafePtr)
	err := d.WriteIOVarN("escan", whd.WWD_STA_INTERFACE, buf[:])
	return err
}

// reference: cyw43_ll_wifi_join
func (d *Dev) wifiJoin(ssid, key string, bssid *[6]byte, authType, channel uint32) (err error) {
	var buf [128]byte
	err = d.WriteIOVar("ampdu_ba_wsize", whd.WWD_STA_INTERFACE, 8)
	if err != nil {
		return err
	}
	if authType == negative1 {
		// Auto auth type.
		if key == "" {
			// No key given, assume this means open security
			authType = 0
		} else {
			// See WICED_SECURITY_WPA2_MIXED_PSK
			authType = whd.CYW43_AUTH_WPA2_MIXED_PSK
		}
	}

	var wpa_auth uint32
	if authType == whd.CYW43_AUTH_WPA2_AES_PSK || authType == whd.CYW43_AUTH_WPA2_MIXED_PSK {
		wpa_auth = whd.CYW43_WPA2_AUTH_PSK
	} else if authType == whd.CYW43_AUTH_WPA_TKIP_PSK {
		wpa_auth = whd.CYW43_WPA_AUTH_PSK
	} else {
		return errors.New("unsupported auth type")
	}
	Debug("Setting wsec=", authType&0xff)
	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_WSEC, uint32(authType)&0xff)
	if err != nil {
		return err
	}

	// Supplicant variable.
	wpaSup := b2u32(wpa_auth != 0)
	Debug("setting up sup_wpa=", wpaSup)
	err = d.WriteIOVar2("bsscfg:sup_wpa", whd.WWD_STA_INTERFACE, 0, wpaSup)
	if err != nil {
		return err
	}

	// set the EAPOL version to whatever the AP is using (-1).
	Debug("setting sup_wpa2_eapver=-1")
	err = d.WriteIOVar2("bsscfg:sup_wpa2_eapver", whd.WWD_STA_INTERFACE, 0, negative1)
	if err != nil {
		return err
	}

	// wwd_wifi_set_supplicant_eapol_key_timeout
	Debug("setting sup_wpa_tm=0x9c4")
	err = d.WriteIOVar2("bsscfg:sup_wpa_tmo", whd.WWD_STA_INTERFACE, 0, 0x9c4)
	if err != nil {
		return
	}

	if authType != 0 {
		// wwd_wifi_set_passphrase
		binary.LittleEndian.PutUint16(buf[:], uint16(len(key)))
		binary.LittleEndian.PutUint16(buf[2:], 1)
		copy(buf[4:], key)
		time.Sleep(2 * time.Millisecond) // Delay required to allow radio firmware to be ready to receive PMK and avoid intermittent failure

		Debug("setting sup_wpa_pmk ", len(key))
		err = d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_WSEC, buf[:68])
		if err != nil {
			return err
		}
	}

	// Set infrastructure mode.
	Debug("setting infra=1")
	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_INFRA, 1)
	if err != nil {
		return err
	}

	// Set auth type (open system).
	Debug("setting auth=0")
	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_AUTH, 0)
	if err != nil {
		return err
	}

	// Set WPA auth mode.
	Debug("setting wpa_auth=", wpa_auth)
	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_WPA_AUTH, wpa_auth)
	if err != nil {
		return err
	}

	// allow relevant events through:
	//  EV_SET_SSID=0
	//  EV_AUTH=3
	//  EV_DEAUTH_IND=6
	//  EV_DISASSOC_IND=12
	//  EV_LINK=16
	//  EV_PSK_SUP=46
	//  EV_ESCAN_RESULT=69
	//  EV_CSA_COMPLETE_IND=80
	/*
	   memcpy(buf, "\x00\x00\x00\x00" "\x49\x10\x01\x00\x00\x40\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00", 4 + 18);
	   cyw43_write_iovar_n(self, "bsscfg:event_msgs", 4 + 18, buf, WWD_STA_INTERFACE);
	*/

	// Set SSID.
	Debug("setting ssid=", ssid)
	binary.LittleEndian.PutUint32(d.lastSSIDJoined[:], uint32(len(ssid)))
	copy(d.lastSSIDJoined[4:], ssid)
	if bssid == nil {
		// Join SSID. Rejoin uses d.lastSSIDJoined.
		Debug("join SSID")
		return d.wifiRejoin()
	}
	// BSSID is not nil so join the AP.
	Debug("setting bssid=", bssid)
	for i := 0; i < 4+32+20+14; i++ {
		buf[i] = 0
	}
	copy(buf[:], d.lastSSIDJoined[:])
	// Scan parameters:
	buf[36] = 0                                        // Scan type
	binary.LittleEndian.PutUint32(buf[40:], negative1) // Nprobes.
	binary.LittleEndian.PutUint32(buf[44:], negative1) // Active time.
	binary.LittleEndian.PutUint32(buf[48:], negative1) // Passive time.
	binary.LittleEndian.PutUint32(buf[52:], negative1) // Home time.
	const (
		WL_CHANSPEC_BW_20       = 0x1000
		WL_CHANSPEC_CTL_SB_LLL  = 0x0000
		WL_CHANSPEC_CTL_SB_NONE = WL_CHANSPEC_CTL_SB_LLL
		WL_CHANSPEC_BAND_2G     = 0x0000
	)
	copy(buf[4+32+20:], bssid[:6])
	binary.LittleEndian.PutUint32(buf[4+32+20+8:], 1) // Channel spec number.
	chspec := uint16(channel) | WL_CHANSPEC_BW_20 | WL_CHANSPEC_CTL_SB_NONE | WL_CHANSPEC_BAND_2G
	binary.LittleEndian.PutUint16(buf[4+32+20+12:], chspec)

	// Join the AP.
	Debug("join AP")
	return d.WriteIOVarN("join", whd.WWD_STA_INTERFACE, buf[:4+32+20+14])
}

// reference: cyw43_ll_wifi_set_wpa_auth
func (d *Dev) setWPAAuth() error {
	return d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_WPA_AUTH, whd.CYW43_WPA_AUTH_PSK)
}

// reference: cyw43_ll_wifi_rejoin
func (d *Dev) wifiRejoin() error {
	return d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_SSID, d.lastSSIDJoined[:36])
}

// reference: cyw43_ll_wifi_ap_init
func (d *Dev) wifiAPInit(ssid, key string, auth, channel uint32) (err error) {
	buf := d.offbuf()

	// Get state of AP.
	// TODO: this can fail with sdpcm status = 0xffffffe2 (NOTASSOCIATED)
	// in such a case the AP is not up and we should not check the result
	copy(buf[:], "bss\x00")
	binary.LittleEndian.PutUint32(buf[4:], uint32(whd.WWD_AP_INTERFACE))
	err = d.doIoctl(whd.SDPCM_GET, whd.WWD_STA_INTERFACE, whd.WLC_GET_VAR, buf[:8])
	if err != nil {
		return err
	}
	res := binary.LittleEndian.Uint32(buf[:])
	if res != 0 {
		// AP is already up.
		return nil
	}

	// Set the AMPDU parameter for AP (window size = 2).
	err = d.WriteIOVar("ampdu_ba_wsize", whd.WWD_AP_INTERFACE, 2)
	if err != nil {
		return err
	}

	// Set SSID.
	binary.LittleEndian.PutUint32(buf, uint32(whd.WWD_AP_INTERFACE))
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(ssid)))
	for i := 0; i < 32; i++ {
		buf[8+i] = 0
	}
	copy(buf[8:], ssid)
	err = d.WriteIOVarN("bsscfg:ssid", whd.WWD_AP_INTERFACE, buf[:8+32])
	if err != nil {
		return err
	}
	// Set channel.
	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_CHANNEL, channel)
	if err != nil {
		return err
	}
	// Set Security type.
	err = d.WriteIOVar2("bsscfg:wsec", whd.WWD_STA_INTERFACE, uint32(whd.WWD_AP_INTERFACE), auth) // More confusing interface arguments.
	if err != nil {
		return err
	}
	if auth != whd.CYW43_AUTH_OPEN {
		// Set WPA/WPA2 auth parameters.
		var val uint16 = whd.CYW43_WPA_AUTH_PSK
		if auth != whd.CYW43_AUTH_WPA_TKIP_PSK {
			val |= whd.CYW43_WPA2_AUTH_PSK
		}
		err = d.WriteIOVar2("bsscfg:wpa_auth", whd.WWD_STA_INTERFACE, uint32(whd.WWD_AP_INTERFACE), uint32(val))
		if err != nil {
			return err
		}
		// Set password.
		binary.LittleEndian.PutUint16(buf, uint16(len(key)))
		binary.LittleEndian.PutUint16(buf[2:], 1)
		for i := 0; i < 64; i++ {
			buf[i] = 0
		}
		copy(buf[4:], key)
		time.Sleep(2 * time.Millisecond) // WICED has this.
		err = d.doIoctl(whd.SDPCM_SET, whd.WWD_AP_INTERFACE, whd.WLC_SET_WSEC_PMK, buf[:4+64])
		if err != nil {
			return err
		}
	}

	// Set GMode to auto (value of 1).
	err = d.SetIoctl32(whd.WWD_AP_INTERFACE, whd.WLC_SET_GMODE, 1)
	if err != nil {
		return err
	}
	// Set multicast tx rate to 11Mbps.
	const rate = 11000000 / 500000
	err = d.WriteIOVar("2g_mrate", whd.WWD_AP_INTERFACE, rate)
	if err != nil {
		return err
	}

	// Set DTIM period to 1.
	err = d.SetIoctl32(whd.WWD_AP_INTERFACE, whd.WLC_SET_DTIMPRD, 1)
	return err
}

// reference: cyw43_ll_wifi_ap_set_up
func (d *Dev) wifiAPSetUp(up bool) error {
	// This line is somewhat confusing. Both the AP and STA interfaces are passed in as arguments,
	// but the STA interface is the one used to set the AP interface up or down.
	return d.WriteIOVar2("bss", whd.WWD_STA_INTERFACE, uint32(whd.WWD_AP_INTERFACE), b2u32(up))
}

// reference: cyw43_ll_wifi_ap_get_stas
func (d *Dev) wifiAPGetSTAs(macs []byte) (stas uint32, err error) {
	buf := d.offbuf()
	copy(buf[:], "maxassoc\x00")
	binary.LittleEndian.PutUint32(buf[9:], uint32(whd.WWD_AP_INTERFACE))
	err = d.doIoctl(whd.SDPCM_GET, whd.WWD_STA_INTERFACE, whd.WLC_GET_VAR, buf[:9+4])
	if err != nil {
		return 0, err
	}
	maxAssoc := binary.LittleEndian.Uint32(buf[:])
	if macs == nil {
		// Return just the maximum number of STAs
		return maxAssoc, nil
	}
	// Return the maximum number of STAs and the MAC addresses of the STAs.
	lim := 4 + maxAssoc*6
	if lim > uint32(len(buf)) {
		lim = uint32(len(buf))
	}
	err = d.doIoctl(whd.SDPCM_GET, whd.WWD_AP_INTERFACE, whd.WLC_GET_ASSOCLIST, buf[:lim])
	if err != nil {
		return 0, err
	}
	stas = binary.LittleEndian.Uint32(buf[:])
	copy(macs[:], buf[4:4+stas*6])
	return stas, err
}
