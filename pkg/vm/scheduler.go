package vm

// ---------------------------------------------------------------------------
// Cooperative M:1 green-thread scheduler
//
// - Round-robin FIFO ready queue
// - 4096-instruction timeslice (configurable)
// - Deadlock detection: all live fibers blocked
// ---------------------------------------------------------------------------

const DefaultTimeslice = 4096

// Scheduler manages fibers in a cooperative M:1 green-thread model.
type Scheduler struct {
	readyQueue []*Fiber  // FIFO of runnable fibers
	allFibers  []*Fiber  // all live fibers (not dead)
	nextID     uint64    // monotonic fiber ID counter
	Timeslice  int       // instructions per fiber slice
}

// NewScheduler creates a scheduler with the given timeslice.
func NewScheduler(timeslice int) *Scheduler {
	if timeslice <= 0 {
		timeslice = DefaultTimeslice
	}
	return &Scheduler{
		Timeslice: timeslice,
	}
}

// NewFiber creates and registers a new fiber.
func (s *Scheduler) NewFiber() *Fiber {
	f := NewFiber(s.nextID)
	s.nextID++
	s.allFibers = append(s.allFibers, f)
	return f
}

// Ready enqueues a fiber into the ready queue.
func (s *Scheduler) Ready(f *Fiber) {
	if f.State == FiberDead {
		return
	}
	f.State = FiberSuspended
	s.readyQueue = append(s.readyQueue, f)
}

// Next dequeues the next runnable fiber. Returns nil if the queue is empty.
func (s *Scheduler) Next() *Fiber {
	for len(s.readyQueue) > 0 {
		f := s.readyQueue[0]
		s.readyQueue = s.readyQueue[1:]
		if f.State == FiberDead {
			continue
		}
		f.State = FiberRunning
		return f
	}
	return nil
}

// MarkDead removes a fiber from the live set.
func (s *Scheduler) MarkDead(f *Fiber) {
	f.State = FiberDead
	for i, af := range s.allFibers {
		if af == f {
			s.allFibers[i] = s.allFibers[len(s.allFibers)-1]
			s.allFibers = s.allFibers[:len(s.allFibers)-1]
			return
		}
	}
}

// HasLiveFibers returns true if any fibers are not dead.
func (s *Scheduler) HasLiveFibers() bool {
	return len(s.allFibers) > 0
}

// HasReadyFibers returns true if there are fibers in the ready queue.
func (s *Scheduler) HasReadyFibers() bool {
	return len(s.readyQueue) > 0
}

// DeadlockDetected returns true when all live fibers are blocked
// (no fibers in ready queue but live fibers exist).
func (s *Scheduler) DeadlockDetected() bool {
	if len(s.allFibers) == 0 {
		return false
	}
	// If there are fibers in the ready queue, no deadlock.
	for _, f := range s.readyQueue {
		if f.State != FiberDead {
			return false
		}
	}
	// Check if all live fibers are blocked.
	for _, f := range s.allFibers {
		if f.State != FiberBlocked {
			return false
		}
	}
	return true
}

// LiveCount returns the number of live (non-dead) fibers.
func (s *Scheduler) LiveCount() int {
	return len(s.allFibers)
}

// BlockedCount returns the number of blocked fibers.
func (s *Scheduler) BlockedCount() int {
	n := 0
	for _, f := range s.allFibers {
		if f.State == FiberBlocked {
			n++
		}
	}
	return n
}
