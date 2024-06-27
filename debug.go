package cyw43439

import (
	"context"
	"encoding/hex"
	"runtime"

	"log/slog"
)

const (
	// currentLevel decides which log levels are printed.
	// A higher currentLevel means less logs (less verbose).
	defaultLevel slog.Level = levelTrace + 1
	levelTrace   slog.Level = slog.LevelDebug - 2
	deviceLevel  slog.Level = slog.LevelDebug - 1

	// heapAllocDebugging enables heap allocation debugging. So Prints do not use the heap.
	heapAllocDebugging = false
)

func (d *Device) logerr(msg string, attrs ...slog.Attr) {
	d.logattrs(slog.LevelError, msg, attrs...)
}

func (d *Device) warn(msg string, attrs ...slog.Attr) {
	d.logattrs(slog.LevelWarn, msg, attrs...)
}

func (d *Device) info(msg string, attrs ...slog.Attr) {
	d.logattrs(slog.LevelInfo, msg, attrs...)
}

func (d *Device) debug(msg string, attrs ...slog.Attr) {
	d.logattrs(slog.LevelDebug, msg, attrs...)
}

func (d *Device) trace(msg string, attrs ...slog.Attr) {
	if d._traceenabled { // Special case for trace since so common. Might save a few nanoseconds.
		d.logattrs(levelTrace, msg, attrs...)
	}
}

func (d *Device) logenabled(level slog.Level) bool {
	if heapAllocDebugging {
		return true
	}
	return d.logger != nil && d.logger.Handler().Enabled(context.Background(), level)
}

func (d *Device) isTraceEnabled() bool {
	return d.logenabled(levelTrace)
}

var (
	memstats   runtime.MemStats
	lastAllocs uint64
)

func (d *Device) logattrs(level slog.Level, msg string, attrs ...slog.Attr) {
	if heapAllocDebugging {
		runtime.ReadMemStats(&memstats)
		if memstats.TotalAlloc != lastAllocs {
			print("[ALLOC] inc=", int64(memstats.TotalAlloc)-int64(lastAllocs))
			print(" tot=", memstats.TotalAlloc, " cyw43439")
			println()
		}
		if level == levelTrace {
			print("TRACE ")
		} else if level < slog.LevelDebug {
			print("CY43 ")
		} else {
			print(level.String(), " ")
		}
		print(msg)
		for _, a := range attrs {
			switch a.Value.Kind() {
			case slog.KindString:
				print(" ", a.Key, "=", a.Value.String())
			}
		}
		println()
		runtime.ReadMemStats(&memstats)
		if memstats.TotalAlloc != lastAllocs {
			lastAllocs = memstats.TotalAlloc
		}
		return
	}
	if d.logger != nil {
		d.logger.LogAttrs(context.Background(), level, msg, attrs...)
	}
}

// SetLogger sets the logger for the device. If nil logging is disabled.
func (d *Device) SetLogger(l *slog.Logger) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logger = l
}

func (d *Device) log_init() error {
	if d.logger == nil || !d.logger.Handler().Enabled(context.Background(), deviceLevel) {
		return nil
	}
	d.trace("log_init")
	const (
		ramBase           = 0
		ramSize           = 512 * 1024
		socram_srmem_size = 64 * 1024
	)
	const addr = ramBase + ramSize - 4 - socram_srmem_size
	sharedAddr, err := d.bp_read32(addr)
	if err != nil {
		return err
	}
	shared := make([]byte, 32)
	d.bp_read(sharedAddr, shared)
	caddr := _busOrder.Uint32(shared[20:])
	smem := decodeSharedMem(_busOrder, shared)
	d.log.addr = smem.console_addr + 8
	if d.isTraceEnabled() {
		d.trace("log addr",
			slog.String("shared", hex.EncodeToString(shared)),
			slog.String("shared[20:]", hex.EncodeToString(shared[20:])),
			slog.Uint64("sharedAddr", uint64(sharedAddr)),
			slog.Uint64("consoleAddr", uint64(d.log.addr)),
			slog.Uint64("caddr", uint64(caddr)),
		)
	}
	return nil
}

// log_read reads the CY43439's internal logs and prints them to the structured logger
// under the CY43 level.
func (d *Device) log_read() error {
	if d.logger == nil || !d.logger.Handler().Enabled(context.Background(), deviceLevel) {
		return nil
	}
	d.trace("log_read")
	buf8 := u32AsU8(d._sendIoctlBuf[:])
	err := d.bp_read(d.log.addr, buf8[:16])
	if err != nil {
		return err
	}
	smem := decodeSharedMemLog(_busOrder, buf8[:16])
	idx := smem.idx
	if idx == d.log.last_idx {
		d.trace("CYLOG: no new data")
		return nil // Pointer not moved, nothing to do.
	}

	err = d.bp_read(smem.buf, buf8[:])
	if err != nil {
		return err
	}

	for d.log.last_idx != idx {
		b := buf8[d.log.last_idx]
		if b == '\r' || b == '\n' {
			if d.log.bufcount != 0 {
				d.logattrs(deviceLevel, string(d.log.buf[:d.log.bufcount]))
				d.log.bufcount = 0
			}
		} else if d.log.bufcount < uint32(len(d.log.buf)) {
			d.log.buf[d.log.bufcount] = b
			d.log.bufcount++
		}
		d.log.last_idx++
		if d.log.last_idx == 0x400 {
			d.log.last_idx = 0
		}
	}
	return nil
}

func hex32(u uint32) string {
	return hex.EncodeToString([]byte{byte(u >> 24), byte(u >> 16), byte(u >> 8), byte(u)})
}
