package cyrw

import (
	"encoding/binary"
	"errors"
	"net"
	"time"

	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/cyw43439/whd"
)

func (d *Device) WifiJoin(ssid, password string) (err error) {
	d.info("WifiJoin", slog.String("ssid", ssid), slog.Int("passlen", len(password)))
	err = d.set_power_management(PowerSave)
	if err != nil {
		return err
	}
	if password == "" {
		return d.join_open(ssid)
	}
	return errors.New("wpa_auth not implemented")
}

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

		err := d.doIoctlSet(whd.WLC_SET_VAR, whd.IF_STA, buf8[:n])
		if err != nil {
			return err
		}
	}
	d.debug("clmload:done")
	v, err := d.get_iovar("clmload_status", whd.IF_STA)
	if v != 0 || err != nil {
		// return errjoin(errors.New("clmload_status failed"), err)
	}

	// Disable tx gloming which transfers multiple packets in one request.
	// 'glom' is short for "conglomerate" which means "gather together into
	// a compact mass".
	d.set_iovar("bus:txglom", whd.IF_STA, 0)
	d.set_iovar("apsta", whd.IF_STA, 1)

	// read MAC Address:

	d.get_iovar_n("cur_etheraddr", whd.IF_STA, d.mac[:6])
	d.debug("MAC", slog.String("mac", d.MAC().String()))

	country := whd.CountryCode("XX", 0)
	d.set_iovar("country", whd.IF_STA, country)

	// set country takes some time, next ioctls fail if we don't wait.
	time.Sleep(100 * time.Millisecond)

	// Set Antenna to chip antenna.
	d.set_ioctl(whd.WLC_GET_ANTDIV, whd.IF_STA, 0)

	d.set_iovar("bus:txglom", whd.IF_STA, 0)
	time.Sleep(100 * time.Millisecond)

	d.set_iovar("ampdu_ba_wsize", whd.IF_STA, 8)
	time.Sleep(100 * time.Millisecond)

	d.set_iovar("ampdu_mpdu", whd.IF_STA, 4)
	time.Sleep(100 * time.Millisecond)

	// Ignore uninteresting/spammy events.
	var evts eventMask
	for i := range evts.events {
		evts.events[i] = 0xff
	}
	evts.Disable(whd.EvRADIO)
	evts.Disable(whd.EvIF)
	evts.Disable(whd.EvPROBREQ_MSG)
	evts.Disable(whd.EvPROBREQ_MSG_RX)
	evts.Disable(whd.EvPROBRESP_MSG)
	evts.Disable(whd.EvROAM)
	buf := make([]byte, evts.Size())
	evts.Put(buf)
	d.set_iovar_n("bsscfg:event_msgs", whd.IF_STA, buf)

	time.Sleep(100 * time.Millisecond)

	// Set wifi up.
	d.doIoctlSet(whd.WLC_UP, whd.IF_STA, nil)

	time.Sleep(100 * time.Millisecond)

	d.set_ioctl(whd.WLC_SET_GMODE, whd.IF_STA, 1) // Set GMODE=auto
	d.set_ioctl(whd.WLC_SET_BAND, whd.IF_STA, 0)  // Set BAND=any

	time.Sleep(100 * time.Millisecond)

	return nil
}

func (d *Device) MAC() net.HardwareAddr {
	return net.HardwareAddr(d.mac[:6])
}

func (d *Device) set_power_management(mode powerManagementMode) error {
	d.debug("set_power_management", slog.String("mode", mode.String()))
	if !mode.IsValid() {
		return errors.New("invalid power management mode")
	}
	mode_num := mode.mode()
	if mode_num == 0 {
		d.set_iovar("pm2_sleep_ret", whd.IF_STA, uint32(mode.sleep_ret_ms()))
		d.set_iovar("bcn_li_bcn", whd.IF_STA, uint32(mode.beacon_period()))
		d.set_iovar("bcn_li_dtim", whd.IF_STA, uint32(mode.dtim_period()))
		d.set_iovar("assoc_listen", whd.IF_STA, uint32(mode.assoc()))
	}
	return d.set_ioctl(whd.WLC_SET_PM, whd.IF_STA, uint32(mode_num))
}

func (d *Device) join_open(ssid string) error {
	d.debug("join_open", slog.String("ssid", ssid))
	if len(ssid) > 32 {
		return errors.New("ssid too long")
	}
	d.set_iovar("ampdu_ba_wsize", whd.IF_STA, 8)
	d.set_ioctl(whd.WLC_SET_WSEC, whd.IF_STA, 0)
	d.set_iovar2("bsscfg:sup_wpa", whd.IF_STA, 0, 0)
	d.set_ioctl(whd.WLC_SET_INFRA, whd.IF_STA, 1)
	d.set_ioctl(whd.WLC_SET_AUTH, whd.IF_STA, 0)

	i := ssidInfo{
		Length: uint32(len(ssid)),
	}
	copy(i.SSID[:], ssid)

	return d.wait_for_join(&i)
}

type ssidInfo struct {
	Length uint32
	SSID   [32]byte
}

func (d *ssidInfo) Put(order binary.ByteOrder, b []byte) {
	order.PutUint32(b[0:4], d.Length)
	copy(b[4:36], d.SSID[:min(len(d.SSID), int(d.Length))])
}

func (d *Device) wait_for_join(ssid *ssidInfo) (err error) {
	d.eventmask.Enable(whd.EvSET_SSID)
	d.eventmask.Enable(whd.EvAUTH)

	var buf [36]byte
	ssid.Put(_busOrder, buf[:])

	err = d.doIoctlSet(whd.WLC_GET_SSID, whd.IF_STA, buf[:36])
	if err != nil {
		return err
	}

	// Poll for async events.
	deadline := time.Now().Add(10 * time.Second)
	for time.Until(deadline) > 0 {
		err = d.check_status(d._sendIoctlBuf[:])
		if err != nil {
			return err
		}
		time.Sleep(time.Second)
	}

	return nil
}
