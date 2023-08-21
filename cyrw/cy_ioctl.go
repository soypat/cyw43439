package cyrw

import (
	"encoding/hex"
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

type eventMask struct {
	iface  uint32
	events [24]uint8
}

func (e *eventMask) Unset(event whd.AsyncEventType) {
	e.events[event/8] &= ^(1 << (event % 8))
}

func (e *eventMask) Put(buf []byte) {
	_busOrder.PutUint32(buf, e.iface)
	copy(buf[1:], e.events[:])
}

func (e *eventMask) Size() int {
	return 4 + len(e.events)
}

// reference: https://github.com/embassy-rs/embassy/blob/26870082427b64d3ca42691c55a2cded5eadc548/cyw43/src/runner.rs#L225
func (d *Device) singleRun() error {
	// We do not loop in here, let user call Poll for now until we understand async mechanics at play better.
	d.log_read()
	if !d.has_credit() {
		d.warn("TX:stalled")
		return d.handle_irq(d._rxBuf[:])
	}
	// For now just do this?
	return d.handle_irq(d._rxBuf[:])
}

func (d *Device) update_credit(hdr *whd.SDPCMHeader) {
	if hdr.ChanAndFlags&0xf >= 3 {
		return // Not Control, Data or Event channel.
	}
	seqMax := hdr.BusDataCredit
	if seqMax-d.sdpcmSeq > 0x40 {
		seqMax = d.sdpcmSeq + 2
	}
	d.sdpcmSeqMax = seqMax
}

func (d *Device) has_credit() bool {
	return d.sdpcmSeq != d.sdpcmSeqMax && (d.sdpcmSeqMax-d.sdpcmSeq)&0x80 == 0
}

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
	d.auxSDPCMHeader.Put(_busOrder, buf8[:whd.SDPCM_HEADER_LEN])

	d.auxBDCHeader = whd.BDCHeader{
		Flags: 2 << 4, // BDC version.
	}
	d.auxBDCHeader.Put(buf8[whd.SDPCM_HEADER_LEN+PADDING_SIZE:])

	copy(buf8[whd.SDPCM_HEADER_LEN+PADDING_SIZE+whd.BDC_HEADER_LEN:], packet)

	totalLen = align(totalLen, 4)
	return d.wlan_write(buf[:totalLen/4])
}

func (d *Device) get_iovar(VAR string, iface whd.IoctlInterface) (_ uint32, err error) {
	const iovarOffset = 256 + 3
	buf8 := u32AsU8(d._iovarBuf[iovarOffset:])
	_, err = d.get_iovar_n(VAR, iface, buf8[:4])
	return _busOrder.Uint32(buf8), err
}

func (d *Device) get_iovar_n(VAR string, iface whd.IoctlInterface, res []byte) (plen int, err error) {
	buf8 := u32AsU8(d._iovarBuf[:])

	length := copy(buf8[:], VAR)
	buf8[length] = 0
	length++
	for i := 0; i < len(res); i++ {
		buf8[length+i] = 0 // Zero out where we'll read.
	}
	totalLen := max(len(VAR)+1, len(res))
	d.debug("get_iovar_n:ini", slog.String("var", VAR), slog.Int("reslen", len(res)), slog.String("buf", hex.EncodeToString(buf8[:totalLen])))
	plen, err = d.doIoctlGet(whd.WLC_GET_VAR, iface, buf8[:totalLen])
	if plen > len(res) {
		plen = len(res) // TODO: implement this correctly here and in IoctlGet.
	}
	d.debug("get_iovar_n:end", slog.String("var", VAR), slog.String("buf", hex.EncodeToString(buf8[:totalLen])))
	copy(res[:], buf8[:plen])
	return plen, err
}

// reference: ioctl_set_u32
func (d *Device) set_ioctl(cmd whd.SDPCMCommand, iface whd.IoctlInterface, val uint32) error {
	return d.doIoctl(whd.SDPCM_SET, cmd, iface, u32PtrTo4U8(&val)[:4])
}

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

func (d *Device) doIoctlGet(cmd whd.SDPCMCommand, iface whd.IoctlInterface, data []byte) (_ int, err error) {
	err = d.doIoctl(ioctlGET, cmd, iface, data)
	println("doIoctlGet not correctly implemented yet")
	return len(data), err // TODO: implement this correctly.
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
	if err != nil || kind == whd.SDPCM_SET {
		return err
	}
	// Should poll SDPCM for read operations to complete succesfully.
	return nil
}

// sendIoctl sends a SDPCM+CDC ioctl command to the device with data.
func (d *Device) sendIoctl(kind ioctlType, cmd whd.SDPCMCommand, iface whd.IoctlInterface, data []byte) (err error) {
	d.debug("sendIoctl", slog.Int("kind", int(kind)), slog.String("cmd", cmd.String()), slog.Int("len", len(data)))
	defer d.check_status(d._rxBuf[:])

	buf := d._sendIoctlBuf[:]
	buf8 := u32AsU8(buf)

	totalLen := uint32(whd.SDPCM_HEADER_LEN + whd.CDC_HEADER_LEN + len(data))
	if int(totalLen) > len(buf8) {
		return errors.New("ioctl data too large " + strconv.Itoa(len(data)))
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
	d.auxSDPCMHeader.Put(_busOrder, buf8[:whd.SDPCM_HEADER_LEN])

	d.auxCDCHeader = whd.CDCHeader{
		Cmd:    cmd,
		Length: uint32(len(data)),
		Flags:  uint16(kind) | (uint16(iface) << whd.CDCF_IOC_IF_SHIFT),
		ID:     d.ioctlID,
	}
	d.auxCDCHeader.Put(_busOrder, buf8[whd.SDPCM_HEADER_LEN:])
	s := hex.EncodeToString(buf8[whd.SDPCM_HEADER_LEN : whd.SDPCM_HEADER_LEN+whd.CDC_HEADER_LEN])
	d.debug("sendIoctl:cdc", slog.String("cdc", s), slog.Any("cdc_struct", &d.auxCDCHeader))

	copy(buf8[whd.SDPCM_HEADER_LEN+whd.CDC_HEADER_LEN:], data)
	totalLen = align(totalLen, 4)

	return d.wlan_write(buf[:totalLen/4])
}

// handle_irq waits for IRQ on F2 packet available
func (d *Device) handle_irq(buf []uint32) (err error) {
	irq := d.getInterrupts()
	d.debug("handle_irq", slog.String("irq", irq.String()))

	if irq.IsF2Available() {
		err = d.check_status(buf)
	}
	if err == nil && irq.IsDataAvailable() {
		d.warn("irq data unavail, clearing")
		err = d.write16(FuncBus, whd.SPI_INTERRUPT_REGISTER, 1)
	}
	return err
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
			err = d.rx(buf8[:length])
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

func (d *Device) rxControl(packet []byte) error {
	d.debug("rxControl", slog.Int("len", len(packet)))
	cdcHdr := whd.DecodeCDCHeader(_busOrder, packet)
	response, err := cdcHdr.Parse(packet)
	if err != nil {
		return err
	}
	d.debug("rxControl:cdc", slog.String("resp", string(response)))
	if cdcHdr.ID == d.ioctlID {
		if cdcHdr.Status != 0 {
			return errors.New("IOCTL error:" + strconv.Itoa(int(cdcHdr.Status)))
		}
		// TODO(sfeldma) rust -> Go
		// self.ioctl_state.ioctl_done(response);
	}
	return nil
}

func (d *Device) rxEvent(packet []byte) error {
	d.debug("rxEvent", slog.Int("len", len(packet)))
	return nil
}

func (d *Device) rxData(packet []byte) error {
	d.debug("rxData", slog.Int("len", len(packet)))
	bdcHdr := whd.DecodeBDCHeader(packet)
	d.debug("rxData:bdc", slog.Any("bdc", &bdcHdr))
	// TODO(sfeldma) send payload up as new rx eth packet
	return nil
}

func (d *Device) rx(packet []byte) error {
	//reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/runner.rs#L347
	d.debug("rx", slog.Int("len", len(packet)))

	sdpcmHdr := whd.DecodeSDPCMHeader(_busOrder, packet)
	payload, err := sdpcmHdr.Parse(packet)
	if err != nil {
		return err
	}

	d.updateCredit(sdpcmHdr)

	switch sdpcmHdr.Type() {
	case whd.CONTROL_HEADER:
		return d.rxControl(payload)
	case whd.ASYNCEVENT_HEADER:
		return d.rxEvent(payload)
	case whd.DATA_HEADER:
		return d.rxData(payload)
	}

	return errors.New("unknown sdpcm hdr type")
}
