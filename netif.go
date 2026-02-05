package cyw43439

import (
	"errors"
	"log/slog"
	"net"

	"github.com/soypat/cyw43439/whd"
)

// MTU (maximum transmission unit) returns the maximum ethernet payload size
// (excluding the 14-byte ethernet header). This is the value expected by
// network stacks like lneto. For full frame capacity, use [MaxFrameSize].
func (d *Device) MTU() int { return MTU }

// HardwareAddr6 returns the device's 6-byte [MAC address].
//
// [MAC address]: https://en.wikipedia.org/wiki/MAC_address
func (d *Device) HardwareAddr6() ([6]byte, error) {
	err := d.acquire(modeWifi)
	defer d.release()
	if err != nil {
		return [6]byte{}, err
	}
	if d.mac == [6]byte{} {
		return [6]byte{}, errors.New("hardware address not acquired")
	}
	return d.mac, nil
}

// PollOne attempts to read a packet from the device. Returns true if a packet
// was read, false if no packet was available.
func (d *Device) PollOne() (bool, error) {
	err := d.acquire(modeWifi)
	defer d.release()
	if err != nil {
		return false, err
	}
	_, cmd, err := d.tryPoll(d._rxBuf[:])
	if err == errNoF2Avail {
		return false, nil
	}
	return cmd == whd.CONTROL_HEADER && err == nil, err
}

// RecvEthHandle sets handler for receiving Ethernet pkt
// If set to nil then incoming packets are ignored.
func (d *Device) RecvEthHandle(handler func(pkt []byte) error) {
	err := d.acquire(modeWifi)
	defer d.release()
	if err != nil {
		d.logerr("cyw:recveth", slog.String("err", err.Error()))
	}
	d.rcvEth = handler
}

// SendEth sends an Ethernet packet over the current interface.
func (d *Device) SendEth(pkt []byte) error {
	err := d.acquire(modeWifi)
	defer d.release()
	if err != nil {
		return err
	}
	return d.tx(pkt)
}

// NetFlags returns the current network flags for the device.
func (d *Device) NetFlags() (flags net.Flags) {
	err := d.acquire(modeWifi)
	defer d.release()
	if err != nil {
		return 0
	}
	// Define net.Flags locally since not all Tinygo versions have them fully defined.
	const (
		FlagUp           net.Flags = 1 << iota // interface is administratively up
		FlagBroadcast                          // interface supports broadcast access capability
		FlagLoopback                           // interface is a loopback interface
		FlagPointToPoint                       // interface belongs to a point-to-point link
		FlagMulticast                          // interface supports multicast access capability
		FlagRunning                            // interface is in running state
	)
	if d.state == linkStateDown {
		return 0
	}
	flags |= FlagUp // TODO: does this device support broadcast/multicast?
	if d.state == linkStateUp {
		flags |= FlagRunning
	}
	return flags
}
