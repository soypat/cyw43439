package cyw43439

import (
	"errors"
	"log/slog"
	"time"

	"github.com/soypat/cyw43439/whd"
)

var (
	errUnalignedBuffer        = errors.New("cyw: unaligned buffer")
	errHCIPacketTooLarge      = errors.New("cyw: hci packet too large")
	errBTWakeTimeout          = errors.New("cyw: bt wake timeout")
	errBTReadyTimeout         = errors.New("cyw: bt ready timeout")
	errTimeout                = errors.New("cyw: timeout")
	errZeroBTAddr             = errors.New("cyw: btaddr=0")
	errBTInvalidVersionLength = errors.New("invalid bt version length")
	errBTWatermark            = errors.New("bt watermark set failed")
)

// SendHCI sends a HCI packet over the CYW43439's interface. Used for bluetooth.
func (d *Device) SendHCI(b []byte) error {
	d.lock()
	defer d.unlock()

	return d.hci_write(b)
}

// RecvHCIHandle registers the incoming HCI packet handler. Used for bluetooth.
func (d *Device) RecvHCIHandle(callback func([]byte) error) {
	d.lock()
	defer d.unlock()
	d.rcvHCI = callback
}

func (d *Device) bt_mode_enabled() bool {
	return d.mode&modeBluetooth != 0
}

func (d *Device) bt_init(firmware string) error {
	d.trace("bt_init")
	err := d.bp_write32(whd.CYW_BT_BASE_ADDRESS+whd.BT2WLAN_PWRUP_ADDR, whd.BT2WLAN_PWRUP_WAKE)
	if err != nil {
		return err
	}
	time.Sleep(2 * time.Millisecond)
	err = d.bt_upload_firmware(firmware)
	if err != nil {
		return err
	}
	err = d.bt_wait_ready()
	if err != nil {
		return err
	}
	err = d.bt_init_buffers()
	if err != nil {
		return err
	}
	err = d.bt_wait_awake()
	if err != nil {
		return err
	}
	err = d.bt_set_host_ready()
	if err != nil {
		return err
	}
	d.bt_toggle_intr()
	if err != nil {
		return err
	}
	return nil
}

func (d *Device) bt_upload_firmware(firmware string) error {
	d.trace("bt_init:start")
	versionlength := firmware[0]
	if versionlength > 8 {
		return errors.New("invalid version length")
	}
	var version uint32
	for i := 0; i < int(versionlength); i++ {
		version |= uint32(firmware[i]) << i
	}
	// Skip version + 1 extra byte as per cybt_shared_bus_driver.c
	firmware = firmware[versionlength+2:]
	// buffers
	rawbuffer := u32AsU8(d._sendIoctlBuf[:])
	alignedDataBuffer := rawbuffer[:256]
	btfwCB := firmware
	hfd := hexFileData{
		addrmode: whd.BTFW_ADDR_MODE_EXTENDED,
	}
	var auxbuf [4]byte
	for {
		numFwBytes := bt_read_firmware_patch_line(btfwCB, &hfd)
		if numFwBytes == 0 {
			break
		}
		fwBytes := hfd.ds[:numFwBytes]
		dstStartAddr := hfd.dstAddr + whd.CYW_BT_BASE_ADDRESS
		var alignedDataBufferIdx uint32
		if !isaligned(dstStartAddr, 4) {
			// Pad with bytes already in memory.
			numPadBytes := dstStartAddr % 4
			paddedDstStartAddr := aligndown(dstStartAddr, 4)
			memoryValue, _ := d.bp_read32(paddedDstStartAddr)

			_busOrder.PutUint32(auxbuf[:], memoryValue)
			for i := 0; i < int(numPadBytes); i++ {
				alignedDataBuffer[alignedDataBufferIdx] = auxbuf[i]
				alignedDataBufferIdx++
			}

			dstStartAddr = paddedDstStartAddr
		}
		// Copy firmware bytes after padding bytes.
		alignedDataBufferIdx += uint32(copy(alignedDataBuffer[alignedDataBufferIdx:], fwBytes))

		dstEndAddr := dstStartAddr + alignedDataBufferIdx
		if !isaligned(dstEndAddr, 4) {
			offset := dstEndAddr % 4
			numPadBytesEnd := 4 - offset
			paddedDstEndAddr := aligndown(dstEndAddr, 4)
			memoryValue, _ := d.bp_read32(paddedDstEndAddr)
			_busOrder.PutUint32(auxbuf[:], memoryValue)
			for i := offset; i < 4; i++ {
				alignedDataBuffer[alignedDataBufferIdx] = auxbuf[i]
				alignedDataBufferIdx++
			}
			dstEndAddr += numPadBytesEnd
		}
		bufferToWrite := alignedDataBuffer[0:alignedDataBufferIdx]
		if dstStartAddr%4 != 0 || dstEndAddr%4 != 0 || alignedDataBufferIdx%4 != 0 {
			return errors.New("unaligned BT firmware bug")
		}
		const chunksize = 64 // Is writing in 64 byte chunks needed?
		numChunks := align(alignedDataBufferIdx, 64)
		for i := uint32(0); i < numChunks; i += chunksize {
			offset := i * chunksize
			end := (i + 1) * chunksize
			if end > alignedDataBufferIdx {
				end = alignedDataBufferIdx
			}
			chunk := bufferToWrite[offset:end]
			err := d.bp_write(dstStartAddr+offset, chunk)
			if err != nil {
				return err
			}
			time.Sleep(time.Millisecond) // TODO: is this sleep needed?
		}
	}
	return nil
}

func (d *Device) hci_set_read_handler(fn func(b []byte) error) {

}

func (d *Device) hci_write(b []byte) error {
	d.trace("hci_write:start")
	buflen := len(b)
	alignBuflen := align(uint32(buflen), 4)
	if buflen <= int(alignBuflen) {
		return errUnalignedBuffer
	}
	cmdlen := buflen + 3 - 4 // Add 3 bytes for SDIO header (revise)

	bufWithCmd := u32AsU8(d._sendIoctlBuf[:256/4])
	if buflen > len(bufWithCmd)-3 {
		return errHCIPacketTooLarge
	}
	bufWithCmd[0] = byte(cmdlen)
	bufWithCmd[1] = byte(cmdlen >> 8)
	bufWithCmd[2] = 0
	copy(bufWithCmd[3:], b)

	paddedBufWithCmd := bufWithCmd[0:alignBuflen]
	err := d.bt_bus_request()
	if err != nil {
		return err
	}
	addr := d.btaddr + whd.BTSDIO_OFFSET_HOST_WRITE_BUF + d.h2bWritePtr
	err = d.bp_write(addr, paddedBufWithCmd)
	if err != nil {
		return err
	}
	d.h2bWritePtr += uint32(len(paddedBufWithCmd))
	err = d.bp_write32(d.btaddr+whd.BTSDIO_OFFSET_HOST2BT_IN, d.h2bWritePtr)
	if err != nil {
		return err
	}
	err = d.bt_toggle_intr()
	if err != nil {
		return err
	}
	return d.bt_bus_release()
}

func (d *Device) bt_wait_ready() error {
	if err := d.bt_wait_ctrl_bits(whd.BTSDIO_REG_FW_RDY_BITMASK, 300); err != nil {
		return errBTReadyTimeout
	}
	return nil
}

func (d *Device) bt_wait_awake() error {
	if err := d.bt_wait_ctrl_bits(whd.BTSDIO_REG_BT_AWAKE_BITMASK, 300); err != nil {
		return errBTWakeTimeout
	}
	return nil
}

func (d *Device) bt_wait_ctrl_bits(bits uint32, timeout_ms int) error {
	d.trace("bt_wait_ctrl_bits:start", slog.Uint64("bits", uint64(bits)))
	for i := 0; i < timeout_ms; i++ {
		val, err := d.bp_read32(whd.BT_CTRL_REG_ADDR)
		if err != nil {
			return err
		}
		if val&bits != 0 {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	return errTimeout
}

func (d *Device) bt_set_host_ready() error {
	d.trace("bt_set_host_ready:start")
	oldval, err := d.bp_read32(whd.HOST_CTRL_REG_ADDR)
	if err != nil {
		return err
	}
	newval := oldval | whd.BTSDIO_REG_SW_RDY_BITMASK
	return d.bp_write32(whd.HOST_CTRL_REG_ADDR, newval)
}

func (d *Device) bt_set_awake(awake bool) error {
	d.trace("bt_set_awake:start")
	oldval, err := d.bp_read32(whd.HOST_CTRL_REG_ADDR)
	if err != nil {
		return err
	}
	// Swap endianness on this read?
	var newval uint32
	if awake {
		newval = oldval | whd.BTSDIO_REG_WAKE_BT_BITMASK
	} else {
		newval = oldval &^ whd.BTSDIO_REG_WAKE_BT_BITMASK
	}
	return d.bp_write32(whd.HOST_CTRL_REG_ADDR, newval)
}

func (d *Device) bt_toggle_intr() error {
	d.trace("bt_toggle_intr:start")
	oldval, err := d.bp_read32(whd.HOST_CTRL_REG_ADDR)
	if err != nil {
		return err
	}
	// TODO(soypat): Swap endianness on this read?
	newval := oldval ^ whd.BTSDIO_REG_DATA_VALID_BITMASK
	return d.bp_write32(whd.HOST_CTRL_REG_ADDR, newval)
}

func (d *Device) bt_set_intr() error {
	d.trace("bt_set_intr:start")
	oldval, err := d.bp_read32(whd.HOST_CTRL_REG_ADDR)
	if err != nil {
		return err
	}
	newval := oldval | whd.BTSDIO_REG_DATA_VALID_BITMASK
	return d.bp_write32(whd.HOST_CTRL_REG_ADDR, newval)
}

func (d *Device) bt_init_buffers() error {
	d.trace("bt_init_buffers:start")
	btaddr, err := d.bp_read32(whd.WLAN_RAM_BASE_REG_ADDR)
	if err != nil {
		return err
	} else if btaddr == 0 {
		return errZeroBTAddr
	}
	d.btaddr = btaddr
	d.bp_write32(btaddr+whd.BTSDIO_OFFSET_HOST2BT_IN, 0)
	d.bp_write32(btaddr+whd.BTSDIO_OFFSET_HOST2BT_OUT, 0)
	d.bp_write32(btaddr+whd.BTSDIO_OFFSET_BT2HOST_IN, 0)
	return d.bp_write32(btaddr+whd.BTSDIO_OFFSET_BT2HOST_OUT, 0)
}

func (d *Device) bt_bus_request() error {
	err := d.bt_set_awake(true)
	if err != nil {
		return err
	}
	return d.bt_wait_awake()
}

func (d *Device) bt_bus_release() error {
	return nil
}

func (d *Device) bt_has_work() bool {
	d.trace("bt_has_work:start")
	intstat, _ := d.bp_read32(whd.SDIO_BASE_ADDRESS)
	if intstat&whd.I_HMB_FC_CHANGE != 0 {
		d.bp_write32(whd.SDIO_BASE_ADDRESS+whd.SDIO_INT_STATUS, intstat&whd.I_HMB_FC_CHANGE)
		return true
	}
	return false
}

type hexFileData struct {
	addrmode int32
	hiaddr   uint16
	dstAddr  uint32
	ds       [256]byte
}

// bt_read_firmware_patch_line reads firmware addressing scheme into hfd and returns the patch line length stored into hfd.
func bt_read_firmware_patch_line(cbFirmware string, hfd *hexFileData) uint32 {
	var absBaseAddr32 uint32
	nxtLineStart := cbFirmware
	for {
		numBytes := nxtLineStart[0]
		nxtLineStart = nxtLineStart[1:]

		addr := uint16(nxtLineStart[0])<<8 | uint16(nxtLineStart[1])
		nxtLineStart = nxtLineStart[2:]

		lineType := nxtLineStart[0]
		nxtLineStart = nxtLineStart[1:]
		if numBytes == 0 {
			break
		}
		copy(hfd.ds[:numBytes], nxtLineStart[:numBytes])
		switch lineType {
		case whd.BTFW_HEX_LINE_TYPE_EXTENDED_ADDRESS:
			hfd.hiaddr = uint16(hfd.ds[0])<<8 | uint16(hfd.ds[1])
			hfd.addrmode = whd.BTFW_ADDR_MODE_EXTENDED

		case whd.BTFW_HEX_LINE_TYPE_EXTENDED_SEGMENT_ADDRESS:
			hfd.hiaddr = uint16(hfd.ds[0])<<8 | uint16(hfd.ds[1])
			hfd.addrmode = whd.BTFW_ADDR_MODE_SEGMENT

		case whd.BTFW_HEX_LINE_TYPE_ABSOLUTE_32BIT_ADDRESS:
			absBaseAddr32 = uint32(hfd.ds[0])<<24 | uint32(hfd.ds[1])<<16 |
				uint32(hfd.ds[2])<<8 | uint32(hfd.ds[3])
			hfd.addrmode = whd.BTFW_ADDR_MODE_LINEAR32

		case whd.BTFW_HEX_LINE_TYPE_DATA:
			hfd.dstAddr = uint32(addr)
			switch hfd.addrmode {
			case whd.BTFW_ADDR_MODE_EXTENDED:
				hfd.dstAddr += uint32(hfd.hiaddr) << 16
			case whd.BTFW_ADDR_MODE_SEGMENT:
				hfd.dstAddr += uint32(hfd.hiaddr) << 4
			case whd.BTFW_ADDR_MODE_LINEAR32:
				hfd.dstAddr += absBaseAddr32
			}
			return uint32(numBytes)
		default:
			// Skip other line types.
		}
	}
	return 0
}
