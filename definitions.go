package cyw43439

import (
	"errors"
	"net"
	"strconv"

	"github.com/soypat/cyw43439/whd"
)

const (
	verbose_debug     = true
	initReadback      = false
	validateDownloads = false
)

type Config struct {
	Firmware        string
	CLM             string
	MAC             net.HardwareAddr
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
		println(clmPtr+clmLen, uint32(len(fw)))
		panic("firmware too small to contain CLM")
	}
	return Config{
		Firmware:        fw[:fwLen],
		CLM:             fw[clmPtr : clmPtr+clmLen],
		MAC:             []byte{0xfe, 0xed, 0xde, 0xad, 0xbe, 0xef},
		EnableBluetooth: enableBT,
	}
}

const (
	sharedDATA = true
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

// GSPIPacketAvailable notifies there is a packet available over
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

func GetCLM(firmware string) string {
	clmAddr := align32(uint32(len(firmware)), 512)
	if uint32(len(firmware)) < clmAddr+clmLen {
		println(len(firmware), clmAddr, clmLen, clmAddr+clmLen)
		panic("firmware slice too small for CLM")
	}
	return firmware[clmAddr : clmAddr+clmLen]
}

//go:inline
func align32(val, align uint32) uint32 { return (val + align - 1) &^ (align - 1) }

var errFirmwareValidationFailed = errors.New("firmware validation failed")

var debugBuf [128]byte

// Debug prints out arguments. Keep in mind it needs primitive Go types to work.
// This means it will not print out user defined types, even if the underlying type is an integer.
// Nil arguments are omitted.
func Debug(a ...any) {
	if verbose_debug {
		for i, v := range a {
			printUi := false
			printSpace := true
			var ui uint64
			switch c := v.(type) {
			case string:
				print(c)
				printSpace = len(c) > 0 && c[len(c)-1] != '='
			case int:
				print(c)
			case uint8:
				printUi = true
				ui = uint64(c)
			case uint16:
				printUi = true
				ui = uint64(c)
			case uint32:
				printUi = true
				ui = uint64(c)
			case bool:
				print(c)
			case error:
				if c == nil {
					print("err=<nil>")
				} else {
					print("err=\"")
					print(c.Error())
					print("\"")
				}
			case nil:
				// probably an error type.
				continue
			default:
				print("<unknown type>")
			}
			if printUi {
				debugBuf[0] = '0'
				debugBuf[1] = 'x'
				n := len(strconv.AppendUint(debugBuf[2:2], ui, 16))
				print(string(debugBuf[:2+n]))
			}

			if i > 0 {
				lastStr, ok := a[i-1].(string)
				if ok && len(lastStr) > 0 && lastStr[0] == '=' {
					printSpace = false
				}
			}

			if printSpace {
				print(" ")
			}
		}
		print("\n")
	}
	flushprint()
}

func validateFirmware(src string) error {
	const cmpString = "Version: "
	fwEnd := 800 // get last 800 bytes
	if fwEnd > len(src) {
		return errors.New("bad firmware size: too small")
	}

	// First we validate the firmware by looking for the Version string:
	b := src[len(src)-fwEnd:]
	// get length of trailer.
	fwEnd -= 16 // skip DVID trailer.
	trailLen := uint32(b[fwEnd-2]) | uint32(b[fwEnd-1])<<8
	found := -1
	if trailLen < 500 && b[fwEnd-3] == 0 {

		for i := 80; i < int(trailLen); i++ {
			ptr := fwEnd - 3 - i
			if b[ptr:ptr+9] == cmpString {
				found = i
				break
			}
		}
	}
	if found == -1 {
		return errors.New("could not find valid firmware")
	}
	if verbose_debug {
		i := 0
		ptrstart := fwEnd - 3 - found
		for ; b[ptrstart+i] != 0; i++ {
		}
		Debug("got version", string(b[ptrstart:ptrstart+i-1]))
	}
	return nil
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
