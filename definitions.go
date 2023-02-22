package cyw43439

import "time"

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

// Status supports status notification to the host after a read/write
// transaction over gSPI. This status notification provides information
// about packet errors, protocol errors, available packets in the RX queue, etc.
// The status information helps reduce the number of interrupts to the host.
// The status-reporting feature can be switched off using a register bit,
// without any timing overhead.
type Status uint32

// IsDataAvailable returns true if requested read data is available.
func (s Status) IsDataAvailable() bool { return s&1 == 0 }

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
