package integration

import (
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Error message integration tests
//
// Each test constructs a Ryx program designed to trigger a specific runtime
// error, compiles and runs it, and verifies the error message matches the
// documented format from pkg/vm/error.go.
// ---------------------------------------------------------------------------

// TestErrorDivisionByZero verifies the "division by zero" error message.
func TestErrorDivisionByZero(t *testing.T) {
	src := `
fn main() {
    let x = 10 / 0;
    println(x)
}
`
	_, err := compileAndRun(src, "div_by_zero.ryx")
	if err == nil {
		t.Fatal("expected a division by zero error, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "division by zero") {
		t.Errorf("expected error containing 'division by zero', got %q", errStr)
	}
}

// TestErrorStackOverflow verifies the stack overflow error by creating
// a deeply recursive function that exhausts the call stack.
func TestErrorStackOverflow(t *testing.T) {
	src := `
fn recurse(n: Int) -> Int {
    recurse(n + 1)
}

fn main() {
    println(recurse(0))
}
`
	_, err := compileAndRun(src, "stack_overflow.ryx")
	if err == nil {
		t.Fatal("expected a stack overflow or call depth error, got nil")
	}
	errStr := err.Error()
	// The error could be either "stack overflow" or "call depth exceeded"
	if !strings.Contains(errStr, "stack overflow") && !strings.Contains(errStr, "call depth") {
		t.Errorf("expected error containing 'stack overflow' or 'call depth', got %q", errStr)
	}
}

// TestErrorIndexOutOfBounds verifies the "index out of bounds" error format.
func TestErrorIndexOutOfBounds(t *testing.T) {
	src := `
fn main() {
    let arr = [1, 2, 3];
    println(arr[5])
}
`
	_, err := compileAndRun(src, "index_oob.ryx")
	if err == nil {
		t.Fatal("expected an index out of bounds error, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "out of bounds") {
		t.Errorf("expected error containing 'out of bounds', got %q", errStr)
	}
}

// TestErrorSendOnClosedChannel verifies "send on closed channel".
func TestErrorSendOnClosedChannel(t *testing.T) {
	src := `
fn main() {
    let ch = channel<Int>(1);
    ch.close();
    ch.send(42)
}
`
	_, err := compileAndRun(src, "send_closed.ryx")
	if err == nil {
		t.Fatal("expected a send on closed channel error, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "closed channel") {
		t.Errorf("expected error containing 'closed channel', got %q", errStr)
	}
}

// TestErrorDeadlock verifies "deadlock" error message.
func TestErrorDeadlock(t *testing.T) {
	src := `
fn waiter(ch: channel<Int>) {
    let val = ch.recv();
    println(val);
}

fn main() {
    let ch1 = channel<Int>(0);
    let ch2 = channel<Int>(0);
    spawn { waiter(ch1) };
    let val = ch2.recv();
    println(val)
}
`
	_, err := compileAndRun(src, "deadlock.ryx")
	if err == nil {
		t.Fatal("expected a deadlock error, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "deadlock") {
		t.Errorf("expected error containing 'deadlock', got %q", errStr)
	}
}

// TestErrorMessageFormat verifies the Error() string format for RuntimeError.
func TestErrorMessageFormat(t *testing.T) {
	// Test with line info.
	errWithLine := &vm.RuntimeError{
		Message: "division by zero",
		Line:    10,
		Col:     5,
	}
	result := errWithLine.Error()
	if !strings.Contains(result, "division by zero") {
		t.Errorf("expected error to contain 'division by zero', got %q", result)
	}
	if !strings.Contains(result, "10") || !strings.Contains(result, "5") {
		t.Errorf("expected error to contain line/col info, got %q", result)
	}

	// Test without line info.
	errNoLine := &vm.RuntimeError{
		Message: "deadlock: all fibers blocked",
	}
	result2 := errNoLine.Error()
	if !strings.Contains(result2, "deadlock: all fibers blocked") {
		t.Errorf("expected error to contain message, got %q", result2)
	}

	// Test with stack trace.
	errWithStack := &vm.RuntimeError{
		Message: "division by zero",
		Line:    10,
		Col:     5,
		StackTrace: []vm.StackFrame{
			{FuncName: "divide", Line: 10, Col: 5},
			{FuncName: "main", Line: 20, Col: 3},
		},
	}
	result3 := errWithStack.Error()
	if !strings.Contains(result3, "division by zero") {
		t.Errorf("expected error header in output, got %q", result3)
	}
	if !strings.Contains(result3, "divide") {
		t.Errorf("expected stack frame 'divide' in output, got %q", result3)
	}
	if !strings.Contains(result3, "main") {
		t.Errorf("expected stack frame 'main' in output, got %q", result3)
	}
}

// TestErrorModuloByZero verifies modulo by zero produces a runtime error.
func TestErrorModuloByZero(t *testing.T) {
	src := `
fn main() {
    let x = 10 % 0;
    println(x)
}
`
	_, err := compileAndRun(src, "mod_by_zero.ryx")
	if err == nil {
		t.Fatal("expected a division/modulo by zero error, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "zero") {
		t.Errorf("expected error containing 'zero', got %q", errStr)
	}
}
