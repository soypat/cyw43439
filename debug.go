//go:build tinygo

package cyw43439

type debug uint8

const (
	debugBasic debug = 1 << iota // show fw version, mac addr, etc
	debugTxRx                    // show Tx/Rx I/O debug info
	debugSpi                     // show SPI debug info

	debugOff = 0
	debugAll = debugBasic | debugTxRx | debugSpi
)

func debugging(want debug) bool {
	return (_debug & want) != 0
}
