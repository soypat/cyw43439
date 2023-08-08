package eth

import (
	"encoding/binary"
)

// CRC791 function as defined by RFC 791. The Checksum field for TCP+IP
// is the 16-bit ones' complement of the ones' complement sum of
// all 16-bit words in the header. In case of uneven number of octet the
// last word is LSB padded with zeros.
//
// The zero value of CRC791 is ready to use.
type CRC791 struct {
	sum      uint32
	excedent uint8
}

// Write adds the bytes in p to the running checksum.
func (c *CRC791) Write(buff []byte) (n int, err error) {
	if len(buff) == 0 {
		return 0, nil
	}
	if c.excedent != 0 {
		// c.sum += uint32(binary.BigEndian.Uint16(buff))
		c.sum += uint32(c.excedent)<<8 + uint32(buff[0])
		buff = buff[1:]
		c.excedent = 0
	}
	count := len(buff)
	for count > 1 {
		c.sum += uint32(binary.BigEndian.Uint16(buff[len(buff)-count:]))
		count -= 2
	}
	if count != 0 && buff[len(buff)-1] != 0 {
		c.excedent = buff[len(buff)-1]
	}
	return len(buff), nil
}

// Sum16 calculates the checksum with the data written to c thus far.
func (c *CRC791) Sum16() uint16 {
	sum := c.sum
	if c.excedent != 0 {
		sum += uint32(c.excedent) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return uint16(^sum)
}

// Reset zeros out the CRC791, resetting it to the initial state.
func (c *CRC791) Reset() { *c = CRC791{} }
