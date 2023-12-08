package cyw43439

import (
	"errors"
	"strings"

	"github.com/soypat/cyw43439/whd"
)

type Config struct {
	Firmware        string
	CLM             string
	EnableBluetooth bool
}

func DefaultConfig(enableBT bool) Config {
	var fw string
	var fwLen uint32
	if enableBT {
		panic("not implemented yet")
		// fw = wifibtFW[:wifibtFWLen]
	} else {
		fwLen = wifiFWLen
		fw = wifiFW[:]
	}
	clmPtr := align32(fwLen, 512)
	if clmPtr+clmLen > uint32(len(fw)) {
		panic("firmware slice too small for CLM")
	}
	return Config{
		Firmware:        fw[:fwLen],
		CLM:             fw[clmPtr : clmPtr+clmLen],
		EnableBluetooth: enableBT,
	}
}

const (
	// The CYW43439 on the Raspberry Pi Pico shares IRQ and both SPI MISO/MOSI on same line.
	sharedDATA        = true
	negative1  uint32 = 0xffffffff
)

type Function uint32

const (
	// All SPI-specific registers.
	FuncBus Function = 0b00
	// Registers and memories belonging to other blocks in the chip (64 bytes max).
	FuncBackplane Function = 0b01
	// DMA channel 1. WLAN packets up to 2048 bytes.
	FuncDMA1 Function = 0b10
	FuncWLAN          = FuncDMA1
	// DMA channel 2 (optional). Packets up to 2048 bytes.
	FuncDMA2 Function = 0b11
)

func (f Function) String() (s string) {
	switch f {
	case FuncBus:
		s = "bus"
	case FuncBackplane:
		s = "backplane"
	case FuncWLAN: // same as FuncDMA1
		s = "wlan"
	case FuncDMA2:
		s = "dma2"
	default:
		s = "unknown"
	}
	return s
}

// Status supports status notification to the host after a read/write
// transaction over gSPI. This status notification provides information
// about packet errors, protocol errors, available packets in the RX queue, etc.
// The status information helps reduce the number of interrupts to the host.
// The status-reporting feature can be switched off using a register bit,
// without any timing overhead.
type Status uint32

func (s Status) String() (str string) {
	if s == 0 {
		return "no status"
	}
	if s.HostCommandDataError() {
		str += "hostcmderr "
	}
	if s.DataUnavailable() {
		str += "dataunavailable "
	}
	if s.IsOverflow() {
		str += "overflow "
	}
	if s.IsUnderflow() {
		str += "underflow "
	}
	if s.F2PacketAvailable() || s.F3PacketAvailable() {
		str += "packetavail "
	}
	if s.F2RxReady() || s.F3RxReady() {
		str += "rxready "
	}
	return str
}

// DataUnavailable returns true if requested read data is unavailable.
func (s Status) DataUnavailable() bool { return s&1 != 0 }

// IsUnderflow returns true if FIFO underflow occurred due to current (F2, F3) read command.
func (s Status) IsUnderflow() bool { return s&(1<<1) != 0 }

// IsOverflow returns true if FIFO overflow occurred due to current (F1, F2, F3) write command.
func (s Status) IsOverflow() bool { return s&(1<<2) != 0 }

// F2Interrupt returns true if F2 channel interrupt set.
func (s Status) F2Interrupt() bool { return s&(1<<3) != 0 }

// F2RxReady returns true if F2 FIFO is ready to receive data (FIFO empty).
func (s Status) F2RxReady() bool { return s&(1<<5) != 0 }

// F3RxReady returns true if F3 FIFO is ready to receive data (FIFO empty).
func (s Status) F3RxReady() bool { return s&0x40 != 0 }

// HostCommandDataError TODO document.
func (s Status) HostCommandDataError() bool { return s&0x80 != 0 }

// GSPIPacketAvailable notifies there is a packet available over gSPI.
func (s Status) GSPIPacketAvailable() bool { return s&0x0100 != 0 }

// F2PacketAvailable returns true if Packet is available/ready in F2 TX FIFO.
func (s Status) F2PacketAvailable() bool { return s&(1<<8) != 0 }

// F3PacketAvailable returns true if Packet is available/ready in F3 TX FIFO.
func (s Status) F3PacketAvailable() bool { return s&0x00100000 != 0 }

// F2PacketAvailable returns F2 packet length.
func (s Status) F2PacketLength() uint16 {
	const mask = 1<<11 - 1
	return uint16(s>>9) & mask
}

// F3PacketAvailable returns F3 packet length.
func (s Status) F3PacketLength() uint16 {
	const mask = 1<<11 - 1
	return uint16(s>>21) & mask
}

type Interrupts uint16

func (Int Interrupts) IsBusOverflowedOrUnderflowed() bool {
	return Int&(whd.F2_F3_FIFO_RD_UNDERFLOW|whd.F2_F3_FIFO_WR_OVERFLOW|whd.F1_OVERFLOW) != 0
}

func (Int Interrupts) IsF2Available() bool {
	return Int&(whd.F2_PACKET_AVAILABLE) != 0
}

func GetCLM(firmware []byte) []byte {
	clmAddr := align32(uint32(len(firmware)), 512)
	if uint32(cap(firmware)) < clmAddr+clmLen {
		panic("firmware slice too small for CLM")
	}
	return firmware[clmAddr : clmAddr+clmLen]
}

//go:inline
func align32(val, align uint32) uint32 { return (val + align - 1) &^ (align - 1) }

var errFirmwareValidationFailed = errors.New("firmware validation failed")

func getFWVersion(src string) (string, error) {
	begin := strings.LastIndex(src, "Version: ")
	if begin == -1 {
		return "", errors.New("FW version not found")
	}
	end := strings.Index(src[begin:], "\x00")
	if end == -1 {
		return "", errors.New("FW version not found")
	}
	fwVersion := src[begin : begin+end]
	if verbose_debug {
		println("got version", fwVersion)
	}
	return fwVersion, nil
}

type _integer = interface {
	~int | ~uint16 | ~uint32 | ~uint64 | ~uint8
}

func max[T _integer](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func min[T _integer](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func (d *Device) lock() {
	d.hw.Lock()
}

func (d *Device) unlock() {
	d.hw.Unlock()
}
