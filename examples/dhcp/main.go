package main

import (
	"machine"
	"time"

	"log/slog"

	"github.com/soypat/cyw43439/examples/common"
)

// Setup Wifi Password and SSID in common/secrets.go

func main() {
	time.Sleep(2 * time.Second)
	println("starting program")
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	client, _, _, err := common.SetupWithDHCP(common.Config{
		Hostname:    "DHCP-pico",
		Logger:      logger,
		RequestedIP: "192.168.1.145",
		UDPPorts:    1,
	})
	if !client.IsDone() {
		println("DHCP did not complete succesfully")
	}
	if err != nil {
		panic(err)
	}
}
