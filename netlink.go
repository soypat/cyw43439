package cyw43439

import (
	"net"

	"tinygo.org/x/drivers/netlink"
)

func (d *Device) GetHardwareAddr() (net.HardwareAddr, error) {
	return net.HardwareAddr(d.mac[:6]), nil
}

func (d *Device) NetNotify(cb func(netlink.Event)) {
	d.notifyCb = cb
}

func (d *Device) NetConnect(params *netlink.ConnectParams) error {

	if d.netConnected {
		return netlink.ErrConnected
	}

	cfg := DefaultWifiConfig()
	if err := d.Init(cfg); err != nil {
		return err
	}

	for i := 0; params.Retries == 0 || i < params.Retries; i++ {
		err := d.JoinWPA2(params.Ssid, params.Passphrase)
		if err == nil {
			d.netConnected = true
			break
		}
		println("wifi join failed:", err.Error())
	}

	if !d.netConnected {
		return netlink.ErrConnectFailed
	}

	mac, _ := d.GetHardwareAddr()
	println("\n\n\nMAC:", mac.String())

	if d.notifyCb != nil {
		d.notifyCb(netlink.EventNetUp)
	}

	/*
	if params.WatchdogTimeout != 0 {
		go d.watchdog()
	}
	*/

	return nil
}

func (d *Device) NetDisconnect() {

	if !d.netConnected {
		return
	}

	/*
	if d.params.WatchdogTimeout != 0 {
		d.killWatchdog <- true
	}
	*/

	// TODO disconnect

	d.netConnected = false

	if d.notifyCb != nil {
		d.notifyCb(netlink.EventNetDown)
	}
}

// RecvEthHandle sets handler for receiving Ethernet pkt
// If set to nil then incoming packets are ignored.
func (d *Device) RecvEthHandle(handler func(pkt []byte) error) {
	d.lock()
	defer d.unlock()
	d.rcvEth = handler
}

// SendEth sends an Ethernet packet over the current interface.
func (d *Device) SendEth(pkt []byte) error {
	d.lock()
	defer d.unlock()
	return d.tx(pkt)
}

// PollOne tries to receive one Ethernet packet and returns true if one was
func (d *Device) PollOne() (gotPacket bool, err error) {
	return d.TryPoll()
}
