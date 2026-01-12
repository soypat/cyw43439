package main

import (
	"io"
	"log/slog"
	"machine"
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
	pubVar      = mqtt.VariablesPublish{
		TopicName:        []byte("tinygo-pico-test"),
		PacketIdentifier: 0xc0fe,
	}
)

const connTimeout = 5 * time.Second
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr

// Set this address to the server's address.
// You may run a local comqtt server: https://github.com/wind-c/comqtt
// build cmd/single, run it and change the IP address to your local server.
const serverAddrStr = "192.168.1.53:1883"

var requestedIP = [4]byte{192, 168, 1, 99}

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

	// Background loop needed to process packets.
	go loopForeverStack(cystack)

	dhcpResults, err := cystack.SetupWithDHCP(cywnet.DHCPConfig{
		RequestedAddr: netip.AddrFrom4(requestedIP),
	})
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

	// Configure TCP connection.
	var conn tcp.Conn
	err = conn.Configure(tcp.ConnConfig{
		RxBuf:             make([]byte, tcpbufsize),
		TxBuf:             make([]byte, tcpbufsize),
		TxPacketQueueSize: 3,
	})
	if err != nil {
		panic("conn configure:" + err.Error())
	}

	// MQTT client configuration.
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
		lport := uint16(stack.Prand32()>>17) + 1024 // Random local port.
		logger.Info("socket:listen", slog.Uint64("localport", uint64(lport)))

		// Dial TCP with retries.
		err = rstack.DoDialTCP(&conn, lport, svAddr, connTimeout, 3)
		if err != nil {
			logger.Error("tcp dial failed", slog.String("err", err.Error()))
			conn.Abort()
			time.Sleep(2 * time.Second)
			continue
		}

		// We start MQTT connect with a deadline on the socket.
		logger.Info("mqtt:start-connecting")
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		err = client.StartConnect(&conn, &varconn)
		if err != nil {
			logger.Error("mqtt:start-connect-failed", slog.String("reason", err.Error()))
			closeConn(&conn)
			continue
		}

		retries := 50
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
			closeConn(&conn)
			continue
		}

		logger.Info("mqtt:connected")
		for client.IsConnected() {
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			pubVar.PacketIdentifier = uint16(stack.Prand32())
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
		closeConn(&conn)
	}
}

func closeConn(conn *tcp.Conn) {
	conn.Close()
	for i := 0; i < 50 && !conn.State().IsClosed(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	conn.Abort()
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}
