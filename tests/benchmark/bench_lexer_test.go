package benchmark

import (
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/lexer"
)

// BenchmarkLexerKeywords tokenizes a source file consisting mainly of keywords
// to measure keyword lookup performance in the lexer.
func BenchmarkLexerKeywords(b *testing.B) {
	// Build a source string with many keyword occurrences.
	keywords := []string{
		"fn", "let", "mut", "if", "else", "match", "for", "while",
		"loop", "break", "continue", "return", "type", "struct",
		"trait", "impl", "spawn", "channel", "in", "as", "pub",
		"import", "module", "self", "Self", "true", "false",
	}
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		for _, kw := range keywords {
			sb.WriteString(kw)
			sb.WriteByte(' ')
		}
		sb.WriteByte('\n')
	}
	src := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l := lexer.New(src, 0)
		l.Tokenize()
	}
}

// BenchmarkLexerLiterals tokenizes a source file consisting of numeric and
// string literals to measure literal scanning performance.
func BenchmarkLexerLiterals(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("42 ")
		sb.WriteString("3.14159 ")
		sb.WriteString("0xFF ")
		sb.WriteString("0b10101010 ")
		sb.WriteString("0o777 ")
		sb.WriteString("1_000_000 ")
		sb.WriteString("2.5e-3 ")
		sb.WriteString(`"hello world" `)
		sb.WriteString(`"escape\nnewline\ttab" `)
		sb.WriteString("'a' ")
		sb.WriteString(`'\n' `)
		sb.WriteByte('\n')
	}
	src := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l := lexer.New(src, 0)
		l.Tokenize()
	}
}

// BenchmarkLexerLargeFile tokenizes a large synthetic Ryx source file to
// measure overall lexer throughput on a realistic workload.
func BenchmarkLexerLargeFile(b *testing.B) {
	src := generateLargeSource(500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l := lexer.New(src, 0)
		l.Tokenize()
	}
}

// BenchmarkLexerOperators tokenizes a source file consisting mainly of
// operator tokens to measure multi-character operator scanning.
func BenchmarkLexerOperators(b *testing.B) {
	var sb strings.Builder
	ops := []string{
		"+", "-", "*", "/", "%", "==", "!=", "<", ">", "<=", ">=",
		"&&", "||", "!", "|>", "++", "..", "..=", "=", "->", "=>",
		"(", ")", "{", "}", "[", "]", ",", ":", ";", ".", "::",
	}
	for i := 0; i < 200; i++ {
		for _, op := range ops {
			sb.WriteString(op)
			sb.WriteByte(' ')
		}
		sb.WriteByte('\n')
	}
	src := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l := lexer.New(src, 0)
		l.Tokenize()
	}
}

// generateLargeSource builds a synthetic Ryx program with n function
// definitions, each containing variable declarations, arithmetic, and
// control flow.
func generateLargeSource(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("fn func_")
		sb.WriteString(strings.Repeat("x", 1))
		sb.WriteString("(a: Int, b: Int) -> Int {\n")
		sb.WriteString("    let mut result = 0;\n")
		sb.WriteString("    let mut i = 0;\n")
		sb.WriteString("    while i < a {\n")
		sb.WriteString("        if i % 2 == 0 {\n")
		sb.WriteString("            result = result + i * b;\n")
		sb.WriteString("        } else {\n")
		sb.WriteString("            result = result - i;\n")
		sb.WriteString("        };\n")
		sb.WriteString("        i = i + 1;\n")
		sb.WriteString("    };\n")
		sb.WriteString("    result\n")
		sb.WriteString("}\n\n")
	}
	sb.WriteString("fn main() {\n")
	sb.WriteString("    let x = 42;\n")
	sb.WriteString("    println(x)\n")
	sb.WriteString("}\n")
	return sb.String()
}
