package cyw43439

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"
)

func TestFirmware(t *testing.T) {
	cfg := DefaultConfig(false)

	err := downloadResource(0x0, cfg.Firmware)
	if err != nil {
		t.Error(err)
	}
}

func downloadResource(addr uint32, src []byte) error {
	// round up length to simplify download.
	rlen := (len(src) + 255) &^ 255
	const BLOCKSIZE = 64
	var srcPtr []byte
	var buf [BLOCKSIZE]byte
	for offset := 0; offset < rlen; offset += BLOCKSIZE {
		sz := BLOCKSIZE
		if offset+sz > rlen {
			sz = rlen - offset
		}
		dstAddr := addr + uint32(offset)
		if dstAddr&backplaneAddrMask+uint32(sz) > backplaneAddrMask+1 {
			panic("invalid dstAddr:" + strconv.Itoa(int(dstAddr)))
		}
		fmt.Println("set backplane window to ", dstAddr, offset)
		// err := d.setBackplaneWindow(dstAddr)
		var err error
		if err != nil {
			return err
		}
		if offset+sz > len(src) {
			fmt.Println("ALLOCA", sz)
			srcPtr = buf[:sz]
		} else {
			srcPtr = src[offset:]
		}

		_ = srcPtr
		fmt.Println("write bytes to addr ", dstAddr&backplaneAddrMask)
		// err = d.WriteBytes(FuncBackplane, dstAddr&backplaneAddrMask, src[:sz])
		if err != nil {
			return err
		}
	}
	Debug("download finished, validate data")
	// Finished writing firmware... should be ready for use. We choose to validate it though.

	for offset := 0; offset < rlen; offset += BLOCKSIZE {
		sz := BLOCKSIZE
		if offset+sz > rlen {
			sz = rlen - offset
		}
		dstAddr := addr + uint32(offset)
		if dstAddr&backplaneAddrMask+uint32(sz) > backplaneAddrMask+1 {
			panic("invalid dstAddr:" + strconv.Itoa(int(dstAddr)))
		}
		fmt.Println("set backplane window", dstAddr)
		// err := d.setBackplaneWindow(dstAddr)
		var err error
		if err != nil {
			return err
		}
		fmt.Println("read back bytes into buf from ", dstAddr&backplaneAddrMask)
		// err = d.ReadBytes(FuncBackplane, dstAddr&backplaneAddrMask, buf[:sz])
		if err != nil {
			return err
		}
		src = src[offset:]
		if !bytes.Equal(buf[:sz], src[:sz]) {
			return errFirmwareValidationFailed
		}
	}
	return nil
}
