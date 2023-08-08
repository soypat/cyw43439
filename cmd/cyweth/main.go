// tinygo flash -monitor -target pico -ldflags '-X "main.ssid=xxx" -X "main.pass=xxx"' cmd/cyweth/main.go

package main

import (
	"fmt"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/internal/tcpctl/eth"
	"github.com/soypat/cyw43439/whd"
)

var (
	ssid string
	pass string
)

func main() {
	defer func() {
		a := recover()
		if a != nil {
			fmt.Println("panic:", a)
		}
		println("program finished")
		time.Sleep(time.Second)
		select {}
	}()
	// Delay before sending output to monitor
	time.Sleep(2 * time.Second)

	spi, cs, wlreg, irq := cyw43439.PicoWSpi(0)
	dev := cyw43439.NewDevice(spi, cs, wlreg, irq, irq)

	// Setup Rx callback
	dev.RecvEthHandle(rx)

	// Enable device for Wifi station mode
	country := whd.CountryCode("XX", 0)
	if err := dev.EnableStaMode(country); err != nil {
		panic(err.Error())
	}
	driver, version, fwVersion, MAC := dev.DeviceInfo()
	fmt.Printf("\n==== DEVICEINFO ====\n\tDriver: %s\n\tVersion:%s\n\tFirmwareVersion:%s\n\tMAC:%s\n\n",
		driver, version, fwVersion, MAC)
	// Wifi connect to AP using WPA2 authorization
	auth := uint32(whd.CYW43_AUTH_WPA2_AES_PSK)
	timeout := 10 * time.Second
	if err := dev.WifiConnectTimeout(ssid, pass, auth, timeout); err != nil {
		panic(err.Error())
	}

	// Everything beyond this point must be implemented!  Need:
	//
	// 0. Get link status (Interrupt or polling?)
	// 1. dev.GetIP()
	// 2. dev.SendEth() implemented for Tx
	// 3. Interrupts (or polling) for Rx

	// Forever send a pkt
	mac, _ := dev.GetHardwareAddr()
	arp := eth.ARPv4Header{
		HardwareType:   1,
		ProtoType:      uint16(eth.EtherTypeIPv4),
		HardwareLength: 6,
		ProtoLength:    4,
		Operation:      1,
		HardwareTarget: [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		ProtoTarget:    [4]byte{192, 168, 0, 44},
		ProtoSender:    [4]byte{192, 168, 0, 255},
	}
	copy(arp.HardwareSender[:], mac[:])
	buf := make([]byte, 28)
	arp.Put(buf)
	for {
		time.Sleep(10 * time.Second)
		if err := dev.SendEth(buf); err != nil {
			panic(err.Error())
		}
	}
}
