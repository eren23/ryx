package benchmark

import (
	"bytes"
	"testing"

	"github.com/ryx-lang/ryx/pkg/codegen"
	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/hir"
	"github.com/ryx-lang/ryx/pkg/mir"
	"github.com/ryx-lang/ryx/pkg/optimize"
	"github.com/ryx-lang/ryx/pkg/parser"
	"github.com/ryx-lang/ryx/pkg/resolver"
	"github.com/ryx-lang/ryx/pkg/types"
	"github.com/ryx-lang/ryx/pkg/vm"
)

// compileSource runs the full Ryx compiler pipeline on the given source code
// and returns the compiled program. It fails the benchmark if any compiler
// phase produces errors.
func compileSource(b *testing.B, src string) *codegen.CompiledProgram {
	b.Helper()

	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("bench.ryx", src)

	parseResult := parser.Parse(src, fileID)
	if parseResult.HasErrors() {
		b.Fatalf("parse errors: %v", parseResult.Errors)
	}

	resolveResult := resolver.Resolve(parseResult.Program, registry)
	for _, d := range resolveResult.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			b.Fatalf("resolve error: %s", d.Message)
		}
	}

	checkResult := types.Check(parseResult.Program, resolveResult, registry)
	if checkResult.HasErrors() {
		b.Fatalf("type check errors: %v", checkResult.Diagnostics)
	}

	lowerResult := hir.Lower(parseResult.Program, checkResult, resolveResult, registry)
	if lowerResult.HasErrors() {
		b.Fatalf("HIR lowering errors: %v", lowerResult.Diagnostics)
	}

	mirProg := mir.Build(lowerResult.Program)
	optimize.Pipeline(mirProg, optimize.O1)

	compiled, err := codegen.Generate(mirProg)
	if err != nil {
		b.Fatalf("codegen error: %v", err)
	}

	return compiled
}

// runProgram creates a VM, runs the compiled program, and discards output.
func runProgram(b *testing.B, prog *codegen.CompiledProgram) {
	b.Helper()
	v := vm.NewVM(prog)
	v.Stdout = &bytes.Buffer{} // suppress output
	if err := v.Run(); err != nil {
		b.Fatalf("VM runtime error: %v", err)
	}
}

// BenchmarkVMDispatch compiles and runs a tight integer loop to measure raw
// VM dispatch overhead.
func BenchmarkVMDispatch(b *testing.B) {
	src := `
fn main() {
    let mut i = 0;
    while i < 1000000 {
        i = i + 1;
    };
    println(i)
}
`
	prog := compileSource(b, src)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runProgram(b, prog)
	}
}

// BenchmarkFibonacci compiles and runs a recursive fibonacci(30) to measure
// function call overhead and recursion performance.
func BenchmarkFibonacci(b *testing.B) {
	src := `
fn fib(n: Int) -> Int {
    if n < 2 {
        n
    } else {
        fib(n - 1) + fib(n - 2)
    }
}

fn main() {
    println(fib(30))
}
`
	prog := compileSource(b, src)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runProgram(b, prog)
	}
}

// BenchmarkQuicksort compiles and runs a quicksort implementation on a
// 100-element array to measure array access and sorting performance.
func BenchmarkQuicksort(b *testing.B) {
	src := `
fn swap(arr: [Int], i: Int, j: Int) {
    let tmp = arr[i];
    arr[i] = arr[j];
    arr[j] = tmp;
}

fn partition(arr: [Int], lo: Int, hi: Int) -> Int {
    let pivot = arr[hi];
    let mut i = lo;
    let mut j = lo;
    while j < hi {
        if arr[j] < pivot {
            swap(arr, i, j);
            i = i + 1;
        };
        j = j + 1;
    };
    swap(arr, i, hi);
    i
}

fn quicksort(arr: [Int], lo: Int, hi: Int) {
    if lo < hi {
        let p = partition(arr, lo, hi);
        quicksort(arr, lo, p - 1);
        quicksort(arr, p + 1, hi);
    }
}

fn main() {
    // Build a reversed array of 100 elements
    let mut arr = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
                   0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
                   0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
                   0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
                   0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
                   0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
                   0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
                   0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
                   0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
                   0, 0, 0, 0, 0, 0, 0, 0, 0, 0];
    let mut i = 0;
    while i < 100 {
        arr[i] = 100 - i;
        i = i + 1;
    };
    quicksort(arr, 0, 99);
    println(arr[0])
}
`
	prog := compileSource(b, src)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runProgram(b, prog)
	}
}

// BenchmarkArithmetic compiles and runs a program that performs intensive
// integer arithmetic to measure ALU instruction throughput.
func BenchmarkArithmetic(b *testing.B) {
	src := `
fn main() {
    let mut sum = 0;
    let mut i = 1;
    while i <= 10000 {
        sum = sum + i * i - i / 2 + i % 3;
        i = i + 1;
    };
    println(sum)
}
`
	prog := compileSource(b, src)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runProgram(b, prog)
	}
}
