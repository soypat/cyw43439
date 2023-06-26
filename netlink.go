//go:build tinygo

// Netlink implmentation of cyw43439

package cyw43439

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/soypat/cyw43439/internal/netlink"
	"github.com/soypat/cyw43439/whd"
)

func (d *Device) showDriver() {
	if d.driverShown {
		return
	}
	if debugging(debugBasic) {
		fmt.Printf("\r\n")
		fmt.Printf("%s\r\n\r\n", driverName)
		fmt.Printf("Driver version           : %s\r\n", version)
	}
	d.driverShown = true
}

func (d *Device) showDevice() {
	if d.deviceShown {
		return
	}
	if debugging(debugBasic) {
		fwVersion := strings.Fields(d.fwVersion)[1]
		fmt.Printf("Firmware version         : %s\r\n", fwVersion)
		fmt.Printf("MAC address              : %s\r\n", d.mac)
		fmt.Printf("\r\n")
	}
	d.deviceShown = true
}

func (d *Device) connectToAP() error {

	if len(d.params.Ssid) == 0 {
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

	if debugging(debugBasic) {
		fmt.Printf("Connecting to Wifi SSID '%s'...", d.params.Ssid)
	}

	err := d.WifiConnectTimeout(d.params.Ssid, d.params.Passphrase, auth, timeout)
	if err != nil {
		if debugging(debugBasic) {
			fmt.Printf("FAILED (%s)\r\n", err.Error())
		}
		return err
	}

	if debugging(debugBasic) {
		fmt.Printf("CONNECTED\r\n")
	}

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
		country := d.params.Country
		if country == "" {
			country = "XX"
		}
		code := whd.CountryCode(country, 0)
		if err := d.EnableStaMode(code); err != nil {
			return err
		}
	}
	d.showDevice()

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
				if debugging(debugBasic) {
					fmt.Printf("Watchdog: Wifi NOT CONNECTED, trying again...\r\n")
				}
				d.notifyDown()
				d.netConnect(false)
			}
		}
	}
}

func (d *Device) NetConnect(params *netlink.ConnectParams) error {

	if d.netConnected {
		return netlink.ErrConnected
	}

	switch params.ConnectMode {
	case netlink.ConnectModeSTA:
	default:
		return netlink.ErrConnectModeNoGood
	}

	d.params = params

	d.showDriver()

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
	d.pollStop()
}

func (d *Device) NetDisconnect() {

	if !d.netConnected {
		return
	}

	if d.params.WatchdogTimeout != 0 {
		d.killWatchdog <- true
	}

	d.netDisconnect()
	d.netConnected = false

	if debugging(debugBasic) {
		fmt.Printf("\r\nDisconnected from Wifi\r\n\r\n")
	}

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
	return d.sendEthernet(pkt)
}

func (d *Device) RecvEthHandle(handler func(pkt []byte) error) {
	d.recvEth = handler
}
