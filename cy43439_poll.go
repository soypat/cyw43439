package cyw43439

import (
	"errors"
	"machine"

	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/cyw43439/whd"
)

func (d *Device) initIRQ() {
	d.reenableIRQ()
	d.irq.Configure(machine.PinConfig{Mode: machine.PinInput})
	// d.irq.SetInterrupt(1<<2, d.irqHandler)
}

func (d *Device) reenableIRQ() {
	const GPIO_IRQ_LEVEL_HIGH = 0x2
	ackIRQ(d.irq, GPIO_IRQ_LEVEL_HIGH)
	setIRQ(d.irq, GPIO_IRQ_LEVEL_HIGH, true)
	// println("IRQ re-enabled")
}

func (d *Device) irqHandler(pin machine.Pin) {
	// Pico-sdk definition.
	const GPIO_IRQ_LEVEL_HIGH = 0x2
	events := getIRQEventMask(d.irq)
	println("IRQ handler", events)
	if events&GPIO_IRQ_LEVEL_HIGH != 0 {
		// As we use a high level interrupt, it will go off forever until it's serviced
		// So disable the interrupt until this is done. It's re-enabled again by CYW43_POST_POLL_HOOK
		// which is called at the end of cyw43_poll_func
		setIRQ(d.irq, GPIO_IRQ_LEVEL_HIGH, false)
		// set work pending...
		println("disable IRQ")
	}
}

// ref: void cyw43_schedule_internal_poll_dispatch(__unused void (*func)(void))
// func (d *Device) pollStart() {
// 	if d.pollCancel != nil {
// 		return
// 	}
// 	d.info("STARTING POLLING")
// 	ctx, cancel := context.WithCancel(context.Background())
// 	d.pollCancel = cancel
// 	go func() {
// 		for ctx.Err() == nil {
// 			d.poll()
// 			time.Sleep(5 * time.Millisecond)
// 		}
// 		d.pollCancel = nil
// 	}()
// }

// ref: void cyw43_cb_process_ethernet(void *cb_data, int itf, size_t len, const uint8_t *buf)
func (d *Device) processEthernet(payload []byte) error {
	d.debug("processEthernet", slog.Int("plen", len(payload)))
	if d.recvEth != nil {
		// The handler MUST not hold on to references to payload when
		// returning, error or not.  Payload is backed by d.buf, and we
		// need d.buf free for the next recv.
		return d.recvEth(payload)
	}

	d.debug("RecvEthHandle handler not set, dropping Rx packet")
	return nil
}

// ref: void cyw43_ll_process_packets(cyw43_ll_t *self_in)
func (d *Device) processPackets() {
	for {
		payloadOffset, plen, header, err := d.sdpcmPoll(d.buf[:])
		d.debug("processPackets:sdpcmPoll",
			slog.Int("payloadOffset", int(payloadOffset)),
			slog.Int("plen", int(plen)),
			slog.Uint64("header", uint64(header)),
			slog.Any("err", err),
		)
		payload := d.buf[payloadOffset : payloadOffset+plen]
		switch {
		case err != nil:
			// no packet or flow control
			return
		case header == whd.ASYNCEVENT_HEADER:
			d.handleAsyncEvent(payload)
		case header == whd.DATA_HEADER:
			err = d.processEthernet(payload)
			if err != nil {
				d.logError("processPackets:processEthernet", slog.Any("err", err))
			}

		default:
			d.logError("processPackets:unexpectedHeader", slog.Uint64("header", uint64(header)))
		}
	}
}

// ref: bool cyw43_ll_has_work(cyw43_ll_t *self_in)
func (d *Device) hasWork() bool {
	if sharedDATA {
		d.irq.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	return d.irq.Get()
}

// ref: void cyw43_poll_func(void)
func (d *Device) Poll() {
	d.lock()
	defer d.unlock()
	hasWork := d.hasWork()
	d.info("Poll", slog.Bool("hadWork", hasWork))
	if hasWork {
		d.processPackets()
	}
}

func (d *Device) pollStop() {
	if d.pollCancel != nil {
		d.info("CANCEL POLLING")
		d.pollCancel()
	}
}

func (d *Device) SendEthernet(buf []byte) error {
	return d.sendEthernet(whd.CYW43_ITF_STA, buf)
}

// ref: int cyw43_ll_send_ethernet(cyw43_ll_t *self_in, int itf, size_t len, const void *buf, bool is_pbuf)
func (d *Device) sendEthernet(itf uint8, buf []byte) error {
	d.info("sendEthernet", slog.Int("itf", int(itf)), slog.Int("len", len(buf)))
	d.lock()
	defer d.unlock()
	const totalHeader = 2 + whd.SDPCM_HEADER_LEN + whd.BDC_HEADER_LEN
	if len(buf)+totalHeader > len(d.buf) {
		return errors.New("sendEthernet: packet too large")
	}

	header := whd.BDCHeader{
		Flags:  0x20,
		Flags2: itf,
	}
	header.Put(d.buf[whd.SDPCM_HEADER_LEN+2:])

	n := copy(d.buf[totalHeader:], buf)
	return d.sendSDPCMCommon(whd.DATA_HEADER, d.buf[:n+totalHeader])
}

func (d *Device) handleAsyncEvent(payload []byte) error {
	d.debug("handleAsyncEvent", slog.Int("plen", len(payload)))
	as, err := whd.ParseAsyncEvent(payload)
	if err != nil {
		return err
	}
	return d.processAsyncEvent(as)
}
