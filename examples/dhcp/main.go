package main

import (
	"errors"
	"strconv"
	"time"

	"github.com/soypat/cyw43439/cyrw"
	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/cyw43439/internal/tcpctl"
)

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
	cfg := cyrw.DefaultConfig()
	// cfg.Level = slog.LevelInfo // Logging level.
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
	mac := dev.MAC()
	println("\n\n\nMAC:", mac.String())
	stack = tcpctl.NewStack(tcpctl.StackConfig{
		MAC:         nil,
		MaxUDPConns: 2,
	})
	stack.GlobalHandler = func(b []byte) {
		lastRx = time.Now()
	}
	dev.RecvEthHandle(stack.RecvEth)
	for {
		println("Trying DoDHCP")
		err = DoDHCP(stack, dev)
		if err == nil {
			println("========\nDHCP done, your IP: ", stack.IP.String(), "\n========")
			break
		}
		println(err.Error())
		time.Sleep(8 * time.Second)
	}

	println("finished init OK")

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

var (
	stack *tcpctl.Stack
	txbuf [1500]byte
)

func DoDHCP(s *tcpctl.Stack, dev *cyrw.Device) error {
	var dc tcpctl.DHCPClient
	copy(dc.MAC[:], dev.MAC())
	err := s.OpenUDP(68, dc.HandleUDP)
	if err != nil {
		return err
	}
	defer s.CloseUDP(68)
	err = s.FlagUDPPending(68) // Force a DHCP discovery.
	if err != nil {
		return err
	}
	for retry := 0; retry < 20 && dc.State < 3; retry++ {
		n, err := stack.HandleEth(txbuf[:])
		if err != nil {
			return err
		}
		if n == 0 {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		err = dev.SendEth(txbuf[:n])
		if err != nil {
			return err
		}
	}
	if dc.State != 3 { // TODO: find way to make this value more self descriptive.
		return errors.New("DHCP did not complete, state=" + strconv.Itoa(int(dc.State)))
	}
	if len(s.IP) == 0 {
		s.IP = make([]byte, 4)
	}
	copy(s.IP, dc.YourIP[:])
	return nil
}
