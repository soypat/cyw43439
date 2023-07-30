package tcpctl

import (
	"errors"
	"sync"

	"github.com/soypat/cyw43439/internal/tcpctl/eth"
)

// connState contains the state of a TCP connection likely to change throughout
// the connection's lifetime. This is so mutable state can be kept in one place
// and wrapped in a mutex for safe concurrent access.
type connState struct {
	mu sync.Mutex
	// # Send Sequence Space
	//
	//	1         2          3          4
	//	----------|----------|----------|----------
	//		   SND.UNA    SND.NXT    SND.UNA
	//								+SND.WND
	//	1. old sequence numbers which have been acknowledged
	//	2. sequence numbers of unacknowledged data
	//	3. sequence numbers allowed for new data transmission
	//	4. future sequence numbers which are not yet allowed
	snd sendSpace
	// # Receive Sequence Space
	//
	//		1          2          3
	//	----------|----------|----------
	//		   RCV.NXT    RCV.NXT
	//					 +RCV.WND
	//	1 - old sequence numbers which have been acknowledged
	//	2 - sequence numbers allowed for new reception
	//	3 - future sequence numbers which are not yet allowed
	rcv             rcvSpace
	pendingCtlFrame eth.TCPFlags
	state           State
}

// sendSpace contains Send Sequence Space data.
type sendSpace struct {
	iss uint32 // initial send sequence number, defined on our side on connection start
	UNA uint32 // send unacknowledged
	NXT uint32 // send next
	WL1 uint32 // segment sequence number used for last window update
	WL2 uint32 // segment acknowledgment number used for last window update
	WND uint16 // send window
	UP  bool   // send urgent pointer (deprecated)
}

// rcvSpace contains Receive Sequence Space data.
type rcvSpace struct {
	irs uint32 // initial receive sequence number, defined in SYN segment received
	NXT uint32 // receive next
	WND uint16 // receive window
	UP  bool   // receive urgent pointer (deprecated)

}

func (cs *connState) SetState(state State) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.state = state
}
func (cs *connState) State() State {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.state
}

func (cs *connState) frameRcv(hdr *eth.TCPHeader) (err error) {
	switch {
	case hdr.Ack <= cs.snd.UNA:
		err = errors.New("seg.ack > snd.UNA")
	case hdr.Ack > cs.snd.NXT:
		err = errors.New("seg.ack <= snd.NXT")
	}
	if err != nil {
		return err
	}
	return nil
}
