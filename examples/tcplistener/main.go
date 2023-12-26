package main

import (
	"errors"
	"machine"
	"net"
	"net/netip"
	"os"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439"
	"github.com/soypat/seqs/eth/dhcp"
	"github.com/soypat/seqs/stacks"
)

const (
	MTU         = cyw43439.MTU // CY43439 permits 2030 bytes of ethernet frame.
	tcpbufsize  = 512          // MTU - ethhdr - iphdr - tcphdr
	connTimeout = 2 * time.Second
)

var (
	lastRx, lastTx time.Time
	logger         *slog.Logger
)

func main() {
	logger = slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Go lower (Debug-1) to see more verbosity on wifi device.
	}))
	time.Sleep(2 * time.Second)
	logger.Info("starting program")
	dev := cyw43439.NewPicoWDevice()
	cfg := cyw43439.DefaultWifiConfig()
	// cfg.Logger = logger // Uncomment to see in depth info on wifi device functioning.
	err := dev.Init(cfg)
	if err != nil {
		panic(err)
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
		MaxOpenPortsUDP: 1,
		MaxOpenPortsTCP: 1,
		MTU:             MTU,
		Logger:          logger,
	})

	dev.RecvEthHandle(stack.RecvEth)

	// Begin asynchronous packet handling.
	go NICLoop(dev, stack)

	// Perform DHCP request.
	dhcpClient := stacks.NewDHCPClient(stack, dhcp.DefaultClientPort)
	err = dhcpClient.BeginRequest(stacks.DHCPRequestConfig{
		RequestedAddr: netip.AddrFrom4([4]byte{192, 168, 1, 69}),
		Xid:           0x12345678,
	})
	if err != nil {
		panic("dhcp failed: " + err.Error())
	}
	for !dhcpClient.Done() {
		logger.Info("DHCP ongoing...")
		time.Sleep(time.Second / 2)
	}
	ip := dhcpClient.Offer()
	logger.Info("DHCP complete", slog.String("ip", ip.String()))
	stack.SetAddr(ip) // It's important to set the IP address after DHCP completes.

	// Start TCP server.
	const listenPort = 1234
	listenAddr := netip.AddrPortFrom(stack.Addr(), listenPort)
	listener, err := stacks.NewTCPListener(stack, stacks.TCPListenerConfig{
		MaxConnections: 3,
		ConnTxBufSize:  tcpbufsize,
		ConnRxBufSize:  tcpbufsize,
	})
	if err != nil {
		panic("listener create:" + err.Error())
	}
	err = listener.StartListening(listenPort)
	if err != nil {
		panic("listener start:" + err.Error())
	}
	var buf [512]byte
	logger.Info("listening", slog.String("addr", listenAddr.String()))
	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error("listener accept:", slog.String("err", err.Error()))
			time.Sleep(time.Second)
			continue
		}
		logger.Info("new connection", slog.String("remote", conn.RemoteAddr().String()))
		err = conn.SetDeadline(time.Now().Add(connTimeout))
		if err != nil {
			logger.Error("conn set deadline:", slog.String("err", err.Error()))
			continue
		}
		for {
			n, err := conn.Read(buf[:])
			if err != nil {
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					logger.Error("conn read:", slog.String("err", err.Error()))
				}
				break
			}
			_, err = conn.Write(buf[:n])
			if err != nil {
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					logger.Error("conn write:", slog.String("err", err.Error()))
				}
				break
			}
		}
		err = conn.Close()
		if err != nil {
			logger.Error("conn close:", slog.String("err", err.Error()))
		}
		time.Sleep(100 * time.Millisecond)
	}

}

func NICLoop(dev *cyw43439.Device, Stack *stacks.PortStack) {
	// Maximum number of packets to queue before sending them.
	const (
		queueSize                = 4
		maxRetriesBeforeDropping = 3
	)
	var queue [queueSize][MTU]byte
	var lenBuf [queueSize]int
	var retries [queueSize]int
	markSent := func(i int) {
		queue[i] = [MTU]byte{} // Not really necessary.
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
