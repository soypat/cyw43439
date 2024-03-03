package main

import (
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
	logger        *slog.Logger
	loggerHandler *slog.TextHandler
	clientID      = []byte("tinygo-pico")
	pubFlags, _   = mqtt.NewPublishFlags(mqtt.QoS0, false, false)
	pubVar        = mqtt.VariablesPublish{
		TopicName:        []byte("tinygo-pico-test"),
		PacketIdentifier: 0x12,
	}
)

const (
	serverHostname = "test.mosquitto.org"
	serverPort     = 1883
	ourPort        = 1883
)

func main() {
	loggerHandler = slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Go lower (Debug-1) to see more verbosity on wifi device.
	})
	logger = slog.New(loggerHandler)

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

	cfg := mqtt.ClientConfig{
		Decoder: mqtt.DecoderNoAlloc{UserBuffer: make([]byte, 4096)},
		OnPub: func(pubHead mqtt.Header, varPub mqtt.VariablesPublish, r io.Reader) error {
			logger.Info("received message", slog.String("topic", string(varPub.TopicName)))
			return nil
		},
	}
	var varconn mqtt.VariablesConnect
	varconn.SetDefaultMQTT(clientID)
	client := mqtt.NewClient(cfg)

	closeConn := func() {
		logger.Info("socket:close-connection", slog.String("state", socket.State().String()))
		socket.FlushOutputBuffer()
		socket.Close()
		for !socket.State().IsClosed() {
			time.Sleep(100 * time.Millisecond)
		}
	}
	// Connection loop for TCP+MQTT.
	for {
		logger.Info("socket:listen")
		err = socket.OpenDialTCP(ourPort, routerhw, serverAddr, 0x123456)
		if err != nil {
			panic("socket dial:" + err.Error())
		}
		retries := 50
		for socket.State() != seqs.StateEstablished && retries > 0 {
			time.Sleep(100 * time.Millisecond)
			retries--
		}
		if retries == 0 {
			logger.Info("socket:no-establish")
			closeConn()
			continue
		}

		// We start MQTT connect with a deadline on the socket.
		logger.Info("mqtt:start-connecting")
		socket.SetDeadline(time.Now().Add(5 * time.Second))
		err = client.StartConnect(socket, &varconn)
		if err != nil {
			logger.Error("mqtt:start-connect-failed", slog.String("reason", err.Error()))
			closeConn()
			continue
		}
		retries = 50
		for retries > 0 && !client.IsConnected() {
			time.Sleep(100 * time.Millisecond)
			retries--
		}
		if !client.IsConnected() {
			logger.Error("mqtt:connect-failed", slog.Any("reason", client.Err()))
			closeConn()
			continue
		}

		for client.IsConnected() {
			socket.SetDeadline(time.Now().Add(5 * time.Second))
			pubVar.PacketIdentifier++
			err = client.PublishPayload(pubFlags, pubVar, []byte("hello world"))
			if err != nil {
				logger.Error("mqtt:publish-failed", slog.Any("reason", err))
			}
			logger.Info("published message", slog.Uint64("packetID", uint64(pubVar.PacketIdentifier)))
			time.Sleep(5 * time.Second)
		}
		logger.Error("mqtt:disconnected", slog.Any("reason", client.Err()))
		closeConn()
	}
}
