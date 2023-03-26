package cyw43439

import (
	"bytes"
	"errors"
	"net"
	"strconv"
	"time"
)

const (
	verbose_debug     = true
	initReadback      = false
	validateDownloads = false
)

type Config struct {
	Firmware        []byte
	CLM             []byte
	MAC             net.HardwareAddr
	EnableBluetooth bool
}

func DefaultConfig(enableBT bool) Config {
	var fw []byte
	if enableBT {
		// fw = wifibtFW[:wifibtFWLen]
	} else {
		fw = wifiFW[:wifiFWLen]
	}
	return Config{
		Firmware:        fw,
		CLM:             GetCLM(fw),
		MAC:             []byte{0xfe, 0xed, 0xde, 0xad, 0xbe, 0xef},
		EnableBluetooth: enableBT,
	}
}

// TODO: delete these auxiliary variables.
const (
	responseDelay                 time.Duration = 0 //20 * time.Microsecond
	whdBusSPIBackplaneReadPadding               = 4
	sharedDATA                                  = true
	pollLimit                                   = 60 * time.Millisecond
)

// 32 bit register addresses on SPI.
const (
	AddrBusControl = 0x0000
	AddrStatus     = 0x0008
	// 32 bit address that contains only-read 0xFEEDBEAD value.
	AddrTest = 0x0014
	// 32 bit test value at gSPI address 0x14.
	TestPattern uint32 = 0xFEEDBEAD
)

// 16 bit register addresses on SPI.
const (
	AddrInterrupt       = 0x0004
	AddrInterruptEnable = 0x0006
	AddrFunc1Info       = 0x000c
	AddrFunc2Info       = 0x000e
	AddrFunc3Info       = 0x0010
)

// 8 bit register addresses on SPI.
const (
	AddrRespDelayF0   = 0x001c // corerev >= 1
	AddrRespDelayF1   = 0x001d // corerev >= 1
	AddrRespDelayF2   = 0x001e // corerev >= 1
	AddrRespDelayF3   = 0x001f // corerev >= 1
	AddrResponseDelay = 0x0001
	AddrStatusEnable  = 0x0002
	AddrResetBP       = 0x0003 // corerev >= 1
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

// Interrupt registers on SPI.
const (
	DATA_UNAVAILABLE        = 0x0001 // Requested data not available; Clear by writing a "1"
	F2_F3_FIFO_RD_UNDERFLOW = 0x0002
	F2_F3_FIFO_WR_OVERFLOW  = 0x0004
	COMMAND_ERROR           = 0x0008 // Cleared by writing 1
	DATA_ERROR              = 0x0010 // Cleared by writing 1
	F2_PACKET_AVAILABLE     = 0x0020
	F3_PACKET_AVAILABLE     = 0x0040
	F1_OVERFLOW             = 0x0080 // Due to last write. Bkplane has pending write requests
	GSPI_PACKET_AVAILABLE   = 0x0100
	MISC_INTR1              = 0x0200
	MISC_INTR2              = 0x0400
	MISC_INTR3              = 0x0800
	MISC_INTR4              = 0x1000
	F1_INTR                 = 0x2000
	F2_INTR                 = 0x4000
	F3_INTR                 = 0x8000
)

// SDIO bus specifics
const (
	SDIOD_CCCR_IOEN          = 0x02
	SDIOD_CCCR_IORDY         = 0x03
	SDIOD_CCCR_INTEN         = 0x04
	SDIOD_CCCR_BICTRL        = 0x07
	SDIOD_CCCR_BLKSIZE_0     = 0x10
	SDIOD_CCCR_SPEED_CONTROL = 0x13
	SDIOD_CCCR_BRCM_CARDCAP  = 0xf0
	SDIOD_SEP_INT_CTL        = 0xf2
	SDIOD_CCCR_F1BLKSIZE_0   = 0x110
	SDIOD_CCCR_F2BLKSIZE_0   = 0x210
	SDIOD_CCCR_F2BLKSIZE_1   = 0x211
	INTR_CTL_MASTER_EN       = 0x01
	INTR_CTL_FUNC1_EN        = 0x02
	INTR_CTL_FUNC2_EN        = 0x04
	SDIO_FUNC_ENABLE_1       = 0x02
	SDIO_FUNC_ENABLE_2       = 0x04
	SDIO_FUNC_READY_1        = 0x02
	SDIO_FUNC_READY_2        = 0x04
	SDIO_64B_BLOCK           = 64
	SDIO_CHIP_CLOCK_CSR      = 0x1000e
	SDIO_PULL_UP             = 0x1000f
)

// SDIOD_CCCR_BRCM_CARDCAP bits
const (
	SDIOD_CCCR_BRCM_CARDCAP_CMD14_SUPPORT = 0x02 // Supports CMD14
	SDIOD_CCCR_BRCM_CARDCAP_CMD14_EXT     = 0x04 // CMD14 is allowed in FSM command state
	SDIOD_CCCR_BRCM_CARDCAP_CMD_NODEC     = 0x08 // sdiod_aos does not decode any command
)

// SDIO_SLEEP_CSR bits
const (
	SBSDIO_SLPCSR_KEEP_SDIO_ON = 1 << 0 // KeepSdioOn bit
	SBSDIO_SLPCSR_DEVICE_ON    = 1 << 1 // DeviceOn bit
)

// IOCTL kinds.
const (
	SDPCM_GET = 0
	SDPCM_SET = 2
)

func GetCLM(firmware []byte) []byte {
	clmAddr := align32(uint32(len(firmware)), 512)
	if uint32(cap(firmware)) < clmAddr+clmLen {
		panic("firmware slice too small for CLM")
	}
	return firmware[clmAddr : clmAddr+clmLen]
}

//go:inline
func align32(val, align uint32) uint32 { return (val + align - 1) &^ (align - 1) }

// SPIWriteRead performs the gSPI Write-Read action.
// Not used!
// func (d *Dev) SPIWriteRead(cmd uint32, w, r []byte) error {
// 	var buf [4]byte
// 	d.csLow()
// 	if sharedDATA {
// 		d.sharedSD.Configure(machine.PinConfig{Mode: machine.PinOutput})
// 	}
// 	binary.BigEndian.PutUint32(buf[:], cmd) // !LE
// 	d.spi.Tx(buf[:], nil)

// 	err := d.spi.Tx(w, nil)
// 	if err != nil {
// 		return err
// 	}
// 	d.responseDelay()
// 	err = d.spi.Tx(nil, r)
// 	if err != nil || !d.enableStatusWord {
// 		return err
// 	}

// 	// Read Status.
// 	buf = [4]byte{}
// 	d.spi.Tx(buf[:], buf[:])
// 	d.csHigh()
// 	status := Status(binary.BigEndian.Uint32(buf[:])) // !LE
// 	status = Status(swap32(uint32(status)))
// 	if !status.IsDataAvailable() {
// 		println("got status:", status)
// 		return ErrDataNotAvailable
// 	}
// 	return nil
// }

const (
	CORE_WLAN_ARM              = 1
	WLAN_ARMCM3_BASE_ADDRESS   = 0x18003000
	WRAPPER_REGISTER_OFFSET    = 0x100_000
	CORE_SOCRAM                = 2
	SOCSRAM_BASE_ADDRESS       = 0x18004000
	SBSDIO_SB_ACCESS_2_4B_FLAG = 0x08000
	CHIPCOMMON_BASE_ADDRESS    = 0x18000000
	backplaneAddrMask          = 0x7fff
	AI_RESETCTRL_OFFSET        = 0x800
	AIRC_RESET                 = 1
	AI_IOCTRL_OFFSET           = 0x408
	SICF_FGC                   = 2
	SICF_CLOCK_EN              = 1
	SICF_CPUHALT               = 0x20
	SOCSRAM_BANKX_INDEX        = (0x18004000) + 0x10

	SOCSRAM_BANKX_PDA        = (SOCSRAM_BASE_ADDRESS + 0x44)
	SBSDIO_HT_AVAIL          = 0x80
	SDIO_BASE_ADDRESS        = 0x18002000
	SDIO_INT_HOST_MASK       = SDIO_BASE_ADDRESS + 0x24
	I_HMB_SW_MASK            = 0x000000f0
	SDIO_FUNCTION2_WATERMARK = 0x10008
	SPI_F2_WATERMARK         = 32

	SDIO_WAKEUP_CTRL = 0x1001e
	SDIO_SLEEP_CSR   = 0x1001f
	SBSDIO_FORCE_ALP = 0x01
	SBSDIO_FORCE_HT  = 0x02
)

var errFirmwareValidationFailed = errors.New("firmware validation failed")

var debugBuf [128]byte

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
				if c < 0 {
					print(c)
				} else {
					printUi = true
					ui = uint64(c)
				}
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

func validateFirmware(src []byte) error {
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
		var cmpString = []byte("Version: ")
		for i := 80; i < int(trailLen); i++ {
			ptr := fwEnd - 3 - i
			if bytes.Equal(b[ptr:ptr+9], cmpString) {
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
