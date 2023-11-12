package seqs

// Here we define internal testing helpers that may be used in any *_test.go file
// but are not exported.

func (tcb *CtlBlock) TestInitState(state State, iss, nxt Value, localWindow Size) {
	tcb.state = state
	tcb.snd = sendSpace{
		ISS: iss,
		UNA: iss,
		NXT: nxt,
		WND: 1, // 1 byte window, so we can test the SEQ field.
		// UP, WL1, WL2 defaults to zero values.
	}
	tcb.rcv = rcvSpace{
		WND: localWindow,
	}
}

func (tcb *CtlBlock) RelativeRcvSegment(seg Segment) Segment {
	seg.SEQ -= tcb.rcv.IRS
	seg.ACK -= tcb.snd.ISS
	return seg
}

func (tcb *CtlBlock) RelativeSndSegment(seg Segment) Segment {
	seg.SEQ -= tcb.snd.ISS
	seg.ACK -= tcb.rcv.IRS
	return seg
}

func (tcb *CtlBlock) RelativeAutoSegment(seg Segment) Segment {
	rcv := tcb.RelativeRcvSegment(seg)
	snd := tcb.RelativeSndSegment(seg)
	if rcv.SEQ > snd.SEQ {
		return snd
	}
	return rcv
}
