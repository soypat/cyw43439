package main

import (
	"context"
	"io"
	"log/slog"
	"machine"
	"net/netip"
	"time"

	"github.com/soypat/cyw43439/examples/common"
	mqtt "github.com/soypat/natiu-mqtt"
	"github.com/soypat/seqs"
	"github.com/soypat/seqs/stacks"
)

var (
	logger   *slog.Logger
	clientID = []byte("tinygo-pico")
)

const (
	serverHostname = "test.mosquitto.org"
	serverPort     = 1883
	ourPort        = 1883
)

func main() {
	logger = slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Go lower (Debug-1) to see more verbosity on wifi device.
	}))

	time.Sleep(2 * time.Second)
	println("starting program")
	dhcpc, stack, _, err := common.SetupWithDHCP(common.SetupConfig{
		Hostname: "TCP-pico",
		Logger:   logger,
		TCPPorts: 1, // For MQTT.
		UDPPorts: 1, // For DNS.
	})
	if err != nil {
		panic("in dhcp setup:" + err.Error())
	}

	routerhw, err := common.ResolveHardwareAddr(stack, dhcpc.Router())
	if err != nil {
		panic("router hwaddr resolving:" + err.Error())
	}

	resolver, err := common.NewResolver(stack, dhcpc)
	if err != nil {
		panic("resolver create:" + err.Error())
	}
	addrs, err := resolver.LookupNetIP(serverHostname)
	if err != nil {
		panic("DNS lookup failed:" + err.Error())
	}
	serverAddr := netip.AddrPortFrom(addrs[0], serverPort)

	// Start TCP server.
	const socketBuf = 4096
	socket, err := stacks.NewTCPConn(stack, stacks.TCPConnConfig{TxBufSize: socketBuf, RxBufSize: socketBuf})
	if err != nil {
		panic("socket create:" + err.Error())
	}

	err = socket.OpenDialTCP(ourPort, routerhw, serverAddr, 0x123456)
	if err != nil {
		panic("socket dial:" + err.Error())
	}
	retries := 50
	for socket.State() != seqs.StateListen && retries > 0 {
		time.Sleep(100 * time.Millisecond)
		retries--
	}
	if retries == 0 {
		panic("socket listen failed")
	}

	cfg := mqtt.ClientConfig{
		Decoder: mqtt.DecoderNoAlloc{UserBuffer: make([]byte, 4096)},
		OnPub: func(pubHead mqtt.Header, varPub mqtt.VariablesPublish, r io.Reader) error {
			logger.Info("received message", slog.String("topic", string(varPub.TopicName)))
			return nil
		},
	}

	socket.SetDeadline(time.Now().Add(5 * time.Second))

	var varconn mqtt.VariablesConnect
	varconn.SetDefaultMQTT(clientID)
	client := mqtt.NewClient(cfg)
	err = client.Connect(context.Background(), socket, &varconn)
	if err != nil {
		panic("mqtt connect:" + err.Error())
	}
	logger.Info("connected to mqtt server")
}
