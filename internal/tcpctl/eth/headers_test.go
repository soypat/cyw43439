package eth

import (
	"encoding/binary"
	"testing"
)

func TestCRC791(t *testing.T) {
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
