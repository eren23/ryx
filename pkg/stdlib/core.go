package stdlib

import (
	"fmt"
	"strconv"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Type conversions
// ---------------------------------------------------------------------------

// IntToFloat converts an Int to a Float.
func IntToFloat(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("int_to_float: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("int_to_float: expected Int, got tag %d", args[0].Tag)
	}
	return vm.FloatVal(float64(args[0].AsInt())), nil
}

// FloatToInt truncates a Float to an Int.
func FloatToInt(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("float_to_int: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagFloat {
		return vm.UnitVal(), fmt.Errorf("float_to_int: expected Float, got tag %d", args[0].Tag)
	}
	return vm.IntVal(int64(args[0].AsFloat())), nil
}

// IntToString converts an Int to its decimal string representation.
func IntToString(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("int_to_string: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("int_to_string: expected Int, got tag %d", args[0].Tag)
	}
	s := strconv.FormatInt(args[0].AsInt(), 10)
	idx := heap.AllocString(s)
	return vm.ObjVal(idx), nil
}

// FloatToString converts a Float to its string representation.
func FloatToString(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("float_to_string: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagFloat {
		return vm.UnitVal(), fmt.Errorf("float_to_string: expected Float, got tag %d", args[0].Tag)
	}
	s := strconv.FormatFloat(args[0].AsFloat(), 'g', -1, 64)
	idx := heap.AllocString(s)
	return vm.ObjVal(idx), nil
}

// ParseInt parses a string as a base-10 integer, returning a Result enum.
// Ok(Int) on success, Err(String) on failure.
func ParseInt(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("parse_int: expected 1 argument, got %d", len(args))
	}
	s, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("parse_int: %w", err)
	}
	n, parseErr := strconv.ParseInt(s, 10, 64)
	if parseErr != nil {
		return makeResultErr(parseErr.Error(), heap), nil
	}
	return makeResultOk(vm.IntVal(n), heap), nil
}

// ParseFloat parses a string as a float, returning a Result enum.
func ParseFloat(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("parse_float: expected 1 argument, got %d", len(args))
	}
	s, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("parse_float: %w", err)
	}
	f, parseErr := strconv.ParseFloat(s, 64)
	if parseErr != nil {
		return makeResultErr(parseErr.Error(), heap), nil
	}
	return makeResultOk(vm.FloatVal(f), heap), nil
}

// ---------------------------------------------------------------------------
// I/O — print / println / read_line
// ---------------------------------------------------------------------------

// OutputWriter is set by the host to capture output. Defaults to fmt.Print.
var OutputWriter func(s string) = func(s string) { fmt.Print(s) }

// InputReader is set by the host to provide input. Defaults to fmt.Scanln.
var InputReader func() (string, error) = func() (string, error) {
	var s string
	_, err := fmt.Scanln(&s)
	return s, err
}

// Print outputs a value without a trailing newline.
func Print(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	for _, a := range args {
		OutputWriter(vm.StringValue(a, heap))
	}
	return vm.UnitVal(), nil
}

// Println outputs a value followed by a newline.
func Println(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	for i, a := range args {
		if i > 0 {
			OutputWriter(" ")
		}
		OutputWriter(vm.StringValue(a, heap))
	}
	OutputWriter("\n")
	return vm.UnitVal(), nil
}

// ReadLine reads a line of input, returning a Result.
func ReadLine(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	s, err := InputReader()
	if err != nil {
		return makeResultErr(err.Error(), heap), nil
	}
	idx := heap.AllocString(s)
	return makeResultOk(vm.ObjVal(idx), heap), nil
}

// ---------------------------------------------------------------------------
// Assertions
// ---------------------------------------------------------------------------

// Assert checks that a condition is true. Panics with a RuntimeError if not.
func Assert(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) < 1 {
		return vm.UnitVal(), fmt.Errorf("assert: expected at least 1 argument")
	}
	if !args[0].IsTruthy() {
		msg := "assertion failed"
		if len(args) >= 2 {
			if s, err := resolveString(args[1], heap); err == nil {
				msg = s
			}
		}
		return vm.UnitVal(), &vm.RuntimeError{Message: msg}
	}
	return vm.UnitVal(), nil
}

// AssertEq checks that two values are equal. Panics with a RuntimeError if not.
func AssertEq(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) < 2 {
		return vm.UnitVal(), fmt.Errorf("assert_eq: expected at least 2 arguments, got %d", len(args))
	}
	if !args[0].Equal(args[1], heap) {
		left := vm.StringValue(args[0], heap)
		right := vm.StringValue(args[1], heap)
		msg := fmt.Sprintf("assertion failed: %s != %s", left, right)
		if len(args) >= 3 {
			if s, err := resolveString(args[2], heap); err == nil {
				msg = s
			}
		}
		return vm.UnitVal(), &vm.RuntimeError{Message: msg}
	}
	return vm.UnitVal(), nil
}

// Panic unconditionally raises a RuntimeError with a message and stack trace.
func Panic(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	msg := "explicit panic"
	if len(args) >= 1 {
		if s, err := resolveString(args[0], heap); err == nil {
			msg = s
		} else {
			msg = vm.StringValue(args[0], heap)
		}
	}
	return vm.UnitVal(), &vm.RuntimeError{Message: "panic: " + msg}
}

// ---------------------------------------------------------------------------
// Result helpers (represented as EnumObj with typeIdx=0xFFFF)
//
//	variant 0 = Ok(value)
//	variant 1 = Err(string)
// ---------------------------------------------------------------------------

const ResultTypeIdx = 0xFFFF

func makeResultOk(val vm.Value, heap *vm.Heap) vm.Value {
	idx := heap.AllocEnum(ResultTypeIdx, 0, []vm.Value{val})
	return vm.ObjVal(idx)
}

func makeResultErr(msg string, heap *vm.Heap) vm.Value {
	strIdx := heap.AllocString(msg)
	idx := heap.AllocEnum(ResultTypeIdx, 1, []vm.Value{vm.ObjVal(strIdx)})
	return vm.ObjVal(idx)
}

// IsResultOk checks whether a Result value is the Ok variant.
func IsResultOk(v vm.Value, heap *vm.Heap) bool {
	if v.Tag != vm.TagObj {
		return false
	}
	obj := heap.Get(v.AsObj())
	e, ok := obj.Data.(*vm.EnumObj)
	if !ok || e.TypeIdx != ResultTypeIdx {
		return false
	}
	return e.VariantIdx == 0
}

// ResultUnwrap returns the inner value of an Ok, or an error for Err.
func ResultUnwrap(v vm.Value, heap *vm.Heap) (vm.Value, error) {
	if v.Tag != vm.TagObj {
		return v, fmt.Errorf("unwrap: not a Result")
	}
	obj := heap.Get(v.AsObj())
	e, ok := obj.Data.(*vm.EnumObj)
	if !ok || e.TypeIdx != ResultTypeIdx {
		return v, fmt.Errorf("unwrap: not a Result")
	}
	if e.VariantIdx == 1 {
		msg := "Err"
		if len(e.Fields) > 0 {
			msg = vm.StringValue(e.Fields[0], heap)
		}
		return vm.UnitVal(), fmt.Errorf("unwrap called on Err: %s", msg)
	}
	if len(e.Fields) > 0 {
		return e.Fields[0], nil
	}
	return vm.UnitVal(), nil
}

// ---------------------------------------------------------------------------
// RegisterAll registers every stdlib function in the given BuiltinRegistry.
// This includes type conversions, I/O, assertions, string/array/math ops,
// file I/O, and built-in trait methods.
// ---------------------------------------------------------------------------

func RegisterAll(r *vm.BuiltinRegistry) {
	// Type conversions (core.go)
	r.Register("int_to_float", IntToFloat)
	r.Register("float_to_int", FloatToInt)
	r.Register("int_to_string", IntToString)
	r.Register("float_to_string", FloatToString)
	r.Register("parse_int", ParseInt)
	r.Register("parse_float", ParseFloat)

	// I/O (core.go)
	r.Register("print", Print)
	r.Register("println", Println)
	r.Register("read_line", ReadLine)

	// Assertions (core.go)
	r.Register("assert", Assert)
	r.Register("assert_eq", AssertEq)
	r.Register("panic", Panic)

	// String operations (string_ops.go)
	r.Register("string_len", StringLen)
	r.Register("string_slice", StringSlice)
	r.Register("string_contains", StringContains)
	r.Register("string_index_of", StringIndexOf)
	r.Register("string_repeat", StringRepeat)
	r.Register("string_pad_left", StringPadLeft)
	r.Register("string_pad_right", StringPadRight)
	r.Register("string_bytes", StringBytes)
	r.Register("string_join", StringJoin)
	r.Register("string_split", StringSplit)
	r.Register("string_trim", StringTrim)
	r.Register("string_chars", StringChars)
	r.Register("char_to_string", CharToString)
	r.Register("string_replace", StringReplace)
	r.Register("string_starts_with", StringStartsWith)
	r.Register("string_ends_with", StringEndsWith)
	r.Register("string_to_upper", StringToUpper)
	r.Register("string_to_lower", StringToLower)

	// Array operations (array_ops.go)
	r.Register("array_len", ArrayLen)
	r.Register("array_push", ArrayPush)
	r.Register("array_pop", ArrayPop)
	r.Register("array_map", ArrayMap)
	r.Register("array_filter", ArrayFilter)
	r.Register("array_fold", ArrayFold)
	r.Register("array_sort", ArraySort)
	r.Register("array_reverse", ArrayReverse)
	r.Register("array_contains", ArrayContains)
	r.Register("array_zip", ArrayZip)
	r.Register("array_enumerate", ArrayEnumerate)
	r.Register("array_flat_map", ArrayFlatMap)

	// Additional array operations (array_ops.go)
	r.Register("array_find", ArrayFind)
	r.Register("array_any", ArrayAny)
	r.Register("array_all", ArrayAll)
	r.Register("array_sum", ArraySum)
	r.Register("array_min", ArrayMin)
	r.Register("array_max", ArrayMax)
	r.Register("array_take", ArrayTake)
	r.Register("array_drop", ArrayDrop)
	r.Register("array_chunk", ArrayChunk)
	r.Register("array_unique", ArrayUnique)
	r.Register("array_join", ArrayJoin)
	r.Register("array_slice", ArraySlice)

	// Math operations (math_ops.go)
	r.Register("abs", Abs)
	r.Register("min", Min)
	r.Register("max", Max)
	r.Register("sqrt", Sqrt)
	r.Register("pow", Pow)
	r.Register("floor", Floor)
	r.Register("ceil", Ceil)
	r.Register("round", Round)
	r.Register("sin", Sin)
	r.Register("cos", Cos)
	r.Register("tan", Tan)
	r.Register("asin", Asin)
	r.Register("acos", Acos)
	r.Register("atan", Atan)
	r.Register("atan2", Atan2)
	r.Register("log", Log)
	r.Register("log2", Log2)
	r.Register("log10", Log10)
	r.Register("exp", Exp)
	r.Register("pi", Pi)
	r.Register("e", E)
	r.Register("gcd", Gcd)
	r.Register("lcm", Lcm)
	r.Register("clamp", Clamp)
	r.Register("random_int", RandomInt)
	r.Register("random_float", RandomFloat)

	// File I/O (io.go)
	r.Register("read_file", ReadFile)
	r.Register("write_file", WriteFile)
	r.Register("file_exists", FileExists)
	r.Register("dir_list", DirList)
	r.Register("dir_create", DirCreate)
	r.Register("path_join", PathJoin)
	r.Register("path_dirname", PathDirname)
	r.Register("path_basename", PathBasename)
	r.Register("path_extension", PathExtension)
	r.Register("file_size", FileSize)

	// Built-in trait methods (vm/builtins.go)
	vm.RegisterBuiltinTraits(r)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func resolveString(v vm.Value, heap *vm.Heap) (string, error) {
	if v.Tag == vm.TagObj {
		obj := heap.Get(v.AsObj())
		if s, ok := obj.Data.(*vm.StringObj); ok {
			return s.Value, nil
		}
		return "", fmt.Errorf("expected String object, got type %d", obj.Header.TypeID)
	}
	return "", fmt.Errorf("expected String (obj), got tag %d", v.Tag)
}
