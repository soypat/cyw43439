package cyw43439

import (
	"errors"
	"machine"
	"time"

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
func (d *Device) processPackets() (gotPackets bool) {
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
			d.logError("processPackets:sdpcmPoll", slog.String("err", err.Error()))
			return gotPackets
		case header == whd.ASYNCEVENT_HEADER:
			gotPackets = true
			d.handleAsyncEvent(payload)
		case header == whd.DATA_HEADER:
			gotPackets = true
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

// PollUntilNextOrDeadline blocks until the next packet is received or the
// deadline is reached, whichever comes first
//
// ref: cyw43_arch_wait_for_work_until
func (d *Device) PollUntilNextOrDeadline(deadline time.Time) (gotPacket bool) {
	d.info("PollUntil", slog.Time("deadline", deadline))
	d.lock()
	defer d.unlock()
	for !gotPacket && time.Until(deadline) > 0 {
		if !d.hasWork() {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		gotPacket = d.processPackets()
	}
	return gotPacket
}

// ref: void cyw43_poll_func(void)
func (d *Device) Poll() (gotPacket bool) {
	d.lock()
	defer d.unlock()
	hasWork := d.hasWork()
	d.info("Poll", slog.Bool("hasWork", hasWork))
	if hasWork {
		gotPacket = d.processPackets()
	}
	return gotPacket
}

// func (d *Device) pollStop() {
// 	if d.pollCancel != nil {
// 		d.info("CANCEL POLLING")
// 		d.pollCancel()
// 	}
// }

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

// ref: void cyw43_cb_process_async_event(void *cb_data, const cyw43_async_event_t *ev)
func (d *Device) processAsyncEvent(ev whd.AsyncEvent) error {
	d.debug("processAsyncEvent", slog.String("EventType", ev.EventType.String()), slog.Uint64("status", uint64(ev.Status)), slog.Uint64("reason", uint64(ev.Reason)))
	switch ev.EventType {
	case whd.CYW43_EV_ESCAN_RESULT:
		// TODO
	case whd.CYW43_EV_DISASSOC:
		d.wifiJoinState = whd.WIFI_JOIN_STATE_DOWN
		d.notifyDown()
	case whd.CYW43_EV_PRUNE:
		// TODO
	case whd.CYW43_EV_SET_SSID:
		switch {
		case ev.Status == 0:
			// Success setting SSID
		case ev.Status == 3 && ev.Reason == 0:
			// No matching SSID found (could be out of range, or down)
			d.wifiJoinState = whd.WIFI_JOIN_STATE_NONET
		default:
			// Other failure setting SSID
			d.wifiJoinState = whd.WIFI_JOIN_STATE_FAIL
		}
	case whd.CYW43_EV_AUTH:
		switch ev.Status {
		case 0:
			if (d.wifiJoinState & whd.WIFI_JOIN_STATE_KIND_MASK) ==
				whd.WIFI_JOIN_STATE_BADAUTH {
				// A good-auth follows a bad-auth, so change
				// the join state back to active.
				d.wifiJoinState = (d.wifiJoinState & ^uint32(whd.WIFI_JOIN_STATE_KIND_MASK)) |
					whd.WIFI_JOIN_STATE_ACTIVE
			}
			d.wifiJoinState |= whd.WIFI_JOIN_STATE_AUTH
		case 6:
			// Unsolicited auth packet, ignore it
		default:
			// Cannot authenticate
			d.wifiJoinState = whd.WIFI_JOIN_STATE_BADAUTH
		}
	case whd.CYW43_EV_DEAUTH_IND:
		// TODO
	case whd.CYW43_EV_LINK:
		if ev.Status == 0 {
			if (ev.Flags & 1) != 0 {
				// Link is UP
				d.wifiJoinState |= whd.WIFI_JOIN_STATE_LINK
				// TODO missing some stuff
			}
		}
	case whd.CYW43_EV_PSK_SUP:
		switch {
		case ev.Status == 6:
			// WLC_SUP_KEYED
			d.wifiJoinState |= whd.WIFI_JOIN_STATE_KEYED
		case (ev.Status == 4 || ev.Status == 8 || ev.Status == 10) && ev.Reason == 15:
			// Timeout waiting for key exchange M1/M3/G1
			// Probably at edge of the cell, retry
			// TODO
		default:
			// PSK_SUP failure
			d.wifiJoinState = whd.WIFI_JOIN_STATE_BADAUTH
		}
	}

	if d.wifiJoinState == whd.WIFI_JOIN_STATE_ALL {
		// STA connected
		d.wifiJoinState = whd.WIFI_JOIN_STATE_ACTIVE
		// TODO notify link UP
	}

	return nil
}
