package cyw43439

import (
	"encoding/binary"
	"errors"
	"time"
	"unsafe"

	"github.com/soypat/cyw43439/internal/netlink"
	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/cyw43439/whd"
)

// ref: int cyw43_arch_wifi_connect_timeout_ms(const char *ssid, const char *pw, uint32_t auth, uint32_t timeout_ms)
// ref: cyw43_arch_wifi_connect_bssid_until
func (d *Device) WifiConnectTimeout(ssid, pass string, auth uint32, timeout time.Duration) error {
	d.info("WifiConnectTimeout",
		slog.String("SSID", ssid),
		slog.Int("passlen", len(pass)),
		slog.Uint64("itf", uint64(d.itfState)),
		slog.Uint64("wifiJoinState", uint64(d.wifiJoinState)),
	)

	deadline := time.Now().Add(timeout)
	if err := d.wifiConnect(ssid, pass, auth); err != nil {
		return err
	}
	status := int32(whd.CYW43_LINK_UP + 1)
	for status >= 0 && status != whd.CYW43_LINK_UP {
		newStatus := d.tcpipLinkStatus(whd.CYW43_ITF_STA)
		if newStatus == whd.CYW43_LINK_NONET {
			newStatus = whd.CYW43_LINK_JOIN
			if err := d.wifiConnect(ssid, pass, auth); err != nil {
				return err
			}
		}
		if newStatus != status {
			d.info("WifiConnectTimeout:status_change", slog.Int("newstatus", int(newStatus)), slog.Int("oldstatus", int(status)))
			status = newStatus
		}
		if time.Until(deadline) <= 0 {
			return netlink.ErrConnectTimeout
		}
		d.PollUntilNextOrDeadline(deadline)
	}
	if status == whd.CYW43_LINK_UP {
		return nil
	} else if status == whd.CYW43_LINK_BADAUTH {
		return netlink.ErrAuthFailure
	}
	return netlink.ErrConnectFailed
}

func (d *Device) isSTAActive() bool { return (d.itfState>>whd.CYW43_ITF_STA)&1 != 0 }
func (d *Device) isAPActive() bool  { return (d.itfState>>whd.CYW43_ITF_AP)&1 != 0 }

// ref: int cyw43_arch_wifi_connect_bssid_async(const char *ssid, const uint8_t *bssid, const char *pw, uint32_t auth)
func (d *Device) wifiConnect(ssid, pass string, auth uint32) error {
	if !d.isSTAActive() {
		return errors.New("wifiConnect: STA not active")
	}
	d.lock()
	defer d.unlock()
	d.info("wifiConnect",
		slog.String("SSID", ssid),
		slog.Int("passlen", len(pass)),
		slog.Uint64("auth", uint64(auth)),
		slog.Uint64("itf", uint64(d.itfState)),
		slog.Uint64("wifiJoinState", uint64(d.wifiJoinState)),
	)
	if pass == "" {
		auth = whd.CYW43_AUTH_OPEN
	}
	err := d.wifiJoin(ssid, pass, nil, auth, whd.CYW43_CHANNEL_NONE)
	if err != nil {
		return err
	}
	// Wait for responses: EV_AUTH, EV_LINK, EV_SET_SSID, EV_PSK_SUP
	// Will get EV_DEAUTH_IND if password is invalid
	d.wifiJoinState = whd.WIFI_JOIN_STATE_ACTIVE
	if auth == whd.CYW43_AUTH_OPEN {
		// For open security we don't need EV_PSK_SUP, so set that flag indicator now
		d.wifiJoinState |= whd.WIFI_JOIN_STATE_KEYED
	}
	d.info("wifiConnect:success",
		slog.Uint64("itf", uint64(d.itfState)),
		slog.Uint64("wifiJoinState", uint64(d.wifiJoinState)),
	)
	return nil
}

// ref: int cyw43_tcpip_link_status(cyw43_t *self, int itf)
func (d *Device) tcpipLinkStatus(itf uint8) int32 {
	// TODO(soypat): add TCPIP netlink status here prob.
	return d.wifiLinkStatus(itf)
}

// Returns enum LINK_JOIN, LINK_FAIL, LINK_NONET, LINK_BADAUTH, LINK_DOWN. LINK_DOWN may indicate further failure.
// ref: int cyw43_wifi_link_status(cyw43_t *self, int itf)
func (d *Device) wifiLinkStatus(itf uint8) (linkStat int32) {
	if itf != whd.CYW43_ITF_STA {
		return whd.CYW43_LINK_DOWN
	}
	switch d.wifiJoinState & whd.WIFI_JOIN_STATE_KIND_MASK {
	case whd.WIFI_JOIN_STATE_ACTIVE:
		return whd.CYW43_LINK_JOIN
	case whd.WIFI_JOIN_STATE_FAIL:
		return whd.CYW43_LINK_FAIL
	case whd.WIFI_JOIN_STATE_NONET:
		return whd.CYW43_LINK_NONET
	case whd.WIFI_JOIN_STATE_BADAUTH:
		return whd.CYW43_LINK_BADAUTH
	default:
		return whd.CYW43_LINK_DOWN
	}
}

// reference: cyw43_ll_wifi_join
func (d *Device) wifiJoin(ssid, key string, bssid *[6]byte, authType, channel uint32) (err error) {
	defer func() {
		if err != nil {
			d.logError("wifiJoin:failed", slog.Any("err", err))
		} else {
			d.info("wifiJoin:success")
		}
	}()
	{
		// Logging.
		bssAttr := slog.String("bssid", "<nil>")
		if bssid != nil {
			bssAttr = slog.String("bssid", string(bssid[:]))
		}
		d.info("wifiJoin",
			slog.String("ssid", ssid),
			slog.Int("passlen", len(key)),
			slog.Uint64("authType", uint64(authType)),
			slog.Uint64("channel", uint64(channel)),
			bssAttr,
		)
	}

	var buf [128]byte
	err = d.WriteIOVar("ampdu_ba_wsize", whd.WWD_STA_INTERFACE, 8)
	if err != nil {
		return err
	}

	var wpa_auth uint32
	if authType == whd.CYW43_AUTH_WPA2_AES_PSK || authType == whd.CYW43_AUTH_WPA2_MIXED_PSK {
		wpa_auth = whd.CYW43_WPA2_AUTH_PSK
	} else if authType == whd.CYW43_AUTH_WPA_TKIP_PSK {
		wpa_auth = whd.CYW43_WPA_AUTH_PSK
	} else if authType == whd.CYW43_AUTH_OPEN {
		// wpa_auth = 0
	} else {
		return errors.New("unsupported auth type")
	}
	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_WSEC, uint32(authType)&0xff)
	if err != nil {
		return err
	}

	// Supplicant variable.
	wpaSup := b2u32(wpa_auth != 0)
	err = d.WriteIOVar2("bsscfg:sup_wpa", whd.WWD_STA_INTERFACE, 0, wpaSup)
	if err != nil {
		return err
	}

	// set the EAPOL version to whatever the AP is using (-1).
	err = d.WriteIOVar2("bsscfg:sup_wpa2_eapver", whd.WWD_STA_INTERFACE, 0, negative1)
	if err != nil {
		return err
	}

	// wwd_wifi_set_supplicant_eapol_key_timeout
	const CYW_EAPOL_KEY_TIMEOUT = 5000
	err = d.WriteIOVar2("bsscfg:sup_wpa_tmo", whd.WWD_STA_INTERFACE, 0, CYW_EAPOL_KEY_TIMEOUT)
	if err != nil {
		return
	}

	if authType != whd.CYW43_AUTH_OPEN {
		// wwd_wifi_set_passphrase
		binary.LittleEndian.PutUint16(buf[:], uint16(len(key)))
		binary.LittleEndian.PutUint16(buf[2:], 1)
		copy(buf[4:], key)
		time.Sleep(2 * time.Millisecond) // Delay required to allow radio firmware to be ready to receive PMK and avoid intermittent failure

		err = d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_WSEC_PMK, buf[:68])
		if err != nil {
			return err
		}
	}

	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_INFRA, 1) // Set infrastructure mode.
	if err != nil {
		return err
	}

	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_AUTH, 0) // Set auth type (open system).
	if err != nil {
		return err
	}

	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_WPA_AUTH, wpa_auth) // Set WPA auth mode.
	if err != nil {
		return err
	}

	// allow relevant events through:
	//  EV_SET_SSID=0
	//  EV_AUTH=3
	//  EV_DEAUTH_IND=6
	//  EV_DISASSOC_IND=12
	//  EV_LINK=16
	//  EV_PSK_SUP=46
	//  EV_ESCAN_RESULT=69
	//  EV_CSA_COMPLETE_IND=80
	/*
	   memcpy(buf, "\x00\x00\x00\x00" "\x49\x10\x01\x00\x00\x40\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00", 4 + 18);
	   cyw43_write_iovar_n(self, "bsscfg:event_msgs", 4 + 18, buf, WWD_STA_INTERFACE);
	*/

	// Set SSID.

	binary.LittleEndian.PutUint32(d.lastSSIDJoined[:], uint32(len(ssid)))
	copy(d.lastSSIDJoined[4:], ssid)

	if bssid == nil {
		// Join SSID. Rejoin uses d.lastSSIDJoined.
		d.debug("wifiJoin:wifiRejoin")
		return d.wifiRejoin()
	}

	// BSSID is not nil so join the AP.
	d.debug("wifiJoin:setbssid")
	for i := 0; i < 4+32+20+14; i++ {
		buf[i] = 0
	}
	copy(buf[:], d.lastSSIDJoined[:])
	// Scan parameters:
	buf[36] = 0                                        // Scan type
	binary.LittleEndian.PutUint32(buf[40:], negative1) // Nprobes.
	binary.LittleEndian.PutUint32(buf[44:], negative1) // Active time.
	binary.LittleEndian.PutUint32(buf[48:], negative1) // Passive time.
	binary.LittleEndian.PutUint32(buf[52:], negative1) // Home time.
	// Assoc params.
	const (
		WL_CHANSPEC_BW_20       = 0x1000
		WL_CHANSPEC_CTL_SB_LLL  = 0x0000
		WL_CHANSPEC_CTL_SB_NONE = WL_CHANSPEC_CTL_SB_LLL
		WL_CHANSPEC_BAND_2G     = 0x0000
	)
	copy(buf[4+32+20:], bssid[:6])
	if channel != whd.CYW43_CHANNEL_NONE {
		binary.LittleEndian.PutUint32(buf[4+32+20+8:], 1) // Channel spec number.
		chspec := uint16(channel) | WL_CHANSPEC_BW_20 | WL_CHANSPEC_CTL_SB_NONE | WL_CHANSPEC_BAND_2G
		binary.LittleEndian.PutUint16(buf[4+32+20+12:], chspec) // Channel spec list.
	}

	// Join the AP.
	d.debug("wifiJoin:joinSTA")
	return d.WriteIOVarN("join", whd.WWD_STA_INTERFACE, buf[:4+32+20+14])
}

// reference: cyw43_ll_wifi_rejoin
func (d *Device) wifiRejoin() error {
	return d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_SSID, d.lastSSIDJoined[:36])
}

// reference: cyw43_ll_wifi_on
func (d *Device) wifiOn(country uint32) error {
	d.info("wifiOn", slog.Int("country", int(country)))
	buf := d.offbuf()
	copy(buf, "country\x00")
	binary.LittleEndian.PutUint32(buf[8:12], country&0xff_ff)
	if country>>16 == 0 {
		binary.LittleEndian.PutUint32(buf[12:16], negative1)
	} else {
		binary.LittleEndian.PutUint32(buf[12:16], country>>16)
	}
	binary.LittleEndian.PutUint32(buf[16:20], country&0xff_ff)

	err := d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_VAR, buf[:20])
	if err != nil {
		return err
	}

	time.Sleep(20 * time.Millisecond)

	// Set antenna to chip antenna
	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_ANTDIV, 0)
	if err != nil {
		return err
	}

	// Set some WiFi config
	err = d.WriteIOVar("bus:txglom", whd.WWD_STA_INTERFACE, 0) // Tx glomming off.
	if err != nil {
		return err
	}
	err = d.WriteIOVar("apsta", whd.WWD_STA_INTERFACE, 1) // apsta on.
	if err != nil {
		return err
	}
	err = d.WriteIOVar("ampdu_ba_wsize", whd.WWD_STA_INTERFACE, 8)
	if err != nil {
		return err
	}
	err = d.WriteIOVar("ampdu_mpdu", whd.WWD_STA_INTERFACE, 4)
	if err != nil {
		return err
	}
	err = d.WriteIOVar("ampdu_rx_factor", whd.WWD_STA_INTERFACE, 0)
	if err != nil {
		return err
	}

	// This delay is needed for the WLAN chip to do some processing, otherwise
	// SDIOIT/OOB WL_HOST_WAKE IRQs in bus-sleep mode do no work correctly.
	time.Sleep(150 * time.Millisecond) // TODO(soypat): Not critical: rewrite to only sleep if 150ms did not elapse since startup.

	/*

		Disable this code chunk for now as it doesn't appear in the C trace

		const (
			msg    = "bsscfg:event_msgs\x00"
			msgLen = len(msg)
		)
		copy(buf, msg)
		for i := 0; i < 19; i++ {
			buf[22+i] = 0xff // Clear async events.
		}
		clrEv := func(buf []byte, i int) {
			buf[18+4+i/8] &= ^(1 << (i % 8))
		}
		clrEv(buf, 19)
		clrEv(buf, 20)
		clrEv(buf, 40)
		clrEv(buf, 44)
		clrEv(buf, 54)
		clrEv(buf, 71)

		err = d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_VAR, buf[:18+4+19])
		if err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)

		// Enable multicast ethernet frames on IPv4 mDNS MAC address
		// (01:00:5e:00:00:fb).
		// This is needed for mDNS to work.
		binary.LittleEndian.PutUint32(buf[:4], 1)
		buf[4] = 0x01
		buf[5] = 0x00
		buf[6] = 0x5e
		buf[7] = 0x00
		buf[8] = 0x00
		buf[9] = 0xfb
		for i := 0; i < 9*6; i++ {
			buf[10+i] = 0
		}
		err = d.WriteIOVarN("mcast_list", whd.WWD_STA_INTERFACE, buf[:4+10*6])
		if err != nil {
			return err
		}
	*/
	time.Sleep(50 * time.Millisecond)

	// Set interface as "up".
	err = d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_UP, nil)
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return nil
}

// EnableStaMode enables the wifi interface in station mode.
//
// ref: void cyw43_arch_enable_sta_mode()
func (d *Device) EnableStaMode(country uint32) error {
	return d.wifiSetup(whd.CYW43_ITF_STA, true, country)
}

// ref: cyw43_wifi_set_up(cyw43_t *self, int itf, bool up, uint32_t country)
func (d *Device) wifiSetup(itf uint8, up bool, country uint32) (err error) {
	d.info("wifiSetup", slog.Uint64("itf", uint64(itf)), slog.Bool("up", up), slog.Uint64("country", uint64(country)))
	d.lock()
	defer d.unlock()
	if !up {
		if itf == whd.CYW43_ITF_AP {
			err = d.wifiAPSetUp(false)
		}
		return err
	}

	if d.itfState == 0 {
		if err := d.wifiOn(country); err != nil {
			return err
		}
		if err := d.wifiPM(defaultPM); err != nil {
			return err
		}
	}

	if itf == whd.CYW43_ITF_AP {
		if err = d.wifiAPInit(); err != nil {
			return err
		}
		if err = d.wifiAPSetUp(true); err != nil {
			return err
		}
	}
	// itf == AP:cyw43_wifi_ap_init(self);  cyw43_wifi_ap_set_up(self, true);
	if d.itfState&(1<<itf) == 0 {
		// Reinitialize tcpip here.
		d.itfState |= 1 << itf
		d.info("wifiSetup:setitf", slog.Int("d.itf", int(d.itfState)), slog.Bool("staactive", d.isSTAActive()))
	}
	return nil
}

// reference: cyw43_ll_wifi_set_wpa_auth
func (d *Device) setWPAAuth() error {
	return d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_WPA_AUTH, whd.CYW43_WPA_AUTH_PSK)
}

func (d *Device) wifiAPInit() error {
	panic("not yet implemented")
}

// reference: cyw43_ll_wifi_ap_init
func (d *Device) wifiAPInitInternal(ssid, key string, auth, channel uint32) (err error) {
	d.debug("wifiAPInitInternal")
	buf := d.offbuf()

	// Get state of AP.
	// TODO: this can fail with sdpcm status = 0xffffffe2 (NOTASSOCIATED)
	// in such a case the AP is not up and we should not check the result
	copy(buf[:], "bss\x00")
	binary.LittleEndian.PutUint32(buf[4:], uint32(whd.WWD_AP_INTERFACE))
	err = d.doIoctl(whd.SDPCM_GET, whd.WWD_STA_INTERFACE, whd.WLC_GET_VAR, buf[:8])
	if err != nil {
		return err
	}
	res := binary.LittleEndian.Uint32(buf[:])
	if res != 0 {
		// AP is already up.
		return nil
	}

	// Set the AMPDU parameter for AP (window size = 2).
	err = d.WriteIOVar("ampdu_ba_wsize", whd.WWD_AP_INTERFACE, 2)
	if err != nil {
		return err
	}

	// Set SSID.
	binary.LittleEndian.PutUint32(buf, uint32(whd.WWD_AP_INTERFACE))
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(ssid)))
	for i := 0; i < 32; i++ {
		buf[8+i] = 0
	}
	copy(buf[8:], ssid)
	err = d.WriteIOVarN("bsscfg:ssid", whd.WWD_AP_INTERFACE, buf[:8+32])
	if err != nil {
		return err
	}
	// Set channel.
	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_CHANNEL, channel)
	if err != nil {
		return err
	}
	// Set Security type.
	err = d.WriteIOVar2("bsscfg:wsec", whd.WWD_STA_INTERFACE, uint32(whd.WWD_AP_INTERFACE), auth) // More confusing interface arguments.
	if err != nil {
		return err
	}
	if auth != whd.CYW43_AUTH_OPEN {
		// Set WPA/WPA2 auth parameters.
		var val uint16 = whd.CYW43_WPA_AUTH_PSK
		if auth != whd.CYW43_AUTH_WPA_TKIP_PSK {
			val |= whd.CYW43_WPA2_AUTH_PSK
		}
		err = d.WriteIOVar2("bsscfg:wpa_auth", whd.WWD_STA_INTERFACE, uint32(whd.WWD_AP_INTERFACE), uint32(val))
		if err != nil {
			return err
		}
		// Set password.
		binary.LittleEndian.PutUint16(buf, uint16(len(key)))
		binary.LittleEndian.PutUint16(buf[2:], 1)
		for i := 0; i < 64; i++ {
			buf[i] = 0
		}
		copy(buf[4:], key)
		time.Sleep(2 * time.Millisecond) // WICED has this.
		err = d.doIoctl(whd.SDPCM_SET, whd.WWD_AP_INTERFACE, whd.WLC_SET_WSEC_PMK, buf[:4+64])
		if err != nil {
			return err
		}
	}

	// Set GMode to auto (value of 1).
	err = d.SetIoctl32(whd.WWD_AP_INTERFACE, whd.WLC_SET_GMODE, 1)
	if err != nil {
		return err
	}
	// Set multicast tx rate to 11Mbps.
	const rate = 11000000 / 500000
	err = d.WriteIOVar("2g_mrate", whd.WWD_AP_INTERFACE, rate)
	if err != nil {
		return err
	}

	// Set DTIM period to 1.
	err = d.SetIoctl32(whd.WWD_AP_INTERFACE, whd.WLC_SET_DTIMPRD, 1)
	return err
}

// reference: cyw43_ll_wifi_ap_set_up
func (d *Device) wifiAPSetUp(up bool) error {
	// This line is somewhat confusing. Both the AP and STA interfaces are passed in as arguments,
	// but the STA interface is the one used to set the AP interface up or down.
	return d.WriteIOVar2("bss", whd.WWD_STA_INTERFACE, uint32(whd.WWD_AP_INTERFACE), b2u32(up))
}

// reference: cyw43_ll_wifi_ap_get_stas
func (d *Device) wifiAPGetSTAs(macs []byte) (stas uint32, err error) {
	buf := d.offbuf()
	copy(buf[:], "maxassoc\x00")
	binary.LittleEndian.PutUint32(buf[9:], uint32(whd.WWD_AP_INTERFACE))
	err = d.doIoctl(whd.SDPCM_GET, whd.WWD_STA_INTERFACE, whd.WLC_GET_VAR, buf[:9+4])
	if err != nil {
		return 0, err
	}
	maxAssoc := binary.LittleEndian.Uint32(buf[:])
	if macs == nil {
		// Return just the maximum number of STAs
		return maxAssoc, nil
	}
	// Return the maximum number of STAs and the MAC addresses of the STAs.
	lim := 4 + maxAssoc*6
	if lim > uint32(len(buf)) {
		lim = uint32(len(buf))
	}
	err = d.doIoctl(whd.SDPCM_GET, whd.WWD_AP_INTERFACE, whd.WLC_GET_ASSOCLIST, buf[:lim])
	if err != nil {
		return 0, err
	}
	stas = binary.LittleEndian.Uint32(buf[:])
	copy(macs[:], buf[4:4+stas*6])
	return stas, err
}

// reference: cyw43_ll_wifi_scan
func (d *Device) wifiScan(opts *whd.ScanOptions) error {
	opts.Version = 1 // ESCAN_REQ_VERSION
	opts.Action = 1  // WL_SCAN_ACTION_START
	for i := 0; i < len(opts.BSSID); i++ {
		opts.BSSID[i] = 0xff
	}
	opts.BSSType = 2 // WICED_BSS_TYPE_ANY
	opts.NProbes = -1
	opts.ActiveTime = -1
	opts.PassiveTime = -1
	opts.HomeTime = -1
	opts.ChannelNum = 0
	opts.ChannelList[0] = 0
	unsafePtr := unsafe.Pointer(opts)
	if uintptr(unsafePtr)&0x3 != 0 {
		return errors.New("opts not aligned to 4 bytes")
	}
	buf := (*[unsafe.Sizeof(*opts)]byte)(unsafePtr)
	err := d.WriteIOVarN("escan", whd.WWD_STA_INTERFACE, buf[:])
	return err
}

// reference: cyw43_wifi_pm
func (d *Device) wifiPM(pm_in uint32) (err error) {

	err = d.ensureUp()
	if err != nil {
		return err
	}
	// pm_in: 0x00adbrrm
	pm := pm_in & 0xf
	pm_sleep_ret := (pm_in >> 4) & 0xff
	li_bcn := (pm_in >> 12) & 0xf
	li_dtim := (pm_in >> 16) & 0xf
	li_assoc := (pm_in >> 20) & 0xf
	err = d.wifiPMinternal(pm, pm_sleep_ret, li_bcn, li_dtim, li_assoc)
	return err
}

// reference: cyw43_ll_wifi_pm
func (d *Device) wifiPMinternal(pm, pm_sleep_ret, li_bcn, li_dtim, li_assoc uint32) error {
	// set some power saving parameters
	// PM1 is very aggressive in power saving and reduces wifi throughput
	// PM2 only saves power when there is no wifi activity for some time
	// Value passed to pm2_sleep_ret measured in ms, must be multiple of 10, between 10 and 2000
	if pm_sleep_ret < 1 {
		pm_sleep_ret = 1
	} else if pm_sleep_ret > 200 {
		pm_sleep_ret = 200
	}
	err := d.WriteIOVar("pm2_sleep_ret", whd.WWD_STA_INTERFACE, pm_sleep_ret*10)
	if err != nil {
		return err
	}

	// these parameters set beacon intervals and are used to reduce power consumption
	// while associated to an AP but not doing tx/rx
	// bcn_li_xxx is what the CYW43x will do; assoc_listen is what is sent to the AP
	// bcn_li_dtim==0 means use bcn_li_bcn
	err = d.WriteIOVar("bcn_li_bcn", whd.WWD_STA_INTERFACE, li_bcn)
	if err != nil {
		return err
	}
	err = d.WriteIOVar("bcn_li_dtim", whd.WWD_STA_INTERFACE, li_dtim)
	if err != nil {
		return err
	}
	err = d.WriteIOVar("assoc_listen", whd.WWD_STA_INTERFACE, li_assoc)
	if err != nil {
		return err
	}
	err = d.SetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_SET_PM, pm)
	if err != nil {
		return err
	}

	// Set GMODE_AUTO
	buf := d.offbuf()
	binary.LittleEndian.PutUint32(buf[:4], 1)
	err = d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_GMODE, buf[:4])
	if err != nil {
		return err
	}
	binary.LittleEndian.PutUint32(buf[:4], 0) // any
	err = d.doIoctl(whd.SDPCM_SET, whd.WWD_STA_INTERFACE, whd.WLC_SET_BAND, buf[:4])
	return err
}

// reference: cyw43_ll_wifi_get_pm
func (d *Device) wifiGetPM() (pm, pm_sleep_ret, li_bcn, li_dtim, li_assoc uint32, err error) {
	// TODO: implement
	pm_sleep_ret, err = d.ReadIOVar("pm2_sleep_ret", whd.WWD_STA_INTERFACE)
	if err != nil {
		goto reterr
	}
	li_bcn, err = d.ReadIOVar("bcn_li_bcn", whd.WWD_STA_INTERFACE)
	if err != nil {
		goto reterr
	}
	li_dtim, err = d.ReadIOVar("bcn_li_dtim", whd.WWD_STA_INTERFACE)
	if err != nil {
		goto reterr
	}
	li_assoc, err = d.ReadIOVar("assoc_listen", whd.WWD_STA_INTERFACE)
	if err != nil {
		goto reterr
	}
	pm, err = d.GetIoctl32(whd.WWD_STA_INTERFACE, whd.WLC_GET_PM)
	if err != nil {
		goto reterr
	}
	return pm, pm_sleep_ret, li_bcn, li_dtim, li_assoc, nil
reterr:
	return 0, 0, 0, 0, 0, err
}
