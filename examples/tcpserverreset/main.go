package main

// WARNING: default -scheduler=cores unsupported, compile with -scheduler=tasks set!

// TCP echo server that reproduces a Device Reset()+Init() while the stack is running.
// Send the string "reset()" over the connection to make the server disconnect,
// reset the CYW43439, call Init, rejoin wifi and restart the lneto stack.

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"machine"
	"net"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/cyw43439/examples/cywnet/credentials"
	"github.com/soypat/lneto/ipv4"
	"github.com/soypat/lneto/tcp"
)

var (
	requestedIP = [4]byte{192, 168, 1, 99}
	ourPort     = uint16(1234)
)

const resetCmd = "reset()"

// Loop control: stack loop is paused while the device is being reset.
var (
	pauseLoop  atomic.Bool
	loopPaused atomic.Bool
)

func main() {
	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	println("starting program")
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{Level: slog.LevelInfo}))
	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	stackcfg := cywnet.StackConfig{
		Hostname:          "DHCP-pico",
		MaxActiveTCPPorts: 1,
	}
	cystack, err := cywnet.NewConfiguredPicoWithStack(credentials.SSID(), credentials.Password(), devcfg, stackcfg)
	if err != nil {
		panic(err)
	}
	go loopForeverStack(cystack)
	setupNetwork(cystack)

	var buf [512]byte
	var conn tcp.Conn
	err = conn.Configure(tcp.ConnConfig{
		RxBuf:             make([]byte, 512),
		TxBuf:             make([]byte, 512),
		TxPacketQueueSize: 3,
	})
	if err != nil {
		panic(err)
	}
	for {
		stack := cystack.StackAsync()
		err = stack.ListenTCP4(&conn, ourPort)
		if err != nil {
			println("listen failed:", err.Error())
			time.Sleep(3 * time.Second)
			conn.Abort()
			continue
		}
		println("listening on:", ipv4.String(stack.Addr4()), "port", ourPort)
		for conn.State().IsPreestablished() {
			time.Sleep(5 * time.Millisecond)
		}
		doReset := false
		for !doReset {
			n, err := conn.Read(buf[:])
			if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				println("read error:", err.Error())
			}
			if n == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if bytes.HasPrefix(bytes.TrimSpace(buf[:n]), []byte(resetCmd)) {
				doReset = true
				break
			}
			_, err = conn.Write(buf[:n])
			if err == nil {
				err = conn.Flush()
			}
			if err != nil {
				println("write error:", err.Error())
			}
		}
		conn.Close()
		time.Sleep(time.Second)
		conn.Abort()
		if doReset {
			println("reset requested: resetting device")
			resetDevice(cystack, devcfg, stackcfg)
			println("device reset done, stack restarted")
		}
	}
}

func resetDevice(cystack *cywnet.Stack, devcfg cyw43439.Config, cfg cywnet.StackConfig) {
	// Stop the stack loop so nothing touches the device during reset.
	pauseLoop.Store(true)
	for !loopPaused.Load() {
		time.Sleep(time.Millisecond)
	}
	dev := cystack.Device()
	dev.Reset()
	err := dev.Init(devcfg)
	if err != nil {
		panic("init after reset: " + err.Error())
	}
	cfg.WifiJoinOptions.Passphrase = credentials.Password()
	err = dev.Join(credentials.SSID(), cfg.WifiJoinOptions)
	if err != nil {
		panic("join after reset: " + err.Error())
	}
	// Reset clears device state, so reattach the ingress handler to the lneto stack.
	stack := cystack.StackAsync()
	dev.RecvEthHandle(func(pkt []byte) { stack.IngressEthernet(pkt) })
	pauseLoop.Store(false) // Restart stack loop, needed for DHCP.
	for loopPaused.Load() {
		time.Sleep(time.Millisecond)
	}
	setupNetwork(cystack)
}

func setupNetwork(cystack *cywnet.Stack) {
	res, err := cystack.SetupWithDHCP(cywnet.DHCPConfig{RequestedAddr: netip.AddrFrom4(requestedIP)})
	if err != nil {
		panic("while performing DHCP: " + err.Error())
	}
	println("dhcp addr:", ipv4.String(res.AssignedAddr4))
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		if pauseLoop.Load() {
			loopPaused.Store(true)
			time.Sleep(time.Millisecond)
			continue
		}
		loopPaused.Store(false)
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}
