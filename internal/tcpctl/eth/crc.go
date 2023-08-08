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
	needPad  bool
}

// Write adds the bytes in p to the running checksum.
func (c *CRC791) Write(buff []byte) (n int, err error) {
	if len(buff) == 0 {
		return 0, nil
	}
	if c.needPad {
		c.sum += uint32(c.excedent)<<8 + uint32(buff[0])
		buff = buff[1:]
		c.excedent = 0
		c.needPad = false
		if len(buff) == 0 {
			return 1, nil
		}
	}
	count := len(buff)
	for count > 1 {
		c.sum += uint32(binary.BigEndian.Uint16(buff[len(buff)-count:]))
		count -= 2
	}
	if count != 0 {
		c.excedent = buff[len(buff)-1]
		c.needPad = true
	}
	return len(buff), nil
}

// Add16 adds value to the running checksum interpreted as BigEndian (network order).
func (c *CRC791) AddUint16(value uint16) {
	if c.needPad {
		c.sum += uint32(c.excedent)<<8 | uint32(value>>8)
		c.excedent = byte(value)
	} else {
		c.sum += uint32(value)
	}
}

// Sum16 calculates the checksum with the data written to c thus far.
func (c *CRC791) Sum16() uint16 {
	sum := c.sum
	if c.needPad {
		sum += uint32(c.excedent) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return uint16(^sum)
}

// Reset zeros out the CRC791, resetting it to the initial state.
func (c *CRC791) Reset() { *c = CRC791{} }
