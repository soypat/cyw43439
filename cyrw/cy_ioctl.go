package cyrw

import (
	"errors"
	"strconv"

	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/cyw43439/whd"
)

type ioctlType uint8

const (
	ioctlGET = 0
	ioctlSET = 2
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

// sendIoctl sends a SDPCM+CDC ioctl command to the device with data.
func (d *Device) sendIoctl(kind ioctlType, cmd uint32, iface uint32, data []byte) (err error) {
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
			err := d.rx(buf8[:length])
			if err != nil {
				return err
			}
		} else {
			break
		}
	}
	return nil
}

func (d *Device) updateCredit(sdpcmHdr whd.SDPCMHeader) {
	//reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/runner.rs#L467
	switch sdpcmHdr.Type() {
	case whd.CONTROL_HEADER, whd.ASYNCEVENT_HEADER, whd.DATA_HEADER:
		max := sdpcmHdr.BusDataCredit
		// TODO(sfeldma) not sure about this math, rust had:
		// if sdpcm_seq_max.wrapping_sub(self.sdpcm_seq) > 0x40
		if (max - d.sdpcmSeq) > 0x40 {
			max = d.sdpcmSeq + 2
		}
		d.sdpcmSeqMax = max
	}
}

func (d *Device) rx_control(payload []byte) error {
}

func (d *Device) rx_event(payload []byte) error {
}

func (d *Device) rx_data(payload []byte) error {
}

func (d *Device) rx(packet []byte) error {
	//reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/runner.rs#L347
	d.debug("rx", slog.Int("len", len(packet)))

	var sdpcmHdr whd.SDPCMHeader

	payload, err := sdpcmHdr.Parse(packet)
	if err != nil {
		return err
	}

	d.updateCredit(sdpcmHdr)

	switch sdpcmHdr.Type() {
	case whd.CONTROL_HEADER:
		return d.rx_control(payload)
	case whd.ASYNCEVENT_HEADER:
		return d.rx_event(payload)
	case whd.DATA_HEADER:
		return d.rx_data(payload)
	}

	return errors.New("unknown sdpcm hdr type")
}
