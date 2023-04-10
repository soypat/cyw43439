package main

import (
	"bytes"
	"errors"
	"fmt"
	"machine"
	"strconv"
	"time"

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

		case 'Z':
			println("reset device")
			dev.Reset()

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
