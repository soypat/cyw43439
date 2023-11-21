package cyrw

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"unsafe"

	"log/slog"
)

const (
	enableDeviceLog = true

	// currentLevel decides which log levels are printed.
	// A higher currentLevel means less logs (less verbose).
	defaultLevel slog.Level = levelTrace + 1
	levelTrace   slog.Level = slog.LevelDebug - 1
	deviceLevel  slog.Level = slog.LevelError - 1
	// dblogattrs decides whether to print key-value log attributes.
	dblogattrs = true
	// print out raw bus transactions.
	printLowestLevelBusTransactions = false
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
	d.logattrs(slog.LevelDebug-1, msg, attrs...)
}

func (d *Device) logattrs(level slog.Level, msg string, attrs ...slog.Attr) {
	canPrintDeviceLog := enableDeviceLog && level == deviceLevel
	if level < d.level && !canPrintDeviceLog {
		return
	}

	print(_levelStr(level))
	print(" ")
	print(msg)
	if dblogattrs {
		for _, a := range attrs {
			print(" ")
			print(a.Key)
			print("=")
			if a.Value.Kind() == slog.KindAny {
				fmt.Printf("%+v", a.Value.Any())
			} else {
				print(a.Value.String())
			}
		}
	}
	println()
}
func _levelStr(level slog.Level) string {
	var levelStr string
	switch level {
	case deviceLevel:
		levelStr = "CY43"
	case levelTrace:
		levelStr = "TRACE"
	default:
		levelStr = level.String()
	}
	return levelStr
}

func (d *Device) log_init() error {
	if !enableDeviceLog {
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
	d.trace("log addr",
		slog.String("shared", hex.EncodeToString(shared)),
		slog.String("shared[20:]", hex.EncodeToString(shared[20:])),
		slog.Uint64("sharedAddr", uint64(sharedAddr)),
		slog.Uint64("consoleAddr", uint64(d.log.addr)),
		slog.Uint64("caddr", uint64(caddr)),
	)
	return nil
}

// log_read reads the CY43439's internal logs and prints them to the structured logger
// under the CY43 level.
func (d *Device) log_read() error {
	if !enableDeviceLog {
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

func printLowLevelTx(cmd uint32, data []uint32) {
	if !printLowestLevelBusTransactions {
		return
	}
	var buf [8]byte
	ptr := unsafe.Pointer(&buf[0])
	strhdr := reflect.StringHeader{
		Data: uintptr(ptr),
		Len:  uintptr(len(buf)),
	}
	h := *(*string)(unsafe.Pointer(&strhdr))
	const hextable = "0123456789abcdef"

	hex.Encode(buf[:], u32PtrTo4U8(&cmd)[:])
	print("tx: ")
	print(h)

	for _, d := range data {
		hex.Encode(buf[:], u32PtrTo4U8(&d)[:])
		print(" ")
		print(h)
	}
	println()
}

func hex32(u uint32) string {
	return hex.EncodeToString([]byte{byte(u >> 24), byte(u >> 16), byte(u >> 8), byte(u)})
}
