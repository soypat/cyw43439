package main

import (
	"io"
	"log/slog"
	"machine"
	"math/rand"
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
	clientID      = []byte("tinygo-mqtt")
	pubFlags, _   = mqtt.NewPublishFlags(mqtt.QoS0, false, false)
	pubVar        = mqtt.VariablesPublish{
		TopicName:        []byte("tinygo-pico-test"),
		PacketIdentifier: 0xc0fe,
	}
)

const connTimeout = 5 * time.Second
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr
// Set this address to the server's address.
// You may run a local comqtt server: https://github.com/wind-c/comqtt
// build cmd/single, run it and change the IP address to your local server.
const serverAddrStr = "192.168.0.44:1883"

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	_, stack, _, err := common.SetupWithDHCP(common.SetupConfig{
		Hostname: string(clientID),
		Logger:   logger,
		TCPPorts: 1, // For HTTP over TCP.
		UDPPorts: 1, // For DNS.
	})
	start := time.Now()
	if err != nil {
		panic("setup DHCP:" + err.Error())
	}
	svAddr, err := netip.ParseAddrPort(serverAddrStr)
	if err != nil {
		panic("parsing server address:" + err.Error())
	}
	// Resolver router's hardware address to dial outside our network to internet.
	serverHWAddr, err := common.ResolveHardwareAddr(stack, svAddr.Addr())
	if err != nil {
		panic("router hwaddr resolving:" + err.Error())
	}

	rng := rand.New(rand.NewSource(int64(time.Now().Sub(start))))
	// Start TCP server.
	clientAddr := netip.AddrPortFrom(stack.Addr(), uint16(rng.Intn(65535-1024)+1024))
	conn, err := stacks.NewTCPConn(stack, stacks.TCPConnConfig{
		TxBufSize: tcpbufsize,
		RxBufSize: tcpbufsize,
	})

	if err != nil {
		panic("conn create:" + err.Error())
	}

	closeConn := func(err string) {
		slog.Error("tcpconn:closing", slog.String("err", err))
		conn.Close()
		for !conn.State().IsClosed() {
			slog.Info("tcpconn:waiting", slog.String("state", conn.State().String()))
			time.Sleep(1000 * time.Millisecond)
		}
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

	// Connection loop for TCP+MQTT.
	for {
		random := rng.Uint32()
		logger.Info("socket:listen")
		err = conn.OpenDialTCP(clientAddr.Port(), serverHWAddr, svAddr, seqs.Value(random))
		if err != nil {
			panic("socket dial:" + err.Error())
		}
		retries := 50
		for conn.State() != seqs.StateEstablished && retries > 0 {
			time.Sleep(100 * time.Millisecond)
			retries--
		}
		if retries == 0 {
			logger.Info("socket:no-establish")
			closeConn("did not establish connection")
			continue
		}

		// We start MQTT connect with a deadline on the socket.
		logger.Info("mqtt:start-connecting")
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		err = client.StartConnect(conn, &varconn)
		if err != nil {
			logger.Error("mqtt:start-connect-failed", slog.String("reason", err.Error()))
			closeConn("connect failed")
			continue
		}
		retries = 50
		for retries > 0 && !client.IsConnected() {
			time.Sleep(100 * time.Millisecond)
			err = client.HandleNext()
			if err != nil {
				println("mqtt:handle-next-failed", err.Error())
			}
			retries--
		}
		if !client.IsConnected() {
			logger.Error("mqtt:connect-failed", slog.Any("reason", client.Err()))
			closeConn("connect timed out")
			continue
		}

		for client.IsConnected() {
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			pubVar.PacketIdentifier = uint16(rng.Uint32())
			err = client.PublishPayload(pubFlags, pubVar, []byte("hello world"))
			if err != nil {
				logger.Error("mqtt:publish-failed", slog.Any("reason", err))
			}
			logger.Info("published message", slog.Uint64("packetID", uint64(pubVar.PacketIdentifier)))
			err = client.HandleNext()
			if err != nil {
				println("mqtt:handle-next-failed", err.Error())
			}
			time.Sleep(5 * time.Second)
		}
		logger.Error("mqtt:disconnected", slog.Any("reason", client.Err()))
		closeConn("disconnected")
	}
}
