package cyrw

import (
	"errors"
	"strconv"

	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/cyw43439/whd"
)

type ioctlType uint8

const (
	ioctlGET = whd.SDPCM_GET
	ioctlSET = whd.SDPCM_SET
)

// tx transmits a SDPCM+BDC data packet to the device.
func (d *Device) tx(packet []byte) (err error) {
	// reference: https://github.com/embassy-rs/embassy/blob/6babd5752e439b234151104d8d20bae32e41d714/cyw43/src/runner.rs#L247
	d.debug("tx", slog.Int("len", len(packet)))
	buf := d._sendIoctlBuf[:]
	buf8 := u32AsU8(buf)

	// There MUST be 2 bytes of padding between the SDPCM and BDC headers (only for data packets). See reference.
	// "¯\_(ツ)_/¯"

	const PADDING_SIZE = 2
	totalLen := uint32(whd.SDPCM_HEADER_LEN + PADDING_SIZE + whd.BDC_HEADER_LEN + len(packet))

	seq := d.sdpcmSeq
	d.sdpcmSeq++ // Go wraps around on overflow by default.

	d.auxSDPCMHeader = whd.SDPCMHeader{
		Size:         uint16(totalLen), // TODO does this len need to be rounded up to u32?
		SizeCom:      ^uint16(totalLen),
		Seq:          uint8(seq),
		ChanAndFlags: 2, // Data channel.
		HeaderLength: whd.SDPCM_HEADER_LEN + PADDING_SIZE,
	}
	d.auxSDPCMHeader.Put(buf8[:whd.SDPCM_HEADER_LEN])

	d.auxBDCHeader = whd.BDCHeader{
		Flags: 2 << 4, // BDC version.
	}
	d.auxBDCHeader.Put(buf8[whd.SDPCM_HEADER_LEN+PADDING_SIZE:])

	copy(buf8[whd.SDPCM_HEADER_LEN+PADDING_SIZE+whd.BDC_HEADER_LEN:], packet)

	totalLen = align(totalLen, 4)
	return d.wlan_write(buf[:totalLen/4])
}

func (d *Device) get_iovar(VAR string, iface whd.IoctlInterface) (_ uint32, err error) {
	d.debug("get_iovar")
	const iovarOffset = 256 + 3
	buf8 := u32AsU8(d._iovarBuf[iovarOffset:])

	length := copy(buf8[:], VAR)
	buf8[length] = 0
	length++
	_busOrder.PutUint32(buf8[length:], 0)

	err = d.doIoctlGet(whd.WLC_GET_VAR, iface, buf8[:length+4])
	return _busOrder.Uint32(buf8[length:]), err
}

func (d *Device) get_iovar_n(VAR string, iface whd.IoctlInterface)

func (d *Device) set_iovar(VAR string, iface whd.IoctlInterface, val uint32) error {
	buf8 := u32AsU8(d._iovarBuf[256:]) // Safe to get offset.
	copy(buf8[:4], u32PtrTo4U8(&val)[:4])
	return d.set_iovar_n(VAR, iface, buf8[:4])
}

func (d *Device) set_iovar2(VAR string, iface whd.IoctlInterface, val0, val1 uint32) error {
	buf8 := u32AsU8(d._iovarBuf[256+1:]) // Safe to get offset.
	copy(buf8[:4], u32PtrTo4U8(&val0)[:4])
	copy(buf8[4:8], u32PtrTo4U8(&val1)[:4])
	return d.set_iovar_n(VAR, iface, buf8[:8])
}

// set_iovar_n is "set_iovar" from reference.
func (d *Device) set_iovar_n(VAR string, iface whd.IoctlInterface, val []byte) (err error) {
	d.debug("set_iovar", slog.String("var", VAR))
	buf8 := u32AsU8(d._iovarBuf[:])
	if len(val)+1+len(VAR) > len(buf8) {
		return errors.New("set_iovar value too large")
	}

	length := copy(buf8[:], VAR)
	buf8[length] = 0
	length++
	length += copy(buf8[length:], val)

	return d.doIoctlSet(whd.WLC_SET_VAR, iface, buf8[:length])
}

func (d *Device) doIoctlGet(cmd whd.SDPCMCommand, iface whd.IoctlInterface, data []byte) (err error) {
	return d.doIoctl(ioctlGET, cmd, iface, data)
}

func (d *Device) doIoctlSet(cmd whd.SDPCMCommand, iface whd.IoctlInterface, data []byte) (err error) {
	return d.doIoctl(ioctlSET, cmd, iface, data)
}

// doIoctl should probably loop until they complete succesfully?
func (d *Device) doIoctl(kind ioctlType, cmd whd.SDPCMCommand, iface whd.IoctlInterface, data []byte) error {
	if !iface.IsValid() {
		return errors.New("invalid ioctl interface")
	} else if !cmd.IsValid() {
		return errors.New("invalid ioctl command")
	} else if kind != whd.SDPCM_GET && kind != whd.SDPCM_SET {
		return errors.New("invalid ioctl kind")
	}
	d.debug("doIoctl", slog.String("cmd", cmd.String()), slog.Int("len", len(data)))
	err := d.sendIoctl(kind, cmd, iface, data)
	if err != nil {
		return err
	}
	// Should poll SDPCM for read operations to complete succesfully.
	return nil
}

// sendIoctl sends a SDPCM+CDC ioctl command to the device with data.
func (d *Device) sendIoctl(kind ioctlType, cmd whd.SDPCMCommand, iface whd.IoctlInterface, data []byte) (err error) {
	d.debug("sendIoctl", slog.Int("cmd", int(cmd)), slog.Int("len", len(data)))
	buf := d._sendIoctlBuf[:]
	buf8 := u32AsU8(buf)

	totalLen := uint32(whd.SDPCM_HEADER_LEN + whd.CDC_HEADER_LEN + len(data))
	if int(totalLen) > len(buf) {
		return errors.New("ioctl data too large " + strconv.Itoa(len(buf)))
	}
	sdpcmSeq := d.sdpcmSeq
	d.sdpcmSeq++
	d.ioctlID++

	d.auxSDPCMHeader = whd.SDPCMHeader{
		Size:         uint16(totalLen), // 2 TODO does this len need to be rounded up to u32?
		SizeCom:      ^uint16(totalLen),
		Seq:          uint8(sdpcmSeq),
		ChanAndFlags: 0, // Channel type control.
		HeaderLength: whd.SDPCM_HEADER_LEN,
	}
	d.auxSDPCMHeader.Put(buf8[:whd.SDPCM_HEADER_LEN])

	d.auxCDCHeader = whd.CDCHeader{
		Cmd:    cmd,
		Length: uint32(len(data)),
		Flags:  uint16(kind) | (uint16(iface) << whd.CDCF_IOC_IF_SHIFT),
		ID:     d.ioctlID,
	}
	d.auxCDCHeader.Put(buf8[whd.SDPCM_HEADER_LEN:])

	copy(buf8[whd.SDPCM_HEADER_LEN+whd.CDC_HEADER_LEN:], data)
	totalLen = align(totalLen, 4)

	d.debug("sendIoctl", slog.Int("totalLen", int(totalLen)))
	return d.wlan_write(buf[:totalLen/4])
}

// check_status handles F2 events while status register is set.
func (d *Device) check_status(buf []uint32) error {
	for {
		status := d.status()
		if status.F2PacketAvailable() {
			length := status.F2PacketLength()
			err := d.wlan_read(buf[:], int(length))
			if err != nil {
				return err
			}
			buf8 := u32AsU8(buf[:])
			d.rx(buf8[:length])
		} else {
			break
		}
	}
	return nil
}

func (d *Device) rx(packet []byte) {
	// Hey scott, wanna program this one? https://github.com/embassy-rs/embassy/blob/main/cyw43/src/runner.rs#L347
}
