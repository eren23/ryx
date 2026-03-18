package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/lexer"
)

const testdataDir = "../testdata/lexer"
const updateGolden = false // set to true to regenerate .golden files

// TestLexerGolden runs the lexer on every .ryx file in the testdata directory
// and compares the output to the corresponding .golden file.
func TestLexerGolden(t *testing.T) {
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Fatalf("failed to read testdata dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ryx") {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			ryxPath := filepath.Join(testdataDir, name)
			goldenPath := filepath.Join(testdataDir, strings.TrimSuffix(name, ".ryx")+".golden")

			src, err := os.ReadFile(ryxPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", ryxPath, err)
			}

			got := tokenize(string(src))

			if updateGolden {
				if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
					t.Fatalf("failed to write golden file: %v", err)
				}
				return
			}

			golden, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden file %s not found (run with updateGolden=true to create): %v",
					goldenPath, err)
			}

			if got != string(golden) {
				t.Errorf("output mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s",
					name, got, string(golden))
			}
		})
	}
}

// TestLexerGoldenNoErrors verifies that non-error test files produce no ERROR tokens.
func TestLexerGoldenNoErrors(t *testing.T) {
	noErrorFiles := []string{
		"literals.ryx",
		"operators.ryx",
		"keywords.ryx",
		"strings.ryx",
		"comments.ryx",
		"unicode.ryx",
	}

	for _, name := range noErrorFiles {
		t.Run(name, func(t *testing.T) {
			ryxPath := filepath.Join(testdataDir, name)
			src, err := os.ReadFile(ryxPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", ryxPath, err)
			}

			l := lexer.New(string(src), 0)
			tokens := l.Tokenize()
			for _, tok := range tokens {
				if tok.Type == lexer.ERROR {
					t.Errorf("unexpected ERROR token at %d:%d: %s", tok.Line, tok.Col, tok.Value)
				}
			}
		})
	}
}

// TestLexerGoldenHasErrors verifies that error test files produce at least one ERROR token.
func TestLexerGoldenHasErrors(t *testing.T) {
	ryxPath := filepath.Join(testdataDir, "errors.ryx")
	src, err := os.ReadFile(ryxPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", ryxPath, err)
	}

	l := lexer.New(string(src), 0)
	tokens := l.Tokenize()
	errorCount := 0
	for _, tok := range tokens {
		if tok.Type == lexer.ERROR {
			errorCount++
		}
	}
	if errorCount == 0 {
		t.Error("expected at least one ERROR token in errors.ryx")
	}
}

// TestLexerGoldenTokenCount ensures each golden file produces a reasonable number of tokens.
func TestLexerGoldenTokenCount(t *testing.T) {
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Fatalf("failed to read testdata dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ryx") {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			ryxPath := filepath.Join(testdataDir, entry.Name())
			src, err := os.ReadFile(ryxPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", ryxPath, err)
			}

			l := lexer.New(string(src), 0)
			tokens := l.Tokenize()
			if len(tokens) < 2 { // at minimum one real token + EOF
				t.Errorf("expected at least 2 tokens, got %d", len(tokens))
			}
		})
	}
}

// tokenize produces the canonical text representation of tokens for golden comparison.
func tokenize(src string) string {
	l := lexer.New(src, 0)
	tokens := l.Tokenize()
	var b strings.Builder
	for _, tok := range tokens {
		b.WriteString(fmt.Sprintf("%-20s %-20q %d:%d [%d..%d)\n",
			tok.Type, tok.Value, tok.Line, tok.Col,
			tok.Span.Start, tok.Span.End))
	}
	return b.String()
}
