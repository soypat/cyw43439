package cyw43439

// This file borrows heavily from control.rs from the reference:
// https://github.com/embassy-rs/embassy/blob/26870082427b64d3ca42691c55a2cded5eadc548/cyw43/src/control.rs

import (
	"encoding/binary"
	"errors"
	"net"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439/whd"
)

var (
	errJoinSetSSID = errors.New("join:SET_SSID failed")
	errJoinGeneric = errors.New("join:failed")
)

// JoinAuth specifies the authentication method for joining a WiFi network.
type JoinAuth uint8

const (
	joinAuthUndefined JoinAuth = iota
	JoinAuthOpen
	JoinAuthWPA
	JoinAuthWPA2
	JoinAuthWPA3
	JoinAuthWPA2WPA3
)

// JoinOptions configures WiFi connection parameters.
// See embassy-rs for reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/control.rs
type JoinOptions struct {
	// Auth specifies the authentication method. Implementation will choose WPA2 if passphrase is set or Open if no passphrase is set.
	Auth JoinAuth
	// CipherNoAES disables AES cipher.
	CipherNoAES bool
	// CipherTKIP enables TKIP cipher. Default is false.
	CipherTKIP bool
	// Passphrase is the WiFi password.
	Passphrase string
}

func (d *Device) clmLoad(clm string) error {
	// reference: https://github.com/embassy-rs/embassy/blob/26870082427b64d3ca42691c55a2cded5eadc548/cyw43/src/control.rs#L35
	d.debug("initControl", slog.Int("clm_len", len(clm)))
	const chunkSize = 1024
	remaining := clm
	offset := 0

	buf8 := u32AsU8(d._iovarBuf[:])[:chunkSize+20]

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
		return errjoin(errors.New("clmload_status failed"), err)
	}
	return nil
}

func (d *Device) initControl(clm string) error {
	if d.bt_mode_enabled() {
		err := d.bt_init(btFW)
		if err != nil {
			return errors.New("cyw bt init failed: " + err.Error())
		}
	}

	err := d.clmLoad(clm)
	if err != nil {
		return err
	}
	// Disable tx gloming which transfers multiple packets in one request.
	// 'glom' is short for "conglomerate" which means "gather together into
	// a compact mass".
	d.set_iovar("bus:txglom", whd.IF_STA, 0)
	d.set_iovar("apsta", whd.IF_STA, 1)

	// read MAC Address:

	d.get_iovar_n("cur_etheraddr", whd.IF_STA, d.mac[:6])
	d.debug("MAC", slog.String("mac", d.hwaddr().String()))
	if d.mode&modeWifi != 0 {
		countryInfo := whd.CountryInfo("XX", 0)
		d.set_iovar_n("country", whd.IF_STA, countryInfo[:])

		// set country takes some time, next ioctls fail if we don't wait.
		time.Sleep(100 * time.Millisecond)

		// Set Antenna to chip antenna.
		d.set_ioctl(whd.WLC_SET_ANTDIV, whd.IF_STA, 0)

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
	}
	if modeBluetooth&d.mode != 0 {
		// TODO: flash bt firmware here?
	}
	return nil
}

func (d *Device) hwaddr() net.HardwareAddr {
	return net.HardwareAddr(d.mac[:6])
}

func (d *Device) set_power_management(mode powerManagementMode) error {
	d.debug("set_power_management", slog.String("mode", mode.String()))
	if !mode.IsValid() {
		return errors.New("invalid power management mode")
	}
	mode_num := mode.mode()
	if mode_num == 2 {
		d.set_iovar("pm2_sleep_ret", whd.IF_STA, uint32(mode.sleep_ret_ms()))
		d.set_iovar("bcn_li_bcn", whd.IF_STA, uint32(mode.beacon_period()))
		d.set_iovar("bcn_li_dtim", whd.IF_STA, uint32(mode.dtim_period()))
		d.set_iovar("assoc_listen", whd.IF_STA, uint32(mode.assoc()))
	}
	return d.set_ioctl(whd.WLC_SET_PM, whd.IF_STA, uint32(mode_num))
}

// join_open connects to an open (unencrypted) WiFi network.
// Reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/control.rs#L316-L321
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
	d.set_ioctl(whd.WLC_SET_WPA_AUTH, whd.IF_STA, whd.WPA_AUTH_DISABLED)

	return d.wait_for_join(ssid, false) // open network
}

// wait_for_join waits for the join operation to complete.
// For open networks (secureNetwork=false), success is indicated by SET_SSID with status=0.
// For secure networks (secureNetwork=true), success is indicated by PSK_SUP with status=6 (KEYED).
// Reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/control.rs#L389-L440
func (d *Device) wait_for_join(ssid string, secureNetwork bool) (err error) {
	d.secureNetwork = secureNetwork
	// Reset flags for new join attempt. ref: runner.rs:120-122
	d.authOK = false
	d.joinOK = false
	d.keyExchangeOK = false

	d.eventmask.Enable(whd.EvSET_SSID)
	d.eventmask.Enable(whd.EvAUTH)
	d.eventmask.Enable(whd.EvJOIN)
	if secureNetwork {
		d.eventmask.Enable(whd.EvPSK_SUP)
	}

	err = d.setSSID(ssid)
	if err != nil {
		return err
	}
	// Poll for async events.
	deadline := time.Now().Add(10 * time.Second)
	keepGoing := true
	for keepGoing {
		time.Sleep(270 * time.Millisecond)
		err = d.check_status(d._sendIoctlBuf[:])
		if err != nil {
			return err
		}
		// Keep trying while state is still Down (waiting for events).
		keepGoing = d.state == linkStateDown && time.Until(deadline) > 0
	}
	switch d.state {
	case linkStateUp:
		// Begin listening in for link change/down events.
		d.eventmask.Enable(whd.EvLINK)
		d.eventmask.Enable(whd.EvDISASSOC)
		d.eventmask.Enable(whd.EvDEAUTH)

	case linkStateFailed:
		err = errJoinSetSSID
	default:
		if d.state == linkStateDown {
			err = errJoinGeneric // Timed out without resolving.
		} else {
			err = errJoinGeneric
		}
	}
	return err
}

type passphraseInfo struct {
	length     uint16
	flags      uint16
	passphrase [64]byte
}

func (p *passphraseInfo) Put(order binary.ByteOrder, b []byte) {
	order.PutUint16(b[0:2], p.length)
	order.PutUint16(b[2:4], p.flags)
	copy(b[4:68], p.passphrase[:])
}

func (d *Device) setPassphrase(pass string) error {
	if len(pass) > 64 {
		return errors.New("passphrase too long")
	}

	var pfi = passphraseInfo{
		length: uint16(len(pass)),
		flags:  1,
	}
	copy(pfi.passphrase[:], pass)

	var buf [68]byte
	pfi.Put(_busOrder, buf[:])

	return d.doIoctlSet(whd.WLC_SET_WSEC_PMK, whd.IF_STA, buf[:])
}

// setSaePassword sets the SAE (WPA3) password via "sae_password" iovar.
// Reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/control.rs#L362-L370
func (d *Device) setSaePassword(pass string) error {
	if len(pass) > 128 {
		return errors.New("sae password too long")
	}
	// SaePassphraseInfo: len (u16) + passphrase ([u8; 128])
	var buf [2 + 128]byte
	_busOrder.PutUint16(buf[0:2], uint16(len(pass)))
	copy(buf[2:], pass)
	// Send full 130-byte struct to match embassy-rs. ref: control.rs:362-369, structs.rs:400-404
	return d.set_iovar_n("sae_password", whd.IF_STA, buf[:])
}

type ssidInfo struct {
	length uint32
	ssid   [32]byte
}

func (s *ssidInfo) put(order binary.ByteOrder, b []byte) {
	order.PutUint32(b[0:4], s.length)
	copy(b[4:36], s.ssid[:])
}

// setSSID sets the SSID through Ioctl interface. This command
// also starts the wifi connect procedure.
func (d *Device) setSSID(ssid string) error {
	if len(ssid) > 32 {
		return errors.New("ssid too long")
	}

	var info = ssidInfo{
		length: uint32(len(ssid)),
	}
	copy(info.ssid[:], ssid)

	var buf [36]byte
	info.put(_busOrder, buf[:])
	d.state = linkStateDown
	return d.doIoctlSet(whd.WLC_SET_SSID, whd.IF_STA, buf[:])
}

type ssidInfoWithIndex struct {
	index uint32
	info  ssidInfo
}

func (s *ssidInfoWithIndex) Put(order binary.ByteOrder, b []byte) {
	order.PutUint32(b[0:4], s.index)
	s.info.put(order, b[4:40])
}

func (d *Device) setSSIDWithIndex(ssid string, index uint32) error {
	if len(ssid) > 32 {
		return errors.New("ssid too long")
	}

	var infoIndex = ssidInfoWithIndex{
		info: ssidInfo{
			length: uint32(len(ssid)),
		},
	}
	copy(infoIndex.info.ssid[:], ssid)

	var buf [40]byte
	infoIndex.Put(_busOrder, buf[:])

	return d.set_iovar_n("bsscfg:ssid", whd.IF_STA, buf[:])
}

// IsLinkUp returns true if the wifi connection is up.
func (d *Device) IsLinkUp() bool {
	return d.state == linkStateUp
}

// Join connects to a WiFi network using the specified options.
// For WPA2/WPA3 networks, provide a passphrase in options.
// For open networks, use JoinAuth=JoinAuthOpen with empty passphrase.
//
// Reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/control.rs see `pub async fn join`
func (d *Device) Join(ssid string, options JoinOptions) error {
	err := d.acquire(modeWifi)
	defer d.release()
	if err != nil {
		return err
	}
	if options.Auth == joinAuthUndefined || options.Auth > JoinAuthWPA2WPA3 {
		options.Auth = JoinAuthOpen
		if options.Passphrase != "" {
			options.Auth = JoinAuthWPA2
		}
	}
	if options.Auth == JoinAuthOpen {
		return d.join_open(ssid)
	}
	d.info("join", slog.String("ssid", ssid), slog.Int("auth", int(options.Auth)), slog.Int("passlen", len(options.Passphrase)))
	if err := d.set_iovar("ampdu_ba_wsize", whd.IF_STA, 8); err != nil {
		return err
	}

	// Set WSEC (wireless security) based on cipher options.
	var wsec uint32
	if !options.CipherNoAES {
		wsec |= whd.WSEC_AES
	}
	if options.CipherTKIP {
		wsec |= whd.WSEC_TKIP
	}
	if err := d.set_ioctl(whd.WLC_SET_WSEC, whd.IF_STA, wsec); err != nil {
		return err
	}
	if err := d.set_iovar2("bsscfg:sup_wpa", whd.IF_STA, 0, 1); err != nil {
		return err
	}
	if err := d.set_iovar2("bsscfg:sup_wpa2_eapver", whd.IF_STA, 0, 0xffff_ffff); err != nil {
		return err
	}
	if err := d.set_iovar2("bsscfg:sup_wpa_tmo", whd.IF_STA, 0, 2500); err != nil {
		return err
	}

	time.Sleep(100 * time.Millisecond)

	// Set passphrase for WPA/WPA2.
	// Reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/control.rs#L346-L360
	if options.Auth == JoinAuthWPA || options.Auth == JoinAuthWPA2 || options.Auth == JoinAuthWPA2WPA3 {
		time.Sleep(3 * time.Millisecond) // Embassy: Timer::after_millis(3)
		if err := d.setPassphrase(options.Passphrase); err != nil {
			return err
		}
	}

	// Set SAE password for WPA3 modes.
	// Reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/control.rs#L362-L370
	if options.Auth == JoinAuthWPA3 || options.Auth == JoinAuthWPA2WPA3 {
		time.Sleep(3 * time.Millisecond) // Embassy: Timer::after_millis(3)
		if err := d.setSaePassword(options.Passphrase); err != nil {
			return err
		}
	}

	// set_infra = 1
	if err := d.set_ioctl(whd.WLC_SET_INFRA, whd.IF_STA, 1); err != nil {
		return err
	}

	// Set auth type.
	// Reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/control.rs#L338-L344
	var auth uint32 = whd.AUTH_OPEN
	if options.Auth == JoinAuthWPA3 || options.Auth == JoinAuthWPA2WPA3 {
		auth = whd.AUTH_SAE
	}
	if err := d.set_ioctl(whd.WLC_SET_AUTH, whd.IF_STA, auth); err != nil {
		return err
	}

	// Set MFP based on auth type.
	var mfp uint32 = whd.MFP_NONE
	switch options.Auth {
	case JoinAuthWPA2, JoinAuthWPA2WPA3:
		mfp = whd.MFP_CAPABLE
	case JoinAuthWPA3:
		mfp = whd.MFP_REQUIRED
	}
	if err := d.set_iovar("mfp", whd.IF_STA, mfp); err != nil {
		return err
	}

	// Set WPA auth mode.
	// Reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/control.rs#L338-L344
	var wpaAuth uint32
	switch options.Auth {
	case JoinAuthWPA:
		wpaAuth = whd.WPA_AUTH_WPA_PSK
	case JoinAuthWPA2:
		wpaAuth = whd.WPA_AUTH_WPA2_PSK
	case JoinAuthWPA3, JoinAuthWPA2WPA3:
		wpaAuth = whd.WPA_AUTH_WPA3_SAE_PSK
	}
	if err := d.set_ioctl(whd.WLC_SET_WPA_AUTH, whd.IF_STA, wpaAuth); err != nil {
		return err
	}

	return d.wait_for_join(ssid, true) // secure network
}

// JoinWPA2 connects to a WPA2 WiFi network. If pass is empty, connects to an open network.
//
// Deprecated: Use [Device.Join] instead.
func (d *Device) JoinWPA2(ssid, pass string) error {
	return d.Join(ssid, JoinOptions{Passphrase: pass})
}

func (d *Device) StartAP(ssid, pass string, channel uint8) error {
	err := d.acquire(modeWifi)
	defer d.release()
	if err != nil {
		return err
	}

	security := whd.CYW43_AUTH_OPEN
	if pass != "" {
		if len(pass) < whd.CYW43_MIN_PSK_LEN || len(pass) > whd.CYW43_MAX_PSK_LEN {
			return errors.New("Passphrase is too short or too long")
		}
		security = whd.CYW43_AUTH_WPA2_AES_PSK
	}

	// Temporarily set wifi down
	if err := d.doIoctlSet(whd.WLC_DOWN, whd.IF_STA, nil); err != nil {
		return err
	}

	// Turn off APSTA mode
	if err := d.set_iovar("apsta", whd.IF_STA, 0); err != nil {
		return err
	}

	// Set wifi up again
	if err := d.doIoctlSet(whd.WLC_UP, whd.IF_STA, nil); err != nil {
		return err
	}

	// Turn on AP mode
	if err := d.set_ioctl(whd.WLC_SET_AP, whd.IF_STA, 1); err != nil {
		return err
	}

	// Set SSID
	if err := d.setSSIDWithIndex(ssid, 0); err != nil {
		return err
	}

	// Set channel number
	if err := d.set_ioctl(whd.WLC_SET_CHANNEL, whd.IF_STA, uint32(channel)); err != nil {
		return err
	}

	// Set security
	if err := d.set_iovar2("bsscfg:wsec", whd.IF_STA, 0, uint32(security)&0xff); err != nil {
		return err
	}

	if security != whd.CYW43_AUTH_OPEN {
		// wpa_auth = WPA2_AUTH_PSK | WPA_AUTH_PSK
		if err := d.set_iovar2("bsscfg:wpa_auth", whd.IF_STA, 0,
			whd.CYW43_WPA_AUTH_PSK|whd.CYW43_WPA2_AUTH_PSK); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
		// Set passphrase
		if err := d.setPassphrase(pass); err != nil {
			return err
		}
	}

	// Change mutlicast rate from 1 Mbps to 11 Mbps
	if err := d.set_iovar("2g_mrate", whd.IF_STA, 11000000/500000); err != nil {
		return err
	}

	// Start AP (bss = BSS_UP)
	if err := d.set_iovar2("bss", whd.IF_STA, 0, 1); err != nil {
		return err
	}

	return nil
}
