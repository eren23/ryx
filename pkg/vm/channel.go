package vm

// ---------------------------------------------------------------------------
// CSP channel operations
//
// Semantics:
//   - Unbuffered (cap=0): send blocks until a receiver is ready (rendezvous).
//   - Buffered (cap>0): send blocks when buffer is full; recv blocks when empty.
//   - Close: marks the channel as closed.
//     • Send on closed channel → runtime error.
//     • Recv on closed channel → returns buffered values first, then zero+false.
// ---------------------------------------------------------------------------

// ChannelSendResult describes the outcome of a send operation.
type ChannelSendResult int

const (
	SendOK      ChannelSendResult = iota // value was delivered or buffered
	SendBlocked                          // sender must block
	SendClosed                           // channel is closed → error
)

// ChannelRecvResult describes the outcome of a receive operation.
type ChannelRecvResult int

const (
	RecvOK      ChannelRecvResult = iota // value was received
	RecvBlocked                          // receiver must block
	RecvClosed                           // channel closed and drained
)

// ChannelTrySend attempts a non-blocking send.
// If a receiver is waiting, it does a direct hand-off (rendezvous).
// If there is buffer space, the value is buffered.
// Otherwise the caller must block.
func ChannelTrySend(ch *ChannelObj, val Value, sender *Fiber, sched *Scheduler) ChannelSendResult {
	if ch.Closed {
		return SendClosed
	}

	// If a receiver is waiting, hand off directly.
	if len(ch.RecvQ) > 0 {
		receiver := ch.RecvQ[0]
		ch.RecvQ = ch.RecvQ[1:]
		// Push the value onto the receiver's stack as the recv result.
		receiver.Push(val)
		receiver.State = FiberSuspended
		receiver.BlockedOn = nil
		sched.Ready(receiver)
		return SendOK
	}

	// If there is buffer space, enqueue.
	if len(ch.Buffer) < ch.Cap {
		ch.Buffer = append(ch.Buffer, val)
		return SendOK
	}

	// Must block.
	sender.State = FiberBlocked
	sender.BlockedOn = ch
	ch.SendQ = append(ch.SendQ, sender)
	ch.SendVals = append(ch.SendVals, val)
	return SendBlocked
}

// ChannelTryRecv attempts a non-blocking receive.
// If the buffer has data, it pops from the front and wakes a blocked sender if any.
// If a sender is waiting (unbuffered), it does a direct hand-off.
// Otherwise the caller must block.
func ChannelTryRecv(ch *ChannelObj, receiver *Fiber, sched *Scheduler) (Value, ChannelRecvResult) {
	// If buffer has data, take from front.
	if len(ch.Buffer) > 0 {
		val := ch.Buffer[0]
		ch.Buffer = ch.Buffer[1:]

		// If a sender was waiting, move its value into the buffer and unblock it.
		if len(ch.SendQ) > 0 {
			sender := ch.SendQ[0]
			sval := ch.SendVals[0]
			ch.SendQ = ch.SendQ[1:]
			ch.SendVals = ch.SendVals[1:]
			ch.Buffer = append(ch.Buffer, sval)
			sender.State = FiberSuspended
			sender.BlockedOn = nil
			sched.Ready(sender)
		}
		return val, RecvOK
	}

	// No buffered data. If a sender is waiting (unbuffered rendezvous), take directly.
	if len(ch.SendQ) > 0 {
		sender := ch.SendQ[0]
		sval := ch.SendVals[0]
		ch.SendQ = ch.SendQ[1:]
		ch.SendVals = ch.SendVals[1:]
		sender.State = FiberSuspended
		sender.BlockedOn = nil
		sched.Ready(sender)
		return sval, RecvOK
	}

	// Channel is closed and fully drained.
	if ch.Closed {
		return UnitVal(), RecvClosed
	}

	// Must block.
	receiver.State = FiberBlocked
	receiver.BlockedOn = ch
	ch.RecvQ = append(ch.RecvQ, receiver)
	return UnitVal(), RecvBlocked
}

// ChannelClose closes the channel and wakes all blocked receivers with zero values.
// Blocked senders receive an error through the scheduler (the VM handles this).
func ChannelClose(ch *ChannelObj, sched *Scheduler) {
	ch.Closed = true

	// Wake all blocked receivers with unit values.
	for _, r := range ch.RecvQ {
		r.Push(UnitVal())
		r.State = FiberSuspended
		r.BlockedOn = nil
		sched.Ready(r)
	}
	ch.RecvQ = nil

	// Wake all blocked senders (they will see the closed channel on retry).
	for _, s := range ch.SendQ {
		s.State = FiberSuspended
		s.BlockedOn = nil
		sched.Ready(s)
	}
	ch.SendQ = nil
	ch.SendVals = nil
}
