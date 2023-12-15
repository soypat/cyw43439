package main

import (
	"net"
	"net/netip"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439"

	"github.com/soypat/seqs/eth"
	"github.com/soypat/seqs/eth/dhcp"
	"github.com/soypat/seqs/stacks"
)

const MTU = cyw43439.MTU // CY43439 permits 2030 bytes of ethernet frame.

var lastRx, lastTx time.Time

func main() {
	defer func() {
		println("program finished")
		if a := recover(); a != nil {
			println("panic:", a)
		}
	}()
	time.Sleep(2 * time.Second)
	println("starting program")
	slog.Debug("starting program")
	dev := cyw43439.NewPicoWDevice()
	// cfg.Level = slog.LevelInfo // Logging level.
	err := dev.Init(cyw43439.DefaultWifiConfig())
	if err != nil {
		panic(err)
	}

	for {
		// Set ssid/pass in secrets.go
		err = dev.JoinWPA2(ssid, pass)
		if err == nil {
			break
		}
		println("wifi join failed:", err.Error())
		time.Sleep(5 * time.Second)
	}
	mac := dev.MACAs6()
	println("\n\n\nMAC:", net.HardwareAddr(mac[:]).String())

	stack := stacks.NewPortStack(stacks.PortStackConfig{
		MAC:             dev.MACAs6(),
		MaxOpenPortsUDP: 1,
		MaxOpenPortsTCP: 1,
		GlobalHandler: func(ehdr *eth.EthernetHeader, ethPayload []byte) error {
			lastRx = time.Now()
			return nil
		},
		MTU:    MTU,
		Logger: slog.Default(),
	})

	dev.RecvEthHandle(stack.RecvEth)

	// Begin asynchronous packet handling.
	go NICLoop(dev, stack)

	println("Start DHCP...")
	dhcp := stacks.NewDHCPClient(stack, dhcp.DefaultClientPort)

	err = dhcp.BeginRequest(stacks.DHCPRequestConfig{
		RequestedAddr: netip.AddrFrom4([4]byte{192, 168, 1, 69}),
		Xid:           0x12345678,
	})
	if err != nil {
		panic("dhcp begin request:" + err.Error())
	}
	for !dhcp.Done() {
		println("trying DHCP...")
		time.Sleep(time.Second)
	}
	ip := dhcp.Offer()
	println("DHCP complete IP:", ip.String())
	stack.SetAddr(ip)

	const refresh = 300 * time.Millisecond
	lastLED := false
	for {
		recentRx := time.Since(lastRx) < refresh*3/2
		recentTx := time.Since(lastTx) < refresh*3/2
		ledStatus := recentRx || recentTx
		if ledStatus != lastLED {
			dev.GPIOSet(0, ledStatus)
			lastLED = ledStatus
		}
		time.Sleep(refresh)
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
		// Poll for incoming packets.
		for i := 0; i < 2; i++ {
			gotPacket, err := dev.TryPoll()
			if err != nil {
				println("poll error:", err.Error())
			}
			if !gotPacket {
				break
			}
		}

		// Queue packets to be sent.
		sending := 0
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
			sending += lenBuf[i]
		}

		if sending == 0 {
			time.Sleep(51 * time.Millisecond) // Nothing to send, sleep.
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
