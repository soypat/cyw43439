package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"strconv"
	"unsafe"
)

func main() {
	var command64 uint64
	var err error
	cmd := os.Args[1]
	switch {
	case len(cmd) > 2 && cmd[0] == '0' && cmd[1] == 'x':
		command64, err = strconv.ParseUint(cmd[2:], 16, 32)
	default:
		command64, err = strconv.ParseUint(cmd[:], 16, 32)
	}
	if err != nil {
		log.Fatal(err)
	}
	buf := (*[4]byte)(unsafe.Pointer(&command64))
	command := binary.BigEndian.Uint32(buf[:])
	write := command&(1<<31) != 0
	autoInc := command&(1<<30) != 0
	fn := Function(command>>28) & 0b11
	addr := (command >> 11) & 0x1ffff
	size := command & ((1 << 11) - 1)
	fmt.Printf("addr=%#x  fn=%v  sz=%v  write=%v   autoinc=%v  x=%#x\n", addr, fn.String(), size, write, autoInc, command)
}

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
