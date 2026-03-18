package stdlib

import (
	"fmt"
	"sort"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Array operations — operate on heap-allocated ArrayObj values
// ---------------------------------------------------------------------------

func resolveArray(v vm.Value, heap *vm.Heap) (*vm.ArrayObj, error) {
	if v.Tag != vm.TagObj {
		return nil, fmt.Errorf("expected Array (obj), got tag %d", v.Tag)
	}
	obj := heap.Get(v.AsObj())
	a, ok := obj.Data.(*vm.ArrayObj)
	if !ok {
		return nil, fmt.Errorf("expected Array object, got type %d", obj.Header.TypeID)
	}
	return a, nil
}

// ArrayLen returns the number of elements in an array.
func ArrayLen(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("array_len: expected 1 argument, got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_len: %w", err)
	}
	return vm.IntVal(int64(len(a.Elements))), nil
}

// ArrayPush appends an element to an array, returning a new array.
func ArrayPush(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("array_push: expected 2 arguments, got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_push: %w", err)
	}
	newElems := make([]vm.Value, len(a.Elements)+1)
	copy(newElems, a.Elements)
	newElems[len(a.Elements)] = args[1]
	idx := heap.AllocArray(newElems)
	return vm.ObjVal(idx), nil
}

// ArrayPop removes the last element from an array, returning a tuple (new_array, popped_element).
// Returns an error Result if the array is empty.
func ArrayPop(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("array_pop: expected 1 argument, got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_pop: %w", err)
	}
	if len(a.Elements) == 0 {
		return makeResultErr("array_pop: empty array", heap), nil
	}
	last := a.Elements[len(a.Elements)-1]
	newElems := make([]vm.Value, len(a.Elements)-1)
	copy(newElems, a.Elements[:len(a.Elements)-1])
	arrIdx := heap.AllocArray(newElems)
	tupleIdx := heap.AllocTuple([]vm.Value{vm.ObjVal(arrIdx), last})
	return makeResultOk(vm.ObjVal(tupleIdx), heap), nil
}

// ArrayMap applies a function to each element, returning a new array.
// The callback is invoked via the provided CallbackInvoker.
func ArrayMap(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("array_map: expected 2 arguments (array, func), got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_map: %w", err)
	}
	fn := args[1]
	if CallbackInvoker == nil {
		return vm.UnitVal(), fmt.Errorf("array_map: no callback invoker registered")
	}
	results := make([]vm.Value, len(a.Elements))
	for i, elem := range a.Elements {
		result, callErr := CallbackInvoker(fn, []vm.Value{elem}, heap)
		if callErr != nil {
			return vm.UnitVal(), fmt.Errorf("array_map: callback error at index %d: %w", i, callErr)
		}
		results[i] = result
	}
	idx := heap.AllocArray(results)
	return vm.ObjVal(idx), nil
}

// ArrayFilter keeps elements for which the predicate returns true.
func ArrayFilter(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("array_filter: expected 2 arguments (array, func), got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_filter: %w", err)
	}
	fn := args[1]
	if CallbackInvoker == nil {
		return vm.UnitVal(), fmt.Errorf("array_filter: no callback invoker registered")
	}
	var results []vm.Value
	for i, elem := range a.Elements {
		result, callErr := CallbackInvoker(fn, []vm.Value{elem}, heap)
		if callErr != nil {
			return vm.UnitVal(), fmt.Errorf("array_filter: callback error at index %d: %w", i, callErr)
		}
		if result.IsTruthy() {
			results = append(results, elem)
		}
	}
	if results == nil {
		results = []vm.Value{}
	}
	idx := heap.AllocArray(results)
	return vm.ObjVal(idx), nil
}

// ArrayFold reduces an array with an accumulator.
// Arguments: (array, initial_value, func(accumulator, element) -> accumulator)
func ArrayFold(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 3 {
		return vm.UnitVal(), fmt.Errorf("array_fold: expected 3 arguments (array, init, func), got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_fold: %w", err)
	}
	acc := args[1]
	fn := args[2]
	if CallbackInvoker == nil {
		return vm.UnitVal(), fmt.Errorf("array_fold: no callback invoker registered")
	}
	for i, elem := range a.Elements {
		result, callErr := CallbackInvoker(fn, []vm.Value{acc, elem}, heap)
		if callErr != nil {
			return vm.UnitVal(), fmt.Errorf("array_fold: callback error at index %d: %w", i, callErr)
		}
		acc = result
	}
	return acc, nil
}

// ArraySort sorts an array of integers or floats in ascending order, returning a new array.
// For mixed or non-numeric types, elements are sorted by their string representation.
func ArraySort(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("array_sort: expected 1 argument, got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_sort: %w", err)
	}
	sorted := make([]vm.Value, len(a.Elements))
	copy(sorted, a.Elements)
	sort.SliceStable(sorted, func(i, j int) bool {
		return compareValues(sorted[i], sorted[j], heap) < 0
	})
	idx := heap.AllocArray(sorted)
	return vm.ObjVal(idx), nil
}

// ArrayReverse returns a new array with elements in reverse order.
func ArrayReverse(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("array_reverse: expected 1 argument, got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_reverse: %w", err)
	}
	n := len(a.Elements)
	reversed := make([]vm.Value, n)
	for i, v := range a.Elements {
		reversed[n-1-i] = v
	}
	idx := heap.AllocArray(reversed)
	return vm.ObjVal(idx), nil
}

// ArrayContains checks whether an array contains a given value.
func ArrayContains(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("array_contains: expected 2 arguments, got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_contains: %w", err)
	}
	target := args[1]
	for _, elem := range a.Elements {
		if elem.Equal(target, heap) {
			return vm.BoolVal(true), nil
		}
	}
	return vm.BoolVal(false), nil
}

// ArrayZip combines two arrays into an array of tuples.
// Stops at the length of the shorter array.
func ArrayZip(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("array_zip: expected 2 arguments, got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_zip: %w", err)
	}
	b, err := resolveArray(args[1], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_zip: %w", err)
	}
	n := len(a.Elements)
	if len(b.Elements) < n {
		n = len(b.Elements)
	}
	elems := make([]vm.Value, n)
	for i := 0; i < n; i++ {
		tIdx := heap.AllocTuple([]vm.Value{a.Elements[i], b.Elements[i]})
		elems[i] = vm.ObjVal(tIdx)
	}
	idx := heap.AllocArray(elems)
	return vm.ObjVal(idx), nil
}

// ArrayEnumerate returns an array of (index, element) tuples.
func ArrayEnumerate(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("array_enumerate: expected 1 argument, got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_enumerate: %w", err)
	}
	elems := make([]vm.Value, len(a.Elements))
	for i, v := range a.Elements {
		tIdx := heap.AllocTuple([]vm.Value{vm.IntVal(int64(i)), v})
		elems[i] = vm.ObjVal(tIdx)
	}
	idx := heap.AllocArray(elems)
	return vm.ObjVal(idx), nil
}

// ArrayFlatMap applies a function that returns an array to each element,
// then flattens the results into a single array.
func ArrayFlatMap(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("array_flat_map: expected 2 arguments (array, func), got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("array_flat_map: %w", err)
	}
	fn := args[1]
	if CallbackInvoker == nil {
		return vm.UnitVal(), fmt.Errorf("array_flat_map: no callback invoker registered")
	}
	var results []vm.Value
	for i, elem := range a.Elements {
		result, callErr := CallbackInvoker(fn, []vm.Value{elem}, heap)
		if callErr != nil {
			return vm.UnitVal(), fmt.Errorf("array_flat_map: callback error at index %d: %w", i, callErr)
		}
		inner, innerErr := resolveArray(result, heap)
		if innerErr != nil {
			// If callback doesn't return an array, treat as single element.
			results = append(results, result)
		} else {
			results = append(results, inner.Elements...)
		}
	}
	if results == nil {
		results = []vm.Value{}
	}
	idx := heap.AllocArray(results)
	return vm.ObjVal(idx), nil
}

// ---------------------------------------------------------------------------
// CallbackInvoker — set by the VM to enable higher-order stdlib functions
// ---------------------------------------------------------------------------

// CallbackInvoker is set by the VM at startup. It calls a Ryx function value
// with the given arguments and returns the result.
var CallbackInvoker func(fn vm.Value, args []vm.Value, heap *vm.Heap) (vm.Value, error)

// ---------------------------------------------------------------------------
// Value comparison helper
// ---------------------------------------------------------------------------

func compareValues(a, b vm.Value, heap *vm.Heap) int {
	// Same-type numeric comparison.
	if a.Tag == vm.TagInt && b.Tag == vm.TagInt {
		ai, bi := a.AsInt(), b.AsInt()
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
		return 0
	}
	if a.Tag == vm.TagFloat && b.Tag == vm.TagFloat {
		af, bf := a.AsFloat(), b.AsFloat()
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	}
	if a.Tag == vm.TagChar && b.Tag == vm.TagChar {
		ac, bc := a.AsChar(), b.AsChar()
		if ac < bc {
			return -1
		}
		if ac > bc {
			return 1
		}
		return 0
	}
	if a.Tag == vm.TagBool && b.Tag == vm.TagBool {
		ai, bi := int(a.Data), int(b.Data)
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
		return 0
	}
	// String comparison.
	if a.Tag == vm.TagObj && b.Tag == vm.TagObj {
		sa := vm.StringValue(a, heap)
		sb := vm.StringValue(b, heap)
		if sa < sb {
			return -1
		}
		if sa > sb {
			return 1
		}
		return 0
	}
	// Fallback: compare by tag then data.
	if a.Tag != b.Tag {
		if a.Tag < b.Tag {
			return -1
		}
		return 1
	}
	if a.Data < b.Data {
		return -1
	}
	if a.Data > b.Data {
		return 1
	}
	return 0
}
