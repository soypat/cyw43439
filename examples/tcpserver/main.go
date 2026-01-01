package main

import (
	"errors"
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
)

// Setup Wifi Password and SSID by creating ssid.text and password.text files in
// ../cywnet/credentials/ directory. Credentials are used for examples in this repo.
// When building your own application use local storage to store wifi credentials securely.
var (
	requestedIP = [4]byte{192, 168, 1, 99}
	ourPort     = uint16(1234)
)

const (
	tcpTimeout = 6 * time.Second
	tcpRetries = 2
)

func main() {
	time.Sleep(2 * time.Second) // Give time to connect to USB and monitor output.
	println("starting program")
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	cystack, err := cywnet.NewConfiguredPicoWithStack(credentials.SSID(), credentials.Password(), devcfg, cywnet.StackConfig{
		Hostname:    "DHCP-pico",
		MaxTCPConns: 1,
	})
	if err != nil {
		panic(err)
	}

	// Goroutine loop needed to use the cywnet.StackBlocking implementation.
	// To avoid goroutines use StackAsync. This however means much more effort and boilerplate done by the user.
	go loopForeverStack(cystack)

	dhcpResults, err := cystack.SetupWithDHCP(cywnet.DHCPConfig{
		RequestedAddr: netip.AddrFrom4(requestedIP),
	})
	if err != nil {
		panic("while performing DHCP: " + err.Error())
	}
	stack := cystack.LnetoStack()
	gatewayHW := stack.Gateway6()
	println("dhcp addr:", dhcpResults.AssignedAddr.String(), "routerhw:", net.HardwareAddr(gatewayHW[:]).String())
	var buf [512]byte
	var conn tcp.Conn
	connlogger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	err = conn.Configure(tcp.ConnConfig{
		RxBuf:             make([]byte, 512),
		TxBuf:             make([]byte, 512),
		TxPacketQueueSize: 3,
		Logger:            connlogger,
	})
	conn.InternalHandler().SetLoggers(connlogger, connlogger)
	if err != nil {
		panic(err)
	}
	println("listening on:", netip.AddrPortFrom(stack.Addr(), ourPort).String())
	for {
		err = stack.ListenTCP(&conn, ourPort)
		if err != nil {
			println("listen failed:", err.Error())
			time.Sleep(3 * time.Second)
			conn.Abort()
			continue
		}
		println("begin listening...")
		for conn.State().IsPreestablished() {
			time.Sleep(5 * time.Millisecond) // Listen for an incoming connection forever.
		}
		for {
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
			_, err = conn.Write(buf[:n]) // Echo back response.
			if err != nil {
				println("write error:", err.Error())
			} else {
				err = conn.Flush()
				println("wrote back response of length", n)
			}
		}
		conn.Close()
		// conn.FlushOutputBuffer()
		time.Sleep(time.Second)
	}
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(5 * time.Millisecond) // No data to send or receive, sleep for a bit.
		}
	}
}
