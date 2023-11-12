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
