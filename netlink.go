//go:build tinygo

// Netlink implmentation of cyw43439

package cyw43439

import (
	"net"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439/internal/netlink"
	"github.com/soypat/cyw43439/whd"
)

func (d *Device) DeviceInfo() (driver, driverVersion, fwVersion string, MAC net.HardwareAddr) {
	return driverName, version, d.fwVersion, append(net.HardwareAddr{}, d.mac[:]...)
}

func (d *Device) connectToAP() error {
	d.info("connectToAP", slog.String("SSID", d.params.SSID), slog.Int("passlen", len(d.params.Passphrase)))
	if len(d.params.SSID) == 0 {
		return netlink.ErrMissingSSID
	}

	timeout := d.params.ConnectTimeout
	if timeout == 0 {
		timeout = netlink.DefaultConnectTimeout
	}

	var auth uint32
	switch d.params.AuthType {
	case netlink.AuthTypeWPA2:
		auth = whd.CYW43_AUTH_WPA2_AES_PSK
	case netlink.AuthTypeOpen:
		auth = whd.CYW43_AUTH_OPEN
	case netlink.AuthTypeWPA:
		auth = whd.CYW43_AUTH_WPA_TKIP_PSK
	case netlink.AuthTypeWPA2Mixed:
		auth = whd.CYW43_AUTH_WPA2_MIXED_PSK
	default:
		panic("ConnectToAP: Unknown AuthType")
	}

	err := d.WifiConnectTimeout(d.params.SSID, d.params.Passphrase, auth, timeout)
	if err != nil {
		d.logError("connectToAP:WifiConnectTimeout", slog.Any("err", err))
		return err
	}
	d.info("connected to AP", slog.String("SSID", d.params.SSID))

	d.notifyUp()

	return nil
}

func (d *Device) networkDown() bool {
	return false
}

func (d *Device) showIP() {
}

func (d *Device) netConnect(reset bool) error {
	if reset {
		/*
			country := d.params.Country
			if country == "" {
				country = "XX"
			}
			code := whd.CountryCode(country, 0)
			if err := d.EnableStaMode(code); err != nil {
				return err
			}
		*/
	}

	for i := 0; d.params.Retries == 0 || i < d.params.Retries; i++ {
		if err := d.connectToAP(); err != nil {
			if err == netlink.ErrConnectFailed {
				continue
			}
			return err
		}
		break
	}

	if d.networkDown() {
		return netlink.ErrConnectFailed
	}

	d.showIP()
	return nil
}

func (d *Device) watchdog() {
	ticker := time.NewTicker(d.params.WatchdogTimeout)
	for {
		select {
		case <-d.killWatchdog:
			return
		case <-ticker.C:
			if d.networkDown() {
				d.logError("Watchdog: Wifi NOT CONNECTED, trying again...")
				d.notifyDown()
				d.netConnect(false)
			}
		}
	}
}

func (d *Device) NetConnect(params *netlink.ConnectParams) error {
	d.info("NetConnect")
	if d.netConnected {
		return netlink.ErrConnected
	}

	switch params.ConnectMode {
	case netlink.ConnectModeSTA:
	default:
		return netlink.ErrConnectModeNoGood
	}

	d.params = params

	if err := d.netConnect(true); err != nil {
		return err
	}

	d.netConnected = true

	if d.params.WatchdogTimeout != 0 {
		go d.watchdog()
	}

	return nil
}

func (d *Device) netDisconnect() {
	// d.pollStop()
}

func (d *Device) NetDisconnect() {
	d.info("NetDisconnect")
	if !d.netConnected {
		return
	}

	if d.params.WatchdogTimeout != 0 {
		d.killWatchdog <- true
	}

	d.netDisconnect()
	d.netConnected = false
	d.notifyDown()
}

func (d *Device) notifyDown() {
	if d.notifyCb != nil {
		d.notifyCb(netlink.EventNetDown)
	}
}

func (d *Device) notifyUp() {
	if d.notifyCb != nil {
		d.notifyCb(netlink.EventNetUp)
	}
}

func (d *Device) NetNotify(cb func(netlink.Event)) {
	d.notifyCb = cb
}

func (d *Device) GetHardwareAddr() (net.HardwareAddr, error) {
	return append(net.HardwareAddr{}, d.mac[:]...), nil
}

func (d *Device) SendEth(pkt []byte) error {
	return d.sendEthernet(whd.CYW43_ITF_STA, pkt)
}

func (d *Device) RecvEthHandle(handler func(pkt []byte) error) {
	d.recvEth = handler
}
