//go:build rp2040 || rp2350

package cywnet

import (
	"context"
	"errors"
	"log/slog"
	"machine"
	"net/netip"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/lneto/x/xnet"
)

type Stack struct {
	s        xnet.StackAsync
	dev      *cyw43439.Device
	log      *slog.Logger
	sendbuf  []byte
	lastrecv uint16
}

type StackConfig struct {
	StaticAddress netip.Addr
	DNSServer     netip.Addr
	NTPServer     netip.Addr
	Hostname      string
	MaxTCPPorts   int
	RandSeed      int64
}

func NewConfiguredPicoWithStack(ssid, password string, cfgDev cyw43439.Config, cfg StackConfig) (*Stack, error) {
	if cfg.Hostname == "" {
		return nil, errors.New("empty hostname")
	}
	start := time.Now()
	dev := cyw43439.NewPicoWDevice()
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	dev.SetLogger(logger)
	err := dev.Init(cfgDev)
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

	// Configure Stack.
	stack := new(Stack)
	stack.dev = dev
	elapsed := time.Since(start)
	err = stack.s.Reset(xnet.StackConfig{
		StaticAddress:   cfg.StaticAddress,
		DNSServer:       cfg.DNSServer,
		NTPServer:       cfg.NTPServer,
		Hostname:        cfg.Hostname,
		MaxTCPConns:     cfg.MaxTCPPorts,
		RandSeed:        elapsed.Nanoseconds() ^ int64(cfg.RandSeed),
		HardwareAddress: mac,
		MTU:             cyw43439.MTU,
	})
	dev.RecvEthHandle(func(pkt []byte) error {
		// return stack.s
		return stack.s.Demux(pkt, 0)
	})
	stack.sendbuf = make([]byte, cyw43439.MTU)
	return stack, err
}

func (stack *Stack) Hostname() string {
	return stack.s.Hostname()
}

func (stack *Stack) Device() *cyw43439.Device {
	return stack.dev
}

func (stack *Stack) LnetoStack() *xnet.StackAsync {
	return &stack.s
}

func (stack *Stack) RecvAndSend() (send, recv int, err error) {
	dev := stack.dev
	gotRecv, errrecv := dev.PollOne()
	if gotRecv {
		recv = int(stack.lastrecv)
	}
	if errrecv != nil {
		stack.logerr("RecvAndSend:PollOne", slog.Int("plen", recv), slog.String("err", errrecv.Error()))
	}
	send, err = stack.s.Encapsulate(stack.sendbuf, -1, 0)
	if err != nil {
		stack.logerr("RecvAndSend:Encapsulate", slog.Int("plen", send), slog.String("err", err.Error()))
	} else {
		err = errrecv // err will be non-nil serror result if present.
	}
	if send == 0 {
		return send, recv, err
	}
	err = dev.SendEth(stack.sendbuf[:send])
	if err != nil {
		stack.logerr("RecvAndSend:SendEth", slog.Int("plen", send), slog.String("err", err.Error()))
	}
	return send, recv, err
}

type DHCPConfig struct {
	RequestedAddr netip.Addr
}

func (stack *Stack) SetupWithDHCP(cfg DHCPConfig) (dhcpResults *xnet.DHCPResults, err error) {
	if !cfg.RequestedAddr.Is4() {
		return dhcpResults, errors.New("only dhcpv4 supported")
	}
	lstack := stack.LnetoStack()
	const pollTime = 50 * time.Millisecond
	rstack := lstack.StackRetrying(pollTime)
	dhcpResults, err = rstack.DoDHCPv4(cfg.RequestedAddr.As4(), 3*time.Second, 3)
	if err != nil {
		return dhcpResults, err
	}
	err = lstack.AssimilateDHCPResults(dhcpResults)
	if err != nil {
		panic(err)
	}

	// Set the router hardware address as the gateway. Defaults to this address.
	gatewayHW, err := rstack.DoResolveHardwareAddress6(dhcpResults.Router, 500*time.Millisecond, 4)
	if err != nil {
		panic(err)
	}
	lstack.SetGateway6(gatewayHW)
	return dhcpResults, nil
}

func (stack *Stack) logerr(msg string, attrs ...slog.Attr) {
	if stack.log != nil {
		stack.log.LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
	}
}
