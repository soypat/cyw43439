package common

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/netif"
)

const backoffMax = 500 * time.Millisecond

func SetupWithEngine(cfg netif.EngineConfig) (*netif.Engine, error) {
	println("program start3")
	logger := cfg.Logger
	println("program start3.4")
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
			Level: slog.Level(127), // Make temporary logger that does no logging.
		}))
	}
	println("program start4")
	dev := cyw43439.NewPicoWDevice()
	wificfg := cyw43439.DefaultWifiConfig()
	// cfg.Logger = logger // Uncomment to see in depth info on wifi device functioning.
	logger.Info("initializing pico W device...")
	devInitTime := time.Now()
	err := dev.Init(wificfg)
	if err != nil {
		return nil, errors.New("wifi init failed:" + err.Error())
	}
	logger.Info("cyw43439:Init", slog.Duration("duration", time.Since(devInitTime)))
	if len(pass) == 0 {
		logger.Info("joining open network:", slog.String("ssid", ssid))
	} else {
		logger.Info("joining WPA secure network", slog.String("ssid", ssid), slog.Int("passlen", len(pass)))
	}
	for {
		// Set ssid/pass in secrets.go
		err = dev.JoinWPA2(ssid, pass)
		if err == nil {
			break
		}
		logger.Error("wifi join faled", slog.String("err", err.Error()))
		time.Sleep(5 * time.Second)
	}

	engine, err := netif.NewEngine(dev, cfg)
	if err != nil {
		return nil, errors.New("netif.Engine creation failed:" + err.Error())
	}

	// Begin asynchronous packet handling.
	go engineLoop(engine, logger)

	return nil, nil
}

func engineLoop(engine *netif.Engine, logger *slog.Logger) {
	stalled := 0
	logEnabled := logger != nil && logger.Enabled(context.Background(), slog.LevelError)
	for {
		rx, tx, err := engine.HandlePoll()
		if err != nil && logEnabled {
			logger.LogAttrs(context.Background(), slog.LevelError, "engineloop:HandlePoll", slog.String("err", err.Error()))
		}
		if rx == 0 && tx == 0 {
			// Exponential backoff.
			stalled += 1
			sleep := time.Duration(1) << stalled
			if sleep > backoffMax {
				sleep = backoffMax
			}
			time.Sleep(sleep)
		} else {
			stalled = 0
		}
	}
}
