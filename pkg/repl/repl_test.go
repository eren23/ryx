package repl

import (
	"bytes"
	"strings"
	"testing"
)

// newTestREPL creates a REPL with string-based I/O for testing.
func newTestREPL(input string) (*REPL, *bytes.Buffer, *bytes.Buffer) {
	stdin := strings.NewReader(input)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	r := NewWithIO(Options{OptLevel: 0, MonomorphizeLimit: 64}, stdin, stdout, stderr)
	return r, stdout, stderr
}

func TestBraceDepth(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"{", 1},
		{"}", -1},
		{"{ }", 0},
		{"fn foo() {", 1},
		{`"{"`, 0},           // brace inside string
		{`{ "}" }`, 0},      // closing brace inside string
		{"{{ }}", 0},
		{"{ { {", 3},
	}
	for _, tt := range tests {
		got := braceDepth(tt.input)
		if got != tt.want {
			t.Errorf("braceDepth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestIsTopLevelDef(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"fn foo() {}", true},
		{"pub fn bar() {}", true},
		{"type Option { Some(Int), None }", true},
		{"struct Point { x: Int, y: Int }", true},
		{"trait Printable { fn print(self) }", true},
		{"impl Printable for Point { fn print(self) {} }", true},
		{"import std::io", true},
		{"let x = 42", false},
		{"println(42)", false},
		{"1 + 2", false},
	}
	for _, tt := range tests {
		got := isTopLevelDef(tt.input)
		if got != tt.want {
			t.Errorf("isTopLevelDef(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestREPLQuit(t *testing.T) {
	r, stdout, _ := newTestREPL(":quit\n")
	r.Run()

	out := stdout.String()
	if !strings.Contains(out, "Ryx REPL") {
		t.Error("expected REPL banner in output")
	}
}

func TestREPLHelp(t *testing.T) {
	r, stdout, _ := newTestREPL(":help\n:quit\n")
	r.Run()

	out := stdout.String()
	if !strings.Contains(out, ":quit") {
		t.Error("expected :quit in help output")
	}
	if !strings.Contains(out, ":type") {
		t.Error("expected :type in help output")
	}
}

func TestREPLReset(t *testing.T) {
	r, stdout, _ := newTestREPL(":reset\n:quit\n")
	r.Run()

	out := stdout.String()
	if !strings.Contains(out, "State cleared") {
		t.Error("expected 'State cleared' after :reset")
	}
}

func TestREPLUnknownCommand(t *testing.T) {
	r, _, stderr := newTestREPL(":foo\n:quit\n")
	r.Run()

	errOut := stderr.String()
	if !strings.Contains(errOut, "unknown command") {
		t.Error("expected unknown command error")
	}
}

func TestREPLShowAST(t *testing.T) {
	r, stdout, _ := newTestREPL(":ast 1 + 2\n:quit\n")
	r.Run()

	out := stdout.String()
	if !strings.Contains(out, "Binary") || !strings.Contains(out, "Int") {
		t.Errorf("expected AST output with Binary and Int, got: %s", out)
	}
}

func TestREPLEmptyInput(t *testing.T) {
	// Empty lines should not cause errors.
	r, _, stderr := newTestREPL("\n\n:quit\n")
	r.Run()

	errOut := stderr.String()
	if errOut != "" {
		t.Errorf("expected no errors on empty input, got: %s", errOut)
	}
}

func TestREPLMultiLineInput(t *testing.T) {
	// Simulates a multi-line input where braces don't close on first line.
	input := "fn add(a: Int, b: Int) -> Int {\n  a + b\n}\n:quit\n"
	r, _, stderr := newTestREPL(input)
	r.Run()

	errOut := stderr.String()
	if errOut != "" {
		t.Errorf("expected no errors for multi-line fn def, got: %s", errOut)
	}
}

func TestREPLBuildSourceExpression(t *testing.T) {
	r := New(Options{})

	src := r.buildSource("1 + 2")
	if !strings.Contains(src, "fn main()") {
		t.Error("expression should be wrapped in main()")
	}
	if !strings.Contains(src, "1 + 2") {
		t.Error("expression should appear in source")
	}
}

func TestREPLBuildSourceTopLevel(t *testing.T) {
	r := New(Options{})

	src := r.buildSource("fn add(a: Int, b: Int) -> Int { a + b }")
	if !strings.Contains(src, "fn add") {
		t.Error("top-level def should appear directly")
	}
	if !strings.Contains(src, "fn main()") {
		t.Error("should have a stub main function")
	}
}

func TestREPLHistoryAccumulation(t *testing.T) {
	r := New(Options{})

	// Simulate adding a function definition.
	r.history = append(r.history, "fn double(x: Int) -> Int { x * 2 }")
	src := r.buildSource("println(double(5))")

	if !strings.Contains(src, "fn double") {
		t.Error("history should appear in built source")
	}
	if !strings.Contains(src, "println(double(5))") {
		t.Error("current input should appear in built source")
	}
}

func TestREPLBuildExprSource(t *testing.T) {
	r := New(Options{})
	r.history = []string{"fn foo() -> Int { 42 }"}

	src := r.buildExprSource("foo()")
	if !strings.Contains(src, "fn foo()") {
		t.Error("history should appear in expr source")
	}
	if !strings.Contains(src, "fn main()") {
		t.Error("should wrap in main")
	}
	if !strings.Contains(src, "foo()") {
		t.Error("expression should appear in source")
	}
}
