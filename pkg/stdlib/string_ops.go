package stdlib

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// String operations — all operate on heap-allocated StringObj values
// ---------------------------------------------------------------------------

// StringLen returns the number of Unicode codepoints in a string.
func StringLen(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("string_len: expected 1 argument, got %d", len(args))
	}
	s, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_len: %w", err)
	}
	return vm.IntVal(int64(utf8.RuneCountInString(s))), nil
}

// StringSlice returns a substring from start (inclusive) to end (exclusive) indices,
// where indices count Unicode codepoints.
func StringSlice(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 3 {
		return vm.UnitVal(), fmt.Errorf("string_slice: expected 3 arguments, got %d", len(args))
	}
	s, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_slice: %w", err)
	}
	start := int(args[1].AsInt())
	end := int(args[2].AsInt())

	runes := []rune(s)
	runeLen := len(runes)

	// Clamp indices to valid range.
	if start < 0 {
		start = 0
	}
	if end > runeLen {
		end = runeLen
	}
	if start > end {
		start = end
	}

	result := string(runes[start:end])
	idx := heap.AllocString(result)
	return vm.ObjVal(idx), nil
}

// StringContains checks whether haystack contains needle.
func StringContains(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("string_contains: expected 2 arguments, got %d", len(args))
	}
	haystack, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_contains: %w", err)
	}
	needle, err := resolveString(args[1], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_contains: %w", err)
	}
	return vm.BoolVal(strings.Contains(haystack, needle)), nil
}

// StringSplit splits a string by a separator, returning an array of strings.
func StringSplit(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("string_split: expected 2 arguments, got %d", len(args))
	}
	s, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_split: %w", err)
	}
	sep, err := resolveString(args[1], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_split: %w", err)
	}
	parts := strings.Split(s, sep)
	elems := make([]vm.Value, len(parts))
	for i, p := range parts {
		elems[i] = vm.ObjVal(heap.AllocString(p))
	}
	idx := heap.AllocArray(elems)
	return vm.ObjVal(idx), nil
}

// StringTrim removes leading and trailing whitespace.
func StringTrim(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("string_trim: expected 1 argument, got %d", len(args))
	}
	s, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_trim: %w", err)
	}
	idx := heap.AllocString(strings.TrimSpace(s))
	return vm.ObjVal(idx), nil
}

// StringChars splits a string into an array of individual character strings.
func StringChars(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("string_chars: expected 1 argument, got %d", len(args))
	}
	s, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_chars: %w", err)
	}
	runes := []rune(s)
	elems := make([]vm.Value, len(runes))
	for i, r := range runes {
		elems[i] = vm.CharVal(r)
	}
	idx := heap.AllocArray(elems)
	return vm.ObjVal(idx), nil
}

// CharToString converts a Char value to a single-character String.
func CharToString(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("char_to_string: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagChar {
		return vm.UnitVal(), fmt.Errorf("char_to_string: expected Char, got tag %d", args[0].Tag)
	}
	s := string(args[0].AsChar())
	idx := heap.AllocString(s)
	return vm.ObjVal(idx), nil
}

// StringReplace replaces all occurrences of old with new in a string.
func StringReplace(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 3 {
		return vm.UnitVal(), fmt.Errorf("string_replace: expected 3 arguments, got %d", len(args))
	}
	s, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_replace: %w", err)
	}
	old, err := resolveString(args[1], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_replace: %w", err)
	}
	newStr, err := resolveString(args[2], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_replace: %w", err)
	}
	result := strings.ReplaceAll(s, old, newStr)
	idx := heap.AllocString(result)
	return vm.ObjVal(idx), nil
}

// StringStartsWith checks if a string starts with the given prefix.
func StringStartsWith(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("string_starts_with: expected 2 arguments, got %d", len(args))
	}
	s, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_starts_with: %w", err)
	}
	prefix, err := resolveString(args[1], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_starts_with: %w", err)
	}
	return vm.BoolVal(strings.HasPrefix(s, prefix)), nil
}

// StringEndsWith checks if a string ends with the given suffix.
func StringEndsWith(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("string_ends_with: expected 2 arguments, got %d", len(args))
	}
	s, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_ends_with: %w", err)
	}
	suffix, err := resolveString(args[1], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_ends_with: %w", err)
	}
	return vm.BoolVal(strings.HasSuffix(s, suffix)), nil
}

// StringToUpper converts a string to uppercase.
func StringToUpper(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("string_to_upper: expected 1 argument, got %d", len(args))
	}
	s, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_to_upper: %w", err)
	}
	idx := heap.AllocString(strings.ToUpper(s))
	return vm.ObjVal(idx), nil
}

// StringToLower converts a string to lowercase.
func StringToLower(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("string_to_lower: expected 1 argument, got %d", len(args))
	}
	s, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("string_to_lower: %w", err)
	}
	idx := heap.AllocString(strings.ToLower(s))
	return vm.ObjVal(idx), nil
}
