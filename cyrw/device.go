package cyrw

import (
	"errors"
	"runtime"
	"sync"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439/whd"
)

type outputPin func(bool)

func DefaultWifiConfig() Config {
	return Config{
		Firmware: wifiFW2,
		CLM:      clmFW,
	}
}

// type OutputPin func(bool)
type Device struct {
	mu              sync.Mutex
	pwr             outputPin
	lastStatusGet   time.Time
	spi             spibus
	log             logstate
	backplaneWindow uint32
	ioctlID         uint16
	sdpcmSeq        uint8
	sdpcmSeqMax     uint8
	mac             [6]byte
	eventmask       eventMask
	// uint32 buffers to ensure alignment of buffers.
	rwBuf         [2]uint32        // rwBuf used for read* and write* functions.
	_sendIoctlBuf [2048 / 4]uint32 // _sendIoctlBuf used only in sendIoctl and tx.
	_iovarBuf     [2048 / 4]uint32 // _iovarBuf used in get_iovar* and set_iovar* calls.
	_rxBuf        [2048 / 4]uint32 // Used in check_status->rx calls and handle_irq.
	// We define headers in the Device struct to alleviate stack growth. Also used along with _sendIoctlBuf
	lastSDPCMHeader whd.SDPCMHeader
	auxCDCHeader    whd.CDCHeader
	auxBDCHeader    whd.BDCHeader
	rcvEth          func([]byte) error
	logger          *slog.Logger
}

func New(pwr, cs outputPin, spi spibus) *Device {
	d := &Device{
		pwr:         pwr,
		spi:         spi,
		sdpcmSeqMax: 1,
	}
	return d
}

type Config struct {
	Firmware string
	CLM      string
	Logger   *slog.Logger
}

func (d *Device) Init(cfg Config) (err error) {
	d.lock()
	defer d.unlock()
	d.logger = cfg.Logger
	d.info("Init:start")

	// Reference: https://github.com/embassy-rs/embassy/blob/6babd5752e439b234151104d8d20bae32e41d714/cyw43/src/runner.rs#L76
	err = d.initBus()
	if err != nil {
		return errjoin(errors.New("failed to init bus"), err)
	}
	d.backplaneWindow = 0xaaaa_aaaa
	d.write8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR, 0x08) // BACKPLANE_ALP_AVAIL_REQ
	for {
		got, _ := d.read8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR)
		if got&0x40 != 0 {
			break // ALP available-> clock OK.
		}
	}

	chip_id, _ := d.bp_read16(0x1800_0000)

	// Upload firmware.
	err = d.core_disable(whd.CORE_WLAN_ARM)
	if err != nil {
		return err
	}
	err = d.core_reset(whd.CORE_SOCRAM, false)
	if err != nil {
		return err
	}
	d.bp_write32(whd.SOCSRAM_BASE_ADDRESS+0x10, 3)
	d.bp_write32(whd.SOCSRAM_BASE_ADDRESS+0x44, 0)

	d.debug("flashing firmware", slog.Uint64("chip_id", uint64(chip_id)), slog.Int("fwlen", len(cfg.Firmware)))
	var ramAddr uint32 // Start at ATCM_RAM_BASE_ADDRESS = 0.
	err = d.bp_writestring(ramAddr, cfg.Firmware)
	if err != nil {
		return err
	}

	// Load NVRAM
	const chipRAMSize = 512 * 1024
	nvramLen := align(uint32(len(nvram43439)), 4)
	d.debug("flashing nvram")
	err = d.bp_writestring(ramAddr+chipRAMSize-4-nvramLen, nvram43439)
	if err != nil {
		return err
	}
	nvramLenWords := nvramLen / 4
	nvramLenMagic := ((^nvramLenWords) << 16) | nvramLenWords
	d.bp_write32(ramAddr+chipRAMSize-4, nvramLenMagic)

	// Start core.
	err = d.core_reset(whd.CORE_WLAN_ARM, false)
	if err != nil {
		return err
	}
	if !d.core_is_up(whd.CORE_WLAN_ARM) {
		return errors.New("core not up after reset")
	}
	d.debug("core up")
	deadline := time.Now().Add(20 * time.Millisecond)
	for {
		got, _ := d.read8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR)
		if got&0x80 != 0 {
			break
		}
		if time.Since(deadline) >= 0 {
			return errors.New("timeout waiting for chip clock")
		}
		runtime.Gosched()
	}

	// "Set up the interrupt mask and enable interrupts"
	d.write16(FuncBus, whd.SPI_INTERRUPT_ENABLE_REGISTER, whd.F2_PACKET_AVAILABLE)

	// ""Lower F2 Watermark to avoid DMA Hang in F2 when SD Clock is stopped.""
	// "Sounds scary..."
	// yea it does
	const REG_BACKPLANE_FUNCTION2_WATERMARK = 0x10008
	d.write8(FuncBackplane, REG_BACKPLANE_FUNCTION2_WATERMARK, 32)

	// Wait for wifi startup.
	deadline = time.Now().Add(100 * time.Millisecond)
	for !d.status().F2RxReady() {
		if time.Since(deadline) >= 0 {
			return errors.New("wifi startup timeout")
		}
		runtime.Gosched()
	}

	// Clear pulls.
	d.write8(FuncBackplane, whd.SDIO_PULL_UP, 0)
	d.read8(FuncBackplane, whd.SDIO_PULL_UP)

	err = d.log_init()
	if err != nil {
		return err
	}
	d.log_read()
	d.debug("base init done")
	if cfg.CLM == "" {
		return nil
	}

	// Starting polling to simulate hw interrupts
	// go d.irqPoll()

	err = d.initControl(cfg.CLM)
	if err != nil {
		return err
	}

	return d.set_power_management(PowerSave)
}

func (d *Device) GPIOSet(wlGPIO uint8, value bool) (err error) {
	d.info("GPIOSet", slog.Uint64("wlGPIO", uint64(wlGPIO)), slog.Bool("value", value))
	if wlGPIO >= 3 {
		return errors.New("gpio out of range")
	}
	val0 := uint32(1) << wlGPIO
	val1 := b2u32(value) << wlGPIO
	d.lock()
	defer d.unlock()
	return d.set_iovar2("gpioout", whd.IF_STA, val0, val1)
}

// RecvEthHandle sets handler for receiving Ethernet pkt
// If set to nil then incoming packets are ignored.
func (d *Device) RecvEthHandle(handler func(pkt []byte) error) {
	d.lock()
	defer d.unlock()
	d.rcvEth = handler
}

// SendEth sends an Ethernet packet over the current interface.
func (d *Device) SendEth(pkt []byte) error {
	d.lock()
	defer d.unlock()
	return d.tx(pkt)
}

// status gets gSPI last bus status or reads it from the device if it's stale, for some definition of stale.
func (d *Device) status() Status {
	// TODO(soypat): Are we sure we don't want to re-acquire status if it's been very long?
	sinceStat := time.Since(d.lastStatusGet)
	if sinceStat < 12*time.Microsecond {
		runtime.Gosched() // Probably in hot loop.
	} else {
		got, _ := d.read32(FuncBus, whd.SPI_STATUS_REGISTER) // Explicitly get Status.
		return Status(got)
	}
	return d.spi.Status()
}

func (d *Device) Reset() {
	d.pwr(false)
	time.Sleep(20 * time.Millisecond)
	d.pwr(true)
	time.Sleep(250 * time.Millisecond)
}

func (d *Device) getInterrupts() Interrupts {
	irq, err := d.read16(FuncBus, whd.SPI_INTERRUPT_REGISTER)
	if err != nil {
		return 0
	}
	return Interrupts(irq)
}

func (d *Device) lock()   { d.mu.Lock() }
func (d *Device) unlock() { d.mu.Unlock() }
