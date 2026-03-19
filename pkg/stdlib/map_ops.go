package stdlib

import (
	"fmt"
	"sort"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Map operations — operate on heap-allocated MapObj values
// ---------------------------------------------------------------------------

func resolveMap(v vm.Value, heap *vm.Heap) (*vm.MapObj, error) {
	if v.Tag != vm.TagObj {
		return nil, fmt.Errorf("expected Map (obj), got tag %d", v.Tag)
	}
	obj := heap.Get(v.AsObj())
	m, ok := obj.Data.(*vm.MapObj)
	if !ok {
		return nil, fmt.Errorf("expected Map object, got type %d", obj.Header.TypeID)
	}
	return m, nil
}

// MapNew creates a new empty map.
func MapNew(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("map_new: expected 0 arguments, got %d", len(args))
	}
	idx := heap.AllocMap()
	return vm.ObjVal(idx), nil
}

// MapGet retrieves the value for a key from a map.
// Returns a Result: Ok(value) if found, Err("key not found") if missing.
func MapGet(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("map_get: expected 2 arguments, got %d", len(args))
	}
	m, err := resolveMap(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_get: %w", err)
	}
	val, found, hashErr := m.Get(args[1], heap)
	if hashErr != nil {
		return vm.UnitVal(), fmt.Errorf("map_get: %w", hashErr)
	}
	if !found {
		return makeResultErr("key not found", heap), nil
	}
	return makeResultOk(val, heap), nil
}

// MapSet inserts or updates a key-value pair, returning the map.
func MapSet(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 3 {
		return vm.UnitVal(), fmt.Errorf("map_set: expected 3 arguments, got %d", len(args))
	}
	m, err := resolveMap(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_set: %w", err)
	}
	_, err = m.Set(args[1], args[2], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_set: %w", err)
	}
	return args[0], nil
}

// MapDelete removes a key from the map, returning the map.
func MapDelete(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("map_delete: expected 2 arguments, got %d", len(args))
	}
	m, err := resolveMap(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_delete: %w", err)
	}
	_, err = m.Delete(args[1], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_delete: %w", err)
	}
	return args[0], nil
}

// MapContains checks whether a key exists in the map.
func MapContains(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("map_contains: expected 2 arguments, got %d", len(args))
	}
	m, err := resolveMap(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_contains: %w", err)
	}
	found, err := m.Contains(args[1], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_contains: %w", err)
	}
	return vm.BoolVal(found), nil
}

// MapLen returns the number of entries in the map.
func MapLen(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("map_len: expected 1 argument, got %d", len(args))
	}
	m, err := resolveMap(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_len: %w", err)
	}
	return vm.IntVal(int64(m.Count)), nil
}

// MapKeys returns all keys as an array.
func MapKeys(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("map_keys: expected 1 argument, got %d", len(args))
	}
	m, err := resolveMap(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_keys: %w", err)
	}
	keys := m.Keys()
	idx := heap.AllocArray(keys)
	return vm.ObjVal(idx), nil
}

// MapValues returns all values as an array.
func MapValues(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("map_values: expected 1 argument, got %d", len(args))
	}
	m, err := resolveMap(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_values: %w", err)
	}
	vals := m.Values()
	idx := heap.AllocArray(vals)
	return vm.ObjVal(idx), nil
}

// MapEntries returns all entries as an array of (key, value) tuples.
func MapEntries(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("map_entries: expected 1 argument, got %d", len(args))
	}
	m, err := resolveMap(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_entries: %w", err)
	}
	entries := m.Entries()
	elems := make([]vm.Value, len(entries))
	for i, e := range entries {
		tIdx := heap.AllocTuple([]vm.Value{e.Key, e.Value})
		elems[i] = vm.ObjVal(tIdx)
	}
	idx := heap.AllocArray(elems)
	return vm.ObjVal(idx), nil
}

// MapMerge merges two maps, with the second map's entries overwriting the first's.
// Returns a new map containing all entries from both.
func MapMerge(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("map_merge: expected 2 arguments, got %d", len(args))
	}
	m1, err := resolveMap(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_merge: first argument: %w", err)
	}
	m2, err := resolveMap(args[1], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_merge: second argument: %w", err)
	}
	newIdx := heap.AllocMap()
	newMap := heap.Get(newIdx).Data.(*vm.MapObj)
	// Copy entries from first map.
	for _, e := range m1.Entries() {
		if _, err := newMap.Set(e.Key, e.Value, heap); err != nil {
			return vm.UnitVal(), fmt.Errorf("map_merge: %w", err)
		}
	}
	// Copy/overwrite entries from second map.
	for _, e := range m2.Entries() {
		if _, err := newMap.Set(e.Key, e.Value, heap); err != nil {
			return vm.UnitVal(), fmt.Errorf("map_merge: %w", err)
		}
	}
	return vm.ObjVal(newIdx), nil
}

// MapFilter keeps entries for which the predicate returns true.
// The predicate receives (key, value) and should return a Bool.
func MapFilter(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("map_filter: expected 2 arguments (map, func), got %d", len(args))
	}
	m, err := resolveMap(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_filter: %w", err)
	}
	fn := args[1]
	if CallbackInvoker == nil {
		return vm.UnitVal(), fmt.Errorf("map_filter: no callback invoker registered")
	}
	newIdx := heap.AllocMap()
	newMap := heap.Get(newIdx).Data.(*vm.MapObj)
	for _, e := range m.Entries() {
		result, callErr := CallbackInvoker(fn, []vm.Value{e.Key, e.Value}, heap)
		if callErr != nil {
			return vm.UnitVal(), fmt.Errorf("map_filter: callback error: %w", callErr)
		}
		if result.IsTruthy() {
			if _, err := newMap.Set(e.Key, e.Value, heap); err != nil {
				return vm.UnitVal(), fmt.Errorf("map_filter: %w", err)
			}
		}
	}
	return vm.ObjVal(newIdx), nil
}

// MapMap applies a transform function to each value, returning a new map with the same keys.
// The transform receives (key, value) and should return the new value.
func MapMap(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("map_map: expected 2 arguments (map, func), got %d", len(args))
	}
	m, err := resolveMap(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("map_map: %w", err)
	}
	fn := args[1]
	if CallbackInvoker == nil {
		return vm.UnitVal(), fmt.Errorf("map_map: no callback invoker registered")
	}
	newIdx := heap.AllocMap()
	newMap := heap.Get(newIdx).Data.(*vm.MapObj)
	for _, e := range m.Entries() {
		result, callErr := CallbackInvoker(fn, []vm.Value{e.Key, e.Value}, heap)
		if callErr != nil {
			return vm.UnitVal(), fmt.Errorf("map_map: callback error: %w", callErr)
		}
		if _, err := newMap.Set(e.Key, result, heap); err != nil {
			return vm.UnitVal(), fmt.Errorf("map_map: %w", err)
		}
	}
	return vm.ObjVal(newIdx), nil
}

// sortedMapKeys returns map keys sorted by string representation for deterministic output.
func sortedMapKeys(m *vm.MapObj, heap *vm.Heap) []vm.Value {
	keys := m.Keys()
	sort.SliceStable(keys, func(i, j int) bool {
		return vm.StringValue(keys[i], heap) < vm.StringValue(keys[j], heap)
	})
	return keys
}
