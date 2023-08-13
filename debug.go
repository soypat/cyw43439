//go:build tinygo

package cyw43439

import (
	"context"
	"io"
	"machine"

	"github.com/soypat/cyw43439/internal/slog"
)

const (
	verbose_debug     = true
	initReadback      = false
	validateDownloads = false
	LevelDebugIO      = slog.LevelDebug
	defaultLevel      = LevelDebugIO
)

func (d *Device) debugIO(msg string, attrs ...slog.Attr) {
	d.log.LogAttrs(context.Background(), LevelDebugIO, msg, attrs...)
}

func (d *Device) debug(msg string, attrs ...slog.Attr) {
	d.log.LogAttrs(context.Background(), slog.LevelDebug, msg, attrs...)
}

func (d *Device) info(msg string, attrs ...slog.Attr) {
	d.log.LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

func (d *Device) logError(msg string, attrs ...slog.Attr) {
	d.log.LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
}

func (d *Device) SetLogger(log *slog.Logger) {
	d.log = log
}

func _setDefaultLogger(d *Device) {
	writer := machine.Serial
	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{Level: defaultLevel})
	// Small slog handler implemented on our side:
	// smallHandler := &handler{w: writer, level: slog.LevelDebug}
	d.log = slog.New(handler)
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
