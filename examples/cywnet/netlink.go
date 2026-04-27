package cywnet

import (
	"github.com/soypat/cyw43439"
	"github.com/soypat/lneto/x/netdev"
)

const ethOffset = 0

var _ netdev.Netlink[ConnectParams] = (*netlink)(nil)
var _ netdev.DevEthernet = (*netlink)(nil)

type netlink struct {
	dev    *cyw43439.Device
	cb     netdev.NotifyCallback[ConnectParams]
	hndler func(rxEthframe []byte)
}

type ConnectParams struct {
	SSID    string
	Options cyw43439.JoinOptions
}

// LinkConnect attempts to connect the Netlink if it was not already connected.
// It will block until it succeeds/fails and not retry after returning.
func (nl *netlink) LinkConnect(connectParams ConnectParams) error {
	return nl.dev.Join(connectParams.SSID, connectParams.Options)
}

// LinkDisconnect disconnects the Netlink immediately.
func (nl *netlink) LinkDisconnect() {
	// Not implemented.
}

// Link notify sets the callback to be executed after connection state
// changes for the Netlink. The callback can signal an immediate reconnect is desired
// by setting reconnectNowRetries to a positive integer. The netlink should then retry connection
// immediately with the given reconnectParams. reconnectParams should not be nil if reconnectNowRetries is positive.
func (nl *netlink) LinkNotify(cb netdev.NotifyCallback[ConnectParams]) {
	nl.cb = cb
}

// HardwareAddr6 returns the device's 6-byte MAC address.
// For PHY-only devices, returns the MAC provided at configuration.
func (nl *netlink) HardwareAddr6() ([6]byte, error) {
	return nl.dev.HardwareAddr6()
}

// SendEthFrameOffset transmits a complete Ethernet frame at offset given by [DevEthernet.MaxFrameSizeAndOffset].
// The frame includes the Ethernet header but NOT the FCS/CRC
// trailer (device or stack handles CRC as appropriate).
// SendEthFrameOffset blocks until the transmission is queued succesfully
// or finished sending. Should not be called concurrently
// unless user is sure the driver supports it.
func (nl *netlink) SendOffsetEthFrame(offsetTxEthFrame []byte) error {
	return nl.dev.SendEth(offsetTxEthFrame[ethOffset:])
}

// SetRecvHandler registers the function called when an Ethernet
// frame is received. Buffers needed by the device to operate efficiently
// should be allocated on its side.
func (nl *netlink) SetEthRecvHandler(handler func(rxEthframe []byte)) {
	// TODO: refactor CYW internals to use same interface.
	nl.hndler = handler
	if handler == nil {
		nl.dev.RecvEthHandle(nil)
	} else {
		nl.dev.RecvEthHandle(nl.rxhandlecyw)
	}
}

func (nl *netlink) rxhandlecyw(rxEthFrame []byte) error {
	if nl.hndler != nil {
		nl.hndler(rxEthFrame)
	}
	return nil
}

// EthPoll services the device. For poll-based devices (e.g. CYW43439
// over SPI), reads from the bus and invokes the handler for each
// received frame. Behaviour for interrupt driven devices is undefined
// at the moment.
func (nl *netlink) EthPoll(buf []byte) (ethFrameOff, ethernetBytes int, err error) {
	_, err = nl.dev.PollOne()
	return 0, 0, err
}

// MaxFrameSizeAndOffset returns the max complete device frame size
// (including headers and any overhead) for buffer allocation.
// The second value returned is the offset at which the ethernet frame
// should be stored when being passed to [DevEthernet.SendOffsetEthFrame].
// Buffers allocated should be maxEthernetFrameSize+frameOff where maxEthernetFrameSize
// is usually 1500 but less or equal to maxFrameSize-frameOff.
// MTU can be calculated doing:
//
//	// mfu-(14+4+4) for:
//	// ethernet header+ethernet CRC if present+ethernet VLAN overhead for VLAN support.
//	mtu := dev.MaxFrameSizeAndOffset() - ethernet.MaxOverheadSize
func (nl *netlink) MaxFrameSizeAndOffset() (maxFrameSize int, frameOff int) {
	return cyw43439.MaxFrameSize, ethOffset
}

func (nl *netlink) ConfigurePico(cfg cyw43439.Config) error {
	nl.dev = cyw43439.NewPicoWDevice()
	err := nl.dev.Init(cfg)
	if err != nil {
		return err
	}
	return nil
}
