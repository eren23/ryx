package stdlib

import (
	"fmt"
	"os"
	"path/filepath"

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

// FileExists returns true if the given path exists.
func FileExists(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("file_exists: expected 1 arg, got %d", len(args))
	}
	path, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("file_exists: %w", err)
	}
	_, statErr := os.Stat(path)
	return vm.BoolVal(statErr == nil), nil
}

// DirList returns the names of directory entries at the given path.
func DirList(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("dir_list: expected 1 arg, got %d", len(args))
	}
	path, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("dir_list: %w", err)
	}
	entries, readErr := os.ReadDir(path)
	if readErr != nil {
		return makeResultErr(readErr.Error(), heap), nil
	}
	names := make([]vm.Value, len(entries))
	for i, entry := range entries {
		idx := heap.AllocString(entry.Name())
		names[i] = vm.ObjVal(idx)
	}
	arrIdx := heap.AllocArray(names)
	return makeResultOk(vm.ObjVal(arrIdx), heap), nil
}

// DirCreate creates the given directory path, including any missing parents.
func DirCreate(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("dir_create: expected 1 arg, got %d", len(args))
	}
	path, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("dir_create: %w", err)
	}
	mkErr := os.MkdirAll(path, 0755)
	if mkErr != nil {
		return makeResultErr(mkErr.Error(), heap), nil
	}
	return makeResultOk(vm.UnitVal(), heap), nil
}

// PathJoin joins path components with the OS-specific separator.
func PathJoin(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("path_join: expected 1 arg, got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("path_join: %w", err)
	}
	parts := make([]string, len(a.Elements))
	for i, elem := range a.Elements {
		s, err := resolveString(elem, heap)
		if err != nil {
			return vm.UnitVal(), fmt.Errorf("path_join: element %d: %w", i, err)
		}
		parts[i] = s
	}
	result := filepath.Join(parts...)
	idx := heap.AllocString(result)
	return vm.ObjVal(idx), nil
}

// PathDirname returns the directory component of a path.
func PathDirname(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("path_dirname: expected 1 arg, got %d", len(args))
	}
	path, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("path_dirname: %w", err)
	}
	dir := filepath.Dir(path)
	idx := heap.AllocString(dir)
	return vm.ObjVal(idx), nil
}

// PathBasename returns the final path component.
func PathBasename(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("path_basename: expected 1 arg, got %d", len(args))
	}
	path, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("path_basename: %w", err)
	}
	base := filepath.Base(path)
	idx := heap.AllocString(base)
	return vm.ObjVal(idx), nil
}

// PathExtension returns the path extension, including the leading dot.
func PathExtension(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("path_extension: expected 1 arg, got %d", len(args))
	}
	path, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("path_extension: %w", err)
	}
	ext := filepath.Ext(path)
	idx := heap.AllocString(ext)
	return vm.ObjVal(idx), nil
}

// FileSize returns the size of the file at the given path.
func FileSize(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("file_size: expected 1 arg, got %d", len(args))
	}
	path, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("file_size: %w", err)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		return makeResultErr(statErr.Error(), heap), nil
	}
	return makeResultOk(vm.IntVal(info.Size()), heap), nil
}
