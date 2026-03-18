package integration

import (
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Concurrency integration tests
//
// Each test constructs a Ryx source program as a string, compiles it through
// the full pipeline, runs it in the VM, and verifies the output or error.
// ---------------------------------------------------------------------------

// Test100FiberCounter spawns 100 fibers that each send 1 to a shared channel.
// The main function collects all 100 values and prints the sum (expected: 100).
func Test100FiberCounter(t *testing.T) {
	src := `
fn worker(ch: channel<Int>) {
    ch.send(1);
}

fn main() {
    let ch = channel<Int>(100);
    let mut i = 0;
    while i < 100 {
        spawn { worker(ch) };
        i = i + 1;
    };
    let mut sum = 0;
    let mut j = 0;
    while j < 100 {
        sum = sum + ch.recv();
        j = j + 1;
    };
    println(sum)
}
`
	got, err := compileAndRun(src, "fiber_counter.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "100\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestFanOutFanIn distributes work to N worker fibers and collects results.
func TestFanOutFanIn(t *testing.T) {
	src := `
fn worker(input: channel<Int>, output: channel<Int>) {
    let val = input.recv();
    output.send(val * 2);
}

fn main() {
    let input = channel<Int>(10);
    let output = channel<Int>(10);
    let mut i = 0;
    while i < 10 {
        spawn { worker(input, output) };
        i = i + 1;
    };
    let mut j = 0;
    while j < 10 {
        input.send(j + 1);
        j = j + 1;
    };
    let mut sum = 0;
    let mut k = 0;
    while k < 10 {
        sum = sum + output.recv();
        k = k + 1;
    };
    println(sum)
}
`
	got, err := compileAndRun(src, "fan_out_fan_in.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Workers double each value 1..10, so sum = 2*(1+2+...+10) = 110
	expected := "110\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestPipeline creates a 3-stage pipeline: generate -> transform -> collect.
func TestPipeline(t *testing.T) {
	src := `
fn generator(out: channel<Int>, count: Int) {
    let mut i = 0;
    while i < count {
        out.send(i + 1);
        i = i + 1;
    };
}

fn doubler(input: channel<Int>, output: channel<Int>, count: Int) {
    let mut i = 0;
    while i < count {
        let val = input.recv();
        output.send(val * 2);
        i = i + 1;
    };
}

fn main() {
    let ch1 = channel<Int>(5);
    let ch2 = channel<Int>(5);
    spawn { generator(ch1, 5) };
    spawn { doubler(ch1, ch2, 5) };
    let mut sum = 0;
    let mut i = 0;
    while i < 5 {
        sum = sum + ch2.recv();
        i = i + 1;
    };
    println(sum)
}
`
	got, err := compileAndRun(src, "pipeline.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Generates 1..5, doubles each => 2+4+6+8+10 = 30
	expected := "30\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestDeadlockDetection verifies that the VM detects a deadlock when all
// fibers are blocked waiting on channels with no way to make progress.
func TestDeadlockDetection(t *testing.T) {
	src := `
fn blocker(ch: channel<Int>) {
    let val = ch.recv();
    println(val);
}

fn main() {
    let ch = channel<Int>(0);
    spawn { blocker(ch) };
    let val = ch.recv();
    println(val)
}
`
	_, err := compileAndRun(src, "deadlock.ryx")
	if err == nil {
		t.Fatal("expected a deadlock error, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "deadlock") {
		t.Errorf("expected error message containing 'deadlock', got %q", errStr)
	}
}

// Test1000Fibers spawns 1000 fibers, each sending to a shared buffered channel.
// Verifies the VM handles a large number of concurrent fibers.
func Test1000Fibers(t *testing.T) {
	src := `
fn sender(ch: channel<Int>) {
    ch.send(1);
}

fn main() {
    let ch = channel<Int>(1000);
    let mut i = 0;
    while i < 1000 {
        spawn { sender(ch) };
        i = i + 1;
    };
    let mut sum = 0;
    let mut j = 0;
    while j < 1000 {
        sum = sum + ch.recv();
        j = j + 1;
    };
    println(sum)
}
`
	got, err := compileAndRun(src, "1000_fibers.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "1000\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestChannelClose verifies that closing a channel and iterating over it works.
func TestChannelClose(t *testing.T) {
	src := `
fn main() {
    let ch = channel<Int>(10);
    spawn {
        let mut i = 0;
        while i < 5 {
            ch.send(i);
            i = i + 1;
        };
        ch.close();
    };
    let mut sum = 0;
    for val in ch {
        sum = sum + val;
    };
    println(sum)
}
`
	got, err := compileAndRun(src, "channel_close.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "10\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestSpawnReturnValue verifies that spawn blocks execute independently.
func TestSpawnReturnValue(t *testing.T) {
	src := `
fn main() {
    let ch = channel<Int>(1);
    spawn {
        ch.send(42);
    };
    println(ch.recv())
}
`
	got, err := compileAndRun(src, "spawn_return.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "42\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// Ensure we don't depend on vm.RuntimeError type assertion
// when the error might not be that type.
var _ error = (*vm.RuntimeError)(nil)
