package cyrw

import (
	"errors"
	"strings"

	"github.com/soypat/cyw43439/whd"
	"golang.org/x/exp/constraints"
)

var ErrDataNotAvailable = errors.New("requested data not available")

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

func (d *Device) GetStatus() (Status, error) {
	busStatus, err := d.read32(FuncBus, whd.SPI_STATUS_REGISTER)
	// d.debug("GetStatus", slog.String("stat", Status(busStatus).String()))
	return Status(busStatus), err
}

type Interrupts uint16

func (Int Interrupts) IsBusOverflowedOrUnderflowed() bool {
	return Int&(whd.F2_F3_FIFO_RD_UNDERFLOW|whd.F2_F3_FIFO_WR_OVERFLOW|whd.F1_OVERFLOW) != 0
}

func (Int Interrupts) IsF2Available() bool {
	return Int&(whd.F2_PACKET_AVAILABLE) != 0
}

func (Int Interrupts) IsDataAvailable() bool {
	return Int&(whd.DATA_UNAVAILABLE) == 0
}

func (Int Interrupts) String() (s string) {
	if Int == 0 {
		return "no interrupts"
	}
	for i := 0; Int != 0; i++ {
		if Int&1 != 0 {
			s += irqmask(1<<i).String() + " "
		}
		Int >>= 1
	}
	return s
}

func GetCLM(firmware []byte) []byte {
	clmAddr := align(uint32(len(firmware)), 512)
	if uint32(cap(firmware)) < clmAddr+clmLen {
		panic("firmware slice too small for CLM")
	}
	return firmware[clmAddr : clmAddr+clmLen]
}

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
	// if verbose_debug {
	// 	println("got version", fwVersion)
	// }
	return fwVersion, nil
}

// errjoion returns an error that wraps the given errors.
// Any nil error values are discarded.
// errjoion returns nil if every value in errs is nil.
// The error formats as the concatenation of the strings obtained
// by calling the Error method of each element of errs, with a newline
// between each string.
//
// A non-nil error returned by errjoion implements the Unwrap() []error method.
func errjoin(errs ...error) error {
	n := 0
	for _, err := range errs {
		if err != nil {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	e := &joinError{
		errs: make([]error, 0, n),
	}
	for _, err := range errs {
		if err != nil {
			e.errs = append(e.errs, err)
		}
	}
	return e
}

//go:generate stringer -type=irqmask -output=interrupts_string.go -trimprefix=irq
type irqmask uint16

const (
	irqDATA_UNAVAILABLE        irqmask = 0x0001 // Requested data not available; Clear by writing a "1"
	irqF2_F3_FIFO_RD_UNDERFLOW irqmask = 0x0002
	irqF2_F3_FIFO_WR_OVERFLOW  irqmask = 0x0004
	irqCOMMAND_ERROR           irqmask = 0x0008 // Cleared by writing 1.
	irqDATA_ERROR              irqmask = 0x0010 // Cleared by writing 1.
	irqF2_PACKET_AVAILABLE     irqmask = 0x0020
	irqF3_PACKET_AVAILABLE     irqmask = 0x0040
	irqF1_OVERFLOW             irqmask = 0x0080 // Due to last write. Bkplane has pending write requests.
	irqMISC_INTR0              irqmask = 0x0100
	irqMISC_INTR1              irqmask = 0x0200
	irqMISC_INTR2              irqmask = 0x0400
	irqMISC_INTR3              irqmask = 0x0800
	irqMISC_INTR4              irqmask = 0x1000
	irqF1_INTR                 irqmask = 0x2000
	irqF2_INTR                 irqmask = 0x4000
	irqF3_INTR                 irqmask = 0x8000
)

type powerManagementMode uint8

const (
	// Custom, officially unsupported mode. Use at your own risk.
	// All power-saving features set to their max at only a marginal decrease in power consumption
	// as oppposed to `Aggressive`.
	SuperSave = iota

	// Aggressive power saving mode.
	Aggressive

	// The default mode.
	PowerSave

	// Performance is prefered over power consumption but still some power is conserved as opposed to
	// `None`.
	Performance

	// Unlike all the other PM modes, this lowers the power consumption at all times at the cost of
	// a much lower throughput.
	ThroughputThrottling

	// No power management is configured. This consumes the most power.
	None
)

func (pm powerManagementMode) IsValid() bool {
	return pm <= None
}

func (pm powerManagementMode) String() string {
	switch pm {
	case SuperSave:
		return "SuperSave"
	case Aggressive:
		return "Aggressive"
	case PowerSave:
		return "PowerSave"
	case Performance:
		return "Performance"
	case ThroughputThrottling:
		return "ThroughputThrottling"
	case None:
		return "None"
	default:
		return "unknown"
	}
}
func (pm powerManagementMode) sleep_ret_ms() uint16 {
	switch pm {
	case SuperSave:
		return 2000
	case Aggressive:
		return 2000
	case PowerSave:
		return 200
	case Performance:
		return 20
	default: // ThroughputThrottling, None
		return 0 // value doesn't matter
	}
}

func (pm powerManagementMode) beacon_period() uint8 {
	switch pm {
	case SuperSave:
		return 255
	case Aggressive:
		return 1
	case PowerSave:
		return 1
	case Performance:
		return 1
	default: // ThroughputThrottling, None
		return 0 // value doesn't matter
	}
}

func (pm powerManagementMode) dtim_period() uint8 {
	switch pm {
	case SuperSave:
		return 255
	case Aggressive:
		return 1
	case PowerSave:
		return 1
	case Performance:
		return 1
	default: // ThroughputThrottling, None
		return 0 // value doesn't matter
	}
}

func (pm powerManagementMode) assoc() uint8 {
	switch pm {
	case SuperSave:
		return 255
	case Aggressive:
		return 10
	case PowerSave:
		return 10
	case Performance:
		return 1
	default: // ThroughputThrottling, None
		return 0 // value doesn't matter
	}
}

// mode returns the WHD's internal mode number.
func (pm powerManagementMode) mode() uint8 {
	switch pm {
	case ThroughputThrottling:
		return 1
	case None:
		return 0
	default:
		return 2
	}
}

type joinError struct {
	errs []error
}

func (e *joinError) Error() string {
	var b []byte
	for i, err := range e.errs {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, err.Error()...)
	}
	return string(b)
}

func (e *joinError) Unwrap() []error {
	return e.errs
}

type _uinteger = interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64 | uintptr
}

type _integer = interface {
	~int | _uinteger
}

// align rounds `val` up to nearest multiple of `align`.
func align[T constraints.Unsigned](val, align T) T {
	return (val + align - 1) &^ (align - 1)
}

func max[T constraints.Integer](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func min[T constraints.Integer](a, b T) T {
	if a < b {
		return a
	}
	return b
}

//go:inline
func b2u32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// swap16 swaps lowest 16 bits with highest 16 bits of a uint32.
//
//go:inline
func swap16(b uint32) uint32 {
	return (b >> 16) | (b << 16)
}

func swap16be(b uint32) uint32 {
	b = swap16(b)
	b0 := b & 0xff
	b1 := (b >> 8) & 0xff
	b2 := (b >> 16) & 0xff
	b3 := (b >> 24) & 0xff
	return b0<<24 | b1<<16 | b2<<8 | b3
}
