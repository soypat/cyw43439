package seqs

import (
	"errors"
	"io"
	"math"
	"time"
)

// CtlBlock implements the Transmission Control Block (TCB) of a TCP connection as specified in RFC 793
// in page 19 and clarified further in page .
type CtlBlock struct {
	// # Send Sequence Space
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
	//		1          2          3
	//	----------|----------|----------
	//		   RCV.NXT    RCV.NXT
	//					 +RCV.WND
	//	1 - old sequence numbers which have been acknowledged
	//	2 - sequence numbers allowed for new reception
	//	3 - future sequence numbers which are not yet allowed
	rcv     rcvSpace
	pending Flags
	state   State
}

// sendSpace contains Send Sequence Space data.
type sendSpace struct {
	ISS Value // initial send sequence number, defined on our side on connection start
	UNA Value // send unacknowledged. Seqs equal to UNA and above have NOT been acked.
	NXT Value // send next. This seq and up to UNA+WND-1 are allowed to be sent.
	WL1 Value // segment sequence number used for last window update
	WL2 Value // segment acknowledgment number used for last window update
	WND Size  // send window defined by remote. Permitted number unacked octets in flight.
}

// rcvSpace contains Receive Sequence Space data.
type rcvSpace struct {
	IRS Value // initial receive sequence number, defined in SYN segment received
	NXT Value // receive next. seqs before this have been acked. this seq and up to NXT+WND-1 are allowed to be sent.
	WND Size  // receive window defined by local. Permitted number unacked octets in flight.
}

// Segment represents a TCP segment as the sequence number of the first octet and the length of the segment.
type Segment struct {
	SEQ     Value // first sequence number of a segment.
	ACK     Value // acknowledgment number.
	WND     Size  // segment window
	DATALEN Size  // The number of octets occupied by the data (payload) not counting SYN and FIN.
	Flags   Flags // TCP flags.
}

// PendingSegment takes the size of the data ready to send and returns
// the TCP segment data for the outgoing packet and the length of the data to send in DATALEN field.
func (ctl *CtlBlock) PendingSegment(payloadLen int) Segment {
	if payloadLen > math.MaxUint16 || Size(payloadLen) > ctl.snd.WND {
		payloadLen = int(ctl.snd.WND)
	}
	seg := Segment{
		SEQ:     ctl.rcv.NXT,
		ACK:     ctl.snd.NXT,
		WND:     ctl.snd.WND,
		Flags:   ctl.pending,
		DATALEN: Size(payloadLen),
	}
	return seg
}

// LEN returns the length of the segment in octets.
func (seg *Segment) LEN() Size {
	add := Size(seg.Flags>>0) & 1 // Add FIN bit.
	add += Size(seg.Flags>>1) & 1 // Add SYN bit.
	return seg.DATALEN + add
}

// End returns the sequence number of the last octet of the segment.
func (seg *Segment) Last() Value {
	return Add(seg.SEQ, seg.LEN()) - 1
}

func (ctl *CtlBlock) Rcv(seg Segment) (err error) {
	err = ctl.validateSegment(seg)
	if err != nil {
		return err
	}
	if seg.Flags.HasAny(FlagRST) {
		ctl.rst(seg.SEQ)
		return nil
	}

	switch ctl.state {
	case StateListen:
		err = ctl.rcvListen(seg)
	case StateSynSent:
		err = ctl.rcvSynSent(seg)
	case StateSynRcvd:
		err = ctl.rcvSynRcvd(seg)
	}
	return err
}

func (ctl *CtlBlock) rcvListen(seg Segment) (err error) {
	switch {
	case !seg.Flags.HasAll(FlagSYN): //|| flags.HasAny(eth.FlagTCP_ACK):
		err = errors.New("rcvListen: no SYN or unexpected flag set")
	}
	if err != nil {
		return err
	}

	iss := ctl.newISS()
	// Initialize connection state:
	ctl.snd = sendSpace{
		ISS: iss,
		UNA: iss,
		NXT: iss,
		WND: seg.WND,
		// UP, WL1, WL2 defaults to zero values.
	}
	ctl.rcv = rcvSpace{
		IRS: seg.SEQ,
		NXT: seg.SEQ + 1, // +1 includes SYN flag as part of sequence octets.
		WND: 10,
	}
	// We must respond with SYN|ACK frame after receiving SYN in listen state (three way handshake).
	ctl.pending = synack
	ctl.state = StateSynRcvd
	return nil
}

func (cs *CtlBlock) rcvSynSent(seg Segment) (err error) {
	switch {
	case !seg.Flags.HasAll(synack):
		err = errors.New("rcvSynSent: expected SYN|ACK") // Not prepared for simultaneous intitialization yet.
	case seg.ACK != cs.snd.UNA+1:
		err = errors.New("rcvSynSent: bad seg.ack")
	}
	if err != nil {
		return err
	}
	cs.snd.UNA = seg.ACK
	cs.state = StateEstablished
	cs.pending = FlagACK
	return nil
}

func (cs *CtlBlock) rcvSynRcvd(seg Segment) (err error) {
	switch {
	case !seg.Flags.HasAll(FlagACK):
		err = errors.New("rcvSynRcvd: expected ACK")
	case seg.ACK != cs.snd.UNA+1:
		err = errors.New("rcvSynRcvd: bad seg.ack")
	}
	if err != nil {
		return err
	}
	cs.snd.UNA = seg.ACK
	cs.state = StateEstablished
	cs.pending = FlagACK
	return nil
}

func (cs *CtlBlock) rst(seq Value) {
	// cs.pending = FlagRST
	cs.state = StateListen
}

func (cs *CtlBlock) validateSegment(seg Segment) (err error) {
	flags := seg.Flags
	hasAck := flags.HasAll(FlagACK)
	checkSEQ := flags.HasAny(FlagSYN)

	// First condition below short circuits if ACK not present to ignore ACK check.
	acceptableAck := !hasAck || LessThan(cs.snd.UNA, seg.ACK) && LessThanEq(seg.ACK, cs.snd.NXT) // RFC 793 page 25.

	segend := seg.Last()
	// First condition below short circuits if SYN present since the incoming segment initializes connection.
	// Second part of test checks to see if beginning of the segment falls in the window,
	// Third part of test checks to see if the end of the segment falls in the window.
	// If test passes then segment contains data in the window and we can accept full or partial data from it.
	validSeqSpace := checkSEQ || InWindow(seg.SEQ, cs.rcv.NXT, cs.rcv.WND) || InWindow(segend, cs.rcv.NXT, cs.rcv.WND) // RFC 793 page 25.
	switch {
	case seg.WND > math.MaxUint16:
		err = errors.New("reject seg: seg.wnd > 2**16")
	case cs.state == StateClosed:
		err = io.ErrClosedPipe
	case !acceptableAck:
		err = errors.New("reject ack: failed condition snd.una<seg.ack<=snd.nxt")
	case !validSeqSpace:
		err = errors.New("reject seq: segment does not occupy valid portion of receive sequence space")
	}
	return err
}

func (ctl *CtlBlock) newISS() Value {
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
