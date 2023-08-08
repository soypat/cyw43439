package eth

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"
)

func TestCRC791_oneshot(t *testing.T) {
	for _, data := range [][]byte{
		{0x23},
		{0x23, 0xfb},
		{0x23, 0xfb, 0xde},
		{0x23, 0xfb, 0xde, 0xad},
		{0x23, 0xfb, 0xde, 0xad, 0xde, 0xad, 0xc0, 0xff, 0xee},
		{0x23, 0xfb, 0xde, 0xad, 0xde, 0xad, 0xc0, 0xff, 0xee, 0x00},
	} {
		crc := CRC791{}
		crc.Write(data)
		got := crc.Sum16()
		expect := sum(data)
		if got != expect {
			t.Errorf("CRC791 mismatch (%d), got %#04x; expected %#04x", len(data), got, expect)
		}
	}
}

func TestCRC791_multifuzz(t *testing.T) {
	data := []byte("00\x0010")
	rng := rand.New(rand.NewSource(1))
	crc := CRC791{}
	dataDiv := data
	for len(dataDiv) > 0 {
		n := rng.Intn(len(dataDiv)) + 1
		crc.Write(dataDiv[:n])
		t.Logf("write: %q", dataDiv[:n])
		dataDiv = dataDiv[n:]
	}
	got := crc.Sum16()
	expect := sum(data)
	if got != expect {
		t.Errorf("crc mismatch, got %#04x; expected %#04x", got, expect)
		panic("CRC791 mismatch for data " + fmt.Sprintf("%q", data))
	}
}

func FuzzCRC(f *testing.F) {
	f.Add([]byte{0x23, 0xfb, 0xde, 0xad, 0xde, 0xad, 0xc0, 0xff, 0xee, 0x00})
	f.Fuzz(func(t *testing.T, data []byte) {
		rng := rand.New(rand.NewSource(1))
		crc := CRC791{}
		dataDiv := data
		for len(dataDiv) > 0 {
			n := rng.Intn(len(dataDiv)) + 1
			if n == 2 {
				crc.AddUint16(binary.BigEndian.Uint16(dataDiv[:n]))
			} else {
				crc.Write(dataDiv[:n])
			}
			dataDiv = dataDiv[n:]
		}
		got := crc.Sum16()
		expect := sum(data)
		if got != expect {
			panic("CRC791 mismatch for data " + fmt.Sprintf("%q", data))
		}
	})
}

// func TestCRC791_multi(t *testing.T) {
// 	rng := rand.New(rand.NewSource(1))
// 	for i := 0; i < 1000; i++ {
// 		// Make random Data.
// 		data := make([]byte, 100+rng.Intn(1000))
// 		for j := range data {
// 			data[j] = byte(rng.Intn(256))
// 		}
// 		expect := sum(data)
// 		crc := CRC791{}
// 		dataDiv := data
// 		for len(dataDiv) > 0 {
// 			n := rng.Intn(len(dataDiv)) + 1
// 			crc.Write(dataDiv[:n])
// 			dataDiv = dataDiv[n:]
// 		}
// 		got := crc.Sum16()
// 		if got != expect {
// 			t.Errorf("CRC791 mismatch (%d), got %#04x; expected %#04x", len(data), got, expect)
// 		}
// 	}
// }

// Checksum is the 16-bit one's complement of the one's complement sum of a
// pseudo header of information from the IP header, the UDP header, and the
// data,  padded  with zero octets  at the end (if  necessary)  to  make  a
// multiple of two octets.
//
// Inspired by: https://gist.github.com/david-hoze/0c7021434796997a4ca42d7731a7073a
func sum(b []byte) uint16 {
	var sum uint32
	count := len(b)
	for count > 1 {
		sum += uint32(binary.BigEndian.Uint16(b[len(b)-count:]))
		count -= 2
	}
	if count > 0 {
		// If any bytes left, pad the bytes and add.
		sum += uint32(b[len(b)-1]) << 8
	}
	// Fold sum to 16 bits: add carrier to result.
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return uint16(^sum) // One's complement.
}
