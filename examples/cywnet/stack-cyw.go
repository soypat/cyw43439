//go:build rp2040 || rp2350

package cywnet

import (
	"context"
	"errors"
	"log/slog"
	"machine"
	"time"

	"github.com/soypat/cyw43439"
)

func (stack *StackAsync) SetupPicoWifiWithDHCPv4(ssid, password string, requestIPv4 [4]byte, cfg cyw43439.Config) (*cyw43439.Device, error) {
	dev, err := stack.SetupPicoWifi(ssid, password, cfg)
	if err != nil {
		return dev, err
	}
	stackBlocking := stack.StackRetrying()
	_, err = stackBlocking.DoDHCPv4(requestIPv4, 6*time.Second, 2) // Do 2 DHCP retries
	if err != nil {
		return dev, err
	}
	err = stack.AssimilateDHCPResults()
	if err != nil {
		return dev, err
	}
	// Do 3 ARP requests before giving up.
	routerHW, err := stackBlocking.DoResolveHardwareAddress6(stack.dhcpResults.Router, 3*time.Second, 10)
	if err != nil {
		return dev, err
	}
	stack.link.SetGateway6(routerHW)
	return dev, nil
}

func (stack *StackAsync) SetupPicoWifi(ssid, password string, cfg cyw43439.Config) (*cyw43439.Device, error) {
	if stack.hostname == "" {
		return nil, errors.New("call stack.Reset with a Hostname before setting up pico")
	}
	start := time.Now()
	dev := cyw43439.NewPicoWDevice()
	println("starting program")
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	dev.SetLogger(logger)
	err := dev.Init(cfg)
	if err != nil {
		return nil, err
	}
	err = dev.JoinWPA2(ssid, password)
	if err != nil {
		return nil, err
	}
	mac, err := dev.HardwareAddr6()
	if err != nil {
		return nil, err
	}
	dev.RecvEthHandle(func(pkt []byte) error {
		return stack.link.Demux(pkt, 0)
	})
	err = stack.SetHardwareAddress(mac)
	if err != nil {
		return nil, err
	}
	elapsed := time.Since(start)
	stack.prng ^= uint32(elapsed) ^ uint32(elapsed>>32)
	return dev, nil
}

func (stack *StackAsync) RecvAndSend(dev *cyw43439.Device, sendBufOrNil []byte) (send, recv int, err error) {
	gotRecv, errrecv := dev.PollOne()
	if gotRecv {
		recv = int(stack.lastrecv)
	}
	if errrecv != nil {
		stack.logerr("RecvAndSend:PollOne", slog.Int("plen", recv), slog.String("err", errrecv.Error()))
	}
	if sendBufOrNil == nil {
		if stack.sendbuf == nil {
			stack.sendbuf = make([]byte, stack.link.MTU())
		}
		sendBufOrNil = stack.sendbuf
	}
	send, err = stack.Encapsulate(sendBufOrNil, 0)
	if err != nil {
		stack.logerr("RecvAndSend:Encapsulate", slog.Int("plen", send), slog.String("err", err.Error()))
	} else {
		err = errrecv // err will be non-nil serror result if present.
	}
	if send == 0 {
		return send, recv, err
	}
	err = dev.SendEth(sendBufOrNil[:send])
	if err != nil {
		stack.logerr("RecvAndSend:SendEth", slog.Int("plen", send), slog.String("err", err.Error()))
	}
	return send, recv, err
}

func (stack *StackAsync) logerr(msg string, attrs ...slog.Attr) {
	if stack.logger != nil {
		stack.logger.LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
	}
}
