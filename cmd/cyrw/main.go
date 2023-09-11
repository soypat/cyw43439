package main

import (
	"time"

	"github.com/soypat/cyw43439/cyrw"
	"github.com/soypat/cyw43439/internal/slog"
)

func main() {
	defer func() {
		println("program finished")
		if a := recover(); a != nil {
			println("panic:", a)
		}
	}()
	// handler := slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{Level: slog.LevelDebug})
	// slog.SetDefault(slog.New(handler))

	time.Sleep(2 * time.Second)
	println("starting program")
	slog.Debug("starting program")
	dev := cyrw.DefaultNew()

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

	println("finished init OK")
	cycle := true
	for {
		if err != nil {
			println(err.Error())
		}
		time.Sleep(time.Second / 2)
		cycle = !cycle
	}
}
