package seqs

// Here we define internal testing helpers that may be used in any *_test.go file
// but are not exported.

func (tcb *CtlBlock) TestInitState(state State, iss Value, localWindow Size) {
	tcb.state = state
	tcb.snd = sendSpace{
		ISS: iss,
		UNA: iss,
		NXT: iss + 1,
		// UP, WL1, WL2 defaults to zero values.
	}
	tcb.rcv = rcvSpace{
		WND: localWindow,
	}
}
