package eth

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"
)

func TestUDPChecksum(t *testing.T) {
	var testUDPPacket = []byte{
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x78, 0x44, 0x76, 0xc4, 0x8d, 0xb0, 0x08, 0x00, 0x45, 0x00, // |......xDv.....E.|
		0x00, 0xa2, 0x4a, 0xb0, 0x00, 0x00, 0x80, 0x11, 0x6c, 0xdc, 0xc0, 0xa8, 0x00, 0x6f, 0xc0, 0xa8, // |..J.....l....o..|
		0x00, 0xff, 0x44, 0x5c, 0x44, 0x5c, 0x00, 0x8e, 0x27, 0x8f, 0x7b, 0x22, 0x76, 0x65, 0x72, 0x73, // |..D\D\..'.{"vers|
		0x69, 0x6f, 0x6e, 0x22, 0x3a, 0x20, 0x5b, 0x32, 0x2c, 0x20, 0x30, 0x5d, 0x2c, 0x20, 0x22, 0x70, // |ion": [2, 0], "p|
		0x6f, 0x72, 0x74, 0x22, 0x3a, 0x20, 0x31, 0x37, 0x35, 0x30, 0x30, 0x2c, 0x20, 0x22, 0x68, 0x6f, // |ort": 17500, "ho|
		0x73, 0x74, 0x5f, 0x69, 0x6e, 0x74, 0x22, 0x3a, 0x20, 0x31, 0x38, 0x31, 0x32, 0x36, 0x35, 0x36, // |st_int": 1812656|
		0x30, 0x39, 0x32, 0x35, 0x37, 0x34, 0x32, 0x31, 0x30, 0x34, 0x36, 0x37, 0x33, 0x33, 0x36, 0x32, // |0925742104673362|
		0x36, 0x31, 0x37, 0x33, 0x32, 0x31, 0x37, 0x30, 0x35, 0x37, 0x36, 0x33, 0x34, 0x37, 0x39, 0x33, // |6173217057634793|
		0x2c, 0x20, 0x22, 0x64, 0x69, 0x73, 0x70, 0x6c, 0x61, 0x79, 0x6e, 0x61, 0x6d, 0x65, 0x22, 0x3a, // |, "displayname":|
		0x20, 0x22, 0x22, 0x2c, 0x20, 0x22, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65, 0x73, // | "", "namespaces|
		0x22, 0x3a, 0x20, 0x5b, 0x38, 0x31, 0x35, 0x32, 0x34, 0x36, 0x32, 0x30, 0x30, 0x30, 0x5d, 0x7d, // |": [8152462000]}|
	}
	// Process Ethernet header.
	ethData := testUDPPacket[:14]
	e := DecodeEthernetHeader(ethData)
	if !bytes.Equal(e.Destination[:], ethData[0:6]) {
		t.Errorf("incorrect ethernet destination: %v", e.String())
	}
	if !bytes.Equal(e.Source[:], ethData[6:12]) {
		t.Errorf("incorrect ethernet source: %v", e.String())
	}
	if e.AssertType() != EtherTypeIPv4 {
		t.Errorf("incorrect ethertype: %v", e.String())
	}
	ethDataGot := make([]byte, len(ethData))
	e.Put(ethDataGot)
	if !bytes.Equal(ethData, ethDataGot) {
		got := DecodeEthernetHeader(ethDataGot)
		t.Error("ethernet marshal does not match original data", e.String(), got.String())
	}
	// Process IP header.
	ipData := testUDPPacket[14:34]
	ip := DecodeIPv4Header(ipData)
	if ip.Protocol != 17 {
		t.Errorf("incorrect IP protocol: %v", ip.String())
	}
	if !bytes.Equal(ip.Source[:], testUDPPacket[26:30]) {
		t.Errorf("incorrect IP source: %v", ip.String())
	}
	if !bytes.Equal(ip.Destination[:], testUDPPacket[30:34]) {
		t.Errorf("incorrect IP destination: %v", ip.String())
	}
	ipDataGot := make([]byte, len(ipData))
	ip.Put(ipDataGot)
	if !bytes.Equal(ipData, ipDataGot) {
		got := DecodeIPv4Header(ipDataGot)
		t.Error("IP marshal does not match original data", ip.String(), got.String())
	}
	// Process UDP header.
	udpData := testUDPPacket[34 : 34+8]
	udp := DecodeUDPHeader(udpData)
	if udp.SourcePort != 17500 {
		t.Errorf("incorrect udp source port: %v", udp.String())
	}
	if udp.DestinationPort != 17500 {
		t.Errorf("incorrect udp destination port: %v", udp.String())
	}
	if udp.Length != 142 {
		t.Errorf("incorrect udp length: %v", udp.String())
	}
	udpDataGot := make([]byte, len(udpData))
	udp.Put(udpDataGot)
	if !bytes.Equal(udpData, udpDataGot) {
		got := DecodeUDPHeader(udpDataGot)
		t.Error("UDP marshal does not match original data", udp.String(), got.String())
	}
}

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
			} else if n == 1 {
				crc.AddUint8(dataDiv[0])
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
