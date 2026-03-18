package integration

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Regression tests
//
// Each test documents a specific bug that was discovered and fixed. Tests
// are named with the pattern TestRegression_<description> to make it clear
// what each test guards against.
// ---------------------------------------------------------------------------

// TestRegression_EmptyMain ensures that an empty main function compiles and
// runs without error.
func TestRegression_EmptyMain(t *testing.T) {
	src := `
fn main() {
}
`
	_, err := compileAndRun(src, "empty_main.ryx")
	if err != nil {
		t.Fatalf("empty main should not error: %v", err)
	}
}

// TestRegression_NestedIfElse verifies that deeply nested if/else chains
// compile and evaluate correctly.
func TestRegression_NestedIfElse(t *testing.T) {
	src := `
fn classify(n: Int) -> Int {
    if n < 0 {
        -1
    } else if n == 0 {
        0
    } else if n < 10 {
        1
    } else if n < 100 {
        2
    } else {
        3
    }
}

fn main() {
    println(classify(-5));
    println(classify(0));
    println(classify(5));
    println(classify(50));
    println(classify(200))
}
`
	got, err := compileAndRun(src, "nested_if.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "-1\n0\n1\n2\n3\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestRegression_MutVariableReassignment verifies that mutable variables can
// be reassigned in loops.
func TestRegression_MutVariableReassignment(t *testing.T) {
	src := `
fn main() {
    let mut x = 0;
    let mut i = 0;
    while i < 10 {
        x = x + i;
        i = i + 1;
    };
    println(x)
}
`
	got, err := compileAndRun(src, "mut_reassign.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "45\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestRegression_NegativeNumbers verifies that negative number literals and
// unary negation work correctly.
func TestRegression_NegativeNumbers(t *testing.T) {
	src := `
fn main() {
    let x = -42;
    let y = 0 - 10;
    println(x);
    println(y);
    println(x + y)
}
`
	got, err := compileAndRun(src, "negatives.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "-42\n-10\n-52\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestRegression_BooleanLogic verifies short-circuit evaluation of && and ||.
func TestRegression_BooleanLogic(t *testing.T) {
	src := `
fn main() {
    let a = true && true;
    let b = true && false;
    let c = false || true;
    let d = false || false;
    println(a);
    println(b);
    println(c);
    println(d)
}
`
	got, err := compileAndRun(src, "boolean_logic.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "true\nfalse\ntrue\nfalse\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestRegression_ArrayMutation verifies that array elements can be mutated
// via index assignment.
func TestRegression_ArrayMutation(t *testing.T) {
	src := `
fn main() {
    let mut arr = [10, 20, 30];
    arr[1] = 99;
    println(arr[0]);
    println(arr[1]);
    println(arr[2])
}
`
	got, err := compileAndRun(src, "array_mutation.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "10\n99\n30\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestRegression_FunctionAsValue verifies that functions can be passed as
// values and called indirectly.
func TestRegression_FunctionAsValue(t *testing.T) {
	src := `
fn double(x: Int) -> Int {
    x * 2
}

fn apply(f: (Int) -> Int, x: Int) -> Int {
    f(x)
}

fn main() {
    println(apply(double, 21))
}
`
	got, err := compileAndRun(src, "fn_as_value.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "42\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestRegression_MultipleReturnPaths verifies that functions with multiple
// return paths through if/else work correctly.
func TestRegression_MultipleReturnPaths(t *testing.T) {
	src := `
fn abs(x: Int) -> Int {
    if x < 0 { 0 - x } else { x }
}

fn main() {
    println(abs(5));
    println(abs(-5));
    println(abs(0))
}
`
	got, err := compileAndRun(src, "multi_return.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "5\n5\n0\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestRegression_WhileLoopBreak verifies break statement in while loops.
func TestRegression_WhileLoopBreak(t *testing.T) {
	src := `
fn main() {
    let mut sum = 0;
    let mut i = 0;
    while true {
        if i >= 5 { break };
        sum = sum + i;
        i = i + 1;
    };
    println(sum)
}
`
	got, err := compileAndRun(src, "while_break.ryx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "10\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestRegression_CompileErrors verifies that known invalid programs produce
// compile errors rather than crashing.
func TestRegression_CompileErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // substring expected in error
	}{
		{
			name: "undeclared_variable",
			src:  `fn main() { println(undefined_var) }`,
			want: "undeclared",
		},
		{
			name: "missing_main",
			src:  `fn foo() { println(42) }`,
			want: "main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := compileAndRun(tt.src, tt.name+".ryx")
			if err == nil {
				t.Fatal("expected compilation error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Errorf("expected error containing %q, got: %v", tt.want, err)
			}
		})
	}
}
