//go:build !cy43firmware

package cyw43439

// This file is here to speedup the IDE
// when developing.

const (
	wifiFWLen   = 224190
	wifibtFWLen = 231077
	clmLen      = 984
	nvram43439  = ""
	nvram1dx    = ""
)

var (
	wifiFW   [225240]byte
	wifibtFW [6164]byte
	btFW     [232408]byte
)
