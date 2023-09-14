package main

import (
	"encoding/hex"
	"errors"

	"github.com/soypat/cyw43439/internal/tcpctl/eth"
)

var (
	errNotTCP     = errors.New("packet not TCP")
	errNotIPv4    = errors.New("packet not IPv4")
	errPacketSmol = errors.New("packet too small")
)

func rx(pkt []byte) error {
	if len(pkt) < 14 {
		return errPacketSmol
	}
	ethHdr := eth.DecodeEthernetHeader(pkt)
	if ethHdr.AssertType() != eth.EtherTypeIPv4 {
		return errNotIPv4
	}
	ipHdr := eth.DecodeIPv4Header(pkt[eth.SizeEthernetHeader:])
	println("ETH:", ethHdr.String())
	println("IPv4:", ipHdr.String())
	println("Rx:", len(pkt))
	println(hex.Dump(pkt))
	if ipHdr.Protocol == 17 {
		// We got an UDP packet and we validate it.
		udpHdr := eth.DecodeUDPHeader(pkt[eth.SizeEthernetHeader+eth.SizeIPv4Header:])
		gotChecksum := udpHdr.CalculateChecksumIPv4(&ipHdr, pkt[eth.SizeEthernetHeader+eth.SizeIPv4Header+eth.SizeUDPHeader:])
		println("UDP:", udpHdr.String())
		if gotChecksum == 0 || gotChecksum == udpHdr.Checksum {
			println("checksum match!")
		} else {
			println("checksum mismatch! Received ", udpHdr.Checksum, " but calculated ", gotChecksum)
		}
		return nil
	}
	if ipHdr.Protocol != 6 {
		return errNotTCP
	}
	tcpHdr := eth.DecodeTCPHeader(pkt[eth.SizeEthernetHeader+eth.SizeIPv4Header:])
	println("TCP:", tcpHdr.String())

	return nil
}
