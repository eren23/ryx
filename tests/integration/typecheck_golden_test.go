package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/parser"
	"github.com/ryx-lang/ryx/pkg/resolver"
	"github.com/ryx-lang/ryx/pkg/types"
)

const typecheckTestdataDir = "../testdata/typecheck"
const updateTypecheckGolden = false // set to true to regenerate .golden files

// TestTypecheckGolden runs the type checker on every .ryx file in the typecheck
// testdata directory and compares the output to the corresponding .golden file.
func TestTypecheckGolden(t *testing.T) {
	entries, err := os.ReadDir(typecheckTestdataDir)
	if err != nil {
		t.Fatalf("failed to read testdata dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ryx") {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			ryxPath := filepath.Join(typecheckTestdataDir, name)
			goldenPath := filepath.Join(typecheckTestdataDir, strings.TrimSuffix(name, ".ryx")+".golden")

			src, err := os.ReadFile(ryxPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", ryxPath, err)
			}

			got := formatTypecheckResult(string(src))

			if updateTypecheckGolden {
				if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
					t.Fatalf("failed to write golden file: %v", err)
				}
				return
			}

			golden, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden file %s not found (run with updateTypecheckGolden=true to create): %v",
					goldenPath, err)
			}

			if got != string(golden) {
				t.Errorf("output mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s",
					name, got, string(golden))
			}
		})
	}
}

// TestTypecheckValidFiles ensures that non-error test files produce no type errors.
func TestTypecheckValidFiles(t *testing.T) {
	validFiles := []string{
		"inference_basic.ryx",
		"inference_generics.ryx",
		"traits.ryx",
		"exhaustiveness.ryx",
		"complex_types.ryx",
	}

	for _, name := range validFiles {
		t.Run(name, func(t *testing.T) {
			ryxPath := filepath.Join(typecheckTestdataDir, name)
			src, err := os.ReadFile(ryxPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", ryxPath, err)
			}

			types.ResetTypeVarCounter()
			registry := diagnostic.NewSourceRegistry()
			fileID := registry.AddFile(name, string(src))

			result := parser.Parse(string(src), fileID)
			if result.HasErrors() {
				for _, e := range result.Errors {
					t.Errorf("parse error: %s", e.Message)
				}
				t.FailNow()
			}

			resolved := resolver.Resolve(result.Program, registry)
			// Check resolver errors (not warnings).
			for _, d := range resolved.Diagnostics {
				if d.Severity == diagnostic.SeverityError {
					t.Errorf("resolver error: [%s] %s", d.Code, d.Message)
				}
			}

			checkResult := types.Check(result.Program, resolved, registry)
			for _, d := range checkResult.Diagnostics {
				if d.Severity == diagnostic.SeverityError {
					t.Errorf("type error: [%s] %s", d.Code, d.Message)
				}
			}
		})
	}
}

// TestTypecheckErrorFile ensures that the error test file produces type errors.
func TestTypecheckErrorFile(t *testing.T) {
	ryxPath := filepath.Join(typecheckTestdataDir, "errors.ryx")
	src, err := os.ReadFile(ryxPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", ryxPath, err)
	}

	types.ResetTypeVarCounter()
	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("errors.ryx", string(src))

	result := parser.Parse(string(src), fileID)
	if result.HasErrors() {
		t.Fatalf("errors.ryx should parse successfully (errors are type-level)")
	}

	resolved := resolver.Resolve(result.Program, registry)
	checkResult := types.Check(result.Program, resolved, registry)

	errorCount := 0
	for _, d := range checkResult.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			errorCount++
		}
	}

	if errorCount == 0 {
		t.Error("expected type errors in errors.ryx but found none")
	}
	t.Logf("errors.ryx produced %d type error(s) as expected", errorCount)
}

// formatTypecheckResult produces the canonical text representation of type check
// results for golden file comparison.
func formatTypecheckResult(src string) string {
	types.ResetTypeVarCounter()
	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("test.ryx", src)

	parseResult := parser.Parse(src, fileID)
	var b strings.Builder

	b.WriteString(fmt.Sprintf("parse_errors: %d\n", len(parseResult.Errors)))
	if parseResult.HasErrors() {
		for _, e := range parseResult.Errors {
			b.WriteString(fmt.Sprintf("  parse: %s\n", e.Message))
		}
		return b.String()
	}

	resolved := resolver.Resolve(parseResult.Program, registry)

	resolverErrorCount := 0
	resolverWarnCount := 0
	for _, d := range resolved.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			resolverErrorCount++
		} else {
			resolverWarnCount++
		}
	}
	b.WriteString(fmt.Sprintf("resolver_errors: %d\n", resolverErrorCount))
	b.WriteString(fmt.Sprintf("resolver_warnings: %d\n", resolverWarnCount))

	checkResult := types.Check(parseResult.Program, resolved, registry)

	typeErrorCount := 0
	typeWarnCount := 0
	for _, d := range checkResult.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			typeErrorCount++
		} else {
			typeWarnCount++
		}
	}
	b.WriteString(fmt.Sprintf("type_errors: %d\n", typeErrorCount))
	b.WriteString(fmt.Sprintf("type_warnings: %d\n", typeWarnCount))

	if len(checkResult.Diagnostics) > 0 {
		b.WriteString("\n--- diagnostics ---\n")
		for _, d := range checkResult.Diagnostics {
			b.WriteString(fmt.Sprintf("  [%s] %s: %s\n", d.Code, d.Severity, d.Message))
		}
	}

	b.WriteString(fmt.Sprintf("\nnode_types: %d\n", len(checkResult.NodeTypes)))
	b.WriteString(fmt.Sprintf("symbol_types: %d\n", len(checkResult.SymbolTypes)))

	return b.String()
}
