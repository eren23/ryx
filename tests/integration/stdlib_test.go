package integration

import (
	"testing"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Built-in trait tests
//
// These tests exercise the builtin trait functions (Eq, Neq, Compare,
// ToString, Default, Clone, Hash) directly through the BuiltinRegistry API,
// using the vm.Heap for heap-allocated objects.
// ---------------------------------------------------------------------------

// newTestHeapAndRegistry returns a fresh vm.Heap and a BuiltinRegistry with
// all standard traits registered.
func newTestHeapAndRegistry() (*vm.Heap, *vm.BuiltinRegistry) {
	heap := vm.NewHeap()
	reg := vm.NewBuiltinRegistry()
	vm.RegisterBuiltinTraits(reg)
	return heap, reg
}

// TestBuiltinEq verifies the "eq" builtin for various value types.
func TestBuiltinEq(t *testing.T) {
	heap, reg := newTestHeapAndRegistry()

	tests := []struct {
		name string
		a, b vm.Value
		want bool
	}{
		{"int_equal", vm.IntVal(42), vm.IntVal(42), true},
		{"int_not_equal", vm.IntVal(1), vm.IntVal(2), false},
		{"bool_equal", vm.BoolVal(true), vm.BoolVal(true), true},
		{"bool_not_equal", vm.BoolVal(true), vm.BoolVal(false), false},
		{"float_equal", vm.FloatVal(3.14), vm.FloatVal(3.14), true},
		{"float_not_equal", vm.FloatVal(1.0), vm.FloatVal(2.0), false},
		{"char_equal", vm.CharVal('a'), vm.CharVal('a'), true},
		{"char_not_equal", vm.CharVal('a'), vm.CharVal('b'), false},
		{"unit_equal", vm.UnitVal(), vm.UnitVal(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := reg.Call("eq", []vm.Value{tt.a, tt.b}, heap)
			if err != nil {
				t.Fatalf("eq returned error: %v", err)
			}
			if result.AsBool() != tt.want {
				t.Errorf("eq(%v, %v) = %v, want %v", tt.a, tt.b, result.AsBool(), tt.want)
			}
		})
	}

	// Test string equality via heap objects.
	t.Run("string_equal", func(t *testing.T) {
		idx1 := heap.AllocString("hello")
		idx2 := heap.AllocString("hello")
		result, err := reg.Call("eq", []vm.Value{vm.ObjVal(idx1), vm.ObjVal(idx2)}, heap)
		if err != nil {
			t.Fatalf("eq returned error: %v", err)
		}
		if !result.AsBool() {
			t.Error("expected equal strings to be eq")
		}
	})

	t.Run("string_not_equal", func(t *testing.T) {
		idx1 := heap.AllocString("hello")
		idx2 := heap.AllocString("world")
		result, err := reg.Call("eq", []vm.Value{vm.ObjVal(idx1), vm.ObjVal(idx2)}, heap)
		if err != nil {
			t.Fatalf("eq returned error: %v", err)
		}
		if result.AsBool() {
			t.Error("expected different strings to be not eq")
		}
	})

	// Test array equality.
	t.Run("array_equal", func(t *testing.T) {
		idx1 := heap.AllocArray([]vm.Value{vm.IntVal(1), vm.IntVal(2)})
		idx2 := heap.AllocArray([]vm.Value{vm.IntVal(1), vm.IntVal(2)})
		result, err := reg.Call("eq", []vm.Value{vm.ObjVal(idx1), vm.ObjVal(idx2)}, heap)
		if err != nil {
			t.Fatalf("eq returned error: %v", err)
		}
		if !result.AsBool() {
			t.Error("expected equal arrays to be eq")
		}
	})

	// Test argument count error.
	t.Run("wrong_arg_count", func(t *testing.T) {
		_, err := reg.Call("eq", []vm.Value{vm.IntVal(1)}, heap)
		if err == nil {
			t.Error("expected error for wrong argument count")
		}
	})
}

// TestBuiltinNeq verifies the "neq" builtin.
func TestBuiltinNeq(t *testing.T) {
	heap, reg := newTestHeapAndRegistry()

	result, err := reg.Call("neq", []vm.Value{vm.IntVal(1), vm.IntVal(2)}, heap)
	if err != nil {
		t.Fatalf("neq returned error: %v", err)
	}
	if !result.AsBool() {
		t.Error("expected 1 neq 2 to be true")
	}

	result, err = reg.Call("neq", []vm.Value{vm.IntVal(5), vm.IntVal(5)}, heap)
	if err != nil {
		t.Fatalf("neq returned error: %v", err)
	}
	if result.AsBool() {
		t.Error("expected 5 neq 5 to be false")
	}
}

// TestBuiltinCompare verifies the "compare" builtin returns -1, 0, or 1.
func TestBuiltinCompare(t *testing.T) {
	heap, reg := newTestHeapAndRegistry()

	tests := []struct {
		name string
		a, b vm.Value
		want int64
	}{
		{"int_less", vm.IntVal(1), vm.IntVal(2), -1},
		{"int_equal", vm.IntVal(5), vm.IntVal(5), 0},
		{"int_greater", vm.IntVal(10), vm.IntVal(3), 1},
		{"float_less", vm.FloatVal(1.0), vm.FloatVal(2.0), -1},
		{"float_equal", vm.FloatVal(3.14), vm.FloatVal(3.14), 0},
		{"float_greater", vm.FloatVal(9.0), vm.FloatVal(1.0), 1},
		{"char_less", vm.CharVal('a'), vm.CharVal('z'), -1},
		{"char_equal", vm.CharVal('m'), vm.CharVal('m'), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := reg.Call("compare", []vm.Value{tt.a, tt.b}, heap)
			if err != nil {
				t.Fatalf("compare returned error: %v", err)
			}
			if result.AsInt() != tt.want {
				t.Errorf("compare(%v, %v) = %d, want %d", tt.a, tt.b, result.AsInt(), tt.want)
			}
		})
	}

	// Test string comparison.
	t.Run("string_less", func(t *testing.T) {
		idx1 := heap.AllocString("apple")
		idx2 := heap.AllocString("banana")
		result, err := reg.Call("compare", []vm.Value{vm.ObjVal(idx1), vm.ObjVal(idx2)}, heap)
		if err != nil {
			t.Fatalf("compare returned error: %v", err)
		}
		if result.AsInt() != -1 {
			t.Errorf("expected 'apple' < 'banana' (compare = -1), got %d", result.AsInt())
		}
	})

	// Test array lexicographic comparison.
	t.Run("array_compare", func(t *testing.T) {
		idx1 := heap.AllocArray([]vm.Value{vm.IntVal(1), vm.IntVal(2)})
		idx2 := heap.AllocArray([]vm.Value{vm.IntVal(1), vm.IntVal(3)})
		result, err := reg.Call("compare", []vm.Value{vm.ObjVal(idx1), vm.ObjVal(idx2)}, heap)
		if err != nil {
			t.Fatalf("compare returned error: %v", err)
		}
		if result.AsInt() != -1 {
			t.Errorf("expected [1,2] < [1,3] (compare = -1), got %d", result.AsInt())
		}
	})
}

// TestBuiltinToString verifies the "to_string" builtin for various types.
func TestBuiltinToString(t *testing.T) {
	heap, reg := newTestHeapAndRegistry()

	tests := []struct {
		name string
		val  vm.Value
		want string
	}{
		{"int", vm.IntVal(42), "42"},
		{"int_negative", vm.IntVal(-7), "-7"},
		{"float", vm.FloatVal(3.14), "3.14"},
		{"bool_true", vm.BoolVal(true), "true"},
		{"bool_false", vm.BoolVal(false), "false"},
		{"unit", vm.UnitVal(), "()"},
		{"char", vm.CharVal('X'), "'X'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := reg.Call("to_string", []vm.Value{tt.val}, heap)
			if err != nil {
				t.Fatalf("to_string returned error: %v", err)
			}
			if result.Tag != vm.TagObj {
				t.Fatalf("expected TagObj result, got tag=%d", result.Tag)
			}
			obj := heap.Get(result.AsObj())
			strObj, ok := obj.Data.(*vm.StringObj)
			if !ok {
				t.Fatalf("expected *StringObj, got %T", obj.Data)
			}
			if strObj.Value != tt.want {
				t.Errorf("to_string(%v) = %q, want %q", tt.val, strObj.Value, tt.want)
			}
		})
	}
}

// TestBuiltinDefault verifies the "default" builtin returns zero values.
func TestBuiltinDefault(t *testing.T) {
	heap, reg := newTestHeapAndRegistry()

	tests := []struct {
		name string
		tag  byte
		check func(vm.Value) bool
	}{
		{"int", vm.TagInt, func(v vm.Value) bool { return v.Tag == vm.TagInt && v.AsInt() == 0 }},
		{"float", vm.TagFloat, func(v vm.Value) bool { return v.Tag == vm.TagFloat && v.AsFloat() == 0 }},
		{"bool", vm.TagBool, func(v vm.Value) bool { return v.Tag == vm.TagBool && !v.AsBool() }},
		{"char", vm.TagChar, func(v vm.Value) bool { return v.Tag == vm.TagChar && v.AsChar() == 0 }},
		{"unit", vm.TagUnit, func(v vm.Value) bool { return v.Tag == vm.TagUnit }},
		{"string", vm.ObjString, func(v vm.Value) bool {
			if v.Tag != vm.TagObj {
				return false
			}
			obj := heap.Get(v.AsObj())
			s, ok := obj.Data.(*vm.StringObj)
			return ok && s.Value == ""
		}},
		{"array", vm.ObjArray, func(v vm.Value) bool {
			if v.Tag != vm.TagObj {
				return false
			}
			obj := heap.Get(v.AsObj())
			a, ok := obj.Data.(*vm.ArrayObj)
			return ok && len(a.Elements) == 0
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := reg.Call("default", []vm.Value{vm.IntVal(int64(tt.tag))}, heap)
			if err != nil {
				t.Fatalf("default returned error: %v", err)
			}
			if !tt.check(result) {
				t.Errorf("default(%d) returned unexpected value: %v", tt.tag, result)
			}
		})
	}
}

// TestBuiltinClone verifies the "clone" builtin performs deep copies.
func TestBuiltinClone(t *testing.T) {
	heap, reg := newTestHeapAndRegistry()

	// Cloning a primitive returns the same value.
	t.Run("clone_int", func(t *testing.T) {
		result, err := reg.Call("clone", []vm.Value{vm.IntVal(42)}, heap)
		if err != nil {
			t.Fatalf("clone returned error: %v", err)
		}
		if result.AsInt() != 42 {
			t.Errorf("expected 42, got %d", result.AsInt())
		}
	})

	// Cloning an array creates a new, independent copy.
	t.Run("clone_array", func(t *testing.T) {
		origIdx := heap.AllocArray([]vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3)})
		origVal := vm.ObjVal(origIdx)

		result, err := reg.Call("clone", []vm.Value{origVal}, heap)
		if err != nil {
			t.Fatalf("clone returned error: %v", err)
		}

		// The cloned object should be at a different heap index.
		if result.AsObj() == origIdx {
			t.Error("cloned array has the same heap index as original (expected a new copy)")
		}

		// Elements should be equal.
		origArr := heap.Get(origIdx).Data.(*vm.ArrayObj)
		clonedArr := heap.Get(result.AsObj()).Data.(*vm.ArrayObj)
		if len(origArr.Elements) != len(clonedArr.Elements) {
			t.Fatalf("length mismatch: orig=%d, cloned=%d", len(origArr.Elements), len(clonedArr.Elements))
		}
		for i := range origArr.Elements {
			if origArr.Elements[i] != clonedArr.Elements[i] {
				t.Errorf("element %d: orig=%v, cloned=%v", i, origArr.Elements[i], clonedArr.Elements[i])
			}
		}
	})

	// Cloning a string creates a new heap object.
	t.Run("clone_string", func(t *testing.T) {
		origIdx := heap.AllocString("hello")
		result, err := reg.Call("clone", []vm.Value{vm.ObjVal(origIdx)}, heap)
		if err != nil {
			t.Fatalf("clone returned error: %v", err)
		}
		if result.AsObj() == origIdx {
			t.Error("cloned string has the same heap index as original")
		}
		clonedStr := heap.Get(result.AsObj()).Data.(*vm.StringObj)
		if clonedStr.Value != "hello" {
			t.Errorf("expected 'hello', got %q", clonedStr.Value)
		}
	})

	// Cloning a struct creates a deep copy.
	t.Run("clone_struct", func(t *testing.T) {
		origIdx := heap.AllocStruct(1, []vm.Value{vm.IntVal(10), vm.IntVal(20)})
		result, err := reg.Call("clone", []vm.Value{vm.ObjVal(origIdx)}, heap)
		if err != nil {
			t.Fatalf("clone returned error: %v", err)
		}
		if result.AsObj() == origIdx {
			t.Error("cloned struct has the same heap index as original")
		}
		origStruct := heap.Get(origIdx).Data.(*vm.StructObj)
		clonedStruct := heap.Get(result.AsObj()).Data.(*vm.StructObj)
		if origStruct.TypeIdx != clonedStruct.TypeIdx {
			t.Errorf("type index mismatch: %d vs %d", origStruct.TypeIdx, clonedStruct.TypeIdx)
		}
	})
}

// TestBuiltinHash verifies the "hash" builtin produces consistent hashes.
func TestBuiltinHash(t *testing.T) {
	heap, reg := newTestHeapAndRegistry()

	// Same values produce the same hash.
	t.Run("deterministic", func(t *testing.T) {
		h1, err := reg.Call("hash", []vm.Value{vm.IntVal(42)}, heap)
		if err != nil {
			t.Fatalf("hash returned error: %v", err)
		}
		h2, err := reg.Call("hash", []vm.Value{vm.IntVal(42)}, heap)
		if err != nil {
			t.Fatalf("hash returned error: %v", err)
		}
		if h1.AsInt() != h2.AsInt() {
			t.Errorf("hash(42) not deterministic: %d vs %d", h1.AsInt(), h2.AsInt())
		}
	})

	// Different values generally produce different hashes (not guaranteed but very likely).
	t.Run("different_values", func(t *testing.T) {
		h1, err := reg.Call("hash", []vm.Value{vm.IntVal(1)}, heap)
		if err != nil {
			t.Fatalf("hash returned error: %v", err)
		}
		h2, err := reg.Call("hash", []vm.Value{vm.IntVal(2)}, heap)
		if err != nil {
			t.Fatalf("hash returned error: %v", err)
		}
		if h1.AsInt() == h2.AsInt() {
			t.Log("warning: hash(1) == hash(2), collision detected (unlikely but possible)")
		}
	})

	// Different types produce different hashes (due to tag byte).
	t.Run("different_types", func(t *testing.T) {
		hInt, err := reg.Call("hash", []vm.Value{vm.IntVal(0)}, heap)
		if err != nil {
			t.Fatalf("hash returned error: %v", err)
		}
		hFloat, err := reg.Call("hash", []vm.Value{vm.FloatVal(0)}, heap)
		if err != nil {
			t.Fatalf("hash returned error: %v", err)
		}
		if hInt.AsInt() == hFloat.AsInt() {
			t.Error("hash(Int(0)) == hash(Float(0)), expected different hashes due to type tags")
		}
	})

	// String hashing.
	t.Run("string_hash", func(t *testing.T) {
		idx1 := heap.AllocString("hello")
		idx2 := heap.AllocString("hello")
		h1, err := reg.Call("hash", []vm.Value{vm.ObjVal(idx1)}, heap)
		if err != nil {
			t.Fatalf("hash returned error: %v", err)
		}
		h2, err := reg.Call("hash", []vm.Value{vm.ObjVal(idx2)}, heap)
		if err != nil {
			t.Fatalf("hash returned error: %v", err)
		}
		if h1.AsInt() != h2.AsInt() {
			t.Errorf("hash of equal strings differs: %d vs %d", h1.AsInt(), h2.AsInt())
		}
	})

	// Array hashing.
	t.Run("array_hash", func(t *testing.T) {
		idx1 := heap.AllocArray([]vm.Value{vm.IntVal(1), vm.IntVal(2)})
		idx2 := heap.AllocArray([]vm.Value{vm.IntVal(1), vm.IntVal(2)})
		h1, err := reg.Call("hash", []vm.Value{vm.ObjVal(idx1)}, heap)
		if err != nil {
			t.Fatalf("hash returned error: %v", err)
		}
		h2, err := reg.Call("hash", []vm.Value{vm.ObjVal(idx2)}, heap)
		if err != nil {
			t.Fatalf("hash returned error: %v", err)
		}
		if h1.AsInt() != h2.AsInt() {
			t.Errorf("hash of equal arrays differs: %d vs %d", h1.AsInt(), h2.AsInt())
		}
	})
}
