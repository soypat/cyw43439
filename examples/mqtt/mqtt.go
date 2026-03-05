package main

// WARNING: default -scheduler=cores unsupported, compile with -scheduler=tasks set!

import (
	"context"
	"io"
	"log/slog"
	"machine"
	"net"
	"net/netip"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/cyw43439/examples/cywnet/credentials"
	"github.com/soypat/lneto/tcp"
	mqtt "github.com/soypat/natiu-mqtt"
)

// Setup Wifi Password and SSID by creating ssid.text and password.text files in
// ../cywnet/credentials/ directory. Credentials are used for examples in this repo.

var (
	logger      *slog.Logger
	clientID    = []byte("tinygo-mqtt")
	pubFlags, _ = mqtt.NewPublishFlags(mqtt.QoS0, false, false)
	topic       = []byte("tinygo-pico-test")
	pubVar      = mqtt.VariablesPublish{
		TopicName: topic,
	}
	subVar = mqtt.VariablesSubscribe{
		TopicFilters: []mqtt.SubscribeRequest{
			{TopicFilter: topic, QoS: mqtt.QoS0},
		},
	}
)

const connTimeout = 5 * time.Second
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr

// Set this address to the server's address.
// You may run a local comqtt server: https://github.com/wind-c/comqtt
// build cmd/single, run it and change the IP address to your local server.
const serverAddrStr = "192.168.1.53:1883"

func main() {
	logger = slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	println("starting MQTT example")

	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	cystack, err := cywnet.NewConfiguredPicoWithStack(credentials.SSID(), credentials.Password(), devcfg, cywnet.StackConfig{
		Hostname:    string(clientID),
		MaxTCPPorts: 1,
	})
	if err != nil {
		panic("setup failed:" + err.Error())
	}

	go loopForeverStack(cystack)

	dhcpResults, err := cystack.SetupWithDHCP(cywnet.DHCPConfig{})
	if err != nil {
		panic("DHCP failed:" + err.Error())
	}
	logger.Info("DHCP complete", slog.String("addr", dhcpResults.AssignedAddr.String()))

	svAddr, err := netip.ParseAddrPort(serverAddrStr)
	if err != nil {
		panic("parsing server address:" + err.Error())
	}

	stack := cystack.LnetoStack()
	const pollTime = 5 * time.Millisecond
	rstack := stack.StackRetrying(pollTime)

	client := mqtt.NewClient(mqtt.ClientConfig{
		Decoder: mqtt.DecoderNoAlloc{UserBuffer: make([]byte, 4096)},
		OnPub: func(_ mqtt.Header, varPub mqtt.VariablesPublish, r io.Reader) error {
			payload, err := io.ReadAll(r)
			if err != nil {
				logger.Error("OnPub:read failed", slog.String("err", err.Error()))
				return err
			}
			logger.Info("mqttrx", slog.String("topic", string(varPub.TopicName)), slog.String("msg", string(payload)))
			return nil
		},
	})
	var conn tcp.Conn
	err = conn.Configure(tcp.ConnConfig{
		RxBuf:             make([]byte, tcpbufsize),
		TxBuf:             make([]byte, tcpbufsize),
		TxPacketQueueSize: 3,
		Logger:            logger,
	})
	if err != nil {
		panic("conn configure:" + err.Error())
	}

	for {
		if !conn.State().IsClosed() {
			// Ensure connection is closed and state reset before Dial.
			conn.Abort()
		}
		lport := uint16(stack.Prand32()>>17) + 1024
		err = rstack.DoDialTCP(&conn, lport, svAddr, connTimeout, 3)
		if err != nil {
			logger.Error("tcp dial failed", slog.String("err", err.Error()))
			time.Sleep(2 * time.Second)
			continue
		}
		handleConn(&conn, client)
		if err := client.Err(); err != nil {
			logger.Error("mqtt:disconnected", slog.String("err", err.Error()))
		}
		conn.Close()
		time.Sleep(time.Second) // Give time for connection to close.
	}
}

func handleConn(conn *tcp.Conn, client *mqtt.Client) {
	defer client.Disconnect(net.ErrClosed)
	var varconn mqtt.VariablesConnect
	varconn.SetDefaultMQTT(clientID)

	conn.SetDeadline(time.Now().Add(connTimeout))
	err := client.StartConnect(conn, &varconn)
	if err != nil {
		logger.Error("mqtt:start-connect-failed", slog.String("err", err.Error()))
		return
	}

	for retries := 50; retries > 0 && !client.IsConnected(); retries-- {
		time.Sleep(100 * time.Millisecond)
		if err = client.HandleNext(); err != nil {
			println("mqtt:handle-next-failed", err.Error())
		}
	}
	if !client.IsConnected() {
		logger.Error("mqtt:connect-failed", slog.Any("reason", client.Err()))
		return
	}
	logger.Info("mqtt:connected")

	ctx, cancel := context.WithTimeout(context.Background(), connTimeout)
	err = client.Subscribe(ctx, subVar)
	cancel()
	if err != nil {
		logger.Error("mqtt:subscribe-failed", slog.String("err", err.Error()))
		return
	}
	// We've connected and subscribed succesfully, unset deadline.
	conn.SetDeadline(time.Time{})

	var lastPub time.Time
	for client.IsConnected() {
		now := time.Now()
		if now.Sub(lastPub) > 5*time.Second {
			lastPub = now
			pubVar.PacketIdentifier = uint16(now.UnixNano())
			err = client.PublishPayload(pubFlags, pubVar, []byte("hello world"))
			if err != nil {
				logger.Error("mqtt:publish-failed", slog.String("err", err.Error()))
			}
			logger.Info("published", slog.Uint64("pktID", uint64(pubVar.PacketIdentifier)))
		}
		if err = client.HandleNext(); err != nil {
			println("mqtt:handle-next-failed", err.Error())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}
