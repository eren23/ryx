package stdlib

import (
	"fmt"
	"os"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// File I/O — read_file and write_file operate on the real filesystem.
// Results are returned as Result enums (Ok/Err).
// ---------------------------------------------------------------------------

// ReadFile reads the entire contents of a file at the given path.
// Returns Result<String, String>.
func ReadFile(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("read_file: expected 1 argument, got %d", len(args))
	}
	path, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("read_file: %w", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return makeResultErr(readErr.Error(), heap), nil
	}
	strIdx := heap.AllocString(string(data))
	return makeResultOk(vm.ObjVal(strIdx), heap), nil
}

// WriteFile writes content to a file at the given path, creating it if necessary.
// Returns Result<Unit, String>.
func WriteFile(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("write_file: expected 2 arguments (path, content), got %d", len(args))
	}
	path, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("write_file: path: %w", err)
	}
	content, err := resolveString(args[1], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("write_file: content: %w", err)
	}
	writeErr := os.WriteFile(path, []byte(content), 0644)
	if writeErr != nil {
		return makeResultErr(writeErr.Error(), heap), nil
	}
	return makeResultOk(vm.UnitVal(), heap), nil
}
