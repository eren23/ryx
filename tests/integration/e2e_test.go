package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryx-lang/ryx/pkg/codegen"
	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/hir"
	"github.com/ryx-lang/ryx/pkg/mir"
	"github.com/ryx-lang/ryx/pkg/optimize"
	"github.com/ryx-lang/ryx/pkg/parser"
	"github.com/ryx-lang/ryx/pkg/resolver"
	"github.com/ryx-lang/ryx/pkg/stdlib"
	"github.com/ryx-lang/ryx/pkg/types"
	"github.com/ryx-lang/ryx/pkg/vm"
)

// compileAndRun runs the full Ryx compile pipeline on src and executes the
// resulting program in the VM. It returns the captured stdout output and any
// error that occurred during compilation or execution.
func compileAndRun(src, filename string) (stdout string, err error) {
	return compileAndRunWithTimeout(src, filename, 30*time.Second)
}

// compileAndRunWithTimeout is like compileAndRun but with a configurable timeout.
func compileAndRunWithTimeout(src, filename string, timeout time.Duration) (stdout string, err error) {
	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile(filename, src)

	// Parse.
	parseResult := parser.Parse(src, fileID)
	if len(parseResult.Errors) > 0 {
		msgs := make([]string, len(parseResult.Errors))
		for i, e := range parseResult.Errors {
			msgs[i] = e.Message
		}
		return "", fmt.Errorf("parse errors: %s", strings.Join(msgs, "; "))
	}

	// Resolve.
	resolved := resolver.Resolve(parseResult.Program, registry)
	if hasDiagErrors(resolved.Diagnostics) {
		return "", fmt.Errorf("resolve errors: %s", formatDiags(resolved.Diagnostics))
	}

	// Type check.
	checkResult := types.Check(parseResult.Program, resolved, registry)
	if checkResult.HasErrors() {
		return "", fmt.Errorf("type errors: %s", formatDiags(checkResult.Diagnostics))
	}

	// HIR lower.
	lowerResult := hir.Lower(parseResult.Program, checkResult, resolved, registry)
	if lowerResult.HasErrors() {
		return "", fmt.Errorf("lower errors: %s", formatDiags(lowerResult.Diagnostics))
	}

	// Monomorphize.
	monoResult := hir.Monomorphize(lowerResult.Program, 64)
	if hasDiagErrors(monoResult.Diagnostics) {
		return "", fmt.Errorf("monomorphize errors: %s", formatDiags(monoResult.Diagnostics))
	}

	// MIR build.
	mirProg := mir.Build(monoResult.Program)

	// Optimize (level 2 = full).
	optimize.Pipeline(mirProg, optimize.O2)

	// Codegen.
	compiled, err := codegen.Generate(mirProg)
	if err != nil {
		return "", fmt.Errorf("codegen error: %w", err)
	}

	// Execute in VM with timeout.
	machine := vm.NewVM(compiled)
	var buf bytes.Buffer
	machine.Stdout = &buf

	// Register stdlib builtins for OpCallBuiltin dispatch.
	reg := vm.NewBuiltinRegistry()
	stdlib.RegisterAll(reg)
	machine.Builtins = reg

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- machine.Run()
	}()

	select {
	case runErr := <-done:
		if runErr != nil {
			return buf.String(), runErr
		}
		return buf.String(), nil
	case <-ctx.Done():
		return buf.String(), fmt.Errorf("execution timed out after %v", timeout)
	}
}

// hasDiagErrors returns true if any diagnostic has error severity.
func hasDiagErrors(diags []diagnostic.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diagnostic.SeverityError {
			return true
		}
	}
	return false
}

// formatDiags formats diagnostics into a single string for error messages.
func formatDiags(diags []diagnostic.Diagnostic) string {
	msgs := make([]string, 0, len(diags))
	for _, d := range diags {
		if d.Severity == diagnostic.SeverityError {
			msgs = append(msgs, d.Message)
		}
	}
	return strings.Join(msgs, "; ")
}

// TestE2EPrograms reads all .ryx files from the testdata/programs/ directory,
// compiles and runs each one, and compares the captured stdout to the
// corresponding .expected file.
func TestE2EPrograms(t *testing.T) {
	programsDir := filepath.Join("..", "testdata", "programs")

	entries, err := os.ReadDir(programsDir)
	if err != nil {
		t.Fatalf("failed to read programs directory %s: %v", programsDir, err)
	}

	ryxFiles := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ryx") {
			ryxFiles = append(ryxFiles, entry.Name())
		}
	}

	if len(ryxFiles) == 0 {
		t.Fatal("no .ryx files found in testdata/programs/")
	}

	for _, ryxFile := range ryxFiles {
		name := strings.TrimSuffix(ryxFile, ".ryx")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srcPath := filepath.Join(programsDir, ryxFile)
			expectedPath := filepath.Join(programsDir, name+".expected")

			srcBytes, err := os.ReadFile(srcPath)
			if err != nil {
				t.Fatalf("failed to read source file: %v", err)
			}

			expectedBytes, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("failed to read expected file %s: %v", expectedPath, err)
			}

			got, runErr := compileAndRunWithTimeout(string(srcBytes), ryxFile, 10*time.Second)
			if runErr != nil {
				// Some tests may intentionally produce runtime errors.
				// If there is an .expected file that matches the error, that is fine.
				// Otherwise, report it as a failure.
				if re, ok := runErr.(*vm.RuntimeError); ok {
					got = got + re.Error() + "\n"
				} else {
					t.Fatalf("compile/run error: %v\npartial stdout: %s", runErr, got)
				}
			}

			expected := string(expectedBytes)
			if got != expected {
				t.Errorf("output mismatch for %s\n--- expected ---\n%s\n--- got ---\n%s",
					ryxFile, expected, got)
			}
		})
	}
}

// TestE2ECompileOnly verifies that all test programs at least compile
// successfully through the full pipeline, even if they may fail at runtime.
func TestE2ECompileOnly(t *testing.T) {
	programsDir := filepath.Join("..", "testdata", "programs")

	entries, err := os.ReadDir(programsDir)
	if err != nil {
		t.Fatalf("failed to read programs directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ryx") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".ryx")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srcPath := filepath.Join(programsDir, entry.Name())
			srcBytes, err := os.ReadFile(srcPath)
			if err != nil {
				t.Fatalf("failed to read source: %v", err)
			}

			_, compileErr := compile(string(srcBytes), entry.Name())
			if compileErr != nil {
				t.Errorf("compilation failed: %v", compileErr)
			}
		})
	}
}

// TestE2EExamplesCompileOnly verifies that example programs (which may use
// graphics/display and cannot run headless) at least compile through the
// full pipeline without errors.
func TestE2EExamplesCompileOnly(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "examples")

	tests := []string{
		"raycaster.ryx",
		"image_viewer.ryx",
	}

	for _, file := range tests {
		name := strings.TrimSuffix(file, ".ryx")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srcPath := filepath.Join(examplesDir, file)
			srcBytes, err := os.ReadFile(srcPath)
			if err != nil {
				t.Fatalf("failed to read source: %v", err)
			}

			_, compileErr := compile(string(srcBytes), file)
			if compileErr != nil {
				t.Errorf("compilation failed: %v", compileErr)
			}
		})
	}
}

// TestE2EClosureMutCapture explicitly verifies that the closure_mut_capture
// test case runs and produces expected output (1, 2, 3).
func TestE2EClosureMutCapture(t *testing.T) {
	srcPath := filepath.Join("..", "testdata", "programs", "closure_mut_capture.ryx")
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("failed to read source: %v", err)
	}

	got, runErr := compileAndRun(string(srcBytes), "closure_mut_capture.ryx")
	if runErr != nil {
		t.Fatalf("compile/run error: %v", runErr)
	}

	expected := "1\n2\n3\n"
	if got != expected {
		t.Errorf("output mismatch\n--- expected ---\n%s\n--- got ---\n%s", expected, got)
	}
}

// compile runs the Ryx compiler pipeline (without executing) and returns
// the compiled program or an error.
func compile(src, filename string) (*codegen.CompiledProgram, error) {
	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile(filename, src)

	parseResult := parser.Parse(src, fileID)
	if len(parseResult.Errors) > 0 {
		msgs := make([]string, len(parseResult.Errors))
		for i, e := range parseResult.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("parse errors: %s", strings.Join(msgs, "; "))
	}

	resolved := resolver.Resolve(parseResult.Program, registry)
	if hasDiagErrors(resolved.Diagnostics) {
		return nil, fmt.Errorf("resolve errors: %s", formatDiags(resolved.Diagnostics))
	}

	checkResult := types.Check(parseResult.Program, resolved, registry)
	if checkResult.HasErrors() {
		return nil, fmt.Errorf("type errors: %s", formatDiags(checkResult.Diagnostics))
	}

	lowerResult := hir.Lower(parseResult.Program, checkResult, resolved, registry)
	if lowerResult.HasErrors() {
		return nil, fmt.Errorf("lower errors: %s", formatDiags(lowerResult.Diagnostics))
	}

	monoResult := hir.Monomorphize(lowerResult.Program, 64)
	if hasDiagErrors(monoResult.Diagnostics) {
		return nil, fmt.Errorf("monomorphize errors: %s", formatDiags(monoResult.Diagnostics))
	}

	mirProg := mir.Build(monoResult.Program)
	optimize.Pipeline(mirProg, optimize.O2)

	compiled, err := codegen.Generate(mirProg)
	if err != nil {
		return nil, fmt.Errorf("codegen error: %w", err)
	}

	return compiled, nil
}
