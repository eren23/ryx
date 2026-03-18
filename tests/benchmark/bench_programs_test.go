package benchmark

import (
	"bytes"
	"testing"

	"github.com/ryx-lang/ryx/pkg/codegen"
	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/hir"
	"github.com/ryx-lang/ryx/pkg/lexer"
	"github.com/ryx-lang/ryx/pkg/mir"
	"github.com/ryx-lang/ryx/pkg/optimize"
	"github.com/ryx-lang/ryx/pkg/parser"
	"github.com/ryx-lang/ryx/pkg/resolver"
	"github.com/ryx-lang/ryx/pkg/types"
	"github.com/ryx-lang/ryx/pkg/vm"
)

const helloSource = `
fn greet(name: String) -> String {
    "Hello, " ++ name ++ "!"
}

fn main() {
    println(greet("World"));
    println(greet("Ryx"));
    let x = 42;
    println(x);
    let is_even = x % 2 == 0;
    println(is_even)
}
`

const fibSource = `
fn fib(n: Int) -> Int {
    if n < 2 {
        n
    } else {
        fib(n - 1) + fib(n - 2)
    }
}

fn main() {
    println(fib(25))
}
`

// compileAndRun is a helper that compiles Ryx source end-to-end and runs it
// in the VM, discarding stdout. It fails the benchmark on any error.
func compileAndRun(b *testing.B, src string) {
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

	v := vm.NewVM(compiled)
	v.Stdout = &bytes.Buffer{}
	if err := v.Run(); err != nil {
		b.Fatalf("VM runtime error: %v", err)
	}
}

// BenchmarkEndToEndHello benchmarks the full compile + run cycle for the
// hello world program.
func BenchmarkEndToEndHello(b *testing.B) {
	for i := 0; i < b.N; i++ {
		compileAndRun(b, helloSource)
	}
}

// BenchmarkEndToEndFibonacci benchmarks the full compile + run cycle for a
// recursive fibonacci computation.
func BenchmarkEndToEndFibonacci(b *testing.B) {
	for i := 0; i < b.N; i++ {
		compileAndRun(b, fibSource)
	}
}

// BenchmarkLexerPhase benchmarks just the lexer phase on the fibonacci source
// to isolate tokenization cost.
func BenchmarkLexerPhase(b *testing.B) {
	src := fibSource
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l := lexer.New(src, 0)
		l.Tokenize()
	}
}

// BenchmarkParserPhase benchmarks just the parser phase on the fibonacci
// source to isolate parsing cost.
func BenchmarkParserPhase(b *testing.B) {
	src := fibSource
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.Parse(src, 0)
	}
}

// BenchmarkTypecheckPhase benchmarks the type checker phase on the fibonacci
// source, reusing the parsed and resolved AST from setup.
func BenchmarkTypecheckPhase(b *testing.B) {
	src := fibSource
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		types.Check(parseResult.Program, resolveResult, registry)
	}
}

// BenchmarkCodegenPhase benchmarks the codegen phase on the fibonacci source,
// reusing the MIR from setup.
func BenchmarkCodegenPhase(b *testing.B) {
	src := fibSource
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := codegen.Generate(mirProg)
		if err != nil {
			b.Fatalf("codegen error: %v", err)
		}
	}
}

// BenchmarkHIRLowerPhase benchmarks the HIR lowering phase on the fibonacci
// source, reusing the parsed, resolved, and type-checked AST from setup.
func BenchmarkHIRLowerPhase(b *testing.B) {
	src := fibSource
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hir.Lower(parseResult.Program, checkResult, resolveResult, registry)
	}
}
