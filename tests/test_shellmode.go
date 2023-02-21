package main

import (
	"bytes"
	"errors"
	"fmt"
	"machine"
	"strconv"
	"time"

	cyw43439 "github.com/soypat/cy43439"
)

func TestShellmode() {
	shell := Shell{
		IO:             machine.USBCDC,
		Loopback:       true,
		WaitForCommand: 30 * time.Second,
	}
	spi, cs, wlreg, irq := cyw43439.PicoWSpi(0)
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
		if bytes.HasPrefix(command[1:], []byte{'0', 'x'}) {
			arg1, arg1Err = strconv.ParseUint(string(command[3:]), 16, 32)
		} else {
			arg1, arg1Err = strconv.ParseUint(string(command[1:]), 10, 32)
		}
		if arg1Err != nil {
			// Require argument for starters
			err = arg1Err
			println("bad argument. need number")
			continue
		}

		switch cmdByte {
		case 'f':
			println("device register func set to ", arg1)
			devFn = cyw43439.Function(arg1) // Dangerous assignment.
		case 'W', 'w':
			println("writing register", arg1, "with value", writeVal)
			if cmdByte == 'W' {
				err = dev.RegisterWriteUint32(devFn, uint32(arg1), uint32(writeVal))
			} else {
				err = dev.RegisterWriteUint16(devFn, uint32(arg1), uint16(writeVal))
			}
			if err != nil {
				break
			}
		case 'v':
			println("write value set to", arg1)
			writeVal = arg1
		case 'R', 'r', 's':
			println("reading register", arg1)
			var value uint32
			if cmdByte == 'R' {
				value, err = dev.RegisterReadUint32(devFn, uint32(arg1))
			} else if cmdByte == 'r' {
				value16, errg := dev.RegisterReadUint16(devFn, uint32(arg1))
				value = uint32(value16)
				err = errg
			} else {
				value, err = dev.ReadReg32Swap(devFn, uint32(arg1))
			}
			if err != nil {
				break
			}
			command[0] = '0'
			command[1] = 'x'
			command = strconv.AppendUint(command[:2], uint64(value), 16)
			shell.Write(command)
		case 'I':
			println("initializing device")
			err = dev.Init()
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
		case 'l':
			b := arg1 > 0
			println("setting shell loopback mode", b)
			shell.Loopback = b
		case 'M':
			// Switch to and from mock mode.
			isMock := arg1 > 0
			println("mocking mode set", isMock)
			if isMock {
				mockSpi := &cyw43439.SPIbb{
					SCK:   mockSCK,
					SDI:   mockSDI,
					SDO:   mockSDO,
					Delay: 10,
				}
				mockSpi.Configure()
				dev = cyw43439.NewDev(mockSpi, mockCS, 0, 0, mockSDI)
			} else {
				dev = cyw43439.NewDev(spi, cs, wlreg, irq, irq)
			}
			dev.GPIOSetup()
		case 'X', 'x':
			println("reading 16bit register", arg1)
			var value uint16
			if cmdByte == 'X' {
				value, err = dev.Read16(devFn, uint32(arg1))
			} else if cmdByte == 'x' {
				value, err = dev.Read16S(devFn, uint32(arg1))
			}
			if err != nil {
				break
			}
			command[0] = '0'
			command[1] = 'x'
			command = strconv.AppendUint(command[:2], uint64(value), 16)
			shell.Write(command)
		case 'Y', 'y':
			println("reading 32bit register", arg1)
			var value uint32
			if cmdByte == 'Y' {
				value, err = dev.Read32(devFn, uint32(arg1))
			} else if cmdByte == 'y' {
				value, err = dev.Read32S(devFn, uint32(arg1))
			}
			if err != nil {
				break
			}
			command[0] = '0'
			command[1] = 'x'
			command = strconv.AppendUint(command[:2], uint64(value), 32)
			shell.Write(command)
		default:
			err = fmt.Errorf("unknown command %q", cmdByte)
		}
		if err != nil {
			shell.Write([]byte("shell error:\""))
			shell.Write([]byte(err.Error()))
			shell.IO.WriteByte('"')
		}
		shell.IO.WriteByte('\n')
	}
}