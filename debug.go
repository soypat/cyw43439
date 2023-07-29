//go:build tinygo

package cyw43439

import (
	"context"
	"io"

	"golang.org/x/exp/slog"
)

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

var _ slog.Handler = (*handler)(nil)

// handler implements slog.Handler interface minimally.
type handler struct {
	level slog.Level
	w     io.Writer
	buf   [256]byte
}

func (h *handler) Enabled(_ context.Context, level slog.Level) bool { return level >= h.level }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	n := copy(h.buf[:len(h.buf)-2], r.Message)
	h.buf[n] = '\n'
	_, err := h.w.Write(h.buf[:n+1])
	return err
}
func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return nil
}
func (h *handler) WithGroup(name string) slog.Handler {
	return nil
}
