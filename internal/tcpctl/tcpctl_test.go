package tcpctl

import (
	"fmt"
	"testing"

	"github.com/soypat/cyw43439/internal/tcpctl/eth"
)

func TestTCPControl_helloWorldFull(t *testing.T) {
	// Full client-server interaction in the sending of "hello world" over TCP in order.
	const (
		cSYN1    = "\x28\xcd\xc1\x05\x4d\xbb\xd8\x5e\xd3\x43\x03\xeb\x08\x00\x45\x00\x00\x3c\x71\xac\x40\x00\x40\x06\x44\x9b\xc0\xa8\x01\x93\xc0\xa8\x01\x91\x84\x96\x04\xd2\x5e\x72\x2b\x7d\x00\x00\x00\x00\xa0\x02\xfa\xf0\x27\x6d\x00\x00\x02\x04\x05\xb4\x04\x02\x08\x0a\x07\x8b\x86\x4a\x00\x00\x00\x00\x01\x03\x03\x07"
		sSYNACK  = "\xd8\x5e\xd3\x43\x03\xeb\x28\xcd\xc1\x05\x4d\xbb\x08\x00\x45\x00\x00\x34\x00\x00\x40\x00\x40\x06\xb6\x4f\xc0\xa8\x01\x91\xc0\xa8\x01\x93\x04\xd2\x84\x96\xbe\x6e\x4c\x0f\x5e\x72\x2b\x7e\x80\x12\x10\x00\xc0\xbb\x00\x00\x02\x04\x05\xb4\x03\x03\x00\x04\x02\x00\x00\x00"
		cACK1    = "\x28\xcd\xc1\x05\x4d\xbb\xd8\x5e\xd3\x43\x03\xeb\x08\x00\x45\x00\x00\x28\x71\xad\x40\x00\x40\x06\x44\xae\xc0\xa8\x01\x93\xc0\xa8\x01\x91\x84\x96\x04\xd2\x5e\x72\x2b\x7e\xbe\x6e\x4c\x10\x50\x10\x01\xf6\x0b\x92\x00\x00"
		cPSHACK0 = "\x28\xcd\xc1\x05\x4d\xbb\xd8\x5e\xd3\x43\x03\xeb\x08\x00\x45\x00\x00\x34\x71\xae\x40\x00\x40\x06\x44\xa1\xc0\xa8\x01\x93\xc0\xa8\x01\x91\x84\x96\x04\xd2\x5e\x72\x2b\x7e\xbe\x6e\x4c\x10\x50\x18\x01\xf6\x79\xa5\x00\x00\x68\x65\x6c\x6c\x6f\x20\x77\x6f\x72\x6c\x64\x0a"
		sACK1    = "\xd8\x5e\xd3\x43\x03\xeb\x28\xcd\xc1\x05\x4d\xbb\x08\x00\x45\x00\x00\x28\x00\x00\x40\x00\x40\x06\xb6\x5b\xc0\xa8\x01\x91\xc0\xa8\x01\x93\x04\xd2\x84\x96\xbe\x6e\x4c\x10\x5e\x72\x2b\x8a\x50\x10\x0f\xf4\xfd\x87\x00\x00\x00\x00\x00\x00\x00\x00"
		sPSHACK1 = "\xd8\x5e\xd3\x43\x03\xeb\x28\xcd\xc1\x05\x4d\xbb\x08\x00\x45\x00\x00\x34\x00\x00\x40\x00\x40\x06\xb6\x4f\xc0\xa8\x01\x91\xc0\xa8\x01\x93\x04\xd2\x84\x96\xbe\x6e\x4c\x10\x5e\x72\x2b\x8a\x50\x18\x10\x00\x6b\x8f\x00\x00\x68\x65\x6c\x6c\x6f\x20\x77\x6f\x72\x6c\x64\x0a"
		cACK2    = "\x28\xcd\xc1\x05\x4d\xbb\xd8\x5e\xd3\x43\x03\xeb\x08\x00\x45\x00\x00\x28\x71\xaf\x40\x00\x40\x06\x44\xac\xc0\xa8\x01\x93\xc0\xa8\x01\x91\x84\x96\x04\xd2\x5e\x72\x2b\x8a\xbe\x6e\x4c\x1c\x50\x10\x01\xf6\x0b\x7a\x00\x00"
		cPSHACK1 = "\x28\xcd\xc1\x05\x4d\xbb\xd8\x5e\xd3\x43\x03\xeb\x08\x00\x45\x00\x00\x34\x71\xb0\x40\x00\x40\x06\x44\x9f\xc0\xa8\x01\x93\xc0\xa8\x01\x91\x84\x96\x04\xd2\x5e\x72\x2b\x8a\xbe\x6e\x4c\x1c\x50\x18\x01\xf6\x79\x8d\x00\x00\x68\x65\x6c\x6c\x6f\x20\x77\x6f\x72\x6c\x64\x0a"
		sPSHACK2 = "\xd8\x5e\xd3\x43\x03\xeb\x28\xcd\xc1\x05\x4d\xbb\x08\x00\x45\x00\x00\x34\x00\x00\x40\x00\x40\x06\xb6\x4f\xc0\xa8\x01\x91\xc0\xa8\x01\x93\x04\xd2\x84\x96\xbe\x6e\x4c\x1c\x5e\x72\x2b\x96\x50\x18\x10\x00\x6b\x77\x00\x00\x68\x65\x6c\x6c\x6f\x20\x77\x6f\x72\x6c\x64\x0a"
		cACK3    = "\x28\xcd\xc1\x05\x4d\xbb\xd8\x5e\xd3\x43\x03\xeb\x08\x00\x45\x00\x00\x28\x71\xb1\x40\x00\x40\x06\x44\xaa\xc0\xa8\x01\x93\xc0\xa8\x01\x91\x84\x96\x04\xd2\x5e\x72\x2b\x96\xbe\x6e\x4c\x28\x50\x10\x01\xf6\x0b\x62\x00\x00"
		cFINACK  = "\x28\xcd\xc1\x05\x4d\xbb\xd8\x5e\xd3\x43\x03\xeb\x08\x00\x45\x00\x00\x28\x71\xb2\x40\x00\x40\x06\x44\xa9\xc0\xa8\x01\x93\xc0\xa8\x01\x91\x84\x96\x04\xd2\x5e\x72\x2b\x96\xbe\x6e\x4c\x28\x50\x11\x01\xf6\x0b\x61\x00\x00"
		sACK     = "\xd8\x5e\xd3\x43\x03\xeb\x28\xcd\xc1\x05\x4d\xbb\x08\x00\x45\x00\x00\x28\x00\x00\x40\x00\x40\x06\xb6\x5b\xc0\xa8\x01\x91\xc0\xa8\x01\x93\x04\xd2\x84\x96\xbe\x6e\x4c\x28\x5e\x72\x2b\x97\x50\x10\x10\x00\xfd\x56\x00\x00\x00\x00\x00\x00\x00\x00"
	)
	p, err := decodeTCPPacket([]byte(cACK1))
	if err != nil {
		t.Fatal(err)
	}

	// payload := p.Payload()
	// t.Error("payload", string(payload))
	// t.Error("tcp options", p.TCPOptions())
	// t.Error("ip options", p.IPOptions())
	svStack := NewStack(StackConfig{
		MAC:         p.Eth.Destination[:],
		IP:          p.IP.Destination[:],
		MaxTCPConns: 1,
	})
	err = svStack.RecvEth([]byte(cACK1))
	if err != nil {
		t.Fatal(err)
	}
	// t.Error(p.String())
	_ = p
}

func decodeTCPPacket(packet []byte) (p TCPPacket, _ error) {
	if len(packet) < eth.SizeEthernetHeader+eth.SizeIPv4Header+eth.SizeTCPHeader {
		return p, fmt.Errorf("too short")
	}
	p.Rx = p.Rx.Add(99)
	p.Eth = eth.DecodeEthernetHeader(packet)
	if p.Eth.AssertType() != eth.EtherTypeIPv4 {
		return p, fmt.Errorf("not IPv4")
	}
	p.IP = eth.DecodeIPv4Header(packet[eth.SizeEthernetHeader:])
	if p.IP.Protocol != 6 {
		return p, fmt.Errorf("not TCP")
	}
	ihlBytes := 4 * p.IP.IHL()
	ipHdrEnd := eth.SizeEthernetHeader + ihlBytes
	if len(packet) < int(ipHdrEnd)+eth.SizeTCPHeader {
		return p, fmt.Errorf("too short")
	}
	p.TCP = eth.DecodeTCPHeader(packet[ipHdrEnd:])
	tcpHdrEnd := ipHdrEnd + p.TCP.OffsetInBytes()
	if len(packet) < int(ipHdrEnd) {
		return p, fmt.Errorf("too short")
	}
	ipOptions := packet[eth.SizeEthernetHeader+eth.SizeIPv4Header : ipHdrEnd]
	tcpOptions := packet[ipHdrEnd+eth.SizeTCPHeader : tcpHdrEnd]
	n := copy(p.data[:], ipOptions)
	n += copy(p.data[n:], tcpOptions)
	copy(p.data[n:], packet[tcpHdrEnd:])
	return p, nil
}
