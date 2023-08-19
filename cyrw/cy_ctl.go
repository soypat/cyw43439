package cyrw

import (
	"encoding/binary"
	"errors"
	"time"

	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/cyw43439/whd"
)

// File based on runner.rs

func (d *Device) initBus() error {
	d.Reset()
	retries := 128
	for {
		got := d.read32_swapped(whd.SPI_READ_TEST_REGISTER)
		if got == whd.TEST_PATTERN {
			break
		} else if retries <= 0 {
			return errors.New("spi test failed:" + hex32(got))
		}
		retries--
	}
	const TestPattern = 0x12345678
	const spiRegTestRW = 0x18
	d.write32_swapped(spiRegTestRW, TestPattern)
	got := d.read32_swapped(spiRegTestRW)
	if got != TestPattern {
		return errors.New("spi test failed:" + hex32(got) + " wanted " + hex32(TestPattern))
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
		// NOTE: embassy uses little endian words and StatusEnablePos.
		setupValue = (1 << WordLengthPos) | (1 << HiSpeedModePos) | (0 << EndianessBigPos) |
			(1 << InterruptPolPos) | (1 << WakeUpPos) |
			(1 << InterruptWithStatusPos) | (1 << StatusEnablePos)
	)
	val := d.read32_swapped(0)

	d.write32_swapped(whd.SPI_BUS_CONTROL, setupValue)
	got, err := d.read32(FuncBus, whd.SPI_READ_TEST_REGISTER)
	println("current bus ctl", hex32(val), "writing:", hex32(setupValue), " got:", hex32(got))
	if err != nil || got != whd.TEST_PATTERN {
		return errjoin(errors.New("spi RO test failed:"+hex32(got)), err)
	}

	d.write32(FuncBus, spiRegTestRW, ^uint32(whd.TEST_PATTERN))
	got, err = d.read32(FuncBus, spiRegTestRW)
	if err != nil || got != ^uint32(whd.TEST_PATTERN) {
		return errjoin(errors.New("spi RW test failed:"+hex32(got)), err)
	}
	return nil
}

func (d *Device) core_disable(coreID uint8) error {
	base := coreaddress(coreID)

	// Check if not already in reset.
	d.bp_read8(base + whd.AI_RESETCTRL_OFFSET) // Dummy read.
	r, _ := d.bp_read8(base + whd.AI_RESETCTRL_OFFSET)
	if r&whd.AIRC_RESET != 0 {
		return nil
	}

	d.bp_write8(base+whd.AI_IOCTRL_OFFSET, 0)
	d.bp_read8(base + whd.AI_IOCTRL_OFFSET) // Another dummy read.
	time.Sleep(time.Millisecond)

	d.bp_write8(base+whd.AI_RESETCTRL_OFFSET, whd.AIRC_RESET)
	r, _ = d.bp_read8(base + whd.AI_RESETCTRL_OFFSET)
	if r&whd.AIRC_RESET != 0 {
		return nil
	}
	return errors.New("core disable failed")
}

func (d *Device) core_reset(coreID uint8, coreHalt bool) error {
	err := d.core_disable(coreID)
	if err != nil {
		return err
	}
	var cpuhaltFlag uint8
	if coreHalt {
		cpuhaltFlag = whd.SICF_CPUHALT
	}
	base := coreaddress(coreID)
	const addr = 0x18103000 + whd.AI_IOCTRL_OFFSET
	d.bp_write8(base+whd.AI_IOCTRL_OFFSET, whd.SICF_FGC|whd.SICF_CLOCK_EN|cpuhaltFlag)
	d.bp_read8(base + whd.AI_IOCTRL_OFFSET) // Dummy read.

	d.bp_write8(base+whd.AI_RESETCTRL_OFFSET, 0)
	time.Sleep(time.Millisecond)

	d.bp_write8(base+whd.AI_IOCTRL_OFFSET, whd.SICF_CLOCK_EN|cpuhaltFlag)
	d.bp_read8(base + whd.AI_IOCTRL_OFFSET) // Dummy read.
	time.Sleep(time.Millisecond)
	return nil
}

// CoreIsActive returns if the specified core is not in reset.
// Can be called with CORE_WLAN_ARM and CORE_SOCRAM global constants.
// It may return true if communications are down (WL_REG_ON at low).
//
//	reference: device_core_is_up
func (d *Device) core_is_up(coreID uint8) bool {
	base := coreaddress(coreID)
	reg, _ := d.bp_read8(base + whd.AI_IOCTRL_OFFSET)
	if reg&(whd.SICF_FGC|whd.SICF_CLOCK_EN) != whd.SICF_CLOCK_EN {
		return false
	}
	reg, _ = d.bp_read8(base + whd.AI_RESETCTRL_OFFSET)
	return reg&whd.AIRC_RESET == 0
}

// coreaddress returns either WLAN=0x18103000  or  SOCRAM=0x18104000
//
//	reference: get_core_address
func coreaddress(coreID uint8) (v uint32) {
	switch coreID {
	case whd.CORE_WLAN_ARM:
		v = whd.WRAPPER_REGISTER_OFFSET + whd.WLAN_ARMCM3_BASE_ADDRESS
	case whd.CORE_SOCRAM:
		v = whd.WRAPPER_REGISTER_OFFSET + whd.SOCSRAM_BASE_ADDRESS
	default:
		panic("bad core id")
	}
	return v
}

func (d *Device) init_log() error {
	const (
		ramBase           = 0
		ramSize           = 512 * 1024
		socram_srmem_size = 64 * 1024
	)
	const addr = ramBase + ramSize - 4 + socram_srmem_size
	sharedAddr, err := d.bp_read32(addr)
	if err != nil {
		return err
	}
	shared := make([]byte, 32)
	d.bp_read(sharedAddr, shared)
	smem := decodeSharedMem(shared)
	d.log.addr = smem.console_addr + 8
	d.debug("log addr",
		slog.Uint64("sharedAddr", uint64(sharedAddr)),
		slog.Uint64("consoleAddr", uint64(d.log.addr)),
	)
	return nil
}

type sharedMem struct {
	flags            uint32
	trap_addr        uint32
	assert_exp_addr  uint32
	assert_file_addr uint32
	assert_line      uint32
	console_addr     uint32
	msgtrace_addr    uint32
	fwid             uint32
}

func decodeSharedMem(buf []byte) (s sharedMem) {
	s.flags = binary.LittleEndian.Uint32(buf[0:4])
	s.trap_addr = binary.LittleEndian.Uint32(buf[4:8])
	s.assert_exp_addr = binary.LittleEndian.Uint32(buf[8:12])
	s.assert_file_addr = binary.LittleEndian.Uint32(buf[12:16])
	s.assert_line = binary.LittleEndian.Uint32(buf[16:20])
	s.console_addr = binary.LittleEndian.Uint32(buf[20:24])
	s.msgtrace_addr = binary.LittleEndian.Uint32(buf[24:28])
	s.fwid = binary.LittleEndian.Uint32(buf[28:32])
	return s
}

type logstate struct {
	addr     uint32
	last_idx int
	buf      [256]byte
	bufcount int
}
