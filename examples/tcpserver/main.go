package main

import (
	"errors"
	"machine"
	"net"
	"net/netip"
	"runtime"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439"
	"github.com/soypat/seqs"
	"github.com/soypat/seqs/eth"
	"github.com/soypat/seqs/eth/dhcp"
	"github.com/soypat/seqs/stacks"
)

const MTU = cyw43439.MTU // CY43439 permits 2030 bytes of ethernet frame.

var lastRx, lastTx time.Time
var logger *slog.Logger

func main() {
	defer func() {
		println("program finished")
		if a := recover(); a != nil {
			println("panic:", a)
		}
	}()
	logger = slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Go lower (Debug-1) to see more verbosity on wifi device.
	}))

	time.Sleep(2 * time.Second)
	println("starting program")
	logger.Debug("starting program")
	dev := cyw43439.NewPicoWDevice()
	cfg := cyw43439.DefaultWifiConfig()
	cfg.Logger = logger // Uncomment to see in depth info on wifi device functioning.
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
		Logger: logger,
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
		println("dhcp ongoing...")
		time.Sleep(time.Second / 2)
	}
	ip := dhcpClient.Offer()
	println("DHCP complete IP:", ip.String())
	stack.SetAddr(ip) // It's important to set the IP address after DHCP completes.

	// Start TCP server.
	const socketBuf = 256
	const listenPort = 1234
	listenAddr := netip.AddrPortFrom(stack.Addr(), listenPort)
	socket, err := stacks.NewTCPSocket(stack, stacks.TCPSocketConfig{TxBufSize: socketBuf, RxBufSize: socketBuf})
	if err != nil {
		panic("socket create:" + err.Error())
	}
	println("start listening on:", listenAddr.String())
	err = ForeverTCPListenEcho(socket, listenAddr)
	if err != nil {
		panic("socket listen:" + err.Error())
	}
}

func ForeverTCPListenEcho(socket *stacks.TCPSocket, addr netip.AddrPort) error {
	var iss seqs.Value = 100
	var buf [512]byte
	for {
		iss += 200
		err := socket.OpenListenTCP(addr.Port(), iss)
		if err != nil {
			return err
		}
		for {
			n, err := socket.Read(buf[:])
			if errors.Is(err, net.ErrClosed) {
				break
			}
			if n == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			_, err = socket.Write(buf[:n])
			if err != nil {
				return err
			}
		}
		socket.Close()
		socket.FlushOutputBuffer()
		time.Sleep(time.Second)
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
		printGCStatsIfChanged(logger)
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

// Test GC stats printing.
var (
	memstats   runtime.MemStats
	lastAllocs uint64
	lastLog    time.Time
)

const enableGCPrint = true
const minLogPeriod = 8 * time.Second

// printGCStatsIfChanged prints GC stats if they have changed since the last call and
// at least minLogPeriod has passed.
func printGCStatsIfChanged(log *slog.Logger) {
	if !enableGCPrint {
		return
	}
	// Split logging into two calls since slog inlines at most 5 arguments per call.
	// This way we avoid heap allocations for the log message to avoid interfering with GC.
	runtime.ReadMemStats(&memstats)
	now := time.Now()
	if memstats.TotalAlloc == lastAllocs || now.Sub(lastLog) < minLogPeriod {
		return // don't print if no change in allocations.
	}
	println("GC stats ", now.Unix(), "heap_inc=", int64(memstats.TotalAlloc)-int64(lastAllocs))
	print(" TotalAlloc= ", memstats.TotalAlloc)
	print(" Frees=", memstats.Frees)
	print(" Mallocs=", memstats.Mallocs)
	print(" GCSys=", memstats.GCSys)
	println(" Sys=", memstats.Sys)
	print("HeapIdle=", memstats.HeapIdle)
	print(" HeapInuse=", memstats.HeapInuse)
	print(" HeapReleased=", memstats.HeapReleased)
	println(" HeapSys=", memstats.HeapSys)
	// log.LogAttrs(context.Background(), slog.LevelInfo, "MemStats",
	// 	slog.Uint64("TotalAlloc", memstats.TotalAlloc),
	// 	slog.Uint64("Frees", memstats.Frees),
	// 	slog.Uint64("Mallocs", memstats.Mallocs),
	// 	slog.Uint64("GCSys", memstats.GCSys),
	// 	slog.Uint64("Sys", memstats.Sys),
	// )
	// log.LogAttrs(context.Background(), slog.LevelInfo, "MemStats.Heap",
	// 	slog.Uint64("HeapIdle", memstats.HeapIdle),
	// 	slog.Uint64("HeapInuse", memstats.HeapInuse),
	// 	slog.Uint64("HeapReleased", memstats.HeapReleased),
	// 	slog.Uint64("HeapSys", memstats.HeapSys),
	// )
	// Above calls may allocate.
	runtime.ReadMemStats(&memstats)
	lastAllocs = memstats.TotalAlloc
	lastLog = now
}
