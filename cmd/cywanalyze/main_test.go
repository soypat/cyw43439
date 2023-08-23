package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestInterpretBytes(t *testing.T) {
	bus := BusCtl{
		Order:           binary.LittleEndian,
		WordInterpreter: binary.BigEndian,
	}
	data := []byte{0x01, 0x02, 0x03, 0x04}
	bus.interpretBytes(data)
	if !bytes.Equal(data, []byte{0x04, 0x03, 0x02, 0x01}) {
		t.Error("expected big endian", data)
	}
	bus = BusCtl{
		Order:           binary.BigEndian,
		WordInterpreter: binary.LittleEndian,
	}
	data = []byte{0x01, 0x02, 0x03, 0x04}
	bus.interpretBytes(data)
	if !bytes.Equal(data, []byte{0x04, 0x03, 0x02, 0x01}) {
		t.Error("expected big endian", data)
	}
	bus = BusCtl{
		Order:           binary.LittleEndian,
		WordInterpreter: binary.LittleEndian,
	}
	data = []byte{0x01, 0x02, 0x03, 0x04}
	bus.interpretBytes(data)
	if !bytes.Equal(data, []byte{0x01, 0x02, 0x03, 0x04}) {
		t.Fatal("expected big endian", data)
	}
	bus = BusCtl{
		Order:           binary.BigEndian,
		WordInterpreter: binary.BigEndian,
	}
	data = []byte{0x01, 0x02, 0x03, 0x04}
	bus.interpretBytes(data)
	if !bytes.Equal(data, []byte{0x01, 0x02, 0x03, 0x04}) {
		t.Fatal("expected big endian", data)
	}
}
