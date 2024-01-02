package common

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/seqs/eth/dhcp"
	"github.com/soypat/seqs/stacks"
)

const mtu = cyw43439.MTU

type Config struct {
	Hostname    string
	RequestedIP string
	Logger      *slog.Logger
	UDPPorts    uint16
	TCPPorts    uint16
}

func SetupWithDHCP(cfg Config) (*stacks.DHCPClient, *stacks.PortStack, *cyw43439.Device, error) {
	cfg.UDPPorts++ // Add extra UDP port for DHCP client.
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
			Level: slog.Level(127), // Make temporary logger that does no logging.
		}))
	}
	var err error
	var reqAddr netip.Addr
	if cfg.RequestedIP != "" {
		reqAddr, err = netip.ParseAddr(cfg.RequestedIP)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	dev := cyw43439.NewPicoWDevice()
	wificfg := cyw43439.DefaultWifiConfig()
	// cfg.Logger = logger // Uncomment to see in depth info on wifi device functioning.
	logger.Info("initializing pico W device...")
	err = dev.Init(wificfg)
	if err != nil {
		return nil, nil, nil, err
	}

	if len(pass) == 0 {
		logger.Info("joining open network:", slog.String("ssid", ssid))
	} else {
		logger.Info("joining WPA secure network", slog.String("ssid", ssid), slog.Int("passlen", len(pass)))
	}
	for {
		// Set ssid/pass in secrets.go
		err = dev.JoinWPA2(ssid, pass)
		if err == nil {
			break
		}
		logger.Error("wifi join faled", slog.String("err", err.Error()))
		time.Sleep(5 * time.Second)
	}
	mac := dev.MACAs6()
	logger.Info("wifi join success!", slog.String("mac", net.HardwareAddr(mac[:]).String()))

	stack := stacks.NewPortStack(stacks.PortStackConfig{
		MAC:             mac,
		MaxOpenPortsUDP: int(cfg.UDPPorts),
		MaxOpenPortsTCP: int(cfg.TCPPorts),
		MTU:             mtu,
		Logger:          logger,
	})

	dev.RecvEthHandle(stack.RecvEth)

	// Begin asynchronous packet handling.
	go nicLoop(dev, stack)

	// Perform DHCP request.
	dhcpClient := stacks.NewDHCPClient(stack, dhcp.DefaultClientPort)
	err = dhcpClient.BeginRequest(stacks.DHCPRequestConfig{
		RequestedAddr: reqAddr,
		Xid:           0x12345678,
		Hostname:      cfg.Hostname,
	})
	if err != nil {
		return nil, stack, dev, err
	}
	i := 0
	for !dhcpClient.IsDone() {
		i++
		logger.Info("DHCP ongoing...")
		time.Sleep(time.Second / 2)
		if i > 10 {
			if !reqAddr.IsValid() {
				return dhcpClient, stack, dev, errors.New("DHCP did not complete and no static IP was requested")
			}
			logger.Info("DHCP did not complete, assigning static IP", slog.String("ip", cfg.RequestedIP))
			stack.SetAddr(reqAddr)
			return dhcpClient, stack, dev, nil
		}
	}
	ip := dhcpClient.Offer()
	logger.Info("DHCP complete",
		slog.Uint64("cidrbits", uint64(dhcpClient.CIDRBits())),
		slog.String("ourIP", ip.String()),
		slog.String("dns", dhcpClient.DNSServer().String()),
		slog.String("broadcast", dhcpClient.BroadcastAddr().String()),
		slog.String("router", dhcpClient.Router().String()),
		slog.String("dhcp", dhcpClient.DHCPServer().String()),
		slog.String("hostname", string(dhcpClient.Hostname())),
	)

	stack.SetAddr(ip) // It's important to set the IP address after DHCP completes.
	return dhcpClient, stack, dev, nil
}

func nicLoop(dev *cyw43439.Device, Stack *stacks.PortStack) {
	// Maximum number of packets to queue before sending them.
	const (
		queueSize                = 3
		maxRetriesBeforeDropping = 3
	)
	var queue [queueSize][mtu]byte
	var lenBuf [queueSize]int
	var retries [queueSize]int
	markSent := func(i int) {
		queue[i] = [mtu]byte{} // Not really necessary.
		lenBuf[i] = 0
		retries[i] = 0
	}
	for {
		stallRx := true
		// Poll for incoming packets.
		for i := 0; i < 1; i++ {
			gotPacket, err := dev.TryPoll()
			if err != nil {
				println("poll error:", err.Error())
			}
			if !gotPacket {
				break
			}
			stallRx = false
		}

		// Queue packets to be sent.
		for i := range queue {
			if retries[i] != 0 {
				continue // Packet currently queued for retransmission.
			}
			var err error
			buf := queue[i][:]
			lenBuf[i], err = Stack.HandleEth(buf[:])
			if err != nil {
				println("stack error n(should be 0)=", lenBuf[i], "err=", err.Error())
				lenBuf[i] = 0
				continue
			}
			if lenBuf[i] == 0 {
				break
			}
		}
		stallTx := lenBuf == [queueSize]int{}
		if stallTx {
			if stallRx {
				// Avoid busy waiting when both Rx and Tx stall.
				time.Sleep(51 * time.Millisecond)
			}
			continue
		}

		// Send queued packets.
		for i := range queue {
			n := lenBuf[i]
			if n <= 0 {
				continue
			}
			err := dev.SendEth(queue[i][:n])
			if err != nil {
				// Queue packet for retransmission.
				retries[i]++
				if retries[i] > maxRetriesBeforeDropping {
					markSent(i)
					println("dropped outgoing packet:", err.Error())
				}
			} else {
				markSent(i)
			}
		}
	}
}
