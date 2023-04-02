package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"machine"
	"strconv"
	"time"
	"unsafe"

	cyw43439 "github.com/soypat/cyw43439"
)

func TestShellmode() {
	shell := Shell{
		IO:             machine.USBCDC,
		Loopback:       true,
		WaitForCommand: 30 * time.Second,
	}
	spi, cs, wlreg, irq := cyw43439.PicoWSpi(0)
	spi.MockTo = &cyw43439.SPIbb{
		SCK:   mockSCK,
		SDI:   mockSDI,
		SDO:   mockSDO,
		Delay: 10,
	}
	println("replicating SPI transactions on GPIOs (SDO,SDI,SCK,CS)=", mockSDO, mockSDI, mockSCK, mockCS)
	spi.Configure()
	dev := cyw43439.NewDev(spi, cs, wlreg, irq, irq)
	dev.GPIOSetup()
	var _commandBuf [128]byte
	var (
		devFn           = cyw43439.FuncBus
		writeVal uint64 = 0
	)
	for {
		n, _, err := shell.Parse('$', _commandBuf[:])
		if err != nil {
			if errors.Is(err, errCmdWithNoContent) {
				shell.Write([]byte("command read timed out\n"))
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		command := _commandBuf[:n]
		cmdByte := command[0]
		var arg1 uint64
		var arg1Err error
		trimmed := command[1:]
		if bytes.HasPrefix(trimmed, []byte{'0', 'x'}) {
			arg1, arg1Err = strconv.ParseUint(string(trimmed[2:]), 16, 32)
		} else if bytes.HasPrefix(trimmed, []byte{'0', 'b'}) {
			arg1, arg1Err = strconv.ParseUint(string(trimmed[2:]), 1, 32)
		} else {
			arg1, arg1Err = strconv.ParseUint(string(trimmed), 10, 32)
		}
		if arg1Err != nil {
			// Require argument for starters
			err = arg1Err
			println("bad argument. need number")
			continue
		}

		switch cmdByte {
		case 'l':
			active := arg1 > 0
			println("set led", active)
			err = dev.LED().Set(active)

		case 'f':
			// Dangerous assignment.
			devFn = cyw43439.Function(arg1)
			println("device register func set to ", devFn.String())

		case 'u':
			println("writing 8bit register", arg1, "with value", uint8(writeVal))
			err = dev.Write8(devFn, uint32(arg1), uint8(writeVal))

		case 'v':
			println("writing 16bit register", arg1, "with value", uint16(writeVal))
			err = dev.Write16(devFn, uint32(arg1), uint16(writeVal))

		case 'W', 'w':
			println("writing 32bit register", arg1, "with value", uint32(writeVal), " wordlen==16:", cmdByte <= 'Z')
			if cmdByte == 'w' {
				err = dev.Write32(devFn, uint32(arg1), uint32(writeVal))
			} else {
				err = dev.Write32S(devFn, uint32(arg1), uint32(writeVal))
			}

		case 't':
			println("write value set to", arg1)
			writeVal = arg1

		case 'y':
			println("reading 8bit register", arg1)
			value, err := dev.Read8(devFn, uint32(arg1))
			if err != nil {
				break
			}
			command[0] = '0'
			command[1] = 'x'
			command = strconv.AppendUint(command[:2], uint64(value), 16)
			shell.Write(command)

		case 'x':
			println("reading 16bit register", arg1, " wordlen==16:")
			var value uint16
			value, err = dev.Read16(devFn, uint32(arg1))
			if err != nil {
				break
			}
			command[0] = '0'
			command[1] = 'x'
			command = strconv.AppendUint(command[:2], uint64(value), 16)
			shell.Write(command)

		case 'R', 'r':
			println("reading 32bit register", arg1, " wordlen==16:", cmdByte <= 'Z')
			var value uint32
			if cmdByte == 'r' {
				value, err = dev.Read32(devFn, uint32(arg1))
			} else {
				value, err = dev.Read32S(devFn, uint32(arg1))
			}
			if err != nil {
				break
			}
			command[0] = '0'
			command[1] = 'x'
			command = strconv.AppendUint(command[:2], uint64(value), 16)
			shell.Write(command)
		case 'Z':
			println("reset device")
			dev.Reset()
		case '~':
			var b = [8]byte{0xfe, 0xed, 0xbe, 0xad, 0xde, 0xad, 0xbe, 0xef}
			words := (*[2]uint32)(unsafe.Pointer(&b))
			for i := range words {
				shell.IO.WriteByte('\n')
				command[0] = '0'
				command[1] = 'x'
				command = strconv.AppendUint(command[:2], uint64(words[0]), 16)
				shell.Write(command)
				command[0] = ' '
				command = strconv.AppendUint(command[:2], uint64(binary.BigEndian.Uint32(b[i*4:])), 16)
				shell.Write(command)
				command = strconv.AppendUint(command[:2], uint64(binary.LittleEndian.Uint32(b[i*4:])), 16)
				shell.Write(command)
			}

		case '!':
			var b = [2]uint32{0xfeedbead, 0xdeadbeef}
			bs := (*[8]byte)(unsafe.Pointer(&b))
			for i := range bs {
				if i%4 == 0 {
					shell.IO.WriteByte('\n')
				}
				command[0] = '0'
				command[1] = 'x'
				command = strconv.AppendUint(command[:2], uint64(bs[i]), 16)
				shell.Write(command)
			}

		case 'I':
			println("initializing device")
			err = dev.Init(cyw43439.DefaultConfig(false))
			if err == nil {
				println("init success")
			}
		case 'i':
			println("running init + blink")
			err = dev.Init(cyw43439.DefaultConfig(false))
			if err != nil {
				break
			}
			println("init success")
			err = dev.LED().High()
			if err != nil {
				break
			}
			time.Sleep(time.Second)
			err = dev.LED().Low()
			if err != nil {
				break
			}
			time.Sleep(time.Second)
			err = dev.LED().High()
		case 'o':
			b := arg1 > 0
			println("setting WL_REG_ON", b)
			wlreg.Set(b)
		case 'D':
			println("setting CY43439 response delay byte count to", uint8(arg1))
			dev.ResponseDelayByteCount = uint8(arg1)
		case 'd':
			println("setting SPI delay to", arg1)
			spi.Delay = uint32(arg1)
		case 'L':
			b := arg1 > 0
			println("setting shell loopback mode", b)
			shell.Loopback = b

		default:
			err = fmt.Errorf("unknown command %q", cmdByte)
		}
		if err != nil {
			shell.Write([]byte("shell error:\""))
			shell.Write([]byte(err.Error()))
			shell.IO.WriteByte('"')
		}
		shell.IO.WriteByte('\r')
		shell.IO.WriteByte('\n')
	}
}
