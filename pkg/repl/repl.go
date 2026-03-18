package repl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

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

// Options configures the REPL behaviour.
type Options struct {
	OptLevel          int
	MonomorphizeLimit int
	DumpAST           bool
	DumpHIR           bool
	DumpMIR           bool
	DumpBytecode      bool
}

// REPL is an interactive read-evaluate-print loop for Ryx.
// It maintains persistent state across inputs by accumulating
// top-level definitions.
type REPL struct {
	opts Options

	// Accumulated source: all successfully evaluated top-level items.
	history []string
	// inputCount tracks how many inputs have been evaluated (for prompts).
	inputCount int

	// I/O
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// New creates a REPL with the given options.
func New(opts Options) *REPL {
	if opts.MonomorphizeLimit <= 0 {
		opts.MonomorphizeLimit = 64
	}
	return &REPL{
		opts:   opts,
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

// NewWithIO creates a REPL with custom I/O streams (useful for testing).
func NewWithIO(opts Options, stdin io.Reader, stdout, stderr io.Writer) *REPL {
	r := New(opts)
	r.stdin = stdin
	r.stdout = stdout
	r.stderr = stderr
	return r
}

// Run starts the interactive REPL loop.
func (r *REPL) Run() {
	fmt.Fprintln(r.stdout, "Ryx REPL (type :quit to exit, :help for commands)")

	scanner := bufio.NewScanner(r.stdin)
	for {
		// Print prompt.
		fmt.Fprint(r.stdout, "ryx> ")

		line, ok := r.readInput(scanner)
		if !ok {
			fmt.Fprintln(r.stdout) // newline on EOF
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Handle special commands.
		if strings.HasPrefix(line, ":") {
			if r.handleCommand(line) {
				break // :quit
			}
			continue
		}

		r.Execute(line)
	}
}

// Execute compiles and runs a single input string. Exported for testing.
func (r *REPL) Execute(input string) {
	r.execute(input)
}

// readInput reads a possibly multi-line input. If the line ends with an
// open brace, or braces are unbalanced, it continues reading with a
// continuation prompt until braces balance.
func (r *REPL) readInput(scanner *bufio.Scanner) (string, bool) {
	if !scanner.Scan() {
		return "", false
	}
	line := scanner.Text()

	depth := braceDepth(line)
	if depth <= 0 {
		return line, true
	}

	// Multi-line: keep reading until braces balance.
	var buf strings.Builder
	buf.WriteString(line)
	for depth > 0 {
		fmt.Fprint(r.stdout, "  .. ")
		if !scanner.Scan() {
			return buf.String(), true
		}
		next := scanner.Text()
		buf.WriteByte('\n')
		buf.WriteString(next)
		depth += braceDepth(next)
	}
	return buf.String(), true
}

// braceDepth returns the net brace depth change for a line.
func braceDepth(s string) int {
	depth := 0
	inStr := false
	prev := rune(0)
	for _, ch := range s {
		if ch == '"' && prev != '\\' {
			inStr = !inStr
		}
		if !inStr {
			switch ch {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		prev = ch
	}
	return depth
}

// handleCommand processes a :command. Returns true if the REPL should exit.
func (r *REPL) handleCommand(input string) bool {
	parts := strings.Fields(input)
	cmd := parts[0]
	arg := ""
	if len(parts) > 1 {
		arg = strings.Join(parts[1:], " ")
	}

	switch cmd {
	case ":quit", ":q", ":exit":
		return true

	case ":help", ":h":
		fmt.Fprintln(r.stdout, "Commands:")
		fmt.Fprintln(r.stdout, "  :quit       Exit the REPL")
		fmt.Fprintln(r.stdout, "  :type <expr>     Show the type of an expression")
		fmt.Fprintln(r.stdout, "  :ast <expr>      Show the AST of an expression")
		fmt.Fprintln(r.stdout, "  :bytecode <expr> Show bytecode for an expression")
		fmt.Fprintln(r.stdout, "  :reset           Clear accumulated state")
		fmt.Fprintln(r.stdout, "  :help            Show this help")

	case ":reset":
		r.history = nil
		r.inputCount = 0
		fmt.Fprintln(r.stdout, "State cleared.")

	case ":type":
		if arg == "" {
			fmt.Fprintln(r.stderr, "usage: :type <expression>")
		} else {
			r.showType(arg)
		}

	case ":ast":
		if arg == "" {
			fmt.Fprintln(r.stderr, "usage: :ast <expression>")
		} else {
			r.showAST(arg)
		}

	case ":bytecode":
		if arg == "" {
			fmt.Fprintln(r.stderr, "usage: :bytecode <expression>")
		} else {
			r.showBytecode(arg)
		}

	default:
		fmt.Fprintf(r.stderr, "unknown command: %s (type :help)\n", cmd)
	}

	return false
}

// execute compiles and runs a single input in the context of accumulated history.
func (r *REPL) execute(input string) {
	// Build full source: accumulated definitions + current input wrapped in main.
	fullSrc := r.buildSource(input)

	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("<repl>", fullSrc)

	// Parse.
	result := parser.Parse(fullSrc, fileID)
	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			fmt.Fprintf(r.stderr, "parse error: %s\n", e.Message)
		}
		return
	}

	if r.opts.DumpAST {
		fmt.Fprintln(r.stdout, parser.FormatAST(result.Program))
	}

	// Resolve.
	resolved := resolver.Resolve(result.Program, registry)
	if hasDiagErrors(resolved.Diagnostics) {
		r.printDiags(resolved.Diagnostics)
		return
	}

	// Type check.
	checkResult := types.Check(result.Program, resolved, registry)
	if checkResult.HasErrors() {
		r.printDiags(checkResult.Diagnostics)
		return
	}

	// HIR lower.
	lowerResult := hir.Lower(result.Program, checkResult, resolved, registry)
	if lowerResult.HasErrors() {
		r.printDiags(lowerResult.Diagnostics)
		return
	}

	// Monomorphize.
	monoResult := hir.Monomorphize(lowerResult.Program, r.opts.MonomorphizeLimit)
	if hasDiagErrors(monoResult.Diagnostics) {
		r.printDiags(monoResult.Diagnostics)
		return
	}

	if r.opts.DumpHIR {
		for _, fn := range monoResult.Program.Functions {
			fmt.Fprintf(r.stdout, "hir fn %s(%d params) -> %s\n", fn.Name, len(fn.Params), fn.ReturnType)
		}
	}

	// MIR build.
	mirProg := mir.Build(monoResult.Program)

	// Optimize.
	optimize.Pipeline(mirProg, optimize.OptLevel(r.opts.OptLevel))

	if r.opts.DumpMIR {
		fmt.Fprint(r.stdout, mir.Print(mirProg))
	}

	// Codegen.
	compiled, err := codegen.Generate(mirProg)
	if err != nil {
		fmt.Fprintf(r.stderr, "codegen error: %v\n", err)
		return
	}

	if r.opts.DumpBytecode {
		fmt.Fprint(r.stdout, codegen.DisassembleProgram(compiled))
	}

	// Run in VM.
	machine := vm.NewVM(compiled)
	machine.Stdout = r.stdout

	if err := machine.Run(); err != nil {
		fmt.Fprintf(r.stderr, "runtime error: %v\n", err)
		return
	}

	// Success — record top-level definitions for future context.
	if isTopLevelDef(input) {
		r.history = append(r.history, input)
	}
	r.inputCount++
}

// buildSource constructs the full program source from accumulated history
// and the current input. Expressions are wrapped in a main function.
func (r *REPL) buildSource(input string) string {
	var sb strings.Builder
	for _, h := range r.history {
		sb.WriteString(h)
		sb.WriteByte('\n')
	}

	if isTopLevelDef(input) {
		// Top-level definition: add it directly.
		sb.WriteString(input)
		sb.WriteByte('\n')
		// Need a main function for the program to be valid.
		sb.WriteString("fn main() {}\n")
	} else {
		// Expression or statement: wrap in main.
		sb.WriteString("fn main() {\n  ")
		sb.WriteString(input)
		sb.WriteString("\n}\n")
	}

	return sb.String()
}

// isTopLevelDef returns true if the input looks like a top-level definition.
func isTopLevelDef(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "fn ") ||
		strings.HasPrefix(trimmed, "pub fn ") ||
		strings.HasPrefix(trimmed, "type ") ||
		strings.HasPrefix(trimmed, "pub type ") ||
		strings.HasPrefix(trimmed, "struct ") ||
		strings.HasPrefix(trimmed, "pub struct ") ||
		strings.HasPrefix(trimmed, "trait ") ||
		strings.HasPrefix(trimmed, "pub trait ") ||
		strings.HasPrefix(trimmed, "impl ") ||
		strings.HasPrefix(trimmed, "import ") ||
		strings.HasPrefix(trimmed, "module ")
}

// showType parses an expression and shows its inferred type.
func (r *REPL) showType(input string) {
	src := r.buildExprSource(input)
	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("<repl>", src)

	result := parser.Parse(src, fileID)
	if len(result.Errors) > 0 {
		fmt.Fprintf(r.stderr, "parse error: %s\n", result.Errors[0].Message)
		return
	}

	resolved := resolver.Resolve(result.Program, registry)
	if hasDiagErrors(resolved.Diagnostics) {
		r.printDiags(resolved.Diagnostics)
		return
	}

	checkResult := types.Check(result.Program, resolved, registry)
	if checkResult.HasErrors() {
		r.printDiags(checkResult.Diagnostics)
		return
	}

	// Find the type of the expression inside main's body.
	for _, item := range result.Program.Items {
		if fn, ok := item.(*parser.FnDef); ok && fn.Name == "main" && fn.Body != nil {
			if fn.Body.TrailingExpr != nil {
				if t, ok := checkResult.NodeTypes[fn.Body.TrailingExpr.Span()]; ok {
					fmt.Fprintln(r.stdout, t.String())
					return
				}
			}
		}
	}
	fmt.Fprintln(r.stdout, "Unit")
}

// showAST parses an expression and prints its AST.
func (r *REPL) showAST(input string) {
	expr, errs := parser.ParseExpr(input, 0)
	if len(errs) > 0 {
		fmt.Fprintf(r.stderr, "parse error: %s\n", errs[0].Message)
		return
	}
	fmt.Fprintln(r.stdout, parser.FormatAST(expr))
}

// showBytecode compiles an expression and shows its bytecode.
func (r *REPL) showBytecode(input string) {
	src := r.buildExprSource(input)
	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("<repl>", src)

	result := parser.Parse(src, fileID)
	if len(result.Errors) > 0 {
		fmt.Fprintf(r.stderr, "parse error: %s\n", result.Errors[0].Message)
		return
	}

	resolved := resolver.Resolve(result.Program, registry)
	if hasDiagErrors(resolved.Diagnostics) {
		r.printDiags(resolved.Diagnostics)
		return
	}

	checkResult := types.Check(result.Program, resolved, registry)
	if checkResult.HasErrors() {
		r.printDiags(checkResult.Diagnostics)
		return
	}

	lowerResult := hir.Lower(result.Program, checkResult, resolved, registry)
	if lowerResult.HasErrors() {
		r.printDiags(lowerResult.Diagnostics)
		return
	}

	monoResult := hir.Monomorphize(lowerResult.Program, r.opts.MonomorphizeLimit)
	if hasDiagErrors(monoResult.Diagnostics) {
		r.printDiags(monoResult.Diagnostics)
		return
	}

	mirProg := mir.Build(monoResult.Program)
	optimize.Pipeline(mirProg, optimize.OptLevel(r.opts.OptLevel))

	compiled, err := codegen.Generate(mirProg)
	if err != nil {
		fmt.Fprintf(r.stderr, "codegen error: %v\n", err)
		return
	}

	fmt.Fprint(r.stdout, codegen.DisassembleProgram(compiled))
}

// buildExprSource wraps an expression in a main function with the accumulated history.
func (r *REPL) buildExprSource(expr string) string {
	var sb strings.Builder
	for _, h := range r.history {
		sb.WriteString(h)
		sb.WriteByte('\n')
	}
	sb.WriteString("fn main() {\n  ")
	sb.WriteString(expr)
	sb.WriteString("\n}\n")
	return sb.String()
}

func (r *REPL) printDiags(diags []diagnostic.Diagnostic) {
	for _, d := range diags {
		if d.Severity == diagnostic.SeverityError {
			fmt.Fprintf(r.stderr, "%s[%s]: %s\n", d.Severity, d.Code, d.Message)
		}
	}
}

func hasDiagErrors(diags []diagnostic.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diagnostic.SeverityError {
			return true
		}
	}
	return false
}
