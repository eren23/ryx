package vm

// ---------------------------------------------------------------------------
// Fiber states
// ---------------------------------------------------------------------------

type FiberState byte

const (
	FiberRunning   FiberState = iota // actively executing
	FiberSuspended                   // timeslice expired, waiting in ready queue
	FiberBlocked                     // blocked on channel or other I/O
	FiberDead                        // finished or errored
)

func (s FiberState) String() string {
	switch s {
	case FiberRunning:
		return "running"
	case FiberSuspended:
		return "suspended"
	case FiberBlocked:
		return "blocked"
	case FiberDead:
		return "dead"
	default:
		return "unknown"
	}
}

// ---------------------------------------------------------------------------
// Call frame
// ---------------------------------------------------------------------------

const MaxCallDepth = 1024

// CallFrame represents one activation record on the call stack.
type CallFrame struct {
	FuncIdx uint32      // index into CompiledProgram.Functions
	IP      int         // absolute offset into Code
	BP      int         // base pointer: index into Stack where locals start
	Closure *ClosureObj // non-nil when executing a closure
}

// ---------------------------------------------------------------------------
// Fiber
// ---------------------------------------------------------------------------

const MaxStackSize = 65536

// Fiber is a lightweight green thread with its own call stack and operand stack.
type Fiber struct {
	ID         uint64
	Frames     [MaxCallDepth]CallFrame
	FrameCount int

	Stack [MaxStackSize]Value
	SP    int // stack pointer: next free slot

	State     FiberState
	BlockedOn any   // *ChannelObj when blocked on channel
	Error     error // set when fiber dies with an error

	// Open upvalues that reference this fiber's stack, sorted by StackIdx descending.
	OpenUpvalues []*UpvalueCell

	// Source location tracking for error reporting.
	LastLine uint32
	LastCol  uint16
}

// NewFiber creates a new fiber with the given ID.
func NewFiber(id uint64) *Fiber {
	return &Fiber{
		ID:    id,
		State: FiberSuspended,
	}
}

// Push pushes a value onto the operand stack.
func (f *Fiber) Push(v Value) {
	f.Stack[f.SP] = v
	f.SP++
}

// Pop removes and returns the top value from the operand stack.
func (f *Fiber) Pop() Value {
	f.SP--
	return f.Stack[f.SP]
}

// Peek returns the top value without removing it.
func (f *Fiber) Peek() Value {
	return f.Stack[f.SP-1]
}

// PeekN returns the value N positions from the top (0 = top).
func (f *Fiber) PeekN(n int) Value {
	return f.Stack[f.SP-1-n]
}

// CurrentFrame returns a pointer to the current (topmost) call frame.
func (f *Fiber) CurrentFrame() *CallFrame {
	return &f.Frames[f.FrameCount-1]
}

// PushFrame pushes a new call frame. Returns false if call depth exceeded.
func (f *Fiber) PushFrame(frame CallFrame) bool {
	if f.FrameCount >= MaxCallDepth {
		return false
	}
	f.Frames[f.FrameCount] = frame
	f.FrameCount++
	return true
}

// PopFrame removes and returns the topmost call frame.
func (f *Fiber) PopFrame() CallFrame {
	f.FrameCount--
	return f.Frames[f.FrameCount]
}

// CloseUpvalues closes all open upvalue cells whose StackIdx >= minIdx.
// This is called when a function returns to capture escaping values.
func (f *Fiber) CloseUpvalues(minIdx int) {
	i := 0
	for i < len(f.OpenUpvalues) {
		cell := f.OpenUpvalues[i]
		if cell.StackIdx >= minIdx {
			cell.Close()
			// Remove by swapping with last.
			f.OpenUpvalues[i] = f.OpenUpvalues[len(f.OpenUpvalues)-1]
			f.OpenUpvalues = f.OpenUpvalues[:len(f.OpenUpvalues)-1]
		} else {
			i++
		}
	}
}

// TrackUpvalue adds an open upvalue cell to this fiber's tracking list.
func (f *Fiber) TrackUpvalue(cell *UpvalueCell) {
	f.OpenUpvalues = append(f.OpenUpvalues, cell)
}

// BuildStackTrace constructs a stack trace from the current call frames.
func (f *Fiber) BuildStackTrace(program interface{ FuncName(idx uint32) string }) []StackFrame {
	trace := make([]StackFrame, 0, f.FrameCount)
	for i := f.FrameCount - 1; i >= 0; i-- {
		frame := &f.Frames[i]
		trace = append(trace, StackFrame{
			FuncName: program.FuncName(frame.FuncIdx),
			Line:     f.LastLine,
			Col:      f.LastCol,
		})
	}
	return trace
}
