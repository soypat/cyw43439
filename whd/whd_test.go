package whd

import (
	"encoding/binary"
	"testing"
)

func TestParseAsyncEvent(t *testing.T) {
	var buf [48]byte
	for i := range buf {
		buf[i] = byte(i)
	}
	ev, err := ParseAsyncEvent(binary.LittleEndian, buf[:])
	if err != nil {
		t.Fatal(err)
	}
	if ev.Flags != 515 {
		t.Error("bad flags")
	}
	if ev.EventType != 67438087 {
		t.Error("bad event type")
	}
	if ev.Status != 134810123 {
		t.Error("bad status")
	}
	if ev.Reason != 202182159 {
		t.Error("bad reason")
	}
}
