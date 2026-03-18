package vm

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/codegen"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

type testFunc struct {
	arity  uint16
	locals uint16
	code   []byte
}

func buildProgram(fns ...testFunc) *codegen.CompiledProgram {
	prog := &codegen.CompiledProgram{}
	var offset uint32
	for i, fn := range fns {
		nameIdx := uint32(i)
		prog.StringPool = append(prog.StringPool, "")
		cf := codegen.CompiledFunc{
			NameIdx:    nameIdx,
			Arity:      fn.arity,
			LocalsCount: fn.locals,
			CodeOffset: offset,
			CodeLength: uint32(len(fn.code)),
		}
		prog.Functions = append(prog.Functions, cf)
		prog.Code = append(prog.Code, fn.code...)
		offset += uint32(len(fn.code))
	}
	return prog
}

func buildProgramWithStrings(strings []string, fns ...testFunc) *codegen.CompiledProgram {
	prog := buildProgram(fns...)
	// Extend string pool.
	for len(prog.StringPool) < len(strings) {
		prog.StringPool = append(prog.StringPool, "")
	}
	copy(prog.StringPool, strings)
	return prog
}

func runProgram(t *testing.T, prog *codegen.CompiledProgram) (*VM, error) {
	t.Helper()
	vm := NewVM(prog)
	var buf bytes.Buffer
	vm.Stdout = &buf
	err := vm.Run()
	return vm, err
}

func mustRun(t *testing.T, prog *codegen.CompiledProgram) *VM {
	t.Helper()
	vm, err := runProgram(t, prog)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return vm
}

func expectError(t *testing.T, prog *codegen.CompiledProgram, substr string) {
	t.Helper()
	_, err := runProgram(t, prog)
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got: %v", substr, err)
	}
}

func getOutput(vm *VM) string {
	if w, ok := vm.Stdout.(*bytes.Buffer); ok {
		return w.String()
	}
	return ""
}

// topValue returns the value at the top of the main fiber's stack (after run).
func topValue(vm *VM) Value {
	// After Run(), the main fiber is dead and its last push is the return value.
	for _, f := range vm.Scheduler.allFibers {
		if f.SP > 0 {
			return f.Stack[f.SP-1]
		}
	}
	// The fiber was removed from allFibers on death. Check if there's a result
	// somewhere. This is a limitation; for testing we use Println output.
	return UnitVal()
}

// ===========================================================================
// Per-opcode unit tests
// ===========================================================================

func TestConstInt(t *testing.T) {
	var code []byte
	codegen.EmitI64(&code, codegen.OpConstInt, 42)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := getOutput(vm); strings.TrimSpace(out) != "42" {
		t.Fatalf("expected 42, got %q", out)
	}
}

func TestConstFloat(t *testing.T) {
	var code []byte
	codegen.EmitF64(&code, codegen.OpConstFloat, 3.14)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "3.14" {
		t.Fatalf("expected 3.14, got %q", out)
	}
}

func TestConstBool(t *testing.T) {
	var code []byte
	codegen.EmitOp(&code, codegen.OpConstTrue)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstFalse)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	out := strings.TrimSpace(getOutput(vm))
	if out != "true\nfalse" {
		t.Fatalf("expected true/false, got %q", out)
	}
}

func TestConstUnit(t *testing.T) {
	var code []byte
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "()" {
		t.Fatalf("expected (), got %q", out)
	}
}

func TestConstString(t *testing.T) {
	var code []byte
	codegen.EmitU16(&code, codegen.OpConstString, 0)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgramWithStrings([]string{"hello"}, testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "hello" {
		t.Fatalf("expected hello, got %q", out)
	}
}

func TestConstChar(t *testing.T) {
	var code []byte
	codegen.EmitU32(&code, codegen.OpConstChar, 'A')
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "A" {
		t.Fatalf("expected A, got %q", out)
	}
}

func TestPopDupSwap(t *testing.T) {
	var code []byte
	// Push 10, dup, add → 20
	codegen.EmitI64(&code, codegen.OpConstInt, 10)
	codegen.EmitOp(&code, codegen.OpDup)
	codegen.EmitOp(&code, codegen.OpAddInt)
	codegen.EmitOp(&code, codegen.OpPrintln)

	// Push 1, 2, swap → stack: 2, 1; add → 3
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	codegen.EmitI64(&code, codegen.OpConstInt, 2)
	codegen.EmitOp(&code, codegen.OpSwap)
	codegen.EmitOp(&code, codegen.OpSubInt)
	codegen.EmitOp(&code, codegen.OpPrintln)

	// Push 99, pop, push unit, return
	codegen.EmitI64(&code, codegen.OpConstInt, 99)
	codegen.EmitOp(&code, codegen.OpPop)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	out := strings.TrimSpace(getOutput(vm))
	// Swap: stack is [1,2] → swap → [2,1], then SubInt pops b=1, a=2 → a-b=1
	if out != "20\n1" {
		t.Fatalf("expected '20\\n1', got %q", out)
	}
}

// ===========================================================================
// Variable access
// ===========================================================================

func TestLocalVariables(t *testing.T) {
	var code []byte
	// local 0 = 42
	codegen.EmitI64(&code, codegen.OpConstInt, 42)
	codegen.EmitU16(&code, codegen.OpStoreLocal, 0)
	// local 1 = 58
	codegen.EmitI64(&code, codegen.OpConstInt, 58)
	codegen.EmitU16(&code, codegen.OpStoreLocal, 1)
	// load both, add, print
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitU16(&code, codegen.OpLoadLocal, 1)
	codegen.EmitOp(&code, codegen.OpAddInt)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 2, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "100" {
		t.Fatalf("expected 100, got %q", out)
	}
}

func TestGlobalVariables(t *testing.T) {
	var code []byte
	codegen.EmitI64(&code, codegen.OpConstInt, 7)
	codegen.EmitU16(&code, codegen.OpStoreGlobal, 5)
	codegen.EmitU16(&code, codegen.OpLoadGlobal, 5)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "7" {
		t.Fatalf("expected 7, got %q", out)
	}
}

// ===========================================================================
// Arithmetic
// ===========================================================================

func TestIntArithmetic(t *testing.T) {
	tests := []struct {
		name string
		a, b int64
		op   codegen.Opcode
		want int64
	}{
		{"add", 10, 20, codegen.OpAddInt, 30},
		{"sub", 50, 30, codegen.OpSubInt, 20},
		{"mul", 6, 7, codegen.OpMulInt, 42},
		{"div", 100, 4, codegen.OpDivInt, 25},
		{"mod", 17, 5, codegen.OpModInt, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var code []byte
			codegen.EmitI64(&code, codegen.OpConstInt, tt.a)
			codegen.EmitI64(&code, codegen.OpConstInt, tt.b)
			codegen.EmitOp(&code, tt.op)
			codegen.EmitOp(&code, codegen.OpPrintln)
			codegen.EmitOp(&code, codegen.OpConstUnit)
			codegen.EmitOp(&code, codegen.OpReturn)

			prog := buildProgram(testFunc{0, 0, code})
			vm := mustRun(t, prog)
			want := IntVal(tt.want).String()
			if out := strings.TrimSpace(getOutput(vm)); out != want {
				t.Fatalf("expected %s, got %q", want, out)
			}
		})
	}
}

func TestNegInt(t *testing.T) {
	var code []byte
	codegen.EmitI64(&code, codegen.OpConstInt, 42)
	codegen.EmitOp(&code, codegen.OpNegInt)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "-42" {
		t.Fatalf("expected -42, got %q", out)
	}
}

func TestFloatArithmetic(t *testing.T) {
	tests := []struct {
		name string
		a, b float64
		op   codegen.Opcode
		want float64
	}{
		{"add", 1.5, 2.5, codegen.OpAddFloat, 4.0},
		{"sub", 10.0, 3.5, codegen.OpSubFloat, 6.5},
		{"mul", 2.0, 3.0, codegen.OpMulFloat, 6.0},
		{"div", 7.5, 2.5, codegen.OpDivFloat, 3.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var code []byte
			codegen.EmitF64(&code, codegen.OpConstFloat, tt.a)
			codegen.EmitF64(&code, codegen.OpConstFloat, tt.b)
			codegen.EmitOp(&code, tt.op)
			codegen.EmitOp(&code, codegen.OpPrintln)
			codegen.EmitOp(&code, codegen.OpConstUnit)
			codegen.EmitOp(&code, codegen.OpReturn)

			prog := buildProgram(testFunc{0, 0, code})
			vm := mustRun(t, prog)
			out := strings.TrimSpace(getOutput(vm))
			wantStr := FloatVal(tt.want).String()
			// Allow for floating point formatting differences.
			if out != wantStr {
				t.Fatalf("expected %s, got %q", wantStr, out)
			}
		})
	}
}

func TestNegFloat(t *testing.T) {
	var code []byte
	codegen.EmitF64(&code, codegen.OpConstFloat, 3.14)
	codegen.EmitOp(&code, codegen.OpNegFloat)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "-3.14" {
		t.Fatalf("expected -3.14, got %q", out)
	}
}

func TestModFloat(t *testing.T) {
	var code []byte
	codegen.EmitF64(&code, codegen.OpConstFloat, 7.5)
	codegen.EmitF64(&code, codegen.OpConstFloat, 2.0)
	codegen.EmitOp(&code, codegen.OpModFloat)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "1.5" {
		t.Fatalf("expected 1.5, got %q", out)
	}
}

func TestConcatString(t *testing.T) {
	var code []byte
	codegen.EmitU16(&code, codegen.OpConstString, 0)
	codegen.EmitU16(&code, codegen.OpConstString, 1)
	codegen.EmitOp(&code, codegen.OpConcatString)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgramWithStrings([]string{"hello ", "world"}, testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "hello world" {
		t.Fatalf("expected 'hello world', got %q", out)
	}
}

// ===========================================================================
// Comparison
// ===========================================================================

func TestComparisons(t *testing.T) {
	tests := []struct {
		name string
		a, b int64
		op   codegen.Opcode
		want bool
	}{
		{"eq_true", 5, 5, codegen.OpEq, true},
		{"eq_false", 5, 6, codegen.OpEq, false},
		{"neq_true", 5, 6, codegen.OpNeq, true},
		{"neq_false", 5, 5, codegen.OpNeq, false},
		{"lt_true", 3, 5, codegen.OpLtInt, true},
		{"lt_false", 5, 3, codegen.OpLtInt, false},
		{"gt_true", 5, 3, codegen.OpGtInt, true},
		{"gt_false", 3, 5, codegen.OpGtInt, false},
		{"leq_true_lt", 3, 5, codegen.OpLeqInt, true},
		{"leq_true_eq", 5, 5, codegen.OpLeqInt, true},
		{"leq_false", 6, 5, codegen.OpLeqInt, false},
		{"geq_true_gt", 5, 3, codegen.OpGeqInt, true},
		{"geq_true_eq", 5, 5, codegen.OpGeqInt, true},
		{"geq_false", 3, 5, codegen.OpGeqInt, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var code []byte
			codegen.EmitI64(&code, codegen.OpConstInt, tt.a)
			codegen.EmitI64(&code, codegen.OpConstInt, tt.b)
			codegen.EmitOp(&code, tt.op)
			codegen.EmitOp(&code, codegen.OpPrintln)
			codegen.EmitOp(&code, codegen.OpConstUnit)
			codegen.EmitOp(&code, codegen.OpReturn)

			prog := buildProgram(testFunc{0, 0, code})
			vm := mustRun(t, prog)
			want := BoolVal(tt.want).String()
			if out := strings.TrimSpace(getOutput(vm)); out != want {
				t.Fatalf("expected %s, got %q", want, out)
			}
		})
	}
}

func TestFloatComparisons(t *testing.T) {
	var code []byte
	codegen.EmitF64(&code, codegen.OpConstFloat, 1.0)
	codegen.EmitF64(&code, codegen.OpConstFloat, 2.0)
	codegen.EmitOp(&code, codegen.OpLtFloat)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "true" {
		t.Fatalf("expected true, got %q", out)
	}
}

func TestNot(t *testing.T) {
	var code []byte
	codegen.EmitOp(&code, codegen.OpConstTrue)
	codegen.EmitOp(&code, codegen.OpNot)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstFalse)
	codegen.EmitOp(&code, codegen.OpNot)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "false\ntrue" {
		t.Fatalf("expected 'false\\ntrue', got %q", out)
	}
}

// ===========================================================================
// Control flow
// ===========================================================================

func TestJump(t *testing.T) {
	var code []byte
	// JUMP over the "10" to print "20" only.
	codegen.EmitI16(&code, codegen.OpJump, 10) // skip next 10 bytes
	// Skipped block: CONST_INT 10 (9 bytes) + PRINTLN (1 byte) = 10
	codegen.EmitI64(&code, codegen.OpConstInt, 10)
	codegen.EmitOp(&code, codegen.OpPrintln)
	// Landing here:
	codegen.EmitI64(&code, codegen.OpConstInt, 20)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "20" {
		t.Fatalf("expected 20, got %q", out)
	}
}

func TestJumpIfTrue(t *testing.T) {
	var code []byte
	codegen.EmitOp(&code, codegen.OpConstTrue)
	codegen.EmitI16(&code, codegen.OpJumpIfTrue, 10)
	codegen.EmitI64(&code, codegen.OpConstInt, 10) // skipped
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitI64(&code, codegen.OpConstInt, 20)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "20" {
		t.Fatalf("expected 20, got %q", out)
	}
}

func TestJumpIfFalse(t *testing.T) {
	var code []byte
	codegen.EmitOp(&code, codegen.OpConstFalse)
	codegen.EmitI16(&code, codegen.OpJumpIfFalse, 10)
	codegen.EmitI64(&code, codegen.OpConstInt, 10) // skipped
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitI64(&code, codegen.OpConstInt, 30)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "30" {
		t.Fatalf("expected 30, got %q", out)
	}
}

// ===========================================================================
// Division by zero
// ===========================================================================

func TestDivByZeroInt(t *testing.T) {
	var code []byte
	codegen.EmitI64(&code, codegen.OpConstInt, 10)
	codegen.EmitI64(&code, codegen.OpConstInt, 0)
	codegen.EmitOp(&code, codegen.OpDivInt)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	expectError(t, prog, "division by zero")
}

func TestDivByZeroFloat(t *testing.T) {
	var code []byte
	codegen.EmitF64(&code, codegen.OpConstFloat, 10.0)
	codegen.EmitF64(&code, codegen.OpConstFloat, 0.0)
	codegen.EmitOp(&code, codegen.OpDivFloat)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	expectError(t, prog, "division by zero")
}

// ===========================================================================
// Function calls
// ===========================================================================

func TestSimpleCall(t *testing.T) {
	// func0 (main): call func1, print result
	var mainCode []byte
	codegen.EmitU16(&mainCode, codegen.OpLoadGlobal, 1) // load func1
	codegen.EmitU16(&mainCode, codegen.OpCall, 0)       // call with 0 args
	codegen.EmitOp(&mainCode, codegen.OpPrintln)
	codegen.EmitOp(&mainCode, codegen.OpConstUnit)
	codegen.EmitOp(&mainCode, codegen.OpReturn)

	// func1: return 42
	var func1Code []byte
	codegen.EmitI64(&func1Code, codegen.OpConstInt, 42)
	codegen.EmitOp(&func1Code, codegen.OpReturn)

	prog := buildProgram(
		testFunc{0, 0, mainCode},
		testFunc{0, 0, func1Code},
	)

	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "42" {
		t.Fatalf("expected 42, got %q", out)
	}
}

func TestCallWithArgs(t *testing.T) {
	// func0 (main): call add(10, 20)
	var mainCode []byte
	codegen.EmitU16(&mainCode, codegen.OpLoadGlobal, 1) // load add
	codegen.EmitI64(&mainCode, codegen.OpConstInt, 10)
	codegen.EmitI64(&mainCode, codegen.OpConstInt, 20)
	codegen.EmitU16(&mainCode, codegen.OpCall, 2)
	codegen.EmitOp(&mainCode, codegen.OpPrintln)
	codegen.EmitOp(&mainCode, codegen.OpConstUnit)
	codegen.EmitOp(&mainCode, codegen.OpReturn)

	// func1 (add): local0 + local1
	var addCode []byte
	codegen.EmitU16(&addCode, codegen.OpLoadLocal, 0)
	codegen.EmitU16(&addCode, codegen.OpLoadLocal, 1)
	codegen.EmitOp(&addCode, codegen.OpAddInt)
	codegen.EmitOp(&addCode, codegen.OpReturn)

	prog := buildProgram(
		testFunc{0, 0, mainCode},
		testFunc{2, 2, addCode},
	)

	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "30" {
		t.Fatalf("expected 30, got %q", out)
	}
}

// ===========================================================================
// Recursion
// ===========================================================================

func TestRecursion(t *testing.T) {
	// func0 (main): print factorial(5)
	var mainCode []byte
	codegen.EmitU16(&mainCode, codegen.OpLoadGlobal, 1) // load factorial
	codegen.EmitI64(&mainCode, codegen.OpConstInt, 5)
	codegen.EmitU16(&mainCode, codegen.OpCall, 1)
	codegen.EmitOp(&mainCode, codegen.OpPrintln)
	codegen.EmitOp(&mainCode, codegen.OpConstUnit)
	codegen.EmitOp(&mainCode, codegen.OpReturn)

	// func1 (factorial): if n <= 1 return 1; else return n * factorial(n-1)
	var factCode []byte
	// Load n
	codegen.EmitU16(&factCode, codegen.OpLoadLocal, 0)
	codegen.EmitI64(&factCode, codegen.OpConstInt, 1)
	codegen.EmitOp(&factCode, codegen.OpLeqInt)
	// If n <= 1, jump to base case
	baseJumpOffset := len(factCode)
	codegen.EmitI16(&factCode, codegen.OpJumpIfTrue, 0) // placeholder

	// Recursive case: n * factorial(n-1)
	codegen.EmitU16(&factCode, codegen.OpLoadLocal, 0) // n
	codegen.EmitU16(&factCode, codegen.OpLoadGlobal, 1) // factorial
	codegen.EmitU16(&factCode, codegen.OpLoadLocal, 0)  // n
	codegen.EmitI64(&factCode, codegen.OpConstInt, 1)
	codegen.EmitOp(&factCode, codegen.OpSubInt)          // n-1
	codegen.EmitU16(&factCode, codegen.OpCall, 1)         // factorial(n-1)
	codegen.EmitOp(&factCode, codegen.OpMulInt)           // n * factorial(n-1)
	codegen.EmitOp(&factCode, codegen.OpReturn)

	// Base case: return 1
	baseCaseOffset := len(factCode)
	codegen.EmitI64(&factCode, codegen.OpConstInt, 1)
	codegen.EmitOp(&factCode, codegen.OpReturn)

	// Patch the jump offset: jump from after JumpIfTrue to baseCaseOffset.
	jumpFrom := baseJumpOffset + 3 // past the JUMP_IF_TRUE instruction
	rel := baseCaseOffset - jumpFrom
	factCode[baseJumpOffset+1] = byte(uint16(int16(rel)))
	factCode[baseJumpOffset+2] = byte(uint16(int16(rel)) >> 8)

	prog := buildProgram(
		testFunc{0, 0, mainCode},
		testFunc{1, 1, factCode},
	)

	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "120" {
		t.Fatalf("expected 120, got %q", out)
	}
}

// ===========================================================================
// Tail calls
// ===========================================================================

func TestTailCall(t *testing.T) {
	// func0 (main): print tailFact(5, 1)
	var mainCode []byte
	codegen.EmitU16(&mainCode, codegen.OpLoadGlobal, 1) // load tailFact
	codegen.EmitI64(&mainCode, codegen.OpConstInt, 5)    // n
	codegen.EmitI64(&mainCode, codegen.OpConstInt, 1)    // acc
	codegen.EmitU16(&mainCode, codegen.OpCall, 2)
	codegen.EmitOp(&mainCode, codegen.OpPrintln)
	codegen.EmitOp(&mainCode, codegen.OpConstUnit)
	codegen.EmitOp(&mainCode, codegen.OpReturn)

	// func1 (tailFact): local0=n, local1=acc
	// if n <= 1 return acc; else tailcall(n-1, acc*n)
	var tfCode []byte
	codegen.EmitU16(&tfCode, codegen.OpLoadLocal, 0)
	codegen.EmitI64(&tfCode, codegen.OpConstInt, 1)
	codegen.EmitOp(&tfCode, codegen.OpLeqInt)
	baseOffset := len(tfCode)
	codegen.EmitI16(&tfCode, codegen.OpJumpIfTrue, 0) // placeholder

	// Recursive: tailcall tailFact(n-1, acc*n)
	codegen.EmitU16(&tfCode, codegen.OpLoadGlobal, 1) // tailFact
	codegen.EmitU16(&tfCode, codegen.OpLoadLocal, 0)  // n
	codegen.EmitI64(&tfCode, codegen.OpConstInt, 1)
	codegen.EmitOp(&tfCode, codegen.OpSubInt)          // n-1
	codegen.EmitU16(&tfCode, codegen.OpLoadLocal, 1)  // acc
	codegen.EmitU16(&tfCode, codegen.OpLoadLocal, 0)  // n
	codegen.EmitOp(&tfCode, codegen.OpMulInt)          // acc*n
	codegen.EmitU16(&tfCode, codegen.OpTailCall, 2)

	// Base: return acc
	baseLanding := len(tfCode)
	codegen.EmitU16(&tfCode, codegen.OpLoadLocal, 1)
	codegen.EmitOp(&tfCode, codegen.OpReturn)

	// Patch jump
	jumpFrom := baseOffset + 3
	rel := baseLanding - jumpFrom
	tfCode[baseOffset+1] = byte(uint16(int16(rel)))
	tfCode[baseOffset+2] = byte(uint16(int16(rel)) >> 8)

	prog := buildProgram(
		testFunc{0, 0, mainCode},
		testFunc{2, 2, tfCode},
	)

	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "120" {
		t.Fatalf("expected 120, got %q", out)
	}
}

// ===========================================================================
// Closures with upvalue closing
// ===========================================================================

func TestClosure(t *testing.T) {
	// func0 (main):
	//   push captured value 10
	//   MakeClosure(func1, 1 upvalue)
	//   Call the closure with 0 args → should return 10
	var mainCode []byte
	codegen.EmitI64(&mainCode, codegen.OpConstInt, 10)            // capture value
	codegen.EmitU16U16(&mainCode, codegen.OpMakeClosure, 1, 1)    // closure of func1, 1 upvalue
	codegen.EmitU16(&mainCode, codegen.OpCall, 0)                  // call closure()
	codegen.EmitOp(&mainCode, codegen.OpPrintln)
	codegen.EmitOp(&mainCode, codegen.OpConstUnit)
	codegen.EmitOp(&mainCode, codegen.OpReturn)

	// func1 (captured): loads upvalue[0] and returns it
	var closureCode []byte
	codegen.EmitU16(&closureCode, codegen.OpLoadUpvalue, 0)
	codegen.EmitOp(&closureCode, codegen.OpReturn)

	prog := buildProgram(
		testFunc{0, 0, mainCode},
		testFunc{0, 0, closureCode},
	)

	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "10" {
		t.Fatalf("expected 10, got %q", out)
	}
}

func TestClosureStoreUpvalue(t *testing.T) {
	// Create a counter closure:
	// func0 (main):
	//   push initial value 0
	//   MakeClosure(func1, 1 upvalue) → closure A
	//   Call A → prints and returns incremented value
	//   We test that upvalue store works
	var mainCode []byte
	codegen.EmitI64(&mainCode, codegen.OpConstInt, 0)
	codegen.EmitU16U16(&mainCode, codegen.OpMakeClosure, 1, 1)
	codegen.EmitU16(&mainCode, codegen.OpStoreLocal, 0)
	// Call closure, which returns its upvalue
	codegen.EmitU16(&mainCode, codegen.OpLoadLocal, 0)
	codegen.EmitU16(&mainCode, codegen.OpCall, 0)
	codegen.EmitOp(&mainCode, codegen.OpPrintln)
	// Call again
	codegen.EmitU16(&mainCode, codegen.OpLoadLocal, 0)
	codegen.EmitU16(&mainCode, codegen.OpCall, 0)
	codegen.EmitOp(&mainCode, codegen.OpPrintln)
	codegen.EmitOp(&mainCode, codegen.OpConstUnit)
	codegen.EmitOp(&mainCode, codegen.OpReturn)

	// func1 (increment): load upvalue[0], add 1, store back, return old
	var incCode []byte
	codegen.EmitU16(&incCode, codegen.OpLoadUpvalue, 0)  // load counter
	codegen.EmitU16(&incCode, codegen.OpLoadUpvalue, 0)  // load counter again
	codegen.EmitI64(&incCode, codegen.OpConstInt, 1)
	codegen.EmitOp(&incCode, codegen.OpAddInt)
	codegen.EmitU16(&incCode, codegen.OpStoreUpvalue, 0) // store counter+1
	codegen.EmitOp(&incCode, codegen.OpReturn)            // return old counter

	prog := buildProgram(
		testFunc{0, 1, mainCode},
		testFunc{0, 0, incCode},
	)

	vm := mustRun(t, prog)
	out := strings.TrimSpace(getOutput(vm))
	if out != "0\n1" {
		t.Fatalf("expected '0\\n1', got %q", out)
	}
}

// ===========================================================================
// Data structures
// ===========================================================================

func TestMakeArray(t *testing.T) {
	var code []byte
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	codegen.EmitI64(&code, codegen.OpConstInt, 2)
	codegen.EmitI64(&code, codegen.OpConstInt, 3)
	codegen.EmitU16(&code, codegen.OpMakeArray, 3)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "[1, 2, 3]" {
		t.Fatalf("expected [1, 2, 3], got %q", out)
	}
}

func TestMakeTuple(t *testing.T) {
	var code []byte
	codegen.EmitI64(&code, codegen.OpConstInt, 10)
	codegen.EmitOp(&code, codegen.OpConstTrue)
	codegen.EmitU16(&code, codegen.OpMakeTuple, 2)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "(10, true)" {
		t.Fatalf("expected (10, true), got %q", out)
	}
}

func TestIndexGetSet(t *testing.T) {
	var code []byte
	// Make array [10, 20, 30]
	codegen.EmitI64(&code, codegen.OpConstInt, 10)
	codegen.EmitI64(&code, codegen.OpConstInt, 20)
	codegen.EmitI64(&code, codegen.OpConstInt, 30)
	codegen.EmitU16(&code, codegen.OpMakeArray, 3)
	codegen.EmitU16(&code, codegen.OpStoreLocal, 0)

	// Get index 1 → 20
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	codegen.EmitOp(&code, codegen.OpIndexGet)
	codegen.EmitOp(&code, codegen.OpPrintln)

	// Set index 1 = 99
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	codegen.EmitI64(&code, codegen.OpConstInt, 99)
	codegen.EmitOp(&code, codegen.OpIndexSet)

	// Get index 1 → 99
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	codegen.EmitOp(&code, codegen.OpIndexGet)
	codegen.EmitOp(&code, codegen.OpPrintln)

	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 1, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "20\n99" {
		t.Fatalf("expected '20\\n99', got %q", out)
	}
}

func TestIndexOutOfBounds(t *testing.T) {
	var code []byte
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	codegen.EmitU16(&code, codegen.OpMakeArray, 1)
	codegen.EmitI64(&code, codegen.OpConstInt, 5) // OOB index
	codegen.EmitOp(&code, codegen.OpIndexGet)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	expectError(t, prog, "index 5 out of bounds")
}

func TestFieldGetSet(t *testing.T) {
	var code []byte
	// Make struct with 2 fields: (100, 200)
	codegen.EmitI64(&code, codegen.OpConstInt, 100)
	codegen.EmitI64(&code, codegen.OpConstInt, 200)
	codegen.EmitU16U16(&code, codegen.OpMakeStruct, 0, 2)
	codegen.EmitU16(&code, codegen.OpStoreLocal, 0)

	// Get field 0 → 100
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitU16(&code, codegen.OpFieldGet, 0)
	codegen.EmitOp(&code, codegen.OpPrintln)

	// Set field 1 = 999
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitI64(&code, codegen.OpConstInt, 999)
	codegen.EmitU16(&code, codegen.OpFieldSet, 1)

	// Get field 1 → 999
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitU16(&code, codegen.OpFieldGet, 1)
	codegen.EmitOp(&code, codegen.OpPrintln)

	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 1, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "100\n999" {
		t.Fatalf("expected '100\\n999', got %q", out)
	}
}

func TestMakeEnum(t *testing.T) {
	var code []byte
	// Make enum type=0, variant=1, 1 field with value 42
	codegen.EmitI64(&code, codegen.OpConstInt, 42)
	codegen.EmitU16U16U16(&code, codegen.OpMakeEnum, 0, 1, 1)
	codegen.EmitU16(&code, codegen.OpStoreLocal, 0)

	// Tag check: variant 1 → true
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitU16(&code, codegen.OpTagCheck, 1)
	codegen.EmitOp(&code, codegen.OpPrintln)

	// Tag check: variant 0 → false
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitU16(&code, codegen.OpTagCheck, 0)
	codegen.EmitOp(&code, codegen.OpPrintln)

	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 1, code})
	vm := mustRun(t, prog)
	out := strings.TrimSpace(getOutput(vm))
	if out != "true\nfalse" {
		t.Fatalf("expected 'true\\nfalse', got %q", out)
	}
}

func TestDestructure(t *testing.T) {
	var code []byte
	// Make enum with 2 fields (10, 20), then destructure
	codegen.EmitI64(&code, codegen.OpConstInt, 10)
	codegen.EmitI64(&code, codegen.OpConstInt, 20)
	codegen.EmitU16U16U16(&code, codegen.OpMakeEnum, 0, 0, 2)
	codegen.EmitU16(&code, codegen.OpDestructure, 2)
	// Stack now has field0, field1
	codegen.EmitOp(&code, codegen.OpPrintln) // prints 20 (top)
	codegen.EmitOp(&code, codegen.OpPrintln) // prints 10
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	out := strings.TrimSpace(getOutput(vm))
	if out != "20\n10" {
		t.Fatalf("expected '20\\n10', got %q", out)
	}
}

// ===========================================================================
// Pattern matching dispatch
// ===========================================================================

func TestPatternMatchDispatch(t *testing.T) {
	// Simulate pattern matching: create enum variant 2, check tags
	var code []byte
	// Make enum type=0, variant=2, 0 fields
	codegen.EmitU16U16U16(&code, codegen.OpMakeEnum, 0, 2, 0)
	codegen.EmitU16(&code, codegen.OpStoreLocal, 0)

	// Check variant 0 → false, jump over "matched 0"
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitU16(&code, codegen.OpTagCheck, 0)
	tagCheck0 := len(code)
	codegen.EmitI16(&code, codegen.OpJumpIfTrue, 0) // placeholder

	// Check variant 1 → false
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitOp(&code, codegen.OpPop) // pop the enum left by TagCheck
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitU16(&code, codegen.OpTagCheck, 1)
	tagCheck1 := len(code)
	codegen.EmitI16(&code, codegen.OpJumpIfTrue, 0) // placeholder

	// Check variant 2 → true
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitOp(&code, codegen.OpPop) // pop the enum left by TagCheck
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitU16(&code, codegen.OpTagCheck, 2)
	tagCheck2 := len(code)
	codegen.EmitI16(&code, codegen.OpJumpIfTrue, 0) // placeholder

	// Default: print -1
	codegen.EmitOp(&code, codegen.OpPop) // pop bool from tag check
	codegen.EmitI64(&code, codegen.OpConstInt, -1)
	codegen.EmitOp(&code, codegen.OpPrintln)
	endJump := len(code)
	codegen.EmitI16(&code, codegen.OpJump, 0) // placeholder

	// Case 0: print 0
	case0 := len(code)
	codegen.EmitOp(&code, codegen.OpPop) // pop bool
	codegen.EmitOp(&code, codegen.OpPop) // pop enum
	codegen.EmitI64(&code, codegen.OpConstInt, 0)
	codegen.EmitOp(&code, codegen.OpPrintln)
	end0 := len(code)
	codegen.EmitI16(&code, codegen.OpJump, 0)

	// Case 1: print 1
	case1 := len(code)
	codegen.EmitOp(&code, codegen.OpPop)
	codegen.EmitOp(&code, codegen.OpPop)
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	codegen.EmitOp(&code, codegen.OpPrintln)
	end1 := len(code)
	codegen.EmitI16(&code, codegen.OpJump, 0)

	// Case 2: print 2
	case2 := len(code)
	codegen.EmitOp(&code, codegen.OpPop)
	codegen.EmitOp(&code, codegen.OpPop)
	codegen.EmitI64(&code, codegen.OpConstInt, 2)
	codegen.EmitOp(&code, codegen.OpPrintln)

	// End
	endLabel := len(code)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	// Patch jumps
	patchI16(code, tagCheck0, case0)
	patchI16(code, tagCheck1, case1)
	patchI16(code, tagCheck2, case2)
	patchI16(code, endJump, endLabel)
	patchI16(code, end0, endLabel)
	patchI16(code, end1, endLabel)

	prog := buildProgram(testFunc{0, 1, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "2" {
		t.Fatalf("expected 2, got %q", out)
	}
}

func patchI16(code []byte, jumpInstrEnd int, target int) {
	// jumpInstrEnd is the offset right after the i16 operand was emitted (= start of next instr).
	// The i16 operand starts at jumpInstrEnd - 2.
	// jumpInstrEnd is the offset of the JUMP_IF_TRUE/JUMP opcode.
	// The i16 is at jumpInstrEnd+1..jumpInstrEnd+2.
	// IP after reading = jumpInstrEnd + 3.
	// Relative offset = target - (jumpInstrEnd + 3).
	rel := target - (jumpInstrEnd + 3)
	code[jumpInstrEnd+1] = byte(uint16(int16(rel)))
	code[jumpInstrEnd+2] = byte(uint16(int16(rel)) >> 8)
}

// ===========================================================================
// Built-in conversions
// ===========================================================================

func TestIntToFloat(t *testing.T) {
	var code []byte
	codegen.EmitI64(&code, codegen.OpConstInt, 42)
	codegen.EmitOp(&code, codegen.OpIntToFloat)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "42" {
		t.Fatalf("expected 42, got %q", out)
	}
}

func TestFloatToInt(t *testing.T) {
	var code []byte
	codegen.EmitF64(&code, codegen.OpConstFloat, 3.99)
	codegen.EmitOp(&code, codegen.OpFloatToInt)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "3" {
		t.Fatalf("expected 3, got %q", out)
	}
}

func TestIntToString(t *testing.T) {
	var code []byte
	codegen.EmitI64(&code, codegen.OpConstInt, 123)
	codegen.EmitOp(&code, codegen.OpIntToString)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "123" {
		t.Fatalf("expected 123, got %q", out)
	}
}

func TestFloatToString(t *testing.T) {
	var code []byte
	codegen.EmitF64(&code, codegen.OpConstFloat, 2.5)
	codegen.EmitOp(&code, codegen.OpFloatToString)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "2.5" {
		t.Fatalf("expected 2.5, got %q", out)
	}
}

func TestStringLen(t *testing.T) {
	var code []byte
	codegen.EmitU16(&code, codegen.OpConstString, 0)
	codegen.EmitOp(&code, codegen.OpStringLen)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgramWithStrings([]string{"hello"}, testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "5" {
		t.Fatalf("expected 5, got %q", out)
	}
}

func TestArrayLen(t *testing.T) {
	var code []byte
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	codegen.EmitI64(&code, codegen.OpConstInt, 2)
	codegen.EmitU16(&code, codegen.OpMakeArray, 2)
	codegen.EmitOp(&code, codegen.OpArrayLen)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "2" {
		t.Fatalf("expected 2, got %q", out)
	}
}

// ===========================================================================
// Source location and error reporting
// ===========================================================================

func TestSourceLocInError(t *testing.T) {
	var code []byte
	codegen.EmitU16U16(&code, codegen.OpSourceLoc, 10, 5)
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	codegen.EmitI64(&code, codegen.OpConstInt, 0)
	codegen.EmitOp(&code, codegen.OpDivInt)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	_, err := runProgram(t, prog)
	if err == nil {
		t.Fatal("expected error")
	}
	re, ok := err.(*RuntimeError)
	if !ok {
		t.Fatalf("expected RuntimeError, got %T", err)
	}
	if re.Line != 10 || re.Col != 5 {
		t.Fatalf("expected line 10, col 5; got line %d, col %d", re.Line, re.Col)
	}
	if !strings.Contains(re.Error(), "10:5") {
		t.Fatalf("error string should contain source location, got: %s", re.Error())
	}
}

func TestStackTraceInError(t *testing.T) {
	// main calls func1 which divides by zero
	var mainCode []byte
	codegen.EmitU16(&mainCode, codegen.OpLoadGlobal, 1)
	codegen.EmitU16(&mainCode, codegen.OpCall, 0)
	codegen.EmitOp(&mainCode, codegen.OpConstUnit)
	codegen.EmitOp(&mainCode, codegen.OpReturn)

	var func1Code []byte
	codegen.EmitU16U16(&func1Code, codegen.OpSourceLoc, 42, 1)
	codegen.EmitI64(&func1Code, codegen.OpConstInt, 1)
	codegen.EmitI64(&func1Code, codegen.OpConstInt, 0)
	codegen.EmitOp(&func1Code, codegen.OpDivInt)
	codegen.EmitOp(&func1Code, codegen.OpReturn)

	prog := buildProgramWithStrings([]string{"main", "badFunc"}, testFunc{0, 0, mainCode}, testFunc{0, 0, func1Code})
	_, err := runProgram(t, prog)
	if err == nil {
		t.Fatal("expected error")
	}
	re := err.(*RuntimeError)
	if len(re.StackTrace) == 0 {
		t.Fatal("expected stack trace")
	}
	errStr := re.Error()
	if !strings.Contains(errStr, "division by zero") {
		t.Fatalf("expected 'division by zero' in %q", errStr)
	}
}

// ===========================================================================
// Channel operations
// ===========================================================================

func TestBufferedChannel(t *testing.T) {
	// main: create buffered channel(2), send 10, send 20, recv, recv, print both
	var code []byte
	codegen.EmitU16(&code, codegen.OpChannelCreate, 2)
	codegen.EmitU16(&code, codegen.OpStoreLocal, 0)

	// send 10
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitI64(&code, codegen.OpConstInt, 10)
	codegen.EmitOp(&code, codegen.OpChannelSend)

	// send 20
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitI64(&code, codegen.OpConstInt, 20)
	codegen.EmitOp(&code, codegen.OpChannelSend)

	// recv → print
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitOp(&code, codegen.OpChannelRecv)
	codegen.EmitOp(&code, codegen.OpPrintln)

	// recv → print
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitOp(&code, codegen.OpChannelRecv)
	codegen.EmitOp(&code, codegen.OpPrintln)

	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 1, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "10\n20" {
		t.Fatalf("expected '10\\n20', got %q", out)
	}
}

func TestChannelClose(t *testing.T) {
	// Create channel, send value, close, recv (should get value), recv again (closed → unit)
	var code []byte
	codegen.EmitU16(&code, codegen.OpChannelCreate, 1)
	codegen.EmitU16(&code, codegen.OpStoreLocal, 0)

	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitI64(&code, codegen.OpConstInt, 42)
	codegen.EmitOp(&code, codegen.OpChannelSend)

	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitOp(&code, codegen.OpChannelClose)

	// Recv buffered value
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitOp(&code, codegen.OpChannelRecv)
	codegen.EmitOp(&code, codegen.OpPrintln)

	// Recv from closed, empty channel → unit
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitOp(&code, codegen.OpChannelRecv)
	codegen.EmitOp(&code, codegen.OpPrintln)

	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 1, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "42\n()" {
		t.Fatalf("expected '42\\n()', got %q", out)
	}
}

func TestSendOnClosedChannel(t *testing.T) {
	var code []byte
	codegen.EmitU16(&code, codegen.OpChannelCreate, 0)
	codegen.EmitU16(&code, codegen.OpStoreLocal, 0)
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitOp(&code, codegen.OpChannelClose)
	codegen.EmitU16(&code, codegen.OpLoadLocal, 0)
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	codegen.EmitOp(&code, codegen.OpChannelSend)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 1, code})
	expectError(t, prog, "send on closed channel")
}

// ===========================================================================
// Fiber scheduling
// ===========================================================================

func TestSpawnAndSchedule(t *testing.T) {
	// main spawns func1, both print
	var mainCode []byte
	codegen.EmitU16(&mainCode, codegen.OpSpawn, 1) // spawn func1
	codegen.EmitOp(&mainCode, codegen.OpPop)
	codegen.EmitI64(&mainCode, codegen.OpConstInt, 1)
	codegen.EmitOp(&mainCode, codegen.OpPrintln)
	codegen.EmitOp(&mainCode, codegen.OpConstUnit)
	codegen.EmitOp(&mainCode, codegen.OpReturn)

	var func1Code []byte
	codegen.EmitI64(&func1Code, codegen.OpConstInt, 2)
	codegen.EmitOp(&func1Code, codegen.OpPrintln)
	codegen.EmitOp(&func1Code, codegen.OpConstUnit)
	codegen.EmitOp(&func1Code, codegen.OpReturn)

	prog := buildProgram(
		testFunc{0, 0, mainCode},
		testFunc{0, 0, func1Code},
	)

	vm := mustRun(t, prog)
	out := strings.TrimSpace(getOutput(vm))
	// Both fibers should have printed (order depends on scheduling).
	if !strings.Contains(out, "1") || !strings.Contains(out, "2") {
		t.Fatalf("expected both 1 and 2 in output, got %q", out)
	}
}

func TestChannelRendezvous(t *testing.T) {
	// main creates unbuffered channel, spawns sender, then receives
	// func0 (main):
	//   ch = channel(0)     ← unbuffered
	//   spawn func1 (which will send 42 to the channel)
	//   recv from ch
	//   print result
	var mainCode []byte
	codegen.EmitU16(&mainCode, codegen.OpChannelCreate, 0) // unbuffered
	codegen.EmitU16(&mainCode, codegen.OpStoreLocal, 0)

	// Store channel as global so spawned fiber can access it
	codegen.EmitU16(&mainCode, codegen.OpLoadLocal, 0)
	codegen.EmitU16(&mainCode, codegen.OpStoreGlobal, 5) // global slot 5

	codegen.EmitU16(&mainCode, codegen.OpSpawn, 1) // spawn sender
	codegen.EmitOp(&mainCode, codegen.OpPop)

	// Recv from channel
	codegen.EmitU16(&mainCode, codegen.OpLoadLocal, 0)
	codegen.EmitOp(&mainCode, codegen.OpChannelRecv)
	codegen.EmitOp(&mainCode, codegen.OpPrintln)

	codegen.EmitOp(&mainCode, codegen.OpConstUnit)
	codegen.EmitOp(&mainCode, codegen.OpReturn)

	// func1 (sender): send 42 to the channel
	var senderCode []byte
	codegen.EmitU16(&senderCode, codegen.OpLoadGlobal, 5) // load channel
	codegen.EmitI64(&senderCode, codegen.OpConstInt, 42)
	codegen.EmitOp(&senderCode, codegen.OpChannelSend)
	codegen.EmitOp(&senderCode, codegen.OpConstUnit)
	codegen.EmitOp(&senderCode, codegen.OpReturn)

	prog := buildProgram(
		testFunc{0, 1, mainCode},
		testFunc{0, 0, senderCode},
	)

	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "42" {
		t.Fatalf("expected 42, got %q", out)
	}
}

// ===========================================================================
// Deadlock detection
// ===========================================================================

func TestDeadlockDetection(t *testing.T) {
	// main tries to recv from an unbuffered channel with no sender → deadlock
	var code []byte
	codegen.EmitU16(&code, codegen.OpChannelCreate, 0) // unbuffered
	codegen.EmitOp(&code, codegen.OpChannelRecv)        // blocks forever
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	expectError(t, prog, "deadlock")
}

// ===========================================================================
// AllocArray
// ===========================================================================

func TestAllocArray(t *testing.T) {
	var code []byte
	codegen.EmitU16(&code, codegen.OpAllocArray, 3)
	codegen.EmitOp(&code, codegen.OpArrayLen)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "3" {
		t.Fatalf("expected 3, got %q", out)
	}
}

// ===========================================================================
// Value equality
// ===========================================================================

func TestValueEquality(t *testing.T) {
	heap := NewHeap()

	// Primitives.
	if !IntVal(42).Equal(IntVal(42), heap) {
		t.Error("42 == 42")
	}
	if IntVal(42).Equal(IntVal(43), heap) {
		t.Error("42 != 43")
	}
	if !BoolVal(true).Equal(BoolVal(true), heap) {
		t.Error("true == true")
	}
	if BoolVal(true).Equal(BoolVal(false), heap) {
		t.Error("true != false")
	}
	if !UnitVal().Equal(UnitVal(), heap) {
		t.Error("() == ()")
	}
	if !FloatVal(3.14).Equal(FloatVal(3.14), heap) {
		t.Error("3.14 == 3.14")
	}

	// Strings.
	s1 := ObjVal(heap.AllocString("hello"))
	s2 := ObjVal(heap.AllocString("hello"))
	s3 := ObjVal(heap.AllocString("world"))
	if !s1.Equal(s2, heap) {
		t.Error("'hello' == 'hello'")
	}
	if s1.Equal(s3, heap) {
		t.Error("'hello' != 'world'")
	}

	// Arrays.
	a1 := ObjVal(heap.AllocArray([]Value{IntVal(1), IntVal(2)}))
	a2 := ObjVal(heap.AllocArray([]Value{IntVal(1), IntVal(2)}))
	a3 := ObjVal(heap.AllocArray([]Value{IntVal(1), IntVal(3)}))
	if !a1.Equal(a2, heap) {
		t.Error("[1,2] == [1,2]")
	}
	if a1.Equal(a3, heap) {
		t.Error("[1,2] != [1,3]")
	}

	// Cross-type.
	if IntVal(42).Equal(FloatVal(42.0), heap) {
		t.Error("int(42) != float(42.0) due to different tags")
	}
}

// ===========================================================================
// Fiber states
// ===========================================================================

func TestFiberStates(t *testing.T) {
	f := NewFiber(0)
	if f.State != FiberSuspended {
		t.Fatalf("expected suspended, got %v", f.State)
	}
	f.State = FiberRunning
	if f.State.String() != "running" {
		t.Fatalf("expected 'running', got %q", f.State.String())
	}
	f.State = FiberBlocked
	if f.State.String() != "blocked" {
		t.Fatalf("expected 'blocked', got %q", f.State.String())
	}
	f.State = FiberDead
	if f.State.String() != "dead" {
		t.Fatalf("expected 'dead', got %q", f.State.String())
	}
}

// ===========================================================================
// Scheduler
// ===========================================================================

func TestSchedulerRoundRobin(t *testing.T) {
	sched := NewScheduler(100)
	f1 := sched.NewFiber()
	f2 := sched.NewFiber()
	sched.Ready(f1)
	sched.Ready(f2)

	got1 := sched.Next()
	if got1 != f1 {
		t.Fatal("expected f1 first")
	}
	got2 := sched.Next()
	if got2 != f2 {
		t.Fatal("expected f2 second")
	}
	got3 := sched.Next()
	if got3 != nil {
		t.Fatal("expected nil when queue empty")
	}
}

func TestSchedulerDeadlock(t *testing.T) {
	sched := NewScheduler(100)
	f1 := sched.NewFiber()
	f1.State = FiberBlocked

	if !sched.DeadlockDetected() {
		t.Fatal("expected deadlock when all fibers blocked")
	}
}

// ===========================================================================
// Heap
// ===========================================================================

func TestHeapAlloc(t *testing.T) {
	h := NewHeap()
	idx := h.AllocString("hello")
	obj := h.Get(idx)
	s := obj.Data.(*StringObj)
	if s.Value != "hello" {
		t.Fatalf("expected 'hello', got %q", s.Value)
	}
	if h.Len() != 1 {
		t.Fatalf("expected 1 object, got %d", h.Len())
	}
}

// ===========================================================================
// RuntimeError formatting
// ===========================================================================

func TestRuntimeErrorFormat(t *testing.T) {
	e := &RuntimeError{
		Message: "test error",
		Line:    10,
		Col:     5,
		StackTrace: []StackFrame{
			{FuncName: "foo", Line: 10, Col: 5},
			{FuncName: "main", Line: 1, Col: 1},
		},
	}
	s := e.Error()
	if !strings.Contains(s, "10:5") {
		t.Fatalf("missing source location in %q", s)
	}
	if !strings.Contains(s, "at foo") {
		t.Fatalf("missing function name in %q", s)
	}
	if !strings.Contains(s, "at main") {
		t.Fatalf("missing main in %q", s)
	}
}

// ===========================================================================
// NaN and special float values
// ===========================================================================

func TestFloatSpecialValues(t *testing.T) {
	var code []byte
	codegen.EmitF64(&code, codegen.OpConstFloat, math.Inf(1))
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "+Inf" {
		t.Fatalf("expected +Inf, got %q", out)
	}
}

// ===========================================================================
// JumpTable
// ===========================================================================

func TestJumpTable(t *testing.T) {
	var code []byte
	// Push index 1
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	// JumpTable with 3 entries
	// Entry 0: offset to "print 0"
	// Entry 1: offset to "print 1"
	// Entry 2: offset to "print 2"
	// Base IP = past the jump table data

	// We need to calculate offsets relative to baseIP.
	// The jump table structure: opcode(1) + count(2) + 3*offset(6) = 9 bytes
	// baseIP = start of jump table + 9
	// We'll place the print blocks right after the table.

	jtStart := len(code)
	codegen.EmitJumpTable(&code, []int16{0, 0, 0}) // placeholders
	baseIP := len(code) // right after the jump table

	// Block 0: print 0
	block0 := len(code)
	codegen.EmitI64(&code, codegen.OpConstInt, 0)
	codegen.EmitOp(&code, codegen.OpPrintln)
	end0 := len(code)
	codegen.EmitI16(&code, codegen.OpJump, 0)

	// Block 1: print 1
	block1 := len(code)
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	codegen.EmitOp(&code, codegen.OpPrintln)
	end1 := len(code)
	codegen.EmitI16(&code, codegen.OpJump, 0)

	// Block 2: print 2
	block2 := len(code)
	codegen.EmitI64(&code, codegen.OpConstInt, 2)
	codegen.EmitOp(&code, codegen.OpPrintln)

	// End
	endLabel := len(code)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	// Patch jump table offsets (relative to baseIP)
	// Entry at jtStart+3 (after opcode + count u16)
	off0 := int16(block0 - baseIP)
	off1 := int16(block1 - baseIP)
	off2 := int16(block2 - baseIP)
	code[jtStart+3] = byte(uint16(off0))
	code[jtStart+4] = byte(uint16(off0) >> 8)
	code[jtStart+5] = byte(uint16(off1))
	code[jtStart+6] = byte(uint16(off1) >> 8)
	code[jtStart+7] = byte(uint16(off2))
	code[jtStart+8] = byte(uint16(off2) >> 8)

	// Patch end jumps
	rel0 := int16(endLabel - (end0 + 3))
	code[end0+1] = byte(uint16(rel0))
	code[end0+2] = byte(uint16(rel0) >> 8)
	rel1 := int16(endLabel - (end1 + 3))
	code[end1+1] = byte(uint16(rel1))
	code[end1+2] = byte(uint16(rel1) >> 8)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "1" {
		t.Fatalf("expected 1, got %q", out)
	}
}

// ===========================================================================
// Breakpoint (no-op)
// ===========================================================================

func TestBreakpoint(t *testing.T) {
	var code []byte
	codegen.EmitOp(&code, codegen.OpBreakpoint)
	codegen.EmitI64(&code, codegen.OpConstInt, 1)
	codegen.EmitOp(&code, codegen.OpPrintln)
	codegen.EmitOp(&code, codegen.OpConstUnit)
	codegen.EmitOp(&code, codegen.OpReturn)

	prog := buildProgram(testFunc{0, 0, code})
	vm := mustRun(t, prog)
	if out := strings.TrimSpace(getOutput(vm)); out != "1" {
		t.Fatalf("expected 1, got %q", out)
	}
}
