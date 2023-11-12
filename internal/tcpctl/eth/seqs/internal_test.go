package seqs

import (
	"fmt"
	"testing"
)

// Here we define internal testing helpers that may be used in any *_test.go file
// but are not exported.

func (tcb *ControlBlock) HelperInitState(state State, iss, nxt Value, localWindow Size) {
	tcb.state = state
	tcb.snd = sendSpace{
		ISS: iss,
		UNA: iss,
		NXT: nxt,
		WND: 1, // 1 byte window, so we can test the SEQ field.
		// UP, WL1, WL2 defaults to zero values.
	}
	tcb.rcv = recvSpace{
		WND: localWindow,
	}
}

func (tcb *ControlBlock) TestRelativeSendSpace() sendSpace {
	snd := tcb.snd
	snd.NXT -= snd.ISS
	snd.UNA -= snd.ISS
	snd.ISS = 0
	return snd
}

func (tcb *ControlBlock) TestRelativeRecvSpace() recvSpace {
	rcv := tcb.rcv
	rcv.NXT -= rcv.IRS
	rcv.IRS = 0
	return rcv
}

func (tcb *ControlBlock) RelativeRecvSegment(seg Segment) Segment {
	seg.SEQ -= tcb.rcv.IRS
	seg.ACK -= tcb.snd.ISS
	return seg
}

func (tcb *ControlBlock) RelativeSendSegment(seg Segment) Segment {
	seg.SEQ -= tcb.snd.ISS
	seg.ACK -= tcb.rcv.IRS
	return seg
}

func (tcb *ControlBlock) RelativeAutoSegment(seg Segment) Segment {
	rcv := tcb.RelativeRecvSegment(seg)
	snd := tcb.RelativeSendSegment(seg)
	if rcv.SEQ > snd.SEQ {
		return snd
	}
	return rcv
}

func (tcb *ControlBlock) HelperPrintSegment(t *testing.T, isReceive bool, seg Segment) {
	const fmtmsg = " Seg=%+v\nRcvSpace=%s\nSndSpace=%s"
	rcv := tcb.TestRelativeRecvSpace()
	rcvStr := rcv.RelativeGoString()
	snd := tcb.TestRelativeSendSpace()
	sndStr := snd.RelativeGoString()
	t.Helper()
	if isReceive {
		t.Logf("RECV:"+fmtmsg, seg.RelativeGoString(tcb.rcv.IRS, tcb.snd.ISS), rcvStr, sndStr)
	} else {
		t.Logf("SEND:"+fmtmsg, seg.RelativeGoString(tcb.snd.ISS, tcb.rcv.IRS), rcvStr, sndStr)
	}
}

func (rcv *recvSpace) RelativeGoString() string {
	return fmt.Sprintf("{NXT:%d} ", rcv.NXT-rcv.IRS)
}

func (rcv *sendSpace) RelativeGoString() string {
	return fmt.Sprintf("{NXT:%d UNA:%d} ", rcv.NXT-rcv.ISS, rcv.UNA-rcv.ISS)
}

func (seg Segment) RelativeGoString(iseq, iack Value) string {
	return fmt.Sprintf("{SEQ:%d ACK:%d DATALEN:%d Flags:%s} ", seg.SEQ-iseq, seg.ACK-iack, seg.DATALEN, seg.Flags)
}
