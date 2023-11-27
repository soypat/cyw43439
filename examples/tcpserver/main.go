package main

import (
	"io"
	"machine"
	"net/netip"
	"time"
	"unsafe"

	"log/slog"

	"github.com/soypat/cyw43439/cyrw"
	"github.com/soypat/seqs/eth"
	"github.com/soypat/seqs/stacks"
)

const MTU = cyrw.MTU // CY43439 permits 2030 bytes of ethernet frame.

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
	dev := cyrw.DefaultNew()
	// cfg.Level = slog.LevelInfo // Logging level.
	err := dev.Init(cyrw.DefaultConfig())
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
	println("\n\n\nMAC:", dev.MAC().String())

	stack := stacks.NewPortStack(stacks.PortStackConfig{
		MAC:             dev.MACAs6(),
		MaxOpenPortsUDP: 1,
		MaxOpenPortsTCP: 1,
		GlobalHandler: func(ehdr *eth.EthernetHeader, ethPayload []byte) error {
			lastRx = time.Now()
			return nil
		},
		MTU: MTU,
		Logger: slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})),
	})

	dev.RecvEthHandle(stack.RecvEth)

	// Begin asynchronous packet handling.
	go NICLoop(dev, stack)

	err = DoDHCP(stack)
	if err != nil {
		panic("dhcp failed: " + err.Error())
	}

	const socketBuf = 256
	const listenPort = 1234
	listenAddr := netip.AddrPortFrom(stack.Addr(), listenPort)
	socket, err := stacks.NewTCPSocket(stack, stacks.TCPSocketConfig{TxBufSize: socketBuf, RxBufSize: socketBuf})
	if err != nil {
		panic("socket create:" + err.Error())
	}
	err = socket.OpenListenTCP(listenPort, 300)
	if err != nil {
		panic("socket listen:" + err.Error())
	}
	var buf [socketBuf]byte
	println("listening at:", listenAddr.String())
	for {
		n, err := socket.Recv(buf[:])
		if err != nil && err != io.EOF {
			panic("socket recv:" + err.Error())
		} else if n == 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		println("got message:", unsafe.String(&buf[0], n))
		socket.Send(buf[:n])
	}
}

func DoDHCP(stack *stacks.PortStack) error {
	dhcp := stacks.DHCPv4Client{
		MAC:         stack.MACAs6(),
		RequestedIP: [4]byte{192, 168, 1, 69},
	}
	dhcpOngoing := true
	for {
		err := stack.OpenUDP(68, dhcp.HandleUDP)
		if err != nil {
			return err
		}
		stack.FlagPendingUDP(68) // Force a DHCP discovery.
		for i := 0; dhcpOngoing && i < 16; i++ {
			time.Sleep(time.Second / 2) // Check every half second for DHCP completion.
			dhcpOngoing = !dhcp.Done()
		}
		if !dhcpOngoing {
			break
		}
		// Redo.
		println("redo DHCP...")
		err = stack.CloseUDP(68) // DHCP failed, reset state.
		if err != nil {
			return err
		}
		dhcp.Reset()
	}
	ip := netip.AddrFrom4(dhcp.YourIP)
	println("DHCP complete IP:", ip.String())
	stack.SetAddr(ip)
	return nil
}

func NICLoop(dev *cyrw.Device, Stack *stacks.PortStack) {
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
