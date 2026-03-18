package vm

import (
	"fmt"
	"strings"
)

// StackFrame represents one frame in a runtime stack trace.
type StackFrame struct {
	FuncName string
	Line     uint32
	Col      uint16
}

func (f StackFrame) String() string {
	if f.Line > 0 {
		return fmt.Sprintf("  at %s (%d:%d)", f.FuncName, f.Line, f.Col)
	}
	return fmt.Sprintf("  at %s", f.FuncName)
}

// RuntimeError is the error type returned by the VM for runtime failures.
type RuntimeError struct {
	Message    string
	Line       uint32
	Col        uint16
	StackTrace []StackFrame
}

func (e *RuntimeError) Error() string {
	var b strings.Builder
	if e.Line > 0 {
		fmt.Fprintf(&b, "runtime error at %d:%d: %s", e.Line, e.Col, e.Message)
	} else {
		fmt.Fprintf(&b, "runtime error: %s", e.Message)
	}
	for _, f := range e.StackTrace {
		b.WriteByte('\n')
		b.WriteString(f.String())
	}
	return b.String()
}

// --- Error constructors ---

func errDivByZero(line uint32, col uint16) *RuntimeError {
	return &RuntimeError{Message: "division by zero", Line: line, Col: col}
}

func errStackOverflow(line uint32, col uint16) *RuntimeError {
	return &RuntimeError{Message: "stack overflow", Line: line, Col: col}
}

func errCallDepthExceeded(line uint32, col uint16) *RuntimeError {
	return &RuntimeError{Message: "call depth exceeded (max 1024)", Line: line, Col: col}
}

func errIndexOutOfBounds(idx, length int, line uint32, col uint16) *RuntimeError {
	return &RuntimeError{
		Message: fmt.Sprintf("index %d out of bounds (length %d)", idx, length),
		Line:    line, Col: col,
	}
}

func errTypeMismatch(expected, got string, line uint32, col uint16) *RuntimeError {
	return &RuntimeError{
		Message: fmt.Sprintf("type mismatch: expected %s, got %s", expected, got),
		Line:    line, Col: col,
	}
}

func errSendOnClosed(line uint32, col uint16) *RuntimeError {
	return &RuntimeError{Message: "send on closed channel", Line: line, Col: col}
}

func errRecvOnClosed(line uint32, col uint16) *RuntimeError {
	return &RuntimeError{Message: "receive on closed channel", Line: line, Col: col}
}

func errDeadlock() *RuntimeError {
	return &RuntimeError{Message: "deadlock: all fibers blocked"}
}

func errInvalidOpcode(op byte, line uint32, col uint16) *RuntimeError {
	return &RuntimeError{
		Message: fmt.Sprintf("invalid opcode 0x%02X", op),
		Line:    line, Col: col,
	}
}
