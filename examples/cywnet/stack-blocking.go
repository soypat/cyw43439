package cywnet

import (
	"errors"
	"net/netip"
	"time"

	"github.com/soypat/lneto/dhcpv4"
	"github.com/soypat/lneto/tcp"
)

const (
	maxIter    = 1000
	maxTimeout = time.Minute
	maxSleep   = maxTimeout / maxIter
)

var (
	errDeadlineExceed = errors.New("cywnet: deadline exceeded")
)

func (s *StackAsync) StackBlocking() StackBlocking {
	return StackBlocking{
		async: s,
	}
}

type StackBlocking struct {
	async *StackAsync
}

func (s StackBlocking) DoDHCPv4(reqAddr [4]byte, timeout time.Duration) (*DHCPResults, error) {
	err := s.async.StartDHCPv4Request(reqAddr)
	if err != nil {
		return nil, err
	}
	sleep := timeout/maxIter + 1
	deadline := time.Now().Add(timeout)
	requested := false
	for i := 0; i < maxIter; i++ {
		state := s.async.dhcp.State()
		requested = requested || state > dhcpv4.StateInit
		if requested && state == dhcpv4.StateInit {
			return nil, errors.New("DHCP NACK")
		} else if state == dhcpv4.StateBound {
			break // DHCP done succesfully.
		} else if err = s.checkDeadline(deadline); err != nil {
			return nil, err
		}
		time.Sleep(sleep)
	}
	return s.async.ResultDHCP()
}

func (s StackBlocking) DoResolveHardwareAddress6(addr netip.Addr, timeout time.Duration) (hw [6]byte, err error) {
	err = s.async.StartResolveHardwareAddress6(addr)
	if err != nil {
		return hw, err
	}
	sleep := timeout/maxIter + 1
	deadline := time.Now().Add(timeout)
	for i := 0; i < maxIter; i++ {
		hw, err = s.async.ResultResolveHardwareAddress6(addr)
		if err == nil {
			break
		} else if err = s.checkDeadline(deadline); err != nil {
			return hw, err
		}
		time.Sleep(sleep)
		err = errDeadlineExceed // Ensure that if iterations done error is returned.
	}
	return hw, err
}

func (s StackBlocking) DoLookupIP(host string, timeout time.Duration) (addrs []netip.Addr, err error) {
	err = s.async.StartLookupIP(host)
	if err != nil {
		return nil, err
	}
	sleep := timeout/maxIter + 1
	deadline := time.Now().Add(timeout)
	for i := 0; i < maxIter; i++ {
		addrs, completed, err := s.async.ResultLookupIP(host)
		if completed {
			return addrs, err
		} else if err = s.checkDeadline(deadline); err != nil {
			return nil, err
		}
		time.Sleep(sleep)
	}
	return nil, errDeadlineExceed
}

var errTCPFailedToConnect = errors.New("tcp failed to connect")

func (s StackBlocking) DoDialTCP(localPort uint16, addrp netip.AddrPort, timeout time.Duration) (conn *tcp.Conn, err error) {
	conn, err = s.async.DialTCP(localPort, addrp)
	if err != nil {
		return nil, err
	}
	sleep := timeout/maxIter + 1
	deadline := time.Now().Add(timeout)
	for i := 0; i < maxIter; i++ {
		state := conn.State()
		switch state {
		case tcp.StateEstablished:
			break
		case tcp.StateSynSent, tcp.StateSynRcvd:
			if err = s.checkDeadline(deadline); err != nil {
				return nil, err
			}
			time.Sleep(sleep)
		default:
			// Unexpected state, abort and terminate connection.
			conn.Abort()
			return nil, errTCPFailedToConnect
		}
	}
	return conn, nil
}

func (s StackBlocking) checkDeadline(deadline time.Time) error {
	if time.Since(deadline) > 0 {
		return errDeadlineExceed
	}
	return nil
}
