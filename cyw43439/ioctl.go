package cyrw

// This file is based heavily on `runner.rs` from the Embassy project.
// https://github.com/embassy-rs/embassy/blob/26870082427b64d3ca42691c55a2cded5eadc548/cyw43/src/runner.rs

import (
	"encoding/binary"
	"errors"
	"strconv"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439/whd"
)

const noPacket = whd.SDPCMHeaderType(0xff)

type ioctlType uint8

const (
	ioctlGET = whd.SDPCM_GET
	ioctlSET = whd.SDPCM_SET
)

type eventMask struct {
	// This struct takes inspiration from two structs in the reference:
	// The EventMask impl for *Enable methods: https://github.com/embassy-rs/embassy/blob/26870082427b64d3ca42691c55a2cded5eadc548/cyw43/src/events.rs#L341
	// The EventMask struct for Disable(unset) method:
	iface  uint32
	events [24]uint8
}

func (e *eventMask) Disable(event whd.AsyncEventType) {
	e.events[event/8] &= ^(1 << (event % 8))
}

func (e *eventMask) Enable(event whd.AsyncEventType) {
	e.events[event/8] |= 1 << (event % 8)
}

func (e *eventMask) IsEnabled(event whd.AsyncEventType) bool {
	return e.events[event/8]&(1<<(event%8)) != 0
}

func (e *eventMask) Put(buf []byte) {
	_busOrder.PutUint32(buf, e.iface)
	copy(buf[4:], e.events[:])
}

func (e *eventMask) Size() int {
	return 4 + len(e.events)
}

func (d *Device) update_credit(hdr *whd.SDPCMHeader) {
	//reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/runner.rs#L467
	switch hdr.Type() {
	case whd.CONTROL_HEADER, whd.ASYNCEVENT_HEADER, whd.DATA_HEADER:
		max := hdr.BusDataCredit
		if (max - d.sdpcmSeq) > 0x40 {
			max = d.sdpcmSeq + 2
		}
		d.sdpcmSeqMax = max
	}
}

func (d *Device) has_credit() bool {
	return d.sdpcmSeq != d.sdpcmSeqMax && (d.sdpcmSeqMax-d.sdpcmSeq)&0x80 == 0
}

var errTxPacketTooLarge = errors.New("tx packet too large")

// 2 is padding necessary in the SDPCM header.
const mtuPrefix = 2 + whd.SDPCM_HEADER_LEN + whd.BDC_HEADER_LEN
const MTU = 2048 - mtuPrefix

// tx transmits a SDPCM+BDC data packet to the device.
func (d *Device) tx(packet []byte) (err error) {
	// reference: https://github.com/embassy-rs/embassy/blob/6babd5752e439b234151104d8d20bae32e41d714/cyw43/src/runner.rs#L247
	d.debug("tx", slog.Int("len", len(packet)))
	buf := d._sendIoctlBuf[:]
	buf8 := u32AsU8(buf)

	// There MUST be 2 bytes of padding between the SDPCM and BDC headers (only for data packets). See reference.
	// "¯\_(ツ)_/¯"

	const PADDING_SIZE = 2
	totalLen := mtuPrefix + len(packet)
	if totalLen > len(buf8) {
		return errTxPacketTooLarge
	}
	d.log_read()

	err = d.waitForCredit(buf)
	if err != nil {
		return err
	}

	seq := d.sdpcmSeq
	d.sdpcmSeq++ // Go wraps around on overflow by default.

	d.lastSDPCMHeader = whd.SDPCMHeader{
		Size:         uint16(totalLen), // TODO does this len need to be rounded up to u32?
		SizeCom:      ^uint16(totalLen),
		Seq:          uint8(seq),
		ChanAndFlags: 2, // Data channel.
		HeaderLength: whd.SDPCM_HEADER_LEN + PADDING_SIZE,
	}
	d.lastSDPCMHeader.Put(_busOrder, buf8[:whd.SDPCM_HEADER_LEN])

	d.auxBDCHeader = whd.BDCHeader{
		Flags: 2 << 4, // BDC version.
	}
	d.auxBDCHeader.Put(buf8[whd.SDPCM_HEADER_LEN+PADDING_SIZE:])

	copy(buf8[whd.SDPCM_HEADER_LEN+PADDING_SIZE+whd.BDC_HEADER_LEN:], packet)

	return d.wlan_write(buf[:align(uint32(totalLen), 4)/4], uint32(totalLen))
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

	totalLen := max(length, len(res))
	d.trace("get_iovar_n:ini", slog.String("var", VAR), slog.Int("reslen", totalLen))
	plen, err = d.doIoctlGet(whd.WLC_GET_VAR, iface, buf8[:totalLen])
	if plen > len(res) {
		plen = len(res) // TODO: implement this correctly here and in IoctlGet.
	}
	copy(res[:], buf8[:plen])
	return plen, err
}

// reference: ioctl_set_u32
func (d *Device) set_ioctl(cmd whd.SDPCMCommand, iface whd.IoctlInterface, val uint32) error {
	return d.doIoctlSet(cmd, iface, u32PtrTo4U8(&val)[:4])
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
	d.trace("set_iovar", slog.String("var", VAR))
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

func (d *Device) doIoctlGet(cmd whd.SDPCMCommand, iface whd.IoctlInterface, data []byte) (n int, err error) {
	d.trace("doIoctlGet:start", slog.String("cmd", cmd.String()), slog.String("iface", iface.String()), slog.Int("len", len(data)))
	packet, err := d.sendIoctlWait(ioctlGET, cmd, iface, data)
	if err != nil {
		return 0, err
	}

	n = copy(data[:], packet)
	return n, nil
}

func (d *Device) doIoctlSet(cmd whd.SDPCMCommand, iface whd.IoctlInterface, data []byte) (err error) {
	_, err = d.sendIoctlWait(ioctlSET, cmd, iface, data)
	return err
}

// sendIoctlWait sends an ioctl and waits for its completion
func (d *Device) sendIoctlWait(kind uint8, cmd whd.SDPCMCommand, iface whd.IoctlInterface, data []byte) ([]byte, error) {
	d.trace("sendIoctlWait:start")
	d.log_read()

	err := d.waitForCredit(d._sendIoctlBuf[:])
	if err != nil {
		return nil, err
	}
	err = d.sendIoctl(kind, cmd, iface, data)
	if err != nil {
		return nil, err
	}
	packet, err := d.pollForIoctl(d._sendIoctlBuf[:])
	if err != nil {
		d.logerr("sendIoctlWait:pollForIoctl", slog.String("err", err.Error()))
		return nil, err
	}

	return packet, err
}

// sendIoctl sends a SDPCM+CDC ioctl command to the device with data.
func (d *Device) sendIoctl(kind uint8, cmd whd.SDPCMCommand, iface whd.IoctlInterface, data []byte) (err error) {
	d.trace("sendIoctl:start")
	if !iface.IsValid() {
		return errors.New("invalid ioctl interface")
	} else if !cmd.IsValid() {
		return errors.New("invalid ioctl command")
	} else if kind != whd.SDPCM_GET && kind != whd.SDPCM_SET {
		return errors.New("invalid ioctl kind")
	}
	d.debug("sendIoctl", slog.Int("kind", int(kind)), slog.String("cmd", cmd.String()), slog.Int("len", len(data)))

	buf := d._sendIoctlBuf[:]
	buf8 := u32AsU8(buf)

	totalLen := uint32(whd.SDPCM_HEADER_LEN + whd.CDC_HEADER_LEN + len(data))
	if int(totalLen) > len(buf8) {
		return errors.New("ioctl data too large " + strconv.Itoa(len(data)))
	}
	sdpcmSeq := d.sdpcmSeq
	d.sdpcmSeq++
	d.ioctlID++

	d.lastSDPCMHeader = whd.SDPCMHeader{
		Size:         uint16(totalLen), // 2 TODO does this len need to be rounded up to u32?
		SizeCom:      ^uint16(totalLen),
		Seq:          uint8(sdpcmSeq),
		ChanAndFlags: 0, // Channel type control.
		HeaderLength: whd.SDPCM_HEADER_LEN,
	}
	d.lastSDPCMHeader.Put(_busOrder, buf8[:whd.SDPCM_HEADER_LEN])

	d.auxCDCHeader = whd.CDCHeader{
		Cmd:    cmd,
		Length: uint32(len(data)),
		Flags:  uint16(kind) | (uint16(iface) << whd.CDCF_IOC_IF_SHIFT),
		ID:     d.ioctlID,
	}
	d.auxCDCHeader.Put(_busOrder, buf8[whd.SDPCM_HEADER_LEN:])

	copy(buf8[whd.SDPCM_HEADER_LEN+whd.CDC_HEADER_LEN:], data)

	return d.wlan_write(buf[:align(totalLen, 4)/4], totalLen)
}

// handle_irq waits for IRQ on F2 packet available
func (d *Device) handle_irq(buf []uint32) (err error) {
	irq := d.getInterrupts()
	d.trace("handle_irq", slog.String("irq", irq.String()))

	if irq.IsF2Available() {
		err = d.check_status(buf)
	}
	if err == nil && irq.IsDataUnavailable() {
		d.warn("irq data unavail, clearing")
		err = d.write16(FuncBus, whd.SPI_INTERRUPT_REGISTER, 1)
	}
	return err
}

// TryPoll attempts to read a packet from the device. Returns true if a packet
// was read, false if no packet was available.
func (d *Device) TryPoll() (gotPacket bool, err error) {
	d.lock()
	defer d.unlock()
	_, cmd, err := d.tryPoll(d._rxBuf[:])
	if err == errNoF2Avail {
		return false, nil
	}
	return err == nil && cmd == whd.CONTROL_HEADER, err
}

// poll services any F2 packets.
//
// This is the moral equivalent of an ISR to service hw interrupts.  In this
// case,  we'll run poll() as a go function to simulate real hw interrupts.
//
// TODO get real hw interrupts working and ditch polling
func (d *Device) irqPoll() {
	for {
		d.lock()
		d.log_read()
		d.handle_irq(d._rxBuf[:])
		d.unlock()
		// Avoid busy waiting on idle.  Trade off here is time sleeping
		// is time added to receive latency.
		time.Sleep(10 * time.Millisecond)
	}
}

// f2PacketAvail checks if a packet is available, and if so, returns
// the packet length.
func (d *Device) f2PacketAvail() (bool, uint16) {
	// First, check cached status from previous cmd_read|cmd_write
	status := d.spi.Status()
	if status.F2PacketAvailable() {
		return true, status.F2PacketLength()
	}
	// If that didn't work, get the interurpt status, which updates cached
	// status
	irq := d.getInterrupts()
	if irq.IsF2Available() {
		status = d.spi.Status()
		if status.F2PacketAvailable() {
			return true, status.F2PacketLength()
		}
	}
	if irq.IsDataUnavailable() {
		d.warn("irq data unavail, clearing")
		d.write16(FuncBus, whd.SPI_INTERRUPT_REGISTER, 1)
	}
	return false, 0
}

// Ioctl polling errors.
var (
	errNoF2Avail            = errors.New("no packet available")
	errWaitForCreditTimeout = errors.New("waitForCredit timeout")
)

// waitForCredit waits for a credit to use for the next transaction
func (d *Device) waitForCredit(buf []uint32) error {
	d.trace("waitForCredit:start")
	if d.has_credit() {
		return nil
	}
	for retries := 0; retries < 10; retries++ {
		_, _, err := d.tryPoll(buf)
		if err != nil && err != errNoF2Avail {
			return err
		} else if d.has_credit() {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errWaitForCreditTimeout
}

// pollForIoctl polls until a control/ioctl/cdc packet is received.
func (d *Device) pollForIoctl(buf []uint32) ([]byte, error) {
	d.trace("pollForIoctl:start")
	for retries := 0; retries < 10; retries++ {
		buf8, hdr, err := d.tryPoll(buf)
		if err != nil && err != errNoF2Avail {
			return nil, err
		} else if hdr == whd.CONTROL_HEADER {
			return buf8, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, errors.New("pollForIoctl timeout")
}

// check_status handles F2 events while status register is set.
func (d *Device) check_status(buf []uint32) error {
	d.trace("check_status:start")
	for {
		_, _, err := d.tryPoll(buf)
		if err == errNoF2Avail {
			return nil
		} else if err != nil {
			return err
		}
	}
}

// tryPoll attempts a single read over the WLAN interface for a SDPCM packet.
// If a packet is received then it is processed by rx and a nil error is returned.
// If no packet is available then it returns errNoPacketAvail as the error.
// If an error is returned it will return whd.UNKNOWN_HEADER as the header type.
func (d *Device) tryPoll(buf []uint32) ([]byte, whd.SDPCMHeaderType, error) {
	d.trace("tryPoll:start")
	avail, length := d.f2PacketAvail()
	if !avail {
		return nil, whd.UNKNOWN_HEADER, errNoF2Avail
	}
	err := d.wlan_read(buf[:], int(length))
	if err != nil {
		return nil, whd.UNKNOWN_HEADER, err
	}
	buf8 := u32AsU8(buf[:])
	offset, plen, hdrType, err := d.rx(buf8[:length])

	if err == whd.ErrInvalidEtherType || err == errBDCInvalidLength { // Both result from event packet parsing.
		// Spurious Poll error correction.
		// TODO(soypat): get to the bottom of this-
		// why are we getting these errors exclusively during first IO operations?
		d.debug("tryPoll:ignore_spurious", slog.String("err", err.Error()))
		err = nil
	}
	return buf8[offset : offset+plen], hdrType, err
}

func (d *Device) rx(packet []byte) (offset, plen uint16, _ whd.SDPCMHeaderType, err error) {
	d.trace("rx:start")
	//reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/runner.rs#L347
	if len(packet) < whd.SDPCM_HEADER_LEN+whd.BDC_HEADER_LEN+1 {
		d.logerr("PACKET TOO SHORT", slog.Int("len", len(packet)))
		return 0, 0, noPacket, errors.New("packet too short")
	}

	d.lastSDPCMHeader = whd.DecodeSDPCMHeader(_busOrder, packet)
	hdrType := d.lastSDPCMHeader.Type()
	d.debug("rx", slog.Int("len", len(packet)), slog.String("hdr", hdrType.String()))
	payload, err := d.lastSDPCMHeader.Parse(packet)
	if err != nil {
		return 0, 0, noPacket, err
	}
	d.update_credit(&d.lastSDPCMHeader)

	// Other Rx methods received the payload without SDPCM header.
	switch hdrType {
	case whd.CONTROL_HEADER:
		offset, plen, err = d.rxControl(payload)
	case whd.ASYNCEVENT_HEADER:
		err = d.rxEvent(payload)
	case whd.DATA_HEADER:
		err = d.rxData(payload)
	default:
		err = errors.New("unknown sdpcm hdr type")
	}
	if err != nil {
		d.logerr("rx", slog.String("err", err.Error()))
	}
	return offset, plen, hdrType, err
}

func (d *Device) rxControl(packet []byte) (offset, plen uint16, err error) {
	defer func() {
		if err != nil {
			d.logerr("rxControl",
				slog.Int("len", len(packet)),
				slog.Int("id", int(d.auxCDCHeader.ID)),
				slog.String("cdc.Cmd", d.auxCDCHeader.Cmd.String()),
				slog.Int("cdc.Flags", int(d.auxCDCHeader.Flags)),
				slog.Int("cdc.Len", int(d.auxCDCHeader.Length)),
			)
		}
	}()
	d.auxCDCHeader = whd.DecodeCDCHeader(_busOrder, packet)

	if d.auxCDCHeader.ID == d.ioctlID {
		if d.auxCDCHeader.Status != 0 {
			return 0, 0, errors.New("IOCTL error:" + strconv.Itoa(int(d.auxCDCHeader.Status)))
		}
	}
	offset = uint16(d.lastSDPCMHeader.HeaderLength + whd.CDC_HEADER_LEN)
	// NB: losing some precision here (uint16(uint32)).
	plen = uint16(d.auxCDCHeader.Length)
	d.trace("rxControl:success", slog.Int("plen", int(plen)))
	return offset, plen, nil
}

var (
	errBDCInvalidLength = errors.New("BDC header invalid length")
	errPacketSmol       = errors.New("asyncEvent packet too small for parsing")
)

func (d *Device) rxEvent(packet []byte) (err error) {
	var bdcHdr whd.BDCHeader
	var aePacket whd.EventPacket
	defer func() {
		if err != nil {
			d.logerr("rxEvent",
				slog.String("err", err.Error()),
				slog.Int("plen", len(packet)),
				slog.Int("bdc.Flags", int(bdcHdr.Flags)),
				slog.Int("bdc.Priority", int(bdcHdr.Priority)),
				slog.Int("ae.Status", int(aePacket.Message.Status)),
			)
		}
	}()
	// Split packet into BDC header:payload.
	if len(packet) < whd.BDC_HEADER_LEN {
		return errPacketSmol
	}
	bdcHdr = whd.DecodeBDCHeader(packet)
	packetStart := whd.BDC_HEADER_LEN + 4*int(bdcHdr.DataOffset)
	if packetStart > len(packet) {
		return errBDCInvalidLength
	}
	bdcPacket := packet[packetStart:]

	// Split BDC payload into Event header:payload.
	// After this point we are in big endian (network order).
	aePacket, err = whd.DecodeEventPacket(binary.BigEndian, bdcPacket)
	if err != nil {
		return err
	}
	ev := aePacket.Message.EventType
	if !d.eventmask.IsEnabled(ev) {
		d.trace("rxEvent:ignore", slog.String("event", ev.String()))
		return nil
	}
	d.info("rxEvent:success", slog.String("event", ev.String()))
	return nil
}

func (d *Device) rxData(packet []byte) (err error) {
	if d.rcvEth != nil {
		bdcHdr := whd.DecodeBDCHeader(packet)
		packetStart := whd.BDC_HEADER_LEN + 4*int(bdcHdr.DataOffset)
		if packetStart > len(packet) {
			return errors.New("rxData: Invalid length in BDC header")
		}
		payload := packet[packetStart:]
		return d.rcvEth(payload)
	}
	return nil
}
