package seqs

import (
	"errors"
	"fmt"
	"io"
	"math"
	"time"
)

// ControlBlock implements the Transmission Control Block (TCB) of a TCP connection as specified in RFC 793
// in page 19 and clarified further in page 25. It records the state of a TCP connection.
type ControlBlock struct {
	// # Send Sequence Space
	//
	// 'Send' sequence numbers correspond to local data being sent.
	//
	//	     1         2          3          4
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
	// 'Receive' sequence numbers correspond to remote data being received.
	//
	//		1          2          3
	//	----------|----------|----------
	//		   RCV.NXT    RCV.NXT
	//					 +RCV.WND
	//	1 - old sequence numbers which have been acknowledged
	//	2 - sequence numbers allowed for new reception
	//	3 - future sequence numbers which are not yet allowed
	rcv recvSpace
	// pending and state are modified by rcv* methods and Close method.
	pending Flags
	state   State
}

// sendSpace contains Send Sequence Space data. Its sequence numbers correspond to local data.
type sendSpace struct {
	ISS Value // initial send sequence number, defined locally on connection start
	UNA Value // send unacknowledged. Seqs equal to UNA and above have NOT been acked by remote. Corresponds to local data.
	NXT Value // send next. This seq and up to UNA+WND-1 are allowed to be sent. Corresponds to local data.
	// WL1 Value // segment sequence number used for last window update
	// WL2 Value // segment acknowledgment number used for last window update
	WND Size // send window defined by remote. Permitted number unacked octets in flight.
}

// recvSpace contains Receive Sequence Space data. Its sequence numbers correspond to remote data.
type recvSpace struct {
	IRS Value // initial receive sequence number, defined by remote in SYN segment received.
	NXT Value // receive next. seqs before this have been acked. this seq and up to NXT+WND-1 are allowed to be sent. Corresponds to remote data.
	WND Size  // receive window defined by local. Permitted number unacked octets in flight.
}

// Segment represents a TCP segment as the sequence number of the first octet and the length of the segment.
type Segment struct {
	SEQ     Value // sequence number of first octet of segment. If SYN is set it is the initial sequence number (ISN) and the first data octet is ISN+1.
	ACK     Value // acknowledgment number. If ACK is set it is sequence number of first octet the sender of the segment is expecting to receive next.
	DATALEN Size  // The number of octets occupied by the data (payload) not counting SYN and FIN.
	WND     Size  // segment window
	Flags   Flags // TCP flags.
}

// LEN returns the length of the segment in octets including SYN and FIN flags.
func (seg *Segment) LEN() Size {
	add := Size(seg.Flags>>0) & 1 // Add FIN bit.
	add += Size(seg.Flags>>1) & 1 // Add SYN bit.
	return seg.DATALEN + add
}

// End returns the sequence number of the last octet of the segment.
func (seg *Segment) Last() Value {
	seglen := seg.LEN()
	if seglen == 0 {
		return seg.SEQ
	}
	return Add(seg.SEQ, seglen) - 1
}

// PendingSegment calculates a suitable next segment to send from a payload length.
func (tcb *ControlBlock) PendingSegment(payloadLen int) Segment {
	if payloadLen > math.MaxUint16 || Size(payloadLen) > tcb.snd.WND {
		payloadLen = int(tcb.snd.WND)
	}
	seg := Segment{
		SEQ:     tcb.snd.NXT,
		ACK:     tcb.rcv.NXT,
		WND:     tcb.rcv.WND,
		Flags:   tcb.pending,
		DATALEN: Size(payloadLen),
	}
	return seg
}

func (tcb *ControlBlock) Snd(seg Segment) error {
	err := tcb.validateOutgoingSegment(seg)
	if err != nil {
		return err
	}

	// The segment is valid, we can update TCB state.
	seglen := seg.LEN()
	tcb.snd.NXT.UpdateForward(seglen)
	tcb.rcv.WND = seg.WND
	return nil
}

func (tcb *ControlBlock) Rcv(seg Segment) (err error) {
	err = tcb.validateIncomingSegment(seg)
	if err != nil {
		return err
	}
	if seg.Flags.HasAny(FlagRST) {
		tcb.rst(seg.SEQ)
		return nil
	}
	prevNxt := tcb.snd.NXT
	switch tcb.state {
	case StateListen:
		err = tcb.rcvListen(seg)
	case StateSynSent:
		err = tcb.rcvSynSent(seg)
	case StateSynRcvd:
		err = tcb.rcvSynRcvd(seg)
	case StateEstablished:
		err = tcb.rcvEstablished(seg)
	default:
		err = errors.New("rcv: unexpected state " + tcb.state.String())
	}
	if err != nil {
		return err
	}
	if prevNxt != 0 && tcb.snd.NXT != prevNxt {
		// NXT modified in Snd() method.
		return fmt.Errorf("rcv %s: snd.nxt changed from %x to %x", tcb.state, prevNxt, tcb.snd.NXT)
	}

	// We accept the segment and update TCB state.
	tcb.snd.WND = seg.WND
	tcb.snd.UNA = seg.ACK
	seglen := seg.LEN()
	tcb.rcv.NXT.UpdateForward(seglen)
	return err
}

func (tcb *ControlBlock) rcvEstablished(seg Segment) (err error) {
	if seg.Flags.HasAny(FlagFIN) {
		// See diagram on page 23 of RFC 793.
		tcb.state = StateCloseWait
		tcb.pending = FlagACK
		return nil
	}
	return nil
}

func (tcb *ControlBlock) rcvListen(seg Segment) (err error) {
	switch {
	case !seg.Flags.HasAll(FlagSYN): //|| flags.HasAny(eth.FlagTCP_ACK):
		err = errors.New("rcvListen: no SYN or unexpected flag set")
	}
	if err != nil {
		return err
	}

	iss := tcb.newISS()
	// Initialize connection state:
	tcb.snd = sendSpace{
		ISS: iss,
		UNA: iss,
		NXT: iss,
		WND: seg.WND,
		// UP, WL1, WL2 defaults to zero values.
	}

	wnd := tcb.rcv.WND
	tcb.rcv = recvSpace{
		IRS: seg.SEQ,
		NXT: seg.SEQ, // +1 includes SYN flag as part of sequence octets is added outside.
		WND: wnd,
	}
	// We must respond with SYN|ACK frame after receiving SYN in listen state (three way handshake).
	tcb.pending = synack
	tcb.state = StateSynRcvd
	return nil
}

func (tcb *ControlBlock) rcvSynSent(seg Segment) (err error) {
	switch {
	case !seg.Flags.HasAll(synack):
		err = errors.New("rcvSynSent: expected SYN|ACK") // Not prepared for simultaneous intitialization yet.
	case seg.ACK != tcb.snd.UNA+1:
		err = errors.New("rcvSynSent: bad seg.ack")
	}
	if err != nil {
		return err
	}
	wnd := tcb.rcv.WND
	tcb.rcv = recvSpace{
		IRS: seg.SEQ,
		NXT: seg.SEQ, // +1 includes SYN flag as part of sequence octets.
		WND: wnd,
	}
	tcb.state = StateEstablished
	tcb.pending = FlagACK
	return nil
}

func (tcb *ControlBlock) rcvSynRcvd(seg Segment) (err error) {
	switch {
	case !seg.Flags.HasAll(FlagACK):
		err = errors.New("rcvSynRcvd: expected ACK")
	case seg.ACK != tcb.snd.UNA+1:
		err = errors.New("rcvSynRcvd: bad seg.ack")
	}
	if err != nil {
		return err
	}
	tcb.state = StateEstablished
	tcb.pending = FlagACK
	return nil
}

func (tcb *ControlBlock) rst(seq Value) {
	// cs.pending = FlagRST
	tcb.state = StateListen
}

func (tcb *ControlBlock) validateIncomingSegment(seg Segment) (err error) {
	const errPfx = "reject incoming seg: "
	flags := seg.Flags
	hasAck := flags.HasAll(FlagACK)
	// Short circuit SEQ checks if SYN present since the incoming segment initializes connection.
	checkSEQ := !flags.HasAny(FlagSYN)

	// See RFC 793 page 25 for more on these checks.
	switch {
	case seg.WND > math.MaxUint16:
		err = errors.New(errPfx + "wnd > 2**16")
	case tcb.state == StateClosed:
		err = io.ErrClosedPipe

	case hasAck && !LessThan(tcb.snd.UNA, seg.ACK):
		err = errors.New(errPfx + "ack points to old local data")

	case hasAck && !LessThanEq(seg.ACK, tcb.snd.NXT):
		err = errors.New(errPfx + "acks unsent data")

	case checkSEQ && !InWindow(seg.SEQ, tcb.rcv.NXT, tcb.rcv.WND):
		err = errors.New(errPfx + "seq not in receive window")

	case checkSEQ && !InWindow(seg.Last(), tcb.rcv.NXT, tcb.rcv.WND):
		err = errors.New(errPfx + "last not in receive window")
	}
	return err
}

func (tcb *ControlBlock) validateOutgoingSegment(seg Segment) (err error) {
	// hasAck := seg.Flags.HasAny(FlagACK)

	const errPfx = "invalid out segment: "
	seglast := seg.Last()
	switch {
	case tcb.state == StateClosed:
		err = io.ErrClosedPipe
	case seg.WND > math.MaxUint16:
		err = errors.New(errPfx + "wnd > 2**16")
	case seg.ACK != tcb.rcv.NXT:
		err = errors.New(errPfx + "ack != rcv.nxt")

	case !InWindow(seg.SEQ, tcb.snd.NXT, tcb.snd.WND):
		err = errors.New(errPfx + "seq not in send window")

	case !InWindow(seglast, tcb.snd.NXT, tcb.snd.WND):
		err = errors.New(errPfx + "last not in send window")
	}
	return err
}

func (tcb *ControlBlock) newISS() Value {
	return Value(time.Now().UnixMicro() / 4)
}

// Flags is a TCP flags masked implementation i.e: SYN, FIN, ACK.
type Flags uint16

const (
	FlagFIN Flags = 1 << iota // FlagFIN - No more data from sender.
	FlagSYN                   // FlagSYN - Synchronize sequence numbers.
	FlagRST                   // FlagRST - Reset the connection.
	FlagPSH                   // FlagPSH - Push function.
	FlagACK                   // FlagACK - Acknowledgment field significant.
	FlagURG                   // FlagURG - Urgent pointer field significant.
	FlagECE                   // FlagECE - ECN-Echo has a nonce-sum in the SYN/ACK.
	FlagCWR                   // FlagCWR - Congestion Window Reduced.
	FlagNS                    // FlagNS  - Nonce Sum flag (see RFC 3540).

	// The union of SYN and ACK flags is commonly found throughout the specification, so we define a shorthand.
	synack = FlagSYN | FlagACK
)

// HasAll checks if mask bits are all set in the receiver flags.
func (flags Flags) HasAll(mask Flags) bool { return flags&mask == mask }

// HasAny checks if one or more mask bits are set in receiver flags.
func (flags Flags) HasAny(mask Flags) bool { return flags&mask != 0 }

// StringFlags returns human readable flag string. i.e:
//
//	"[SYN,ACK]"
//
// Flags are printed in order from LSB (FIN) to MSB (NS).
// All flags are printed with length of 3, so a NS flag will
// end with a space i.e. [ACK,NS ]
func (flags Flags) String() string {
	if flags == 0 {
		return "[]"
	}
	// String Flag const
	const flaglen = 3
	var flagbuff [2 + (flaglen+1)*9]byte
	const strflags = "FINSYNRSTPSHACKURGECECWRNS "
	n := 0
	for i := 0; i*3 < len(strflags)-flaglen; i++ {
		if flags&(1<<i) != 0 {
			if n == 0 {
				flagbuff[0] = '['
				n++
			} else {
				flagbuff[n] = ','
				n++
			}
			copy(flagbuff[n:n+3], []byte(strflags[i*flaglen:i*flaglen+flaglen]))
			n += 3
		}
	}
	if n > 0 {
		flagbuff[n] = ']'
		n++
	}
	return string(flagbuff[:n])
}

// State enumerates states a TCP connection progresses through during its lifetime.
//
//go:generate stringer -type=State -trimprefix=State
type State uint8

const (
	// CLOSED - represents no connection state at all.
	StateClosed State = iota
	// LISTEN - represents waiting for a connection request from any remote TCP and port.
	StateListen
	// SYN-RECEIVED - represents waiting for a confirming connection request acknowledgment
	// after having both received and sent a connection request.
	StateSynRcvd
	// SYN-SENT - represents waiting for a matching connection request after having sent a connection request.
	StateSynSent
	// ESTABLISHED - represents an open connection, data received can be delivered
	// to the user.  The normal state for the data transfer phase of the connection.
	StateEstablished
	// FIN-WAIT-1 - represents waiting for a connection termination request
	// from the remote TCP, or an acknowledgment of the connection
	// termination request previously sent.
	StateFinWait1
	// FIN-WAIT-2 - represents waiting for a connection termination request
	// from the remote TCP.
	StateFinWait2
	// CLOSING - represents waiting for a connection termination request
	// acknowledgment from the remote TCP.
	StateClosing
	// TIME-WAIT - represents waiting for enough time to pass to be sure the remote
	// TCP received the acknowledgment of its connection termination request.
	StateTimeWait
	// CLOSE-WAIT - represents waiting for a connection termination request
	// from the local user.
	StateCloseWait
	// LAST-ACK - represents waiting for an acknowledgment of the
	// connection termination request previously sent to the remote TCP
	// (which includes an acknowledgment of its connection termination request).
	StateLastAck
)
