// tinygo flash -monitor -target pico -ldflags '-X "main.ssid=xxx" -X "main.pass=xxx"' cmd/cyweth/main.go

package main

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/whd"
)

var (
	ssid string
	pass string
)

func rx(pkt []byte) error {
	println("Rx:", len(pkt))
	println(hex.Dump(pkt))
	return nil
}

func main() {

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
	for {
		var pkt []byte

		// TODO encode pkt, for example to broadcast a gratuitous ARP pkt:
		//
		//    Eth.Src:  dev.GetHardwareAddr()
		//    Eth.Dst:  net.ParseMAC("ff:ff:ff:ff:ff:ff") // broadcast
		//    Eth.Type: ARP (0x0806)
		//        Arp.HardwareType: 1
		//        Arp.ProtocolType: IPv4 (0x0800)
		//        Arp.HardwareSize: 6
		//        Arp.ProtocolSize: 4
		//        Arp.Opcode:       request (1)
		//        Arp.SenderMAC:    dev.GetHardwareAddr()
		//        Arp.SenderIP:     dev.GetIP()
		//        Arp.TargetMAC:    net.ParseMAC("ff:ff:ff:ff:ff:ff")
		//        Arp.TargetIP:     dev.GetIP()

		println("Tx:", len(pkt))
		println(hex.Dump(pkt))

		if err := dev.SendEth(pkt); err != nil {
			panic(err.Error())
		}

		time.Sleep(time.Second)
	}
}
