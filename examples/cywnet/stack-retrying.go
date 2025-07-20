package cywnet

import (
	"errors"
	"net/netip"
	"time"
)

func (s *StackAsync) StackRetrying() StackRetrying {
	return StackRetrying{
		block: s.StackBlocking(),
	}
}

var (
	errRetriesExceeded = errors.New("cywnet: retries exceeded")
)

type StackRetrying struct {
	block StackBlocking
}

func (s StackRetrying) DoDHCPv4(reqAddr [4]byte, timeout time.Duration, retries int) (*DHCPResults, error) {
	for i := 0; i < retries; i++ {
		if i > 0 {
			println("Retrying DHCP")
		}
		results, err := s.block.DoDHCPv4(reqAddr, timeout)
		if err == nil {
			return results, err
		}
	}
	return nil, errRetriesExceeded
}

func (s StackRetrying) DoLookupIP(host string, timeout time.Duration, retries int) (addrs []netip.Addr, err error) {
	for i := 0; i < retries; i++ {
		results, err := s.block.DoLookupIP(host, timeout)
		if err == nil {
			return results, nil
		}
	}
	return nil, errRetriesExceeded
}

func (s StackRetrying) DoResolveHardwareAddress6(addr netip.Addr, timeout time.Duration, retries int) (hw [6]byte, err error) {
	for i := 0; i < retries; i++ {
		results, err := s.block.DoResolveHardwareAddress6(addr, timeout)
		if err == nil {
			return results, nil
		}
	}
	return hw, errRetriesExceeded
}
