package cyw43439

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439/whd"
	"golang.org/x/exp/constraints"
)

// opMode determines the enabled modes of operation as a bitfield.
// To select multiple modes use OR operation:
//
//	mode := ModeWifi | ModeBluetooth
type opMode uint32

const (
	modeInit opMode = 1 << iota
	modeWifi
	modeBluetooth
)

// CYW43439 internal link state enum.
type linkState uint8

const (
	linkStateDown = iota
	linkStateUpWaitForSSID
	linkStateUp
	linkStateFailed
	linkStateAuthFailed
	linkStateWaitForReconnect
)

type outputPin func(bool)

func DefaultBluetoothConfig() Config {
	return Config{
		Firmware: embassyFWbt,
		CLM:      embassyFWclm,
		mode:     modeInit | modeBluetooth,
	}
}

func DefaultWifiBluetoothConfig() Config {
	return Config{
		Firmware: wifibtFW,
		CLM:      clmFW,
		mode:     modeInit | modeWifi | modeBluetooth,
	}
}

func DefaultWifiConfig() Config {
	return Config{
		Firmware: wifiFW2,
		CLM:      clmFW,
		mode:     modeInit | modeWifi,
	}
}

// type OutputPin func(bool)
type Device struct {
	mu              sync.Mutex
	pwr             outputPin
	lastStatusGet   time.Time
	spi             spibus
	log             logstate
	mode            opMode
	btaddr          uint32
	b2hReadPtr      uint32
	h2bWritePtr     uint32
	backplaneWindow uint32
	ioctlID         uint16
	sdpcmSeq        uint8
	sdpcmSeqMax     uint8
	mac             [6]byte
	eventmask       eventMask
	// uint32 buffers to ensure alignment of buffers.
	rwBuf         [2]uint32        // rwBuf used for read* and write* functions.
	_sendIoctlBuf [2048 / 4]uint32 // _sendIoctlBuf used only in sendIoctl and tx.
	_iovarBuf     [2048 / 4]uint32 // _iovarBuf used in get_iovar*, set_iovar* and write_backplane calls.
	_rxBuf        [2048 / 4]uint32 // Used in check_status->rx calls and handle_irq.
	// We define headers in the Device struct to alleviate stack growth. Also used along with _sendIoctlBuf
	lastSDPCMHeader whd.SDPCMHeader
	auxCDCHeader    whd.CDCHeader
	auxBDCHeader    whd.BDCHeader
	rcvEth          func([]byte) error
	rcvHCI          func([]byte) error
	logger          *slog.Logger
	_traceenabled   bool
	state           linkState
}

type Config struct {
	Firmware string
	CLM      string
	Logger   *slog.Logger
	// mode selects the enabled operation modes of the CYW43439.
	mode opMode
}

func (d *Device) Init(cfg Config) (err error) {
	if cfg.mode&(modeBluetooth|modeWifi) == 0 {
		return errors.New("no operation mode selected")
	}
	err = d.acquire(0)
	defer d.release()
	if err != nil {
		return err
	}
	d.info("Init:start")
	start := time.Now()
	// Reference: https://github.com/embassy-rs/embassy/blob/6babd5752e439b234151104d8d20bae32e41d714/cyw43/src/runner.rs#L76
	d.logger = cfg.Logger
	d._traceenabled = d.logger != nil && d.logger.Handler().Enabled(context.Background(), levelTrace)

	d.backplaneWindow = 0xaaaa_aaaa

	err = d.initBus(cfg.mode)
	if err != nil {
		return errjoin(errors.New("failed to init bus"), err)
	}

	d.debug("Init:alp")
	d.write8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR, whd.SBSDIO_ALP_AVAIL_REQ)

	// Check if we can set the bluetooth watermark during ALP.
	if d.bt_mode_enabled() {
		d.debug("Init:bt-watermark")
		d.write8(FuncBackplane, whd.REG_BACKPLANE_FUNCTION2_WATERMARK, 0x10)
		watermark, _ := d.read8(FuncBackplane, whd.REG_BACKPLANE_FUNCTION2_WATERMARK)
		if watermark != 0x10 {
			return errBTWatermark
		}
	}

	for {
		got, _ := d.read8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR)
		if got&whd.SBSDIO_ALP_AVAIL != 0 {
			break // ALP available-> clock OK.
		}
		time.Sleep(time.Millisecond)
	}

	// Clear request for ALP.
	d.write8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR, 0)

	chip_id, _ := d.bp_read16(0x1800_0000)

	// Upload firmware.
	err = d.core_disable(whd.CORE_WLAN_ARM)
	if err != nil {
		return err
	}
	err = d.core_disable(whd.CORE_SOCRAM) // TODO:is this needed if we reset right after?
	if err != nil {
		return err
	}
	err = d.core_reset(whd.CORE_SOCRAM, false)
	if err != nil {
		return err
	}

	// this is 4343x specific stuff: Disable remap for SRAM_3
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
	nvramLen := alignup(uint32(len(nvram43439)), 4)
	d.debug("flashing nvram")
	err = d.bp_writestring(ramAddr+chipRAMSize-4-nvramLen, nvram43439)
	if err != nil {
		return err
	}
	nvramLenWords := nvramLen / 4
	nvramLenMagic := ((^nvramLenWords) << 16) | nvramLenWords
	d.bp_write32(ramAddr+chipRAMSize-4, nvramLenMagic)

	// Start core.
	d.debug("Init:start-core")
	err = d.core_reset(whd.CORE_WLAN_ARM, false)
	if err != nil {
		return err
	}
	if !d.core_is_up(whd.CORE_WLAN_ARM) {
		return errors.New("core not up after reset")
	}
	d.debug("core up")

	// Wait until HT clock is available, takes about 29ms.
	deadline := time.Now().Add(1000 * time.Millisecond)
	for {
		got, _ := d.read8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR)
		if got&0x80 != 0 {
			break
		}
		if time.Since(deadline) >= 0 {
			return errors.New("timeout waiting for chip clock")
		}
		time.Sleep(time.Millisecond)
	}

	// "Set up the interrupt mask and enable interrupts"
	d.debug("Init:intr-mask")
	d.bp_write32(whd.SDIO_BASE_ADDRESS+whd.SDIO_INT_HOST_MASK, whd.I_HMB_SW_MASK)
	if d.bt_mode_enabled() {
		d.bp_write32(whd.SDIO_BASE_ADDRESS+whd.SDIO_INT_HOST_MASK, whd.I_HMB_FC_CHANGE)
	}

	d.write16(FuncBus, whd.SPI_INTERRUPT_ENABLE_REGISTER, whd.F2_PACKET_AVAILABLE)

	// ""Lower F2 Watermark to avoid DMA Hang in F2 when SD Clock is stopped.""
	// "Sounds scary..."
	// yea it does
	const REG_BACKPLANE_FUNCTION2_WATERMARK = 0x10008
	d.write8(FuncBackplane, REG_BACKPLANE_FUNCTION2_WATERMARK, whd.SPI_F2_WATERMARK)

	// Wait for F2 to be ready
	deadline = time.Now().Add(100 * time.Millisecond)
	for !d.status().F2RxReady() {
		if time.Since(deadline) >= 0 {
			return errors.New("wifi startup timeout")
		}
		time.Sleep(time.Millisecond)
	}

	// Clear pulls.
	d.write8(FuncBackplane, whd.SDIO_PULL_UP, 0)
	d.read8(FuncBackplane, whd.SDIO_PULL_UP)

	// Start HT clock.
	d.write8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR, whd.SBSDIO_HT_AVAIL_REQ)
	deadline = time.Now().Add(64 * time.Millisecond)
	for {
		got, err := d.read8(FuncBackplane, whd.SDIO_CHIP_CLOCK_CSR)
		if err != nil {
			return err
		}
		if got&0x80 != 0 {
			break
		} else if time.Since(deadline) > 0 {
			return errors.New("ht clock timeout")
		}
		time.Sleep(time.Millisecond)
	}

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

	err = d.set_power_management(pmPowerSave)
	d.state = linkStateDown
	d.info("Init:done", slog.Duration("took", time.Since(start)))
	return err
}

func (d *Device) GPIOSet(wlGPIO uint8, value bool) (err error) {
	d.info("GPIOSet", slog.Uint64("wlGPIO", uint64(wlGPIO)), slog.Bool("value", value))
	if wlGPIO >= 3 {
		return errors.New("gpio out of range")
	}
	val0 := uint32(1) << wlGPIO
	val1 := b2u32(value) << wlGPIO
	err = d.acquire(modeInit)
	defer d.release()
	if err != nil {
		return err
	}
	return d.set_iovar2("gpioout", whd.IF_STA, val0, val1)
}

// status gets gSPI last bus status or reads it from the device if it's stale, for some definition of stale.
func (d *Device) status() Status {
	// TODO(soypat): Are we sure we don't want to re-acquire status if it's been very long?
	sinceStat := time.Since(d.lastStatusGet)
	if sinceStat < 10*time.Microsecond {
		runtime.Gosched() // Probably in hot loop.
	} else {
		d.lastStatusGet = time.Now()
		got, _ := d.read32(FuncBus, whd.SPI_STATUS_REGISTER) // Explicitly get Status.
		return Status(got)
	}
	return d.spi.Status()
}

// Reset power-cycles the CYW43439 by turning WLREGON off and on
// and waiting the suggested amount of time for SPI bus to initialize.
// To use Device again Init should be called after a Reset.
func (d *Device) Reset() {
	d.acquire(0)
	d.reset()
	d.release()
}

func (d *Device) reset() {
	d.pwr(false)
	time.Sleep(20 * time.Millisecond)
	d.pwr(true)
	time.Sleep(250 * time.Millisecond) // Wait for bus to initialize.
	d.mode = 0
	d.backplaneWindow = 0
	d.state = 0
	d.ioctlID = 0
	d.sdpcmSeq = 0
	d.sdpcmSeqMax = 1
}

func (d *Device) getInterrupts() Interrupts {
	irq, err := d.read16(FuncBus, whd.SPI_INTERRUPT_REGISTER)
	if err != nil {
		return 0
	}
	return Interrupts(irq)
}

func (d *Device) acquire(mode opMode) error {
	d.mu.Lock()
	if mode != 0 && d.mode == 0 {
		return errors.New("device uninitialized")
	} else if mode&d.mode != mode {
		return errors.New("device mode uninitialized")
	}
	return nil
}

func (d *Device) release() {
	d.mu.Unlock()
}

// alignup rounds `val` up to nearest multiple of `alignup`. `alignup` must be a power of 2.
func alignup[T constraints.Unsigned](val, align T) T {
	return (val + align - 1) &^ (align - 1)
}

// align rounds `val` down to nearest multiple of `align`. `align` must be a power of 2.
func aligndown[T constraints.Unsigned](val, align T) T {
	return val &^ (align - 1)
}

// isaligned checks if `val` is wholly divisible by `align`. `align` must be a power of 2.
func isaligned[T constraints.Unsigned](val, align T) bool {
	return val&(align-1) == 0
}
