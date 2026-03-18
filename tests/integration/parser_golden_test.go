package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/parser"
)

const parserTestdataDir = "../testdata/parser"
const updateParserGolden = false // set to true to regenerate .golden files

// TestParserGolden runs the parser on every .ryx file in the parser testdata
// directory and compares the output to the corresponding .golden file.
func TestParserGolden(t *testing.T) {
	entries, err := os.ReadDir(parserTestdataDir)
	if err != nil {
		t.Fatalf("failed to read testdata dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ryx") {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			ryxPath := filepath.Join(parserTestdataDir, name)
			goldenPath := filepath.Join(parserTestdataDir, strings.TrimSuffix(name, ".ryx")+".golden")

			src, err := os.ReadFile(ryxPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", ryxPath, err)
			}

			got := formatParseResult(string(src))

			if updateParserGolden {
				if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
					t.Fatalf("failed to write golden file: %v", err)
				}
				return
			}

			golden, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden file %s not found (run with updateParserGolden=true to create): %v",
					goldenPath, err)
			}

			if got != string(golden) {
				t.Errorf("output mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s",
					name, got, string(golden))
			}
		})
	}
}

// TestParserGoldenNoErrors verifies that non-error test files produce no parse errors.
func TestParserGoldenNoErrors(t *testing.T) {
	noErrorFiles := []string{
		"expressions.ryx",
		"statements.ryx",
		"functions.ryx",
		"types.ryx",
		"patterns.ryx",
		"generics.ryx",
		"traits.ryx",
		"closures.ryx",
		"concurrency.ryx",
	}

	for _, name := range noErrorFiles {
		t.Run(name, func(t *testing.T) {
			ryxPath := filepath.Join(parserTestdataDir, name)
			src, err := os.ReadFile(ryxPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", ryxPath, err)
			}

			result := parser.Parse(string(src), 0)
			if result.HasErrors() {
				for _, e := range result.Errors {
					t.Errorf("unexpected parse error: %s", e.Message)
				}
			}
		})
	}
}

// TestParserGoldenHasErrors verifies that error recovery test files produce errors.
func TestParserGoldenHasErrors(t *testing.T) {
	ryxPath := filepath.Join(parserTestdataDir, "error_recovery.ryx")
	src, err := os.ReadFile(ryxPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", ryxPath, err)
	}

	result := parser.Parse(string(src), 0)
	if !result.HasErrors() {
		t.Error("expected parse errors in error_recovery.ryx")
	}
}

// TestParserGoldenItemCount ensures each valid file produces at least one top-level item.
func TestParserGoldenItemCount(t *testing.T) {
	validFiles := []string{
		"expressions.ryx",
		"statements.ryx",
		"functions.ryx",
		"types.ryx",
		"patterns.ryx",
		"generics.ryx",
		"traits.ryx",
		"closures.ryx",
		"concurrency.ryx",
	}

	for _, name := range validFiles {
		t.Run(name, func(t *testing.T) {
			ryxPath := filepath.Join(parserTestdataDir, name)
			src, err := os.ReadFile(ryxPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", ryxPath, err)
			}

			result := parser.Parse(string(src), 0)
			if result.Program == nil {
				t.Fatal("expected non-nil program")
			}
			if len(result.Program.Items) == 0 {
				t.Error("expected at least one top-level item")
			}
		})
	}
}

// formatParseResult produces the canonical text representation of parse results
// for golden file comparison. It shows: items count, errors count, and a
// structured walk of the AST.
func formatParseResult(src string) string {
	result := parser.Parse(src, 0)
	var b strings.Builder

	b.WriteString(fmt.Sprintf("items: %d\n", len(result.Program.Items)))
	b.WriteString(fmt.Sprintf("errors: %d\n", len(result.Errors)))

	if len(result.Errors) > 0 {
		b.WriteString("\n--- errors ---\n")
		for _, e := range result.Errors {
			b.WriteString(fmt.Sprintf("  [%d..%d) %s\n", e.Span.Start, e.Span.End, e.Message))
			if e.Hint != "" {
				b.WriteString(fmt.Sprintf("    hint: %s\n", e.Hint))
			}
		}
	}

	b.WriteString("\n--- items ---\n")
	for i, item := range result.Program.Items {
		b.WriteString(fmt.Sprintf("[%d] ", i))
		formatItem(&b, item, 0)
	}

	return b.String()
}

func formatItem(b *strings.Builder, item parser.Item, indent int) {
	prefix := strings.Repeat("  ", indent)
	span := item.Span()

	switch it := item.(type) {
	case *parser.FnDef:
		pub := ""
		if it.Public {
			pub = "pub "
		}
		b.WriteString(fmt.Sprintf("%s%sfn %s", prefix, pub, it.Name))
		if it.GenParams != nil {
			b.WriteString(fmt.Sprintf("<%d params>", len(it.GenParams.Params)))
		}
		b.WriteString(fmt.Sprintf("(%d params)", len(it.Params)))
		if it.ReturnType != nil {
			b.WriteString(" -> <type>")
		}
		if it.Body != nil {
			b.WriteString(fmt.Sprintf(" { %d stmts", len(it.Body.Stmts)))
			if it.Body.TrailingExpr != nil {
				b.WriteString(" + trailing")
			}
			b.WriteString(" }")
		} else {
			b.WriteString(" ;")
		}
		b.WriteString(fmt.Sprintf(" [%d..%d)\n", span.Start, span.End))

	case *parser.TypeDef:
		pub := ""
		if it.Public {
			pub = "pub "
		}
		b.WriteString(fmt.Sprintf("%s%stype %s", prefix, pub, it.Name))
		if it.GenParams != nil {
			b.WriteString(fmt.Sprintf("<%d params>", len(it.GenParams.Params)))
		}
		b.WriteString(fmt.Sprintf(" { %d variants }", len(it.Variants)))
		b.WriteString(fmt.Sprintf(" [%d..%d)\n", span.Start, span.End))
		for _, v := range it.Variants {
			b.WriteString(fmt.Sprintf("%s  variant %s(%d fields)\n", prefix, v.Name, len(v.Fields)))
		}

	case *parser.StructDef:
		pub := ""
		if it.Public {
			pub = "pub "
		}
		b.WriteString(fmt.Sprintf("%s%sstruct %s", prefix, pub, it.Name))
		if it.GenParams != nil {
			b.WriteString(fmt.Sprintf("<%d params>", len(it.GenParams.Params)))
		}
		b.WriteString(fmt.Sprintf(" { %d fields }", len(it.Fields)))
		b.WriteString(fmt.Sprintf(" [%d..%d)\n", span.Start, span.End))
		for _, f := range it.Fields {
			pub := ""
			if f.Public {
				pub = "pub "
			}
			b.WriteString(fmt.Sprintf("%s  %sfield %s\n", prefix, pub, f.Name))
		}

	case *parser.TraitDef:
		pub := ""
		if it.Public {
			pub = "pub "
		}
		b.WriteString(fmt.Sprintf("%s%strait %s", prefix, pub, it.Name))
		if it.GenParams != nil {
			b.WriteString(fmt.Sprintf("<%d params>", len(it.GenParams.Params)))
		}
		b.WriteString(fmt.Sprintf(" { %d methods }", len(it.Methods)))
		b.WriteString(fmt.Sprintf(" [%d..%d)\n", span.Start, span.End))
		for _, m := range it.Methods {
			formatItem(b, m, indent+1)
		}

	case *parser.ImplBlock:
		b.WriteString(fmt.Sprintf("%simpl", prefix))
		if it.GenParams != nil {
			b.WriteString(fmt.Sprintf("<%d params>", len(it.GenParams.Params)))
		}
		if it.TraitName != "" {
			b.WriteString(fmt.Sprintf(" %s for", it.TraitName))
		}
		b.WriteString(fmt.Sprintf(" <type> { %d methods }", len(it.Methods)))
		b.WriteString(fmt.Sprintf(" [%d..%d)\n", span.Start, span.End))
		for _, m := range it.Methods {
			formatItem(b, m, indent+1)
		}

	case *parser.ImportDecl:
		b.WriteString(fmt.Sprintf("%simport %s", prefix, strings.Join(it.Path, "::")))
		if it.Alias != "" {
			b.WriteString(fmt.Sprintf(" as %s", it.Alias))
		}
		b.WriteString(fmt.Sprintf(" [%d..%d)\n", span.Start, span.End))

	case *parser.ModuleDecl:
		b.WriteString(fmt.Sprintf("%smodule %s [%d..%d)\n", prefix, it.Name, span.Start, span.End))

	default:
		b.WriteString(fmt.Sprintf("%s<unknown item> [%d..%d)\n", prefix, span.Start, span.End))
	}
}
