package cyrw

import (
	"net"
	"time"

	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/cyw43439/whd"
)

func (d *Device) initControl(clm string) error {
	d.debug("initControl", slog.Int("clm_len", len(clm)))
	const chunkSize = 1024
	remaining := clm
	offset := 0
	var buf8 [chunkSize + 20]byte

	for len(remaining) > 0 {
		chunk := remaining[:min(len(remaining), chunkSize)]
		remaining = remaining[len(chunk):]
		var flag uint16 = 0x1000 // Download flag handler version.
		if offset == 0 {
			flag |= 0x0002 // Flag begin.
		}
		offset += len(chunk)
		if offset == len(clm) {
			flag |= 0x0004 // Flag end.
		}
		header := whd.DownloadHeader{ // No CRC.
			Flags: flag,
			Type:  2, // CLM download type.
			Len:   uint32(len(chunk)),
		}
		n := copy(buf8[:8], "clmload\x00")
		header.Put(_busOrder, buf8[8:20])
		n += whd.DL_HEADER_LEN
		n += copy(buf8[20:], chunk)

		err := d.sendIoctl(ioctlSET, whd.WLC_SET_VAR, whd.WWD_STA_INTERFACE, buf8[:n])
		if err != nil {
			return err
		}
	}
	d.debug("clmload:done")
	v, err := d.get_iovar("clmload_status", whd.WWD_STA_INTERFACE)
	if v != 0 || err != nil {
		// return errjoin(errors.New("clmload_status failed"), err)
	}

	// Disable tx gloming which transfers multiple packets in one request.
	// 'glom' is short for "conglomerate" which means "gather together into
	// a compact mass".
	d.set_iovar("bus:txglom", whd.WWD_STA_INTERFACE, 0)
	d.set_iovar("apsta", whd.WWD_STA_INTERFACE, 1)

	// read MAC Address:

	d.get_iovar_n("cur_etheraddr", whd.WWD_STA_INTERFACE, d.mac[:6])
	d.debug("MAC", slog.String("mac", d.MAC().String()))

	country := whd.CountryCode("XX", 0)
	d.set_iovar("country", whd.WWD_STA_INTERFACE, country)

	// set country takes some time, next ioctls fail if we don't wait.
	time.Sleep(100 * time.Millisecond)

	// Set Antenna to chip antenna.
	d.set_ioctl(whd.WLC_GET_ANTDIV, whd.WWD_STA_INTERFACE, 0)

	d.set_iovar("bus:txglom", whd.WWD_STA_INTERFACE, 0)
	time.Sleep(100 * time.Millisecond)

	d.set_iovar("ampdu_ba_wsize", whd.WWD_STA_INTERFACE, 8)
	time.Sleep(100 * time.Millisecond)

	d.set_iovar("ampdu_mpdu", whd.WWD_STA_INTERFACE, 4)
	time.Sleep(100 * time.Millisecond)

	// Ignore uninteresting/spammy events.
	var evts eventMask
	for i := range evts.events {
		evts.events[i] = 0xff
	}
	evts.Unset(whd.EvRADIO)
	evts.Unset(whd.EvIF)
	evts.Unset(whd.EvPROBREQ_MSG)
	evts.Unset(whd.EvPROBREQ_MSG_RX)
	evts.Unset(whd.EvPROBRESP_MSG)
	evts.Unset(whd.EvROAM)
	buf := make([]byte, evts.Size())
	evts.Put(buf)
	d.set_iovar_n("bsscfg:event_msgs", whd.WWD_STA_INTERFACE, buf)

	time.Sleep(100 * time.Millisecond)

	// Set wifi up.
	d.doIoctlSet(whd.WLC_UP, whd.WWD_STA_INTERFACE, nil)

	time.Sleep(100 * time.Millisecond)

	d.set_ioctl(whd.WLC_SET_GMODE, whd.WWD_STA_INTERFACE, 1) // Set GMODE=auto
	d.set_ioctl(whd.WLC_SET_BAND, whd.WWD_STA_INTERFACE, 0)  // Set BAND=any

	time.Sleep(100 * time.Millisecond)

	return nil
}

func (d *Device) MAC() net.HardwareAddr {
	return net.HardwareAddr(d.mac[:6])
}
