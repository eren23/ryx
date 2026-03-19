package stdlib

import (
	"sort"
	"testing"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Helpers for map tests
// ---------------------------------------------------------------------------

// makeMap creates a map on the heap and returns the vm.Value and the *vm.MapObj.
func makeMap(h *vm.Heap) (vm.Value, *vm.MapObj) {
	idx := h.AllocMap()
	mapVal := vm.ObjVal(idx)
	mapObj := h.Get(idx).Data.(*vm.MapObj)
	return mapVal, mapObj
}

// getMapObj resolves a vm.Value to a *vm.MapObj.
func getMapObj(v vm.Value, h *vm.Heap) *vm.MapObj {
	return h.Get(v.AsObj()).Data.(*vm.MapObj)
}

// sortedInts sorts a slice of int64 values.
func sortedInts(vals []int64) []int64 {
	sort.SliceStable(vals, func(i, j int) bool { return vals[i] < vals[j] })
	return vals
}

// collectIntKeys extracts int64 keys from an array vm.Value.
func collectIntKeys(arrVal vm.Value, h *vm.Heap) []int64 {
	elems := getArray(arrVal, h)
	result := make([]int64, len(elems))
	for i, e := range elems {
		result[i] = e.AsInt()
	}
	return result
}

// collectIntVals extracts int64 values from an array vm.Value.
func collectIntVals(arrVal vm.Value, h *vm.Heap) []int64 {
	return collectIntKeys(arrVal, h) // same logic
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestMapNewAndLen(t *testing.T) {
	h := newHeap()

	// MapNew returns an empty map.
	v, err := MapNew(nil, h)
	if err != nil {
		t.Fatalf("MapNew: %v", err)
	}
	if v.Tag != vm.TagObj {
		t.Fatal("MapNew should return an Obj")
	}

	// MapLen on the empty map should be 0.
	lenVal, err := MapLen([]vm.Value{v}, h)
	if err != nil {
		t.Fatalf("MapLen: %v", err)
	}
	if lenVal.Tag != vm.TagInt || lenVal.AsInt() != 0 {
		t.Errorf("MapLen(empty) = %d, want 0", lenVal.AsInt())
	}

	// Wrong arg count for MapNew.
	if _, err := MapNew([]vm.Value{vm.IntVal(1)}, h); err == nil {
		t.Error("MapNew with 1 arg should error")
	}

	// Wrong arg count for MapLen.
	if _, err := MapLen(nil, h); err == nil {
		t.Error("MapLen with 0 args should error")
	}
}

func TestMapSetAndGet(t *testing.T) {
	h := newHeap()
	mapVal, _ := makeMap(h)

	// Set key 1 -> 100.
	ret, err := MapSet([]vm.Value{mapVal, vm.IntVal(1), vm.IntVal(100)}, h)
	if err != nil {
		t.Fatalf("MapSet: %v", err)
	}
	// MapSet returns the map itself.
	if ret.Tag != vm.TagObj || ret.Data != mapVal.Data {
		t.Error("MapSet should return the same map value")
	}

	// Get key 1.
	result, err := MapGet([]vm.Value{mapVal, vm.IntVal(1)}, h)
	if err != nil {
		t.Fatalf("MapGet: %v", err)
	}
	if !IsResultOk(result, h) {
		t.Fatal("MapGet should return Ok for existing key")
	}
	inner, err := ResultUnwrap(result, h)
	if err != nil {
		t.Fatalf("ResultUnwrap: %v", err)
	}
	if inner.Tag != vm.TagInt || inner.AsInt() != 100 {
		t.Errorf("MapGet(1) = %d, want 100", inner.AsInt())
	}

	// Len should be 1.
	lenVal, _ := MapLen([]vm.Value{mapVal}, h)
	if lenVal.AsInt() != 1 {
		t.Errorf("MapLen = %d, want 1", lenVal.AsInt())
	}

	// Wrong arg count for MapSet.
	if _, err := MapSet([]vm.Value{mapVal, vm.IntVal(1)}, h); err == nil {
		t.Error("MapSet with 2 args should error")
	}

	// Wrong arg count for MapGet.
	if _, err := MapGet([]vm.Value{mapVal}, h); err == nil {
		t.Error("MapGet with 1 arg should error")
	}
}

func TestMapSetOverwrite(t *testing.T) {
	h := newHeap()
	mapVal, _ := makeMap(h)

	// Set key 1 -> 100, then overwrite key 1 -> 200.
	MapSet([]vm.Value{mapVal, vm.IntVal(1), vm.IntVal(100)}, h)
	MapSet([]vm.Value{mapVal, vm.IntVal(1), vm.IntVal(200)}, h)

	// Length should still be 1 (not 2).
	lenVal, _ := MapLen([]vm.Value{mapVal}, h)
	if lenVal.AsInt() != 1 {
		t.Errorf("after overwrite, MapLen = %d, want 1", lenVal.AsInt())
	}

	// Value should be the updated one.
	result, _ := MapGet([]vm.Value{mapVal, vm.IntVal(1)}, h)
	inner, _ := ResultUnwrap(result, h)
	if inner.AsInt() != 200 {
		t.Errorf("MapGet(1) after overwrite = %d, want 200", inner.AsInt())
	}
}

func TestMapContains(t *testing.T) {
	h := newHeap()
	mapVal, mapObj := makeMap(h)
	mapObj.Set(vm.IntVal(10), vm.IntVal(100), h)

	// Existing key.
	v, err := MapContains([]vm.Value{mapVal, vm.IntVal(10)}, h)
	if err != nil {
		t.Fatalf("MapContains: %v", err)
	}
	if v.Tag != vm.TagBool || !v.AsBool() {
		t.Error("MapContains(10) should be true")
	}

	// Missing key.
	v, err = MapContains([]vm.Value{mapVal, vm.IntVal(99)}, h)
	if err != nil {
		t.Fatalf("MapContains: %v", err)
	}
	if v.Tag != vm.TagBool || v.AsBool() {
		t.Error("MapContains(99) should be false")
	}

	// Wrong arg count.
	if _, err := MapContains([]vm.Value{mapVal}, h); err == nil {
		t.Error("MapContains with 1 arg should error")
	}
}

func TestMapDelete(t *testing.T) {
	h := newHeap()
	mapVal, mapObj := makeMap(h)
	mapObj.Set(vm.IntVal(1), vm.IntVal(100), h)
	mapObj.Set(vm.IntVal(2), vm.IntVal(200), h)

	// Delete existing key.
	ret, err := MapDelete([]vm.Value{mapVal, vm.IntVal(1)}, h)
	if err != nil {
		t.Fatalf("MapDelete: %v", err)
	}
	// Returns the map.
	if ret.Tag != vm.TagObj || ret.Data != mapVal.Data {
		t.Error("MapDelete should return the same map")
	}
	lenVal, _ := MapLen([]vm.Value{mapVal}, h)
	if lenVal.AsInt() != 1 {
		t.Errorf("after delete, MapLen = %d, want 1", lenVal.AsInt())
	}

	// Contains should return false for deleted key.
	v, _ := MapContains([]vm.Value{mapVal, vm.IntVal(1)}, h)
	if v.AsBool() {
		t.Error("deleted key 1 should not be found")
	}

	// Wrong arg count.
	if _, err := MapDelete([]vm.Value{mapVal}, h); err == nil {
		t.Error("MapDelete with 1 arg should error")
	}
}

func TestMapDeleteMissing(t *testing.T) {
	h := newHeap()
	mapVal, mapObj := makeMap(h)
	mapObj.Set(vm.IntVal(1), vm.IntVal(100), h)

	// Delete non-existent key should not error.
	_, err := MapDelete([]vm.Value{mapVal, vm.IntVal(999)}, h)
	if err != nil {
		t.Fatalf("MapDelete missing key: %v", err)
	}

	// Length should be unchanged.
	lenVal, _ := MapLen([]vm.Value{mapVal}, h)
	if lenVal.AsInt() != 1 {
		t.Errorf("after deleting missing key, MapLen = %d, want 1", lenVal.AsInt())
	}
}

func TestMapGetMissing(t *testing.T) {
	h := newHeap()
	mapVal, _ := makeMap(h)

	// Get on empty map should return Err result.
	result, err := MapGet([]vm.Value{mapVal, vm.IntVal(42)}, h)
	if err != nil {
		t.Fatalf("MapGet: %v", err)
	}
	if IsResultOk(result, h) {
		t.Fatal("MapGet on missing key should return Err, not Ok")
	}
	// Trying to unwrap should error.
	_, unwrapErr := ResultUnwrap(result, h)
	if unwrapErr == nil {
		t.Error("ResultUnwrap on Err should return error")
	}
}

func TestMapIntKeys(t *testing.T) {
	h := newHeap()
	mapVal, mapObj := makeMap(h)
	mapObj.Set(vm.IntVal(10), vm.IntVal(100), h)
	mapObj.Set(vm.IntVal(20), vm.IntVal(200), h)
	mapObj.Set(vm.IntVal(30), vm.IntVal(300), h)

	// Len.
	lenVal, _ := MapLen([]vm.Value{mapVal}, h)
	if lenVal.AsInt() != 3 {
		t.Errorf("MapLen = %d, want 3", lenVal.AsInt())
	}

	// Get each key.
	for _, kv := range []struct{ k, v int64 }{{10, 100}, {20, 200}, {30, 300}} {
		result, _ := MapGet([]vm.Value{mapVal, vm.IntVal(kv.k)}, h)
		inner, err := ResultUnwrap(result, h)
		if err != nil {
			t.Fatalf("MapGet(%d): %v", kv.k, err)
		}
		if inner.AsInt() != kv.v {
			t.Errorf("MapGet(%d) = %d, want %d", kv.k, inner.AsInt(), kv.v)
		}
	}
}

func TestMapStringKeys(t *testing.T) {
	h := newHeap()
	mapVal, mapObj := makeMap(h)
	keyA := allocStr(h, "alpha")
	keyB := allocStr(h, "beta")
	mapObj.Set(keyA, vm.IntVal(1), h)
	mapObj.Set(keyB, vm.IntVal(2), h)

	// Get "alpha".
	result, _ := MapGet([]vm.Value{mapVal, keyA}, h)
	inner, err := ResultUnwrap(result, h)
	if err != nil {
		t.Fatalf("MapGet(alpha): %v", err)
	}
	if inner.AsInt() != 1 {
		t.Errorf("MapGet(alpha) = %d, want 1", inner.AsInt())
	}

	// Get "beta".
	result, _ = MapGet([]vm.Value{mapVal, keyB}, h)
	inner, err = ResultUnwrap(result, h)
	if err != nil {
		t.Fatalf("MapGet(beta): %v", err)
	}
	if inner.AsInt() != 2 {
		t.Errorf("MapGet(beta) = %d, want 2", inner.AsInt())
	}

	// Contains check with a fresh string allocation for "alpha" (same content, different obj).
	freshAlpha := allocStr(h, "alpha")
	v, _ := MapContains([]vm.Value{mapVal, freshAlpha}, h)
	if !v.AsBool() {
		t.Error("MapContains should find string key by value, not identity")
	}
}

func TestMapKeys(t *testing.T) {
	h := newHeap()
	mapVal, mapObj := makeMap(h)
	mapObj.Set(vm.IntVal(1), vm.IntVal(10), h)
	mapObj.Set(vm.IntVal(2), vm.IntVal(20), h)
	mapObj.Set(vm.IntVal(3), vm.IntVal(30), h)

	keysVal, err := MapKeys([]vm.Value{mapVal}, h)
	if err != nil {
		t.Fatalf("MapKeys: %v", err)
	}
	keys := collectIntKeys(keysVal, h)
	sortedInts(keys)
	if len(keys) != 3 || keys[0] != 1 || keys[1] != 2 || keys[2] != 3 {
		t.Errorf("MapKeys = %v, want [1 2 3]", keys)
	}

	// Wrong arg count.
	if _, err := MapKeys(nil, h); err == nil {
		t.Error("MapKeys with 0 args should error")
	}
}

func TestMapValues(t *testing.T) {
	h := newHeap()
	mapVal, mapObj := makeMap(h)
	mapObj.Set(vm.IntVal(1), vm.IntVal(10), h)
	mapObj.Set(vm.IntVal(2), vm.IntVal(20), h)
	mapObj.Set(vm.IntVal(3), vm.IntVal(30), h)

	valsVal, err := MapValues([]vm.Value{mapVal}, h)
	if err != nil {
		t.Fatalf("MapValues: %v", err)
	}
	vals := collectIntVals(valsVal, h)
	sortedInts(vals)
	if len(vals) != 3 || vals[0] != 10 || vals[1] != 20 || vals[2] != 30 {
		t.Errorf("MapValues = %v, want [10 20 30]", vals)
	}

	// Wrong arg count.
	if _, err := MapValues(nil, h); err == nil {
		t.Error("MapValues with 0 args should error")
	}
}

func TestMapEntries(t *testing.T) {
	h := newHeap()
	mapVal, mapObj := makeMap(h)
	mapObj.Set(vm.IntVal(1), vm.IntVal(10), h)
	mapObj.Set(vm.IntVal(2), vm.IntVal(20), h)
	mapObj.Set(vm.IntVal(3), vm.IntVal(30), h)

	entriesVal, err := MapEntries([]vm.Value{mapVal}, h)
	if err != nil {
		t.Fatalf("MapEntries: %v", err)
	}
	entryElems := getArray(entriesVal, h)
	if len(entryElems) != 3 {
		t.Fatalf("MapEntries length = %d, want 3", len(entryElems))
	}
	// Collect (key, value) pairs and sort by key.
	type kv struct{ k, v int64 }
	pairs := make([]kv, len(entryElems))
	for i, e := range entryElems {
		tuple := getTuple(e, h)
		if len(tuple) != 2 {
			t.Fatalf("entry tuple length = %d, want 2", len(tuple))
		}
		pairs[i] = kv{tuple[0].AsInt(), tuple[1].AsInt()}
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
	expected := []kv{{1, 10}, {2, 20}, {3, 30}}
	for i, e := range expected {
		if pairs[i] != e {
			t.Errorf("entry[%d] = {%d,%d}, want {%d,%d}", i, pairs[i].k, pairs[i].v, e.k, e.v)
		}
	}

	// Wrong arg count.
	if _, err := MapEntries(nil, h); err == nil {
		t.Error("MapEntries with 0 args should error")
	}
}

func TestMapMerge(t *testing.T) {
	h := newHeap()

	// Map 1: {1:10, 2:20}.
	m1Val, m1 := makeMap(h)
	m1.Set(vm.IntVal(1), vm.IntVal(10), h)
	m1.Set(vm.IntVal(2), vm.IntVal(20), h)

	// Map 2: {2:99, 3:30}.
	m2Val, m2 := makeMap(h)
	m2.Set(vm.IntVal(2), vm.IntVal(99), h)
	m2.Set(vm.IntVal(3), vm.IntVal(30), h)

	merged, err := MapMerge([]vm.Value{m1Val, m2Val}, h)
	if err != nil {
		t.Fatalf("MapMerge: %v", err)
	}

	// Merged map should have 3 entries.
	lenVal, _ := MapLen([]vm.Value{merged}, h)
	if lenVal.AsInt() != 3 {
		t.Errorf("merged len = %d, want 3", lenVal.AsInt())
	}

	// Key 2 should have value 99 (second map overwrites).
	result, _ := MapGet([]vm.Value{merged, vm.IntVal(2)}, h)
	inner, _ := ResultUnwrap(result, h)
	if inner.AsInt() != 99 {
		t.Errorf("merged[2] = %d, want 99", inner.AsInt())
	}

	// Key 1 should have value 10 (from first map).
	result, _ = MapGet([]vm.Value{merged, vm.IntVal(1)}, h)
	inner, _ = ResultUnwrap(result, h)
	if inner.AsInt() != 10 {
		t.Errorf("merged[1] = %d, want 10", inner.AsInt())
	}

	// Key 3 should have value 30 (from second map).
	result, _ = MapGet([]vm.Value{merged, vm.IntVal(3)}, h)
	inner, _ = ResultUnwrap(result, h)
	if inner.AsInt() != 30 {
		t.Errorf("merged[3] = %d, want 30", inner.AsInt())
	}

	// Original maps should be unchanged.
	lenVal, _ = MapLen([]vm.Value{m1Val}, h)
	if lenVal.AsInt() != 2 {
		t.Errorf("m1 len after merge = %d, want 2", lenVal.AsInt())
	}
	lenVal, _ = MapLen([]vm.Value{m2Val}, h)
	if lenVal.AsInt() != 2 {
		t.Errorf("m2 len after merge = %d, want 2", lenVal.AsInt())
	}

	// Wrong arg count.
	if _, err := MapMerge([]vm.Value{m1Val}, h); err == nil {
		t.Error("MapMerge with 1 arg should error")
	}
}

func TestMapFilter(t *testing.T) {
	h := newHeap()

	// Save and restore CallbackInvoker.
	oldInvoker := CallbackInvoker
	defer func() { CallbackInvoker = oldInvoker }()

	// Mock invoker: keeps entries where value > 15.
	CallbackInvoker = func(fn vm.Value, args []vm.Value, heap *vm.Heap) (vm.Value, error) {
		// args[0] = key, args[1] = value
		return vm.BoolVal(args[1].AsInt() > 15), nil
	}

	mapVal, mapObj := makeMap(h)
	mapObj.Set(vm.IntVal(1), vm.IntVal(10), h)
	mapObj.Set(vm.IntVal(2), vm.IntVal(20), h)
	mapObj.Set(vm.IntVal(3), vm.IntVal(30), h)

	filterFn := vm.Value{Tag: vm.TagFunc, Data: 1}
	result, err := MapFilter([]vm.Value{mapVal, filterFn}, h)
	if err != nil {
		t.Fatalf("MapFilter: %v", err)
	}

	// Should keep keys 2 and 3.
	lenVal, _ := MapLen([]vm.Value{result}, h)
	if lenVal.AsInt() != 2 {
		t.Errorf("filtered len = %d, want 2", lenVal.AsInt())
	}

	// Verify the kept keys.
	keysVal, _ := MapKeys([]vm.Value{result}, h)
	keys := collectIntKeys(keysVal, h)
	sortedInts(keys)
	if len(keys) != 2 || keys[0] != 2 || keys[1] != 3 {
		t.Errorf("filtered keys = %v, want [2 3]", keys)
	}

	// Original map should be unchanged.
	lenVal, _ = MapLen([]vm.Value{mapVal}, h)
	if lenVal.AsInt() != 3 {
		t.Errorf("original map len after filter = %d, want 3", lenVal.AsInt())
	}

	// Filter on empty map.
	emptyMapVal, _ := makeMap(h)
	result, err = MapFilter([]vm.Value{emptyMapVal, filterFn}, h)
	if err != nil {
		t.Fatalf("MapFilter on empty: %v", err)
	}
	lenVal, _ = MapLen([]vm.Value{result}, h)
	if lenVal.AsInt() != 0 {
		t.Error("filter on empty map should return empty map")
	}

	// Wrong arg count.
	if _, err := MapFilter([]vm.Value{mapVal}, h); err == nil {
		t.Error("MapFilter with 1 arg should error")
	}
}

func TestMapMap(t *testing.T) {
	h := newHeap()

	// Save and restore CallbackInvoker.
	oldInvoker := CallbackInvoker
	defer func() { CallbackInvoker = oldInvoker }()

	// Mock invoker: transforms each value to value * 10.
	CallbackInvoker = func(fn vm.Value, args []vm.Value, heap *vm.Heap) (vm.Value, error) {
		// args[0] = key, args[1] = value
		return vm.IntVal(args[1].AsInt() * 10), nil
	}

	mapVal, mapObj := makeMap(h)
	mapObj.Set(vm.IntVal(1), vm.IntVal(5), h)
	mapObj.Set(vm.IntVal(2), vm.IntVal(7), h)

	mapFn := vm.Value{Tag: vm.TagFunc, Data: 1}
	result, err := MapMap([]vm.Value{mapVal, mapFn}, h)
	if err != nil {
		t.Fatalf("MapMap: %v", err)
	}

	// Same number of entries.
	lenVal, _ := MapLen([]vm.Value{result}, h)
	if lenVal.AsInt() != 2 {
		t.Errorf("mapped len = %d, want 2", lenVal.AsInt())
	}

	// Key 1 should have value 50.
	getResult, _ := MapGet([]vm.Value{result, vm.IntVal(1)}, h)
	inner, _ := ResultUnwrap(getResult, h)
	if inner.AsInt() != 50 {
		t.Errorf("mapped[1] = %d, want 50", inner.AsInt())
	}

	// Key 2 should have value 70.
	getResult, _ = MapGet([]vm.Value{result, vm.IntVal(2)}, h)
	inner, _ = ResultUnwrap(getResult, h)
	if inner.AsInt() != 70 {
		t.Errorf("mapped[2] = %d, want 70", inner.AsInt())
	}

	// Original map should be unchanged.
	getResult, _ = MapGet([]vm.Value{mapVal, vm.IntVal(1)}, h)
	inner, _ = ResultUnwrap(getResult, h)
	if inner.AsInt() != 5 {
		t.Errorf("original[1] after map = %d, want 5", inner.AsInt())
	}

	// Map on empty map.
	emptyMapVal, _ := makeMap(h)
	result, err = MapMap([]vm.Value{emptyMapVal, mapFn}, h)
	if err != nil {
		t.Fatalf("MapMap on empty: %v", err)
	}
	lenVal, _ = MapLen([]vm.Value{result}, h)
	if lenVal.AsInt() != 0 {
		t.Error("map on empty map should return empty map")
	}

	// Wrong arg count.
	if _, err := MapMap([]vm.Value{mapVal}, h); err == nil {
		t.Error("MapMap with 1 arg should error")
	}
}

func TestMapUnhashableKey(t *testing.T) {
	h := newHeap()
	mapVal, _ := makeMap(h)

	// A closure is not hashable; using it as a key should produce an error.
	closureIdx := h.AllocClosure(0, nil)
	closureVal := vm.ObjVal(closureIdx)

	// MapSet with closure key should error.
	_, err := MapSet([]vm.Value{mapVal, closureVal, vm.IntVal(1)}, h)
	if err == nil {
		t.Error("MapSet with closure key should error (unhashable)")
	}

	// MapGet with closure key should error.
	_, err = MapGet([]vm.Value{mapVal, closureVal}, h)
	if err == nil {
		t.Error("MapGet with closure key should error (unhashable)")
	}

	// MapContains with closure key should error.
	_, err = MapContains([]vm.Value{mapVal, closureVal}, h)
	if err == nil {
		t.Error("MapContains with closure key should error (unhashable)")
	}

	// MapDelete with closure key should error.
	_, err = MapDelete([]vm.Value{mapVal, closureVal}, h)
	if err == nil {
		t.Error("MapDelete with closure key should error (unhashable)")
	}
}
