package stdlib

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newHeap() *vm.Heap { return vm.NewHeap() }

func allocStr(heap *vm.Heap, s string) vm.Value {
	return vm.ObjVal(heap.AllocString(s))
}

func allocArr(heap *vm.Heap, elems []vm.Value) vm.Value {
	return vm.ObjVal(heap.AllocArray(elems))
}

func getString(v vm.Value, heap *vm.Heap) string {
	obj := heap.Get(v.AsObj())
	return obj.Data.(*vm.StringObj).Value
}

func getArray(v vm.Value, heap *vm.Heap) []vm.Value {
	obj := heap.Get(v.AsObj())
	return obj.Data.(*vm.ArrayObj).Elements
}

func getTuple(v vm.Value, heap *vm.Heap) []vm.Value {
	obj := heap.Get(v.AsObj())
	return obj.Data.(*vm.TupleObj).Elements
}

// ---------------------------------------------------------------------------
// core.go — Type conversions
// ---------------------------------------------------------------------------

func TestIntToFloat(t *testing.T) {
	h := newHeap()
	tests := []struct {
		in  int64
		out float64
	}{
		{0, 0.0},
		{42, 42.0},
		{-100, -100.0},
		{math.MaxInt32, float64(math.MaxInt32)},
	}
	for _, tc := range tests {
		v, err := IntToFloat([]vm.Value{vm.IntVal(tc.in)}, h)
		if err != nil {
			t.Fatalf("IntToFloat(%d): %v", tc.in, err)
		}
		if v.Tag != vm.TagFloat || v.AsFloat() != tc.out {
			t.Errorf("IntToFloat(%d) = %v, want %v", tc.in, v.AsFloat(), tc.out)
		}
	}
	// Wrong arg count.
	if _, err := IntToFloat(nil, h); err == nil {
		t.Error("expected error for 0 args")
	}
	// Wrong type.
	if _, err := IntToFloat([]vm.Value{vm.BoolVal(true)}, h); err == nil {
		t.Error("expected error for Bool arg")
	}
}

func TestFloatToInt(t *testing.T) {
	h := newHeap()
	tests := []struct {
		in  float64
		out int64
	}{
		{0.0, 0},
		{3.7, 3},
		{-2.9, -2},
		{42.0, 42},
	}
	for _, tc := range tests {
		v, err := FloatToInt([]vm.Value{vm.FloatVal(tc.in)}, h)
		if err != nil {
			t.Fatalf("FloatToInt(%v): %v", tc.in, err)
		}
		if v.Tag != vm.TagInt || v.AsInt() != tc.out {
			t.Errorf("FloatToInt(%v) = %d, want %d", tc.in, v.AsInt(), tc.out)
		}
	}
}

func TestIntToString(t *testing.T) {
	h := newHeap()
	tests := []struct {
		in  int64
		out string
	}{
		{0, "0"},
		{-42, "-42"},
		{12345, "12345"},
	}
	for _, tc := range tests {
		v, err := IntToString([]vm.Value{vm.IntVal(tc.in)}, h)
		if err != nil {
			t.Fatalf("IntToString(%d): %v", tc.in, err)
		}
		got := getString(v, h)
		if got != tc.out {
			t.Errorf("IntToString(%d) = %q, want %q", tc.in, got, tc.out)
		}
	}
}

func TestFloatToString(t *testing.T) {
	h := newHeap()
	v, err := FloatToString([]vm.Value{vm.FloatVal(3.14)}, h)
	if err != nil {
		t.Fatal(err)
	}
	got := getString(v, h)
	if got != "3.14" {
		t.Errorf("FloatToString(3.14) = %q, want %q", got, "3.14")
	}
}

func TestParseInt(t *testing.T) {
	h := newHeap()
	// Success.
	v, err := ParseInt([]vm.Value{allocStr(h, "42")}, h)
	if err != nil {
		t.Fatal(err)
	}
	if !IsResultOk(v, h) {
		t.Fatal("expected Ok result")
	}
	inner, _ := ResultUnwrap(v, h)
	if inner.AsInt() != 42 {
		t.Errorf("ParseInt(\"42\") = %d, want 42", inner.AsInt())
	}

	// Failure — not a number.
	v, err = ParseInt([]vm.Value{allocStr(h, "abc")}, h)
	if err != nil {
		t.Fatal(err)
	}
	if IsResultOk(v, h) {
		t.Error("expected Err result for non-numeric input")
	}

	// Empty string.
	v, _ = ParseInt([]vm.Value{allocStr(h, "")}, h)
	if IsResultOk(v, h) {
		t.Error("expected Err result for empty string")
	}

	// Negative number.
	v, _ = ParseInt([]vm.Value{allocStr(h, "-99")}, h)
	inner, _ = ResultUnwrap(v, h)
	if inner.AsInt() != -99 {
		t.Errorf("ParseInt(\"-99\") = %d, want -99", inner.AsInt())
	}
}

func TestParseFloat(t *testing.T) {
	h := newHeap()
	v, err := ParseFloat([]vm.Value{allocStr(h, "3.14")}, h)
	if err != nil {
		t.Fatal(err)
	}
	if !IsResultOk(v, h) {
		t.Fatal("expected Ok result")
	}
	inner, _ := ResultUnwrap(v, h)
	if math.Abs(inner.AsFloat()-3.14) > 1e-10 {
		t.Errorf("ParseFloat(\"3.14\") = %v, want 3.14", inner.AsFloat())
	}

	// Failure.
	v, _ = ParseFloat([]vm.Value{allocStr(h, "not_a_float")}, h)
	if IsResultOk(v, h) {
		t.Error("expected Err result")
	}
}

// ---------------------------------------------------------------------------
// core.go — Print / Println
// ---------------------------------------------------------------------------

func TestPrint(t *testing.T) {
	h := newHeap()
	var buf strings.Builder
	origWriter := OutputWriter
	OutputWriter = func(s string) { buf.WriteString(s) }
	defer func() { OutputWriter = origWriter }()

	Print([]vm.Value{vm.IntVal(42)}, h)
	if buf.String() != "42" {
		t.Errorf("Print(42) output = %q, want %q", buf.String(), "42")
	}
}

func TestPrintln(t *testing.T) {
	h := newHeap()
	var buf strings.Builder
	origWriter := OutputWriter
	OutputWriter = func(s string) { buf.WriteString(s) }
	defer func() { OutputWriter = origWriter }()

	Println([]vm.Value{allocStr(h, "hello"), vm.IntVal(42)}, h)
	if buf.String() != "hello 42\n" {
		t.Errorf("Println output = %q, want %q", buf.String(), "hello 42\n")
	}
}

// ---------------------------------------------------------------------------
// core.go — Assertions
// ---------------------------------------------------------------------------

func TestAssertPass(t *testing.T) {
	h := newHeap()
	_, err := Assert([]vm.Value{vm.BoolVal(true)}, h)
	if err != nil {
		t.Errorf("Assert(true) returned error: %v", err)
	}
}

func TestAssertFail(t *testing.T) {
	h := newHeap()
	_, err := Assert([]vm.Value{vm.BoolVal(false)}, h)
	if err == nil {
		t.Error("Assert(false) should return error")
	}
	re, ok := err.(*vm.RuntimeError)
	if !ok {
		t.Fatalf("expected RuntimeError, got %T", err)
	}
	if !strings.Contains(re.Message, "assertion failed") {
		t.Errorf("unexpected message: %s", re.Message)
	}
}

func TestAssertWithMessage(t *testing.T) {
	h := newHeap()
	_, err := Assert([]vm.Value{vm.BoolVal(false), allocStr(h, "custom msg")}, h)
	re := err.(*vm.RuntimeError)
	if re.Message != "custom msg" {
		t.Errorf("Assert message = %q, want %q", re.Message, "custom msg")
	}
}

func TestAssertEqPass(t *testing.T) {
	h := newHeap()
	_, err := AssertEq([]vm.Value{vm.IntVal(5), vm.IntVal(5)}, h)
	if err != nil {
		t.Errorf("AssertEq(5, 5) returned error: %v", err)
	}
}

func TestAssertEqFail(t *testing.T) {
	h := newHeap()
	_, err := AssertEq([]vm.Value{vm.IntVal(5), vm.IntVal(6)}, h)
	if err == nil {
		t.Error("AssertEq(5, 6) should return error")
	}
}

func TestPanic(t *testing.T) {
	h := newHeap()
	_, err := Panic([]vm.Value{allocStr(h, "oh no")}, h)
	if err == nil {
		t.Error("Panic should return error")
	}
	if !strings.Contains(err.Error(), "oh no") {
		t.Errorf("Panic error should contain message, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// string_ops.go
// ---------------------------------------------------------------------------

func TestStringLen(t *testing.T) {
	h := newHeap()
	tests := []struct {
		in  string
		out int64
	}{
		{"", 0},
		{"hello", 5},
		{"日本語", 3},    // Unicode: 3 codepoints
		{"café", 4},   // e with accent
		{"a\x00b", 3}, // null byte
		{"🎉🎊", 2},     // emoji
	}
	for _, tc := range tests {
		v, err := StringLen([]vm.Value{allocStr(h, tc.in)}, h)
		if err != nil {
			t.Fatalf("StringLen(%q): %v", tc.in, err)
		}
		if v.AsInt() != tc.out {
			t.Errorf("StringLen(%q) = %d, want %d", tc.in, v.AsInt(), tc.out)
		}
	}
}

func TestStringSlice(t *testing.T) {
	h := newHeap()
	// Basic.
	v, err := StringSlice([]vm.Value{allocStr(h, "hello"), vm.IntVal(1), vm.IntVal(4)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "ell" {
		t.Errorf("StringSlice(\"hello\", 1, 4) = %q, want %q", getString(v, h), "ell")
	}

	// Unicode.
	v, _ = StringSlice([]vm.Value{allocStr(h, "日本語"), vm.IntVal(0), vm.IntVal(2)}, h)
	if getString(v, h) != "日本" {
		t.Errorf("StringSlice(\"日本語\", 0, 2) = %q, want %q", getString(v, h), "日本")
	}

	// Clamped out-of-range.
	v, _ = StringSlice([]vm.Value{allocStr(h, "hi"), vm.IntVal(-5), vm.IntVal(100)}, h)
	if getString(v, h) != "hi" {
		t.Errorf("StringSlice clamped = %q, want %q", getString(v, h), "hi")
	}

	// Empty result.
	v, _ = StringSlice([]vm.Value{allocStr(h, "abc"), vm.IntVal(2), vm.IntVal(2)}, h)
	if getString(v, h) != "" {
		t.Errorf("StringSlice(2,2) = %q, want empty", getString(v, h))
	}
}

func TestStringContains(t *testing.T) {
	h := newHeap()
	v, _ := StringContains([]vm.Value{allocStr(h, "hello world"), allocStr(h, "world")}, h)
	if !v.AsBool() {
		t.Error("expected true")
	}
	v, _ = StringContains([]vm.Value{allocStr(h, "hello"), allocStr(h, "xyz")}, h)
	if v.AsBool() {
		t.Error("expected false")
	}
	// Empty needle.
	v, _ = StringContains([]vm.Value{allocStr(h, "hello"), allocStr(h, "")}, h)
	if !v.AsBool() {
		t.Error("empty needle should always match")
	}
}

func TestStringIndexOf(t *testing.T) {
	h := newHeap()
	tests := []struct {
		haystack string
		needle   string
		want     int64
	}{
		{"hello", "he", 0},       // at start
		{"hello", "ll", 2},       // at middle
		{"hello", "lo", 3},       // at end
		{"hello", "xyz", -1},     // missing
		{"hello", "", 0},         // empty needle
		{"", "x", -1},            // empty haystack
		{"café", "é", 3},         // Unicode
		{"日本語", "本", 1},          // Unicode multibyte
	}
	for _, tc := range tests {
		v, err := StringIndexOf([]vm.Value{allocStr(h, tc.haystack), allocStr(h, tc.needle)}, h)
		if err != nil {
			t.Fatalf("StringIndexOf(%q, %q): %v", tc.haystack, tc.needle, err)
		}
		if v.AsInt() != tc.want {
			t.Errorf("StringIndexOf(%q, %q) = %d, want %d", tc.haystack, tc.needle, v.AsInt(), tc.want)
		}
	}
}

func TestStringRepeat(t *testing.T) {
	h := newHeap()
	v, err := StringRepeat([]vm.Value{allocStr(h, "ab"), vm.IntVal(3)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "ababab" {
		t.Errorf("StringRepeat(\"ab\", 3) = %q, want %q", getString(v, h), "ababab")
	}

	v, err = StringRepeat([]vm.Value{allocStr(h, "ab"), vm.IntVal(0)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "" {
		t.Errorf("StringRepeat(\"ab\", 0) = %q, want empty", getString(v, h))
	}

	// Repeat 1 time.
	v, err = StringRepeat([]vm.Value{allocStr(h, "xy"), vm.IntVal(1)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "xy" {
		t.Errorf("StringRepeat(\"xy\", 1) = %q, want %q", getString(v, h), "xy")
	}

	// Empty string repeated.
	v, err = StringRepeat([]vm.Value{allocStr(h, ""), vm.IntVal(5)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "" {
		t.Errorf("StringRepeat(\"\", 5) = %q, want empty", getString(v, h))
	}

	if _, err := StringRepeat([]vm.Value{allocStr(h, "ab"), vm.IntVal(-1)}, h); err == nil {
		t.Error("expected error for negative repeat count")
	}
}

func TestStringSplit(t *testing.T) {
	h := newHeap()
	v, _ := StringSplit([]vm.Value{allocStr(h, "a,b,c"), allocStr(h, ",")}, h)
	arr := getArray(v, h)
	if len(arr) != 3 {
		t.Fatalf("split result length = %d, want 3", len(arr))
	}
	parts := []string{"a", "b", "c"}
	for i, p := range parts {
		if getString(arr[i], h) != p {
			t.Errorf("split[%d] = %q, want %q", i, getString(arr[i], h), p)
		}
	}

	// Split empty string.
	v, _ = StringSplit([]vm.Value{allocStr(h, ""), allocStr(h, ",")}, h)
	arr = getArray(v, h)
	if len(arr) != 1 || getString(arr[0], h) != "" {
		t.Errorf("split empty: got %d elements", len(arr))
	}
}

func TestStringTrim(t *testing.T) {
	h := newHeap()
	tests := []struct {
		in, out string
	}{
		{"  hello  ", "hello"},
		{"\t\nhello\n\t", "hello"},
		{"", ""},
		{"no_space", "no_space"},
	}
	for _, tc := range tests {
		v, _ := StringTrim([]vm.Value{allocStr(h, tc.in)}, h)
		if getString(v, h) != tc.out {
			t.Errorf("StringTrim(%q) = %q, want %q", tc.in, getString(v, h), tc.out)
		}
	}
}

func TestStringChars(t *testing.T) {
	h := newHeap()
	v, _ := StringChars([]vm.Value{allocStr(h, "abc")}, h)
	arr := getArray(v, h)
	if len(arr) != 3 {
		t.Fatalf("StringChars length = %d, want 3", len(arr))
	}
	for i, expected := range []rune{'a', 'b', 'c'} {
		if arr[i].AsChar() != expected {
			t.Errorf("char[%d] = %c, want %c", i, arr[i].AsChar(), expected)
		}
	}

	// Unicode.
	v, _ = StringChars([]vm.Value{allocStr(h, "日本")}, h)
	arr = getArray(v, h)
	if len(arr) != 2 {
		t.Fatalf("StringChars(日本) length = %d, want 2", len(arr))
	}

	// Empty.
	v, _ = StringChars([]vm.Value{allocStr(h, "")}, h)
	arr = getArray(v, h)
	if len(arr) != 0 {
		t.Errorf("StringChars(\"\") length = %d, want 0", len(arr))
	}
}

func TestStringBytes(t *testing.T) {
	h := newHeap()

	// ASCII string.
	v, err := StringBytes([]vm.Value{allocStr(h, "ABC")}, h)
	if err != nil {
		t.Fatal(err)
	}
	arr := getArray(v, h)
	wantASCII := []int64{65, 66, 67}
	if len(arr) != len(wantASCII) {
		t.Fatalf("StringBytes(\"ABC\") length = %d, want %d", len(arr), len(wantASCII))
	}
	for i, expected := range wantASCII {
		if arr[i].AsInt() != expected {
			t.Errorf("StringBytes(\"ABC\")[%d] = %d, want %d", i, arr[i].AsInt(), expected)
		}
	}

	// UTF-8 string (multibyte chars).
	v, err = StringBytes([]vm.Value{allocStr(h, "Aé")}, h)
	if err != nil {
		t.Fatal(err)
	}
	arr = getArray(v, h)
	want := []int64{65, 195, 169}
	if len(arr) != len(want) {
		t.Fatalf("StringBytes(\"Aé\") length = %d, want %d", len(arr), len(want))
	}
	for i, expected := range want {
		if arr[i].AsInt() != expected {
			t.Errorf("StringBytes(\"Aé\")[%d] = %d, want %d", i, arr[i].AsInt(), expected)
		}
	}

	// Empty string.
	v, err = StringBytes([]vm.Value{allocStr(h, "")}, h)
	if err != nil {
		t.Fatal(err)
	}
	arr = getArray(v, h)
	if len(arr) != 0 {
		t.Errorf("StringBytes(\"\") length = %d, want 0", len(arr))
	}
}

func TestCharToString(t *testing.T) {
	h := newHeap()
	v, err := CharToString([]vm.Value{vm.CharVal('Z')}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "Z" {
		t.Errorf("CharToString('Z') = %q, want %q", getString(v, h), "Z")
	}

	// Unicode char.
	v, _ = CharToString([]vm.Value{vm.CharVal('日')}, h)
	if getString(v, h) != "日" {
		t.Errorf("CharToString('日') = %q", getString(v, h))
	}

	// Wrong type.
	_, err = CharToString([]vm.Value{vm.IntVal(65)}, h)
	if err == nil {
		t.Error("expected error for non-Char arg")
	}
}

func TestStringReplace(t *testing.T) {
	h := newHeap()
	v, _ := StringReplace([]vm.Value{allocStr(h, "foo bar foo"), allocStr(h, "foo"), allocStr(h, "baz")}, h)
	if getString(v, h) != "baz bar baz" {
		t.Errorf("replace = %q, want %q", getString(v, h), "baz bar baz")
	}

	// No match.
	v, _ = StringReplace([]vm.Value{allocStr(h, "hello"), allocStr(h, "xyz"), allocStr(h, "abc")}, h)
	if getString(v, h) != "hello" {
		t.Errorf("replace no match = %q, want %q", getString(v, h), "hello")
	}
}

func TestStringStartsEndsWith(t *testing.T) {
	h := newHeap()
	v, _ := StringStartsWith([]vm.Value{allocStr(h, "hello world"), allocStr(h, "hello")}, h)
	if !v.AsBool() {
		t.Error("starts_with should be true")
	}
	v, _ = StringStartsWith([]vm.Value{allocStr(h, "hello"), allocStr(h, "world")}, h)
	if v.AsBool() {
		t.Error("starts_with should be false")
	}
	v, _ = StringEndsWith([]vm.Value{allocStr(h, "hello world"), allocStr(h, "world")}, h)
	if !v.AsBool() {
		t.Error("ends_with should be true")
	}

	// Empty prefix/suffix always matches.
	v, _ = StringStartsWith([]vm.Value{allocStr(h, "abc"), allocStr(h, "")}, h)
	if !v.AsBool() {
		t.Error("empty prefix should match")
	}
	v, _ = StringEndsWith([]vm.Value{allocStr(h, "abc"), allocStr(h, "")}, h)
	if !v.AsBool() {
		t.Error("empty suffix should match")
	}
}

func TestStringToUpperLower(t *testing.T) {
	h := newHeap()
	v, _ := StringToUpper([]vm.Value{allocStr(h, "hello")}, h)
	if getString(v, h) != "HELLO" {
		t.Errorf("to_upper = %q, want %q", getString(v, h), "HELLO")
	}
	v, _ = StringToLower([]vm.Value{allocStr(h, "HELLO")}, h)
	if getString(v, h) != "hello" {
		t.Errorf("to_lower = %q, want %q", getString(v, h), "hello")
	}
	// Unicode case.
	v, _ = StringToUpper([]vm.Value{allocStr(h, "café")}, h)
	if getString(v, h) != "CAFÉ" {
		t.Errorf("to_upper(café) = %q, want %q", getString(v, h), "CAFÉ")
	}
	// Empty.
	v, _ = StringToUpper([]vm.Value{allocStr(h, "")}, h)
	if getString(v, h) != "" {
		t.Errorf("to_upper(\"\") = %q, want empty", getString(v, h))
	}
}

func TestStringPadLeftRight(t *testing.T) {
	h := newHeap()

	v, err := StringPadLeft([]vm.Value{allocStr(h, "hi"), vm.IntVal(5), vm.CharVal('.')}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "...hi" {
		t.Errorf("StringPadLeft(\"hi\", 5, '.') = %q, want %q", getString(v, h), "...hi")
	}

	v, err = StringPadRight([]vm.Value{allocStr(h, "hi"), vm.IntVal(5), vm.CharVal('.')}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "hi..." {
		t.Errorf("StringPadRight(\"hi\", 5, '.') = %q, want %q", getString(v, h), "hi...")
	}

	original := allocStr(h, "café")
	v, err = StringPadLeft([]vm.Value{original, vm.IntVal(4), vm.CharVal('.')}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v != original {
		t.Error("StringPadLeft should return the original value when width is already satisfied")
	}

	original = allocStr(h, "日本")
	v, err = StringPadRight([]vm.Value{original, vm.IntVal(1), vm.CharVal('.')}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v != original {
		t.Error("StringPadRight should return the original value when width is already satisfied")
	}

	// Pad with different char ('0' for zero-padding).
	v, err = StringPadLeft([]vm.Value{allocStr(h, "42"), vm.IntVal(5), vm.CharVal('0')}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "00042" {
		t.Errorf("StringPadLeft(\"42\", 5, '0') = %q, want %q", getString(v, h), "00042")
	}

	v, err = StringPadRight([]vm.Value{allocStr(h, "42"), vm.IntVal(5), vm.CharVal(' ')}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "42   " {
		t.Errorf("StringPadRight(\"42\", 5, ' ') = %q, want %q", getString(v, h), "42   ")
	}
}

func TestStringJoin(t *testing.T) {
	h := newHeap()
	parts := allocArr(h, []vm.Value{allocStr(h, "a"), allocStr(h, "b"), allocStr(h, "c")})
	v, err := StringJoin([]vm.Value{parts, allocStr(h, "-")}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "a-b-c" {
		t.Errorf("StringJoin = %q, want %q", getString(v, h), "a-b-c")
	}
}

// ---------------------------------------------------------------------------
// array_ops.go
// ---------------------------------------------------------------------------

func TestArrayLen(t *testing.T) {
	h := newHeap()
	// Non-empty.
	v, _ := ArrayLen([]vm.Value{allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3)})}, h)
	if v.AsInt() != 3 {
		t.Errorf("ArrayLen = %d, want 3", v.AsInt())
	}
	// Empty.
	v, _ = ArrayLen([]vm.Value{allocArr(h, []vm.Value{})}, h)
	if v.AsInt() != 0 {
		t.Errorf("ArrayLen(empty) = %d, want 0", v.AsInt())
	}
}

func TestArrayPush(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(1)})
	v, _ := ArrayPush([]vm.Value{arr, vm.IntVal(2)}, h)
	elems := getArray(v, h)
	if len(elems) != 2 || elems[1].AsInt() != 2 {
		t.Errorf("ArrayPush result: got %d elements", len(elems))
	}

	// Push to empty.
	arr = allocArr(h, []vm.Value{})
	v, _ = ArrayPush([]vm.Value{arr, vm.IntVal(99)}, h)
	elems = getArray(v, h)
	if len(elems) != 1 || elems[0].AsInt() != 99 {
		t.Errorf("ArrayPush(empty, 99): got %v", elems)
	}
}

func TestArrayPop(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(10), vm.IntVal(20), vm.IntVal(30)})
	result, _ := ArrayPop([]vm.Value{arr}, h)

	if !IsResultOk(result, h) {
		t.Fatal("expected Ok result")
	}
	inner, _ := ResultUnwrap(result, h)
	tuple := getTuple(inner, h)
	newArr := getArray(tuple[0], h)
	if len(newArr) != 2 {
		t.Errorf("ArrayPop: new array length = %d, want 2", len(newArr))
	}
	if tuple[1].AsInt() != 30 {
		t.Errorf("ArrayPop: popped = %d, want 30", tuple[1].AsInt())
	}

	// Pop from empty.
	arr = allocArr(h, []vm.Value{})
	result, _ = ArrayPop([]vm.Value{arr}, h)
	if IsResultOk(result, h) {
		t.Error("expected Err for empty array pop")
	}
}

func TestArraySort(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(3), vm.IntVal(1), vm.IntVal(2)})
	v, _ := ArraySort([]vm.Value{arr}, h)
	elems := getArray(v, h)
	expected := []int64{1, 2, 3}
	for i, e := range expected {
		if elems[i].AsInt() != e {
			t.Errorf("sort[%d] = %d, want %d", i, elems[i].AsInt(), e)
		}
	}

	// Already sorted.
	arr = allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2)})
	v, _ = ArraySort([]vm.Value{arr}, h)
	elems = getArray(v, h)
	if elems[0].AsInt() != 1 || elems[1].AsInt() != 2 {
		t.Error("already sorted array changed")
	}

	// Empty.
	arr = allocArr(h, []vm.Value{})
	v, _ = ArraySort([]vm.Value{arr}, h)
	elems = getArray(v, h)
	if len(elems) != 0 {
		t.Error("sorting empty array should return empty")
	}

	// Float sort.
	arr = allocArr(h, []vm.Value{vm.FloatVal(3.1), vm.FloatVal(1.5), vm.FloatVal(2.0)})
	v, _ = ArraySort([]vm.Value{arr}, h)
	elems = getArray(v, h)
	if elems[0].AsFloat() != 1.5 {
		t.Errorf("float sort[0] = %v, want 1.5", elems[0].AsFloat())
	}
}

func TestArrayReverse(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3)})
	v, _ := ArrayReverse([]vm.Value{arr}, h)
	elems := getArray(v, h)
	expected := []int64{3, 2, 1}
	for i, e := range expected {
		if elems[i].AsInt() != e {
			t.Errorf("reverse[%d] = %d, want %d", i, elems[i].AsInt(), e)
		}
	}

	// Single element.
	arr = allocArr(h, []vm.Value{vm.IntVal(99)})
	v, _ = ArrayReverse([]vm.Value{arr}, h)
	elems = getArray(v, h)
	if len(elems) != 1 || elems[0].AsInt() != 99 {
		t.Error("reverse single element failed")
	}

	// Empty.
	arr = allocArr(h, []vm.Value{})
	v, _ = ArrayReverse([]vm.Value{arr}, h)
	elems = getArray(v, h)
	if len(elems) != 0 {
		t.Error("reverse empty should return empty")
	}
}

func TestArrayContains(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3)})
	v, _ := ArrayContains([]vm.Value{arr, vm.IntVal(2)}, h)
	if !v.AsBool() {
		t.Error("expected true for existing element")
	}
	v, _ = ArrayContains([]vm.Value{arr, vm.IntVal(99)}, h)
	if v.AsBool() {
		t.Error("expected false for missing element")
	}

	// String elements.
	arr = allocArr(h, []vm.Value{allocStr(h, "a"), allocStr(h, "b")})
	v, _ = ArrayContains([]vm.Value{arr, allocStr(h, "b")}, h)
	if !v.AsBool() {
		t.Error("expected true for string element")
	}
}

func TestArrayZip(t *testing.T) {
	h := newHeap()
	a := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3)})
	b := allocArr(h, []vm.Value{allocStr(h, "a"), allocStr(h, "b")})
	v, _ := ArrayZip([]vm.Value{a, b}, h)
	elems := getArray(v, h)
	if len(elems) != 2 {
		t.Fatalf("zip length = %d, want 2 (min of 3 and 2)", len(elems))
	}
	t0 := getTuple(elems[0], h)
	if t0[0].AsInt() != 1 || getString(t0[1], h) != "a" {
		t.Error("zip[0] mismatch")
	}

	// Empty arrays.
	a = allocArr(h, []vm.Value{})
	b = allocArr(h, []vm.Value{vm.IntVal(1)})
	v, _ = ArrayZip([]vm.Value{a, b}, h)
	elems = getArray(v, h)
	if len(elems) != 0 {
		t.Error("zip with empty should be empty")
	}
}

func TestArrayEnumerate(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{allocStr(h, "a"), allocStr(h, "b"), allocStr(h, "c")})
	v, _ := ArrayEnumerate([]vm.Value{arr}, h)
	elems := getArray(v, h)
	if len(elems) != 3 {
		t.Fatalf("enumerate length = %d, want 3", len(elems))
	}
	for i, elem := range elems {
		tuple := getTuple(elem, h)
		if tuple[0].AsInt() != int64(i) {
			t.Errorf("enumerate[%d] index = %d", i, tuple[0].AsInt())
		}
	}
}

func TestArrayMapFilterFold(t *testing.T) {
	h := newHeap()
	// Set up a simple callback invoker that doubles ints.
	oldInvoker := CallbackInvoker
	defer func() { CallbackInvoker = oldInvoker }()

	CallbackInvoker = func(fn vm.Value, args []vm.Value, heap *vm.Heap) (vm.Value, error) {
		// We use fn.Data to determine the operation:
		// 1 = double, 2 = is_even, 3 = add
		op := int(fn.Data)
		switch op {
		case 1: // double
			return vm.IntVal(args[0].AsInt() * 2), nil
		case 2: // is_even
			return vm.BoolVal(args[0].AsInt()%2 == 0), nil
		case 3: // add
			return vm.IntVal(args[0].AsInt() + args[1].AsInt()), nil
		}
		return vm.UnitVal(), nil
	}

	// Map: double each element.
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3)})
	doubleFn := vm.Value{Tag: vm.TagFunc, Data: 1}
	v, err := ArrayMap([]vm.Value{arr, doubleFn}, h)
	if err != nil {
		t.Fatal(err)
	}
	elems := getArray(v, h)
	expected := []int64{2, 4, 6}
	for i, e := range expected {
		if elems[i].AsInt() != e {
			t.Errorf("map[%d] = %d, want %d", i, elems[i].AsInt(), e)
		}
	}

	// Filter: keep even numbers.
	arr = allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3), vm.IntVal(4)})
	evenFn := vm.Value{Tag: vm.TagFunc, Data: 2}
	v, _ = ArrayFilter([]vm.Value{arr, evenFn}, h)
	elems = getArray(v, h)
	if len(elems) != 2 || elems[0].AsInt() != 2 || elems[1].AsInt() != 4 {
		t.Errorf("filter result = %v", elems)
	}

	// Fold: sum.
	arr = allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3)})
	addFn := vm.Value{Tag: vm.TagFunc, Data: 3}
	v, _ = ArrayFold([]vm.Value{arr, vm.IntVal(0), addFn}, h)
	if v.AsInt() != 6 {
		t.Errorf("fold sum = %d, want 6", v.AsInt())
	}

	// Filter on empty array.
	arr = allocArr(h, []vm.Value{})
	v, _ = ArrayFilter([]vm.Value{arr, evenFn}, h)
	elems = getArray(v, h)
	if len(elems) != 0 {
		t.Error("filter empty should return empty")
	}
}

func TestArraySum(t *testing.T) {
	h := newHeap()
	// Sum of ints.
	v, err := ArraySum([]vm.Value{allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3)})}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.Tag != vm.TagInt || v.AsInt() != 6 {
		t.Errorf("sum([1,2,3]) = %d, want 6", v.AsInt())
	}

	// Sum of floats.
	v, err = ArraySum([]vm.Value{allocArr(h, []vm.Value{vm.FloatVal(1.5), vm.FloatVal(2.5), vm.FloatVal(3.0)})}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.Tag != vm.TagFloat || v.AsFloat() != 7.0 {
		t.Errorf("sum([1.5,2.5,3.0]) = %v, want 7.0", v.AsFloat())
	}

	// Empty array.
	v, err = ArraySum([]vm.Value{allocArr(h, []vm.Value{})}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.Tag != vm.TagInt || v.AsInt() != 0 {
		t.Errorf("sum([]) = %v, want 0", v.AsInt())
	}
}

func TestArrayMin(t *testing.T) {
	h := newHeap()
	// Min of ints.
	v, err := ArrayMin([]vm.Value{allocArr(h, []vm.Value{vm.IntVal(3), vm.IntVal(1), vm.IntVal(2)})}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 1 {
		t.Errorf("min([3,1,2]) = %d, want 1", v.AsInt())
	}

	// Min of floats.
	v, err = ArrayMin([]vm.Value{allocArr(h, []vm.Value{vm.FloatVal(3.5), vm.FloatVal(1.2), vm.FloatVal(2.8)})}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsFloat() != 1.2 {
		t.Errorf("min([3.5,1.2,2.8]) = %v, want 1.2", v.AsFloat())
	}

	// Empty array error.
	_, err = ArrayMin([]vm.Value{allocArr(h, []vm.Value{})}, h)
	if err == nil {
		t.Error("expected error for empty array min")
	}
}

func TestArrayMax(t *testing.T) {
	h := newHeap()
	// Max of ints.
	v, err := ArrayMax([]vm.Value{allocArr(h, []vm.Value{vm.IntVal(3), vm.IntVal(1), vm.IntVal(2)})}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 3 {
		t.Errorf("max([3,1,2]) = %d, want 3", v.AsInt())
	}

	// Max of floats.
	v, err = ArrayMax([]vm.Value{allocArr(h, []vm.Value{vm.FloatVal(3.5), vm.FloatVal(1.2), vm.FloatVal(2.8)})}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsFloat() != 3.5 {
		t.Errorf("max([3.5,1.2,2.8]) = %v, want 3.5", v.AsFloat())
	}

	// Empty array error.
	_, err = ArrayMax([]vm.Value{allocArr(h, []vm.Value{})}, h)
	if err == nil {
		t.Error("expected error for empty array max")
	}
}

func TestArrayTake(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3), vm.IntVal(4)})

	// Take N from array.
	v, err := ArrayTake([]vm.Value{arr, vm.IntVal(2)}, h)
	if err != nil {
		t.Fatal(err)
	}
	elems := getArray(v, h)
	if len(elems) != 2 || elems[0].AsInt() != 1 || elems[1].AsInt() != 2 {
		t.Errorf("take(2) = %v, want [1,2]", elems)
	}

	// Take more than length.
	v, _ = ArrayTake([]vm.Value{arr, vm.IntVal(10)}, h)
	elems = getArray(v, h)
	if len(elems) != 4 {
		t.Errorf("take(10) length = %d, want 4", len(elems))
	}

	// Take 0.
	v, _ = ArrayTake([]vm.Value{arr, vm.IntVal(0)}, h)
	elems = getArray(v, h)
	if len(elems) != 0 {
		t.Errorf("take(0) length = %d, want 0", len(elems))
	}
}

func TestArrayDrop(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3), vm.IntVal(4)})

	// Drop N from array.
	v, err := ArrayDrop([]vm.Value{arr, vm.IntVal(2)}, h)
	if err != nil {
		t.Fatal(err)
	}
	elems := getArray(v, h)
	if len(elems) != 2 || elems[0].AsInt() != 3 || elems[1].AsInt() != 4 {
		t.Errorf("drop(2) = %v, want [3,4]", elems)
	}

	// Drop more than length.
	v, _ = ArrayDrop([]vm.Value{arr, vm.IntVal(10)}, h)
	elems = getArray(v, h)
	if len(elems) != 0 {
		t.Errorf("drop(10) length = %d, want 0", len(elems))
	}

	// Drop 0.
	v, _ = ArrayDrop([]vm.Value{arr, vm.IntVal(0)}, h)
	elems = getArray(v, h)
	if len(elems) != 4 {
		t.Errorf("drop(0) length = %d, want 4", len(elems))
	}
}

func TestArrayChunk(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3), vm.IntVal(4), vm.IntVal(5)})

	// Chunk into groups of 2.
	v, err := ArrayChunk([]vm.Value{arr, vm.IntVal(2)}, h)
	if err != nil {
		t.Fatal(err)
	}
	chunks := getArray(v, h)
	if len(chunks) != 3 {
		t.Fatalf("chunk(2) produced %d chunks, want 3", len(chunks))
	}
	c0 := getArray(chunks[0], h)
	if len(c0) != 2 || c0[0].AsInt() != 1 || c0[1].AsInt() != 2 {
		t.Errorf("chunk[0] = %v, want [1,2]", c0)
	}
	// Last chunk may be smaller.
	cLast := getArray(chunks[2], h)
	if len(cLast) != 1 || cLast[0].AsInt() != 5 {
		t.Errorf("chunk[2] = %v, want [5]", cLast)
	}

	// Chunk size >= array length.
	v, _ = ArrayChunk([]vm.Value{arr, vm.IntVal(10)}, h)
	chunks = getArray(v, h)
	if len(chunks) != 1 {
		t.Errorf("chunk(10) produced %d chunks, want 1", len(chunks))
	}
	c0 = getArray(chunks[0], h)
	if len(c0) != 5 {
		t.Errorf("chunk(10)[0] length = %d, want 5", len(c0))
	}
}

func TestArrayUnique(t *testing.T) {
	h := newHeap()
	// Int array with duplicates.
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(2), vm.IntVal(3), vm.IntVal(1)})
	v, err := ArrayUnique([]vm.Value{arr}, h)
	if err != nil {
		t.Fatal(err)
	}
	elems := getArray(v, h)
	if len(elems) != 3 {
		t.Fatalf("unique int length = %d, want 3", len(elems))
	}
	if elems[0].AsInt() != 1 || elems[1].AsInt() != 2 || elems[2].AsInt() != 3 {
		t.Errorf("unique ints = %v, want [1,2,3]", elems)
	}

	// String array with duplicates.
	arr = allocArr(h, []vm.Value{allocStr(h, "a"), allocStr(h, "b"), allocStr(h, "a")})
	v, _ = ArrayUnique([]vm.Value{arr}, h)
	elems = getArray(v, h)
	if len(elems) != 2 {
		t.Fatalf("unique string length = %d, want 2", len(elems))
	}
	if getString(elems[0], h) != "a" || getString(elems[1], h) != "b" {
		t.Errorf("unique strings = [%q, %q], want [a, b]", getString(elems[0], h), getString(elems[1], h))
	}
}

func TestArrayJoin(t *testing.T) {
	h := newHeap()
	// Join string array with separator.
	arr := allocArr(h, []vm.Value{allocStr(h, "a"), allocStr(h, "b"), allocStr(h, "c")})
	v, err := ArrayJoin([]vm.Value{arr, allocStr(h, ", ")}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "a, b, c" {
		t.Errorf("join = %q, want %q", getString(v, h), "a, b, c")
	}

	// Empty array.
	arr = allocArr(h, []vm.Value{})
	v, _ = ArrayJoin([]vm.Value{arr, allocStr(h, ",")}, h)
	if getString(v, h) != "" {
		t.Errorf("join empty = %q, want empty", getString(v, h))
	}

	// Single element.
	arr = allocArr(h, []vm.Value{allocStr(h, "only")})
	v, _ = ArrayJoin([]vm.Value{arr, allocStr(h, ",")}, h)
	if getString(v, h) != "only" {
		t.Errorf("join single = %q, want %q", getString(v, h), "only")
	}
}

func TestArraySlice(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(10), vm.IntVal(20), vm.IntVal(30), vm.IntVal(40), vm.IntVal(50)})

	// Valid range.
	v, err := ArraySlice([]vm.Value{arr, vm.IntVal(1), vm.IntVal(4)}, h)
	if err != nil {
		t.Fatal(err)
	}
	elems := getArray(v, h)
	if len(elems) != 3 || elems[0].AsInt() != 20 || elems[2].AsInt() != 40 {
		t.Errorf("slice(1,4) = %v, want [20,30,40]", elems)
	}

	// Out-of-bounds handling (clamped).
	v, _ = ArraySlice([]vm.Value{arr, vm.IntVal(-10), vm.IntVal(100)}, h)
	elems = getArray(v, h)
	if len(elems) != 5 {
		t.Errorf("slice(-10,100) length = %d, want 5", len(elems))
	}

	// Empty result (start >= end).
	v, _ = ArraySlice([]vm.Value{arr, vm.IntVal(3), vm.IntVal(3)}, h)
	elems = getArray(v, h)
	if len(elems) != 0 {
		t.Errorf("slice(3,3) length = %d, want 0", len(elems))
	}
}

func TestArrayFindAnyAll(t *testing.T) {
	h := newHeap()
	oldInvoker := CallbackInvoker
	defer func() { CallbackInvoker = oldInvoker }()

	// Callback ops: 1 = is_even, 2 = is_positive, 3 = always_false
	CallbackInvoker = func(fn vm.Value, args []vm.Value, heap *vm.Heap) (vm.Value, error) {
		op := int(fn.Data)
		switch op {
		case 1: // is_even
			return vm.BoolVal(args[0].AsInt()%2 == 0), nil
		case 2: // is_positive
			return vm.BoolVal(args[0].AsInt() > 0), nil
		case 3: // always_false
			return vm.BoolVal(false), nil
		}
		return vm.UnitVal(), nil
	}

	isEvenFn := vm.Value{Tag: vm.TagFunc, Data: 1}
	isPositiveFn := vm.Value{Tag: vm.TagFunc, Data: 2}
	alwaysFalseFn := vm.Value{Tag: vm.TagFunc, Data: 3}

	// --- ArrayFind ---
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(3), vm.IntVal(4), vm.IntVal(6)})

	// Find returns first match.
	result, err := ArrayFind([]vm.Value{arr, isEvenFn}, h)
	if err != nil {
		t.Fatal(err)
	}
	if !IsResultOk(result, h) {
		t.Fatal("find: expected Ok")
	}
	inner, _ := ResultUnwrap(result, h)
	if inner.AsInt() != 4 {
		t.Errorf("find(is_even) = %d, want 4 (first even)", inner.AsInt())
	}

	// Find with no match.
	arr = allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(3), vm.IntVal(5)})
	result, _ = ArrayFind([]vm.Value{arr, isEvenFn}, h)
	if IsResultOk(result, h) {
		t.Error("find: expected Err for no match")
	}

	// --- ArrayAny ---
	arr = allocArr(h, []vm.Value{vm.IntVal(-1), vm.IntVal(0), vm.IntVal(3)})

	// Any returns true if any element matches.
	v, _ := ArrayAny([]vm.Value{arr, isPositiveFn}, h)
	if !v.AsBool() {
		t.Error("any(is_positive) should be true")
	}

	// Any returns false if none match.
	arr = allocArr(h, []vm.Value{vm.IntVal(-1), vm.IntVal(-2)})
	v, _ = ArrayAny([]vm.Value{arr, isPositiveFn}, h)
	if v.AsBool() {
		t.Error("any(is_positive) should be false when all negative")
	}

	// Any on empty array returns false.
	arr = allocArr(h, []vm.Value{})
	v, _ = ArrayAny([]vm.Value{arr, isPositiveFn}, h)
	if v.AsBool() {
		t.Error("any on empty should be false")
	}

	// --- ArrayAll ---
	arr = allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3)})

	// All returns true if all match.
	v, _ = ArrayAll([]vm.Value{arr, isPositiveFn}, h)
	if !v.AsBool() {
		t.Error("all(is_positive) should be true")
	}

	// All returns false if any doesn't.
	arr = allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(-2), vm.IntVal(3)})
	v, _ = ArrayAll([]vm.Value{arr, isPositiveFn}, h)
	if v.AsBool() {
		t.Error("all(is_positive) should be false when one is negative")
	}

	// All on empty array returns true.
	arr = allocArr(h, []vm.Value{})
	v, _ = ArrayAll([]vm.Value{arr, alwaysFalseFn}, h)
	if !v.AsBool() {
		t.Error("all on empty should be true (vacuous truth)")
	}
}

func TestArrayFlatMap(t *testing.T) {
	h := newHeap()
	oldInvoker := CallbackInvoker
	defer func() { CallbackInvoker = oldInvoker }()

	// FlatMap: each element becomes [x, x+1].
	CallbackInvoker = func(fn vm.Value, args []vm.Value, heap *vm.Heap) (vm.Value, error) {
		n := args[0].AsInt()
		return allocArr(heap, []vm.Value{vm.IntVal(n), vm.IntVal(n + 1)}), nil
	}

	arr := allocArr(h, []vm.Value{vm.IntVal(10), vm.IntVal(20)})
	v, _ := ArrayFlatMap([]vm.Value{arr, vm.FuncVal(0)}, h)
	elems := getArray(v, h)
	if len(elems) != 4 {
		t.Fatalf("flat_map length = %d, want 4", len(elems))
	}
	expectedVals := []int64{10, 11, 20, 21}
	for i, e := range expectedVals {
		if elems[i].AsInt() != e {
			t.Errorf("flat_map[%d] = %d, want %d", i, elems[i].AsInt(), e)
		}
	}
}

// ---------------------------------------------------------------------------
// math_ops.go
// ---------------------------------------------------------------------------

func TestAbs(t *testing.T) {
	h := newHeap()
	tests := []struct {
		in  vm.Value
		out float64
		tag byte
	}{
		{vm.IntVal(-5), 5, vm.TagInt},
		{vm.IntVal(5), 5, vm.TagInt},
		{vm.IntVal(0), 0, vm.TagInt},
		{vm.FloatVal(-3.14), 3.14, vm.TagFloat},
		{vm.FloatVal(0.0), 0.0, vm.TagFloat},
	}
	for _, tc := range tests {
		v, err := Abs([]vm.Value{tc.in}, h)
		if err != nil {
			t.Fatal(err)
		}
		if v.Tag != tc.tag {
			t.Errorf("abs tag = %d, want %d", v.Tag, tc.tag)
		}
		if tc.tag == vm.TagInt && float64(v.AsInt()) != tc.out {
			t.Errorf("abs = %d, want %v", v.AsInt(), tc.out)
		}
		if tc.tag == vm.TagFloat && v.AsFloat() != tc.out {
			t.Errorf("abs = %v, want %v", v.AsFloat(), tc.out)
		}
	}
}

func TestMinMax(t *testing.T) {
	h := newHeap()
	// Int min/max.
	v, _ := Min([]vm.Value{vm.IntVal(3), vm.IntVal(7)}, h)
	if v.AsInt() != 3 {
		t.Errorf("min(3,7) = %d", v.AsInt())
	}
	v, _ = Max([]vm.Value{vm.IntVal(3), vm.IntVal(7)}, h)
	if v.AsInt() != 7 {
		t.Errorf("max(3,7) = %d", v.AsInt())
	}
	// Float min/max.
	v, _ = Min([]vm.Value{vm.FloatVal(1.5), vm.FloatVal(2.5)}, h)
	if v.AsFloat() != 1.5 {
		t.Errorf("min(1.5,2.5) = %v", v.AsFloat())
	}
	v, _ = Max([]vm.Value{vm.FloatVal(1.5), vm.FloatVal(2.5)}, h)
	if v.AsFloat() != 2.5 {
		t.Errorf("max(1.5,2.5) = %v", v.AsFloat())
	}
	// Mixed.
	v, _ = Min([]vm.Value{vm.IntVal(3), vm.FloatVal(2.5)}, h)
	if v.AsFloat() != 2.5 {
		t.Errorf("min(3, 2.5) = %v", v.AsFloat())
	}
	// Equal values.
	v, _ = Min([]vm.Value{vm.IntVal(5), vm.IntVal(5)}, h)
	if v.AsInt() != 5 {
		t.Errorf("min(5,5) = %d", v.AsInt())
	}
}

func TestSqrt(t *testing.T) {
	h := newHeap()
	v, _ := Sqrt([]vm.Value{vm.FloatVal(9.0)}, h)
	if v.AsFloat() != 3.0 {
		t.Errorf("sqrt(9) = %v", v.AsFloat())
	}
	v, _ = Sqrt([]vm.Value{vm.IntVal(16)}, h)
	if v.AsFloat() != 4.0 {
		t.Errorf("sqrt(16) = %v", v.AsFloat())
	}
	// sqrt(0).
	v, _ = Sqrt([]vm.Value{vm.FloatVal(0)}, h)
	if v.AsFloat() != 0 {
		t.Errorf("sqrt(0) = %v", v.AsFloat())
	}
	// sqrt(negative) = NaN.
	v, _ = Sqrt([]vm.Value{vm.FloatVal(-1)}, h)
	if !math.IsNaN(v.AsFloat()) {
		t.Errorf("sqrt(-1) should be NaN, got %v", v.AsFloat())
	}
}

func TestPow(t *testing.T) {
	h := newHeap()
	v, _ := Pow([]vm.Value{vm.FloatVal(2.0), vm.FloatVal(10.0)}, h)
	if v.AsFloat() != 1024.0 {
		t.Errorf("pow(2,10) = %v", v.AsFloat())
	}
	v, _ = Pow([]vm.Value{vm.IntVal(3), vm.IntVal(3)}, h)
	if v.AsFloat() != 27.0 {
		t.Errorf("pow(3,3) = %v", v.AsFloat())
	}
	// x^0 = 1.
	v, _ = Pow([]vm.Value{vm.FloatVal(5), vm.FloatVal(0)}, h)
	if v.AsFloat() != 1.0 {
		t.Errorf("pow(5,0) = %v", v.AsFloat())
	}
}

func TestFloorCeilRound(t *testing.T) {
	h := newHeap()
	v, _ := Floor([]vm.Value{vm.FloatVal(3.7)}, h)
	if v.AsFloat() != 3.0 {
		t.Errorf("floor(3.7) = %v", v.AsFloat())
	}
	v, _ = Ceil([]vm.Value{vm.FloatVal(3.2)}, h)
	if v.AsFloat() != 4.0 {
		t.Errorf("ceil(3.2) = %v", v.AsFloat())
	}
	v, _ = Round([]vm.Value{vm.FloatVal(3.5)}, h)
	if v.AsFloat() != 4.0 {
		t.Errorf("round(3.5) = %v", v.AsFloat())
	}
	v, _ = Round([]vm.Value{vm.FloatVal(3.4)}, h)
	if v.AsFloat() != 3.0 {
		t.Errorf("round(3.4) = %v", v.AsFloat())
	}
	// Negative.
	v, _ = Floor([]vm.Value{vm.FloatVal(-1.5)}, h)
	if v.AsFloat() != -2.0 {
		t.Errorf("floor(-1.5) = %v", v.AsFloat())
	}
	v, _ = Ceil([]vm.Value{vm.FloatVal(-1.5)}, h)
	if v.AsFloat() != -1.0 {
		t.Errorf("ceil(-1.5) = %v", v.AsFloat())
	}
}

func TestTrigFunctions(t *testing.T) {
	h := newHeap()

	v, err := Sin([]vm.Value{vm.FloatVal(0)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.AsFloat()-0.0) > 1e-10 {
		t.Errorf("sin(0) = %v, want 0", v.AsFloat())
	}

	v, err = Sin([]vm.Value{vm.FloatVal(math.Pi / 2)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.AsFloat()-1.0) > 1e-10 {
		t.Errorf("sin(pi/2) = %v, want 1", v.AsFloat())
	}

	v, err = Cos([]vm.Value{vm.FloatVal(math.Pi)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.AsFloat()+1.0) > 1e-10 {
		t.Errorf("cos(pi) = %v, want -1", v.AsFloat())
	}

	v, err = Tan([]vm.Value{vm.FloatVal(0)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.AsFloat()-0.0) > 1e-10 {
		t.Errorf("tan(0) = %v, want 0", v.AsFloat())
	}

	v, err = Asin([]vm.Value{vm.FloatVal(1)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.AsFloat()-math.Pi/2) > 1e-10 {
		t.Errorf("asin(1) = %v, want pi/2", v.AsFloat())
	}

	v, err = Acos([]vm.Value{vm.FloatVal(1)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.AsFloat()-0.0) > 1e-10 {
		t.Errorf("acos(1) = %v, want 0", v.AsFloat())
	}

	v, err = Atan([]vm.Value{vm.FloatVal(1)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.AsFloat()-math.Pi/4) > 1e-10 {
		t.Errorf("atan(1) = %v, want pi/4", v.AsFloat())
	}
}

func TestAtan2(t *testing.T) {
	h := newHeap()

	v, err := Atan2([]vm.Value{vm.FloatVal(1), vm.FloatVal(0)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.AsFloat()-math.Pi/2) > 1e-10 {
		t.Errorf("atan2(1,0) = %v, want pi/2", v.AsFloat())
	}
}

func TestLogExpConstants(t *testing.T) {
	h := newHeap()

	v, err := Log([]vm.Value{vm.FloatVal(math.E)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.AsFloat()-1.0) > 1e-10 {
		t.Errorf("log(e) = %v, want 1", v.AsFloat())
	}

	v, err = Log2([]vm.Value{vm.FloatVal(8)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.AsFloat()-3.0) > 1e-10 {
		t.Errorf("log2(8) = %v, want 3", v.AsFloat())
	}

	v, err = Log10([]vm.Value{vm.FloatVal(100)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.AsFloat()-2.0) > 1e-10 {
		t.Errorf("log10(100) = %v, want 2", v.AsFloat())
	}

	v, err = Exp([]vm.Value{vm.FloatVal(1)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.AsFloat()-math.E) > 1e-10 {
		t.Errorf("exp(1) = %v, want e", v.AsFloat())
	}

	v, err = Pi(nil, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsFloat() != math.Pi {
		t.Errorf("pi() = %v, want %v", v.AsFloat(), math.Pi)
	}

	v, err = E(nil, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsFloat() != math.E {
		t.Errorf("e() = %v, want %v", v.AsFloat(), math.E)
	}
}

func TestGcdLcmClamp(t *testing.T) {
	h := newHeap()

	v, err := Gcd([]vm.Value{vm.IntVal(12), vm.IntVal(8)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 4 {
		t.Errorf("gcd(12,8) = %d, want 4", v.AsInt())
	}

	v, err = Gcd([]vm.Value{vm.IntVal(-12), vm.IntVal(8)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 4 {
		t.Errorf("gcd(-12,8) = %d, want 4", v.AsInt())
	}

	v, err = Lcm([]vm.Value{vm.IntVal(4), vm.IntVal(6)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 12 {
		t.Errorf("lcm(4,6) = %d, want 12", v.AsInt())
	}

	v, err = Lcm([]vm.Value{vm.IntVal(0), vm.IntVal(5)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 0 {
		t.Errorf("lcm(0,5) = %d, want 0", v.AsInt())
	}

	v, err = Clamp([]vm.Value{vm.IntVal(15), vm.IntVal(0), vm.IntVal(10)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.Tag != vm.TagInt || v.AsInt() != 10 {
		t.Errorf("clamp(15,0,10) = (%d, %d), want (Int, 10)", v.Tag, v.AsInt())
	}

	v, err = Clamp([]vm.Value{vm.FloatVal(5.5), vm.FloatVal(0), vm.FloatVal(10)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.Tag != vm.TagFloat || math.Abs(v.AsFloat()-5.5) > 1e-10 {
		t.Errorf("clamp(5.5,0,10) = (%d, %v), want (Float, 5.5)", v.Tag, v.AsFloat())
	}

	v, err = Clamp([]vm.Value{vm.IntVal(4), vm.IntVal(5), vm.IntVal(3)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 5 {
		t.Errorf("clamp(4,5,3) = %d, want 5", v.AsInt())
	}
}

func TestRandomInt(t *testing.T) {
	h := newHeap()
	for i := 0; i < 100; i++ {
		v, err := RandomInt([]vm.Value{vm.IntVal(0), vm.IntVal(10)}, h)
		if err != nil {
			t.Fatal(err)
		}
		n := v.AsInt()
		if n < 0 || n >= 10 {
			t.Errorf("random_int out of range: %d", n)
		}
	}
	// Invalid range.
	_, err := RandomInt([]vm.Value{vm.IntVal(10), vm.IntVal(5)}, h)
	if err == nil {
		t.Error("expected error for low >= high")
	}
}

func TestRandomFloat(t *testing.T) {
	h := newHeap()
	for i := 0; i < 100; i++ {
		v, err := RandomFloat(nil, h)
		if err != nil {
			t.Fatal(err)
		}
		f := v.AsFloat()
		if f < 0.0 || f >= 1.0 {
			t.Errorf("random_float out of range: %v", f)
		}
	}
}

// ---------------------------------------------------------------------------
// io.go — File I/O
// ---------------------------------------------------------------------------

func TestReadWriteFile(t *testing.T) {
	h := newHeap()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "hello, file I/O! 日本語"

	// Write.
	pathVal := allocStr(h, path)
	contentVal := allocStr(h, content)
	result, err := WriteFile([]vm.Value{pathVal, contentVal}, h)
	if err != nil {
		t.Fatal(err)
	}
	if !IsResultOk(result, h) {
		t.Fatal("WriteFile returned Err")
	}

	// Read.
	result, err = ReadFile([]vm.Value{allocStr(h, path)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if !IsResultOk(result, h) {
		t.Fatal("ReadFile returned Err")
	}
	inner, _ := ResultUnwrap(result, h)
	got := getString(inner, h)
	if got != content {
		t.Errorf("ReadFile = %q, want %q", got, content)
	}

	// Read nonexistent file.
	result, _ = ReadFile([]vm.Value{allocStr(h, filepath.Join(dir, "nope.txt"))}, h)
	if IsResultOk(result, h) {
		t.Error("expected Err for nonexistent file")
	}
}

func TestWriteFileCreatesDir(t *testing.T) {
	h := newHeap()
	dir := t.TempDir()
	path := filepath.Join(dir, "test_overwrite.txt")

	// Write twice to same path — second overwrites first.
	WriteFile([]vm.Value{allocStr(h, path), allocStr(h, "first")}, h)
	WriteFile([]vm.Value{allocStr(h, path), allocStr(h, "second")}, h)

	data, _ := os.ReadFile(path)
	if string(data) != "second" {
		t.Errorf("overwrite: got %q, want %q", string(data), "second")
	}
}

func TestFileExists(t *testing.T) {
	h := newHeap()
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	v, err := FileExists([]vm.Value{allocStr(h, path)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if !v.AsBool() {
		t.Error("expected true for existing path")
	}

	v, err = FileExists([]vm.Value{allocStr(h, filepath.Join(dir, "missing.txt"))}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsBool() {
		t.Error("expected false for missing path")
	}
}

func TestDirListAndCreate(t *testing.T) {
	h := newHeap()
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")

	result, err := DirCreate([]vm.Value{allocStr(h, nested)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if !IsResultOk(result, h) {
		t.Fatal("DirCreate returned Err")
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("created directory missing: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "z.txt"), []byte("z"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	result, err = DirList([]vm.Value{allocStr(h, root)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if !IsResultOk(result, h) {
		t.Fatal("DirList returned Err")
	}
	inner, err := ResultUnwrap(result, h)
	if err != nil {
		t.Fatal(err)
	}
	items := getArray(inner, h)
	got := make([]string, len(items))
	for i, item := range items {
		got[i] = getString(item, h)
	}
	sort.Strings(got)
	want := []string{"a", "subdir", "z.txt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("DirList() = %v, want %v", got, want)
	}
}

func TestPathHelpers(t *testing.T) {
	h := newHeap()

	joined, err := PathJoin([]vm.Value{allocArr(h, []vm.Value{
		allocStr(h, "usr"),
		allocStr(h, "local"),
		allocStr(h, "bin"),
	})}, h)
	if err != nil {
		t.Fatal(err)
	}
	if got := getString(joined, h); got != filepath.Join("usr", "local", "bin") {
		t.Errorf("PathJoin() = %q, want %q", got, filepath.Join("usr", "local", "bin"))
	}

	dirname, err := PathDirname([]vm.Value{allocStr(h, filepath.Join("usr", "local", "bin", "ryx"))}, h)
	if err != nil {
		t.Fatal(err)
	}
	if got := getString(dirname, h); got != filepath.Join("usr", "local", "bin") {
		t.Errorf("PathDirname() = %q, want %q", got, filepath.Join("usr", "local", "bin"))
	}

	basename, err := PathBasename([]vm.Value{allocStr(h, filepath.Join("tmp", "data.csv"))}, h)
	if err != nil {
		t.Fatal(err)
	}
	if got := getString(basename, h); got != "data.csv" {
		t.Errorf("PathBasename() = %q, want %q", got, "data.csv")
	}

	extension, err := PathExtension([]vm.Value{allocStr(h, "archive.tar.gz")}, h)
	if err != nil {
		t.Fatal(err)
	}
	if got := getString(extension, h); got != ".gz" {
		t.Errorf("PathExtension() = %q, want %q", got, ".gz")
	}

	// No extension.
	extension, err = PathExtension([]vm.Value{allocStr(h, "Makefile")}, h)
	if err != nil {
		t.Fatal(err)
	}
	if got := getString(extension, h); got != "" {
		t.Errorf("PathExtension(\"Makefile\") = %q, want empty", got)
	}
}

func TestFileSize(t *testing.T) {
	h := newHeap()
	dir := t.TempDir()
	path := filepath.Join(dir, "size.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := FileSize([]vm.Value{allocStr(h, path)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if !IsResultOk(result, h) {
		t.Fatal("FileSize returned Err")
	}
	inner, err := ResultUnwrap(result, h)
	if err != nil {
		t.Fatal(err)
	}
	if inner.AsInt() != 5 {
		t.Errorf("FileSize() = %d, want 5", inner.AsInt())
	}

	result, err = FileSize([]vm.Value{allocStr(h, filepath.Join(dir, "missing.txt"))}, h)
	if err != nil {
		t.Fatal(err)
	}
	if IsResultOk(result, h) {
		t.Error("expected Err for missing file")
	}
}

// ---------------------------------------------------------------------------
// vm/builtins.go — Trait implementations
// ---------------------------------------------------------------------------

func TestBuiltinEq(t *testing.T) {
	h := newHeap()
	v, _ := vm.BuiltinEq([]vm.Value{vm.IntVal(42), vm.IntVal(42)}, h)
	if !v.AsBool() {
		t.Error("42 == 42 should be true")
	}
	v, _ = vm.BuiltinEq([]vm.Value{vm.IntVal(1), vm.IntVal(2)}, h)
	if v.AsBool() {
		t.Error("1 == 2 should be false")
	}

	// String equality.
	v, _ = vm.BuiltinEq([]vm.Value{allocStr(h, "abc"), allocStr(h, "abc")}, h)
	if !v.AsBool() {
		t.Error("\"abc\" == \"abc\" should be true")
	}
	v, _ = vm.BuiltinEq([]vm.Value{allocStr(h, "abc"), allocStr(h, "xyz")}, h)
	if v.AsBool() {
		t.Error("\"abc\" == \"xyz\" should be false")
	}

	// Array equality.
	a1 := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2)})
	a2 := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2)})
	v, _ = vm.BuiltinEq([]vm.Value{a1, a2}, h)
	if !v.AsBool() {
		t.Error("[1,2] == [1,2] should be true")
	}
}

func TestBuiltinCompare(t *testing.T) {
	h := newHeap()
	v, _ := vm.BuiltinCompare([]vm.Value{vm.IntVal(1), vm.IntVal(2)}, h)
	if v.AsInt() != -1 {
		t.Errorf("compare(1,2) = %d, want -1", v.AsInt())
	}
	v, _ = vm.BuiltinCompare([]vm.Value{vm.IntVal(2), vm.IntVal(2)}, h)
	if v.AsInt() != 0 {
		t.Errorf("compare(2,2) = %d, want 0", v.AsInt())
	}
	v, _ = vm.BuiltinCompare([]vm.Value{vm.IntVal(3), vm.IntVal(2)}, h)
	if v.AsInt() != 1 {
		t.Errorf("compare(3,2) = %d, want 1", v.AsInt())
	}

	// String comparison.
	v, _ = vm.BuiltinCompare([]vm.Value{allocStr(h, "abc"), allocStr(h, "def")}, h)
	if v.AsInt() != -1 {
		t.Errorf("compare(\"abc\",\"def\") = %d, want -1", v.AsInt())
	}
}

func TestBuiltinToString(t *testing.T) {
	h := newHeap()
	v, _ := vm.BuiltinToString([]vm.Value{vm.IntVal(42)}, h)
	if getString(v, h) != "42" {
		t.Errorf("to_string(42) = %q", getString(v, h))
	}
	v, _ = vm.BuiltinToString([]vm.Value{vm.BoolVal(true)}, h)
	if getString(v, h) != "true" {
		t.Errorf("to_string(true) = %q", getString(v, h))
	}
	v, _ = vm.BuiltinToString([]vm.Value{vm.UnitVal()}, h)
	if getString(v, h) != "()" {
		t.Errorf("to_string(()) = %q", getString(v, h))
	}
}

func TestBuiltinDefault(t *testing.T) {
	h := newHeap()
	v, _ := vm.BuiltinDefault([]vm.Value{vm.IntVal(int64(vm.TagInt))}, h)
	if v.Tag != vm.TagInt || v.AsInt() != 0 {
		t.Errorf("default(Int) = %v, want 0", v)
	}
	v, _ = vm.BuiltinDefault([]vm.Value{vm.IntVal(int64(vm.TagFloat))}, h)
	if v.Tag != vm.TagFloat || v.AsFloat() != 0.0 {
		t.Errorf("default(Float) = %v, want 0.0", v)
	}
	v, _ = vm.BuiltinDefault([]vm.Value{vm.IntVal(int64(vm.TagBool))}, h)
	if v.Tag != vm.TagBool || v.AsBool() {
		t.Error("default(Bool) should be false")
	}
	v, _ = vm.BuiltinDefault([]vm.Value{vm.IntVal(int64(vm.ObjString))}, h)
	if v.Tag != vm.TagObj || getString(v, h) != "" {
		t.Error("default(String) should be empty string")
	}
	v, _ = vm.BuiltinDefault([]vm.Value{vm.IntVal(int64(vm.ObjArray))}, h)
	if v.Tag != vm.TagObj {
		t.Error("default(Array) should be obj")
	}
	arr := getArray(v, h)
	if len(arr) != 0 {
		t.Error("default(Array) should be empty")
	}
}

func TestBuiltinClone(t *testing.T) {
	h := newHeap()
	// Primitive clone.
	v, _ := vm.BuiltinClone([]vm.Value{vm.IntVal(42)}, h)
	if v.Tag != vm.TagInt || v.AsInt() != 42 {
		t.Errorf("clone(42) = %v", v)
	}

	// String clone (separate heap allocation).
	orig := allocStr(h, "hello")
	clone, _ := vm.BuiltinClone([]vm.Value{orig}, h)
	if getString(clone, h) != "hello" {
		t.Errorf("clone string = %q", getString(clone, h))
	}
	if orig.AsObj() == clone.AsObj() {
		t.Error("clone should create new heap object")
	}

	// Array clone (deep).
	origArr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2)})
	cloneArr, _ := vm.BuiltinClone([]vm.Value{origArr}, h)
	elems := getArray(cloneArr, h)
	if len(elems) != 2 || elems[0].AsInt() != 1 || elems[1].AsInt() != 2 {
		t.Error("clone array mismatch")
	}
	if origArr.AsObj() == cloneArr.AsObj() {
		t.Error("clone array should create new heap object")
	}

	// Nested array clone.
	inner := allocArr(h, []vm.Value{vm.IntVal(10)})
	outer := allocArr(h, []vm.Value{inner})
	cloneOuter, _ := vm.BuiltinClone([]vm.Value{outer}, h)
	outerElems := getArray(cloneOuter, h)
	innerElems := getArray(outerElems[0], h)
	if innerElems[0].AsInt() != 10 {
		t.Error("deep clone nested array failed")
	}
	// Verify different heap indices.
	if outerElems[0].AsObj() == inner.AsObj() {
		t.Error("deep clone should copy inner arrays too")
	}
}

func TestBuiltinHash(t *testing.T) {
	h := newHeap()
	// Same values should produce same hash.
	h1, _ := vm.BuiltinHash([]vm.Value{vm.IntVal(42)}, h)
	h2, _ := vm.BuiltinHash([]vm.Value{vm.IntVal(42)}, h)
	if h1.AsInt() != h2.AsInt() {
		t.Error("hash(42) should be deterministic")
	}

	// Different values should (usually) produce different hashes.
	h3, _ := vm.BuiltinHash([]vm.Value{vm.IntVal(43)}, h)
	if h1.AsInt() == h3.AsInt() {
		t.Error("hash(42) and hash(43) should differ")
	}

	// String hash.
	h4, _ := vm.BuiltinHash([]vm.Value{allocStr(h, "hello")}, h)
	h5, _ := vm.BuiltinHash([]vm.Value{allocStr(h, "hello")}, h)
	if h4.AsInt() != h5.AsInt() {
		t.Error("hash(\"hello\") should be deterministic")
	}
}

func TestBuiltinRegistry(t *testing.T) {
	r := vm.NewBuiltinRegistry()
	vm.RegisterBuiltinTraits(r)

	// Verify all trait methods are registered.
	expected := []string{"eq", "neq", "compare", "to_string", "default", "clone", "hash"}
	for _, name := range expected {
		if _, ok := r.Lookup(name); !ok {
			t.Errorf("trait method %q not registered", name)
		}
	}

	// Call via registry.
	h := newHeap()
	v, err := r.Call("eq", []vm.Value{vm.IntVal(1), vm.IntVal(1)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if !v.AsBool() {
		t.Error("registry eq(1,1) should be true")
	}

	// Unknown builtin.
	_, err = r.Call("nonexistent", nil, h)
	if err == nil {
		t.Error("expected error for unknown builtin")
	}
}

func TestRegisterAll(t *testing.T) {
	r := vm.NewBuiltinRegistry()
	RegisterAll(r)

	// Verify all stdlib functions are registered.
	allNames := []string{
		// Core
		"int_to_float", "float_to_int", "int_to_string", "float_to_string",
		"parse_int", "parse_float",
		"bool_to_string", "char_to_int", "int_to_char",
		"print", "println", "read_line",
		"assert", "assert_eq", "panic",
		// String ops
		"string_len", "string_slice", "string_contains", "string_index_of",
		"string_repeat", "string_pad_left", "string_pad_right", "string_bytes",
		"string_join", "string_split", "string_trim", "string_chars",
		"char_to_string", "string_replace", "string_starts_with",
		"string_ends_with", "string_to_upper", "string_to_lower",
		// Array ops
		"array_len", "array_push", "array_pop", "array_map", "array_filter",
		"array_fold", "array_sort", "array_reverse", "array_contains",
		"array_zip", "array_enumerate", "array_flat_map",
		"array_find", "array_any", "array_all",
		"array_sum", "array_min", "array_max",
		"array_take", "array_drop", "array_chunk",
		"array_unique", "array_join", "array_slice",
		// Math ops
		"abs", "min", "max", "sqrt", "pow", "floor", "ceil", "round",
		"sin", "cos", "tan", "asin", "acos", "atan", "atan2",
		"log", "log2", "log10", "exp", "pi", "e", "gcd", "lcm", "clamp",
		"random_int", "random_float",
		// File I/O
		"read_file", "write_file",
		"file_exists", "dir_list", "dir_create", "path_join",
		"path_dirname", "path_basename", "path_extension", "file_size",
		// Time/Random
		"time_now_ms", "sleep_ms", "random_seed", "random_shuffle", "random_choice",
		// Map ops
		"map_new", "map_get", "map_set", "map_delete", "map_contains",
		"map_len", "map_keys", "map_values", "map_entries",
		"map_merge", "map_filter", "map_map",
		// Trait methods
		"eq", "neq", "compare", "to_string", "default", "clone", "hash",
	}
	for _, name := range allNames {
		if _, ok := r.Lookup(name); !ok {
			t.Errorf("stdlib function %q not registered", name)
		}
	}

	// Verify the registry includes at least the expected stdlib surface.
	registered := r.Names()
	if len(registered) < len(allNames) {
		t.Errorf("registered %d functions, want at least %d", len(registered), len(allNames))
	}

	// Smoke test: call a few through the registry.
	h := newHeap()
	v, err := r.Call("abs", []vm.Value{vm.IntVal(-5)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 5 {
		t.Errorf("abs(-5) via registry = %d, want 5", v.AsInt())
	}

	v, err = r.Call("string_len", []vm.Value{allocStr(h, "hello")}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 5 {
		t.Errorf("string_len(\"hello\") via registry = %d, want 5", v.AsInt())
	}
}

// ---------------------------------------------------------------------------
// Result helpers
// ---------------------------------------------------------------------------

func TestResultHelpers(t *testing.T) {
	h := newHeap()
	ok := makeResultOk(vm.IntVal(42), h)
	if !IsResultOk(ok, h) {
		t.Error("expected Ok")
	}
	inner, err := ResultUnwrap(ok, h)
	if err != nil {
		t.Fatal(err)
	}
	if inner.AsInt() != 42 {
		t.Errorf("unwrap = %d, want 42", inner.AsInt())
	}

	errVal := makeResultErr("oops", h)
	if IsResultOk(errVal, h) {
		t.Error("expected Err")
	}
	_, err = ResultUnwrap(errVal, h)
	if err == nil {
		t.Error("unwrap on Err should return error")
	}

	// Non-Result value.
	if IsResultOk(vm.IntVal(5), h) {
		t.Error("plain int should not be Result Ok")
	}
}

// ---------------------------------------------------------------------------
// Edge cases and error conditions
// ---------------------------------------------------------------------------

func TestArgCountErrors(t *testing.T) {
	h := newHeap()
	fns := []struct {
		name string
		fn   func([]vm.Value, *vm.Heap) (vm.Value, error)
	}{
		{"IntToFloat", IntToFloat},
		{"FloatToInt", FloatToInt},
		{"IntToString", IntToString},
		{"FloatToString", FloatToString},
		{"StringLen", StringLen},
		{"StringTrim", StringTrim},
		{"StringChars", StringChars},
		{"CharToString", CharToString},
		{"ArrayLen", ArrayLen},
		{"ArrayReverse", ArrayReverse},
		{"ArraySort", ArraySort},
		{"Abs", Abs},
		{"Sqrt", Sqrt},
		{"Floor", Floor},
		{"Ceil", Ceil},
		{"Round", Round},
		{"Sin", Sin},
		{"Cos", Cos},
		{"Tan", Tan},
		{"Asin", Asin},
		{"Acos", Acos},
		{"Atan", Atan},
		{"Atan2", Atan2},
		{"Log", Log},
		{"Log2", Log2},
		{"Log10", Log10},
		{"Exp", Exp},
		{"Gcd", Gcd},
		{"Lcm", Lcm},
		{"Clamp", Clamp},
	}
	for _, tc := range fns {
		_, err := tc.fn(nil, h)
		if err == nil {
			t.Errorf("%s(nil) should return error", tc.name)
		}
	}
}

// ---------------------------------------------------------------------------
// time_ops.go — Time functions
// ---------------------------------------------------------------------------

func TestTimeNowMs(t *testing.T) {
	h := newHeap()
	before := time.Now().UnixMilli()
	v, err := TimeNowMs(nil, h)
	after := time.Now().UnixMilli()
	if err != nil {
		t.Fatalf("TimeNowMs: %v", err)
	}
	if v.Tag != vm.TagInt {
		t.Fatalf("TimeNowMs: expected TagInt, got %d", v.Tag)
	}
	ms := v.AsInt()
	if ms < before || ms > after {
		t.Errorf("TimeNowMs = %d, want in [%d, %d]", ms, before, after)
	}
	// Wrong arg count.
	if _, err := TimeNowMs([]vm.Value{vm.IntVal(1)}, h); err == nil {
		t.Error("expected error for 1 arg")
	}
}

func TestSleepMs(t *testing.T) {
	h := newHeap()
	start := time.Now()
	_, err := SleepMs([]vm.Value{vm.IntVal(10)}, h)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("SleepMs(10): %v", err)
	}
	if elapsed < 10*time.Millisecond {
		t.Errorf("SleepMs(10) elapsed %v, want >= 10ms", elapsed)
	}
	// Negative value.
	if _, err := SleepMs([]vm.Value{vm.IntVal(-1)}, h); err == nil {
		t.Error("expected error for negative ms")
	}
	// Wrong type.
	if _, err := SleepMs([]vm.Value{vm.FloatVal(1.0)}, h); err == nil {
		t.Error("expected error for Float arg")
	}
	// Wrong arg count.
	if _, err := SleepMs(nil, h); err == nil {
		t.Error("expected error for 0 args")
	}
}

// ---------------------------------------------------------------------------
// time_ops.go — Random functions
// ---------------------------------------------------------------------------

func TestRandomSeed(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(10), vm.IntVal(20), vm.IntVal(30), vm.IntVal(40), vm.IntVal(50)})

	// Seed, then collect a sequence of choices.
	RandomSeed([]vm.Value{vm.IntVal(42)}, h)
	seq1 := make([]int64, 10)
	for i := range seq1 {
		v, err := RandomChoice([]vm.Value{arr}, h)
		if err != nil {
			t.Fatalf("RandomChoice: %v", err)
		}
		seq1[i] = v.AsInt()
	}

	// Re-seed with same value, verify identical sequence.
	RandomSeed([]vm.Value{vm.IntVal(42)}, h)
	for i := range seq1 {
		v, _ := RandomChoice([]vm.Value{arr}, h)
		if v.AsInt() != seq1[i] {
			t.Errorf("determinism broken at index %d: got %d, want %d", i, v.AsInt(), seq1[i])
		}
	}

	// Wrong arg count.
	if _, err := RandomSeed(nil, h); err == nil {
		t.Error("expected error for 0 args")
	}
	// Wrong type.
	if _, err := RandomSeed([]vm.Value{vm.FloatVal(1.0)}, h); err == nil {
		t.Error("expected error for Float arg")
	}
}

func TestRandomShuffle(t *testing.T) {
	h := newHeap()
	elems := []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3), vm.IntVal(4), vm.IntVal(5)}
	arr := allocArr(h, elems)

	v, err := RandomShuffle([]vm.Value{arr}, h)
	if err != nil {
		t.Fatalf("RandomShuffle: %v", err)
	}
	shuffled := getArray(v, h)
	if len(shuffled) != len(elems) {
		t.Fatalf("RandomShuffle: length %d, want %d", len(shuffled), len(elems))
	}
	// Verify same elements present (sort both and compare).
	orig := make([]int64, len(elems))
	got := make([]int64, len(shuffled))
	for i := range elems {
		orig[i] = elems[i].AsInt()
		got[i] = shuffled[i].AsInt()
	}
	sort.Slice(orig, func(i, j int) bool { return orig[i] < orig[j] })
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	for i := range orig {
		if orig[i] != got[i] {
			t.Errorf("RandomShuffle: sorted element %d = %d, want %d", i, got[i], orig[i])
		}
	}

	// Wrong arg count.
	if _, err := RandomShuffle(nil, h); err == nil {
		t.Error("expected error for 0 args")
	}
}

func TestRandomChoice(t *testing.T) {
	h := newHeap()
	elems := []vm.Value{vm.IntVal(10), vm.IntVal(20), vm.IntVal(30)}
	arr := allocArr(h, elems)

	valid := map[int64]bool{10: true, 20: true, 30: true}
	for i := 0; i < 20; i++ {
		v, err := RandomChoice([]vm.Value{arr}, h)
		if err != nil {
			t.Fatalf("RandomChoice: %v", err)
		}
		if !valid[v.AsInt()] {
			t.Errorf("RandomChoice returned %d, not in {10, 20, 30}", v.AsInt())
		}
	}

	// Empty array.
	emptyArr := allocArr(h, nil)
	if _, err := RandomChoice([]vm.Value{emptyArr}, h); err == nil {
		t.Error("expected error for empty array")
	}
	// Wrong arg count.
	if _, err := RandomChoice(nil, h); err == nil {
		t.Error("expected error for 0 args")
	}
}

// ---------------------------------------------------------------------------
// core.go — Conversion functions (bool_to_string, char_to_int, int_to_char)
// ---------------------------------------------------------------------------

func TestBoolToString(t *testing.T) {
	h := newHeap()
	tests := []struct {
		in  bool
		out string
	}{
		{true, "true"},
		{false, "false"},
	}
	for _, tc := range tests {
		v, err := BoolToString([]vm.Value{vm.BoolVal(tc.in)}, h)
		if err != nil {
			t.Fatalf("BoolToString(%v): %v", tc.in, err)
		}
		got := getString(v, h)
		if got != tc.out {
			t.Errorf("BoolToString(%v) = %q, want %q", tc.in, got, tc.out)
		}
	}
	// Wrong type.
	if _, err := BoolToString([]vm.Value{vm.IntVal(1)}, h); err == nil {
		t.Error("expected error for Int arg")
	}
	// Wrong arg count.
	if _, err := BoolToString(nil, h); err == nil {
		t.Error("expected error for 0 args")
	}
}

func TestCharToInt(t *testing.T) {
	h := newHeap()
	tests := []struct {
		in  rune
		out int64
	}{
		{'a', 97},
		{'0', 48},
		{'Z', 90},
	}
	for _, tc := range tests {
		v, err := CharToInt([]vm.Value{vm.CharVal(tc.in)}, h)
		if err != nil {
			t.Fatalf("CharToInt(%c): %v", tc.in, err)
		}
		if v.Tag != vm.TagInt || v.AsInt() != tc.out {
			t.Errorf("CharToInt(%c) = %d, want %d", tc.in, v.AsInt(), tc.out)
		}
	}
	// Wrong type.
	if _, err := CharToInt([]vm.Value{vm.IntVal(97)}, h); err == nil {
		t.Error("expected error for Int arg")
	}
	// Wrong arg count.
	if _, err := CharToInt(nil, h); err == nil {
		t.Error("expected error for 0 args")
	}
}

func TestIntToChar(t *testing.T) {
	h := newHeap()
	tests := []struct {
		in  int64
		out rune
	}{
		{97, 'a'},
		{48, '0'},
		{90, 'Z'},
	}
	for _, tc := range tests {
		v, err := IntToChar([]vm.Value{vm.IntVal(tc.in)}, h)
		if err != nil {
			t.Fatalf("IntToChar(%d): %v", tc.in, err)
		}
		if v.Tag != vm.TagChar || v.AsChar() != tc.out {
			t.Errorf("IntToChar(%d) = %c, want %c", tc.in, v.AsChar(), tc.out)
		}
	}
	// Wrong type.
	if _, err := IntToChar([]vm.Value{vm.FloatVal(97.0)}, h); err == nil {
		t.Error("expected error for Float arg")
	}
	// Wrong arg count.
	if _, err := IntToChar(nil, h); err == nil {
		t.Error("expected error for 0 args")
	}
}

// ---------------------------------------------------------------------------
// Additional coverage: Unicode for CharToInt / IntToChar
// ---------------------------------------------------------------------------

func TestCharToIntUnicode(t *testing.T) {
	h := newHeap()
	// 'A' -> 65
	v, err := CharToInt([]vm.Value{vm.CharVal('A')}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 65 {
		t.Errorf("CharToInt('A') = %d, want 65", v.AsInt())
	}

	// CJK character: U+65E5 = 26085
	v, err = CharToInt([]vm.Value{vm.CharVal('\u65E5')}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 26085 {
		t.Errorf("CharToInt('\\u65E5') = %d, want 26085", v.AsInt())
	}
}

func TestIntToCharUnicode(t *testing.T) {
	h := newHeap()
	// 65 -> 'A'
	v, err := IntToChar([]vm.Value{vm.IntVal(65)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsChar() != 'A' {
		t.Errorf("IntToChar(65) = %c, want A", v.AsChar())
	}

	// 26085 -> U+65E5
	v, err = IntToChar([]vm.Value{vm.IntVal(26085)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsChar() != '\u65E5' {
		t.Errorf("IntToChar(26085) = %c, want \\u65E5", v.AsChar())
	}
}

// ---------------------------------------------------------------------------
// Additional coverage: Deterministic RandomShuffle after seed
// ---------------------------------------------------------------------------

func TestRandomShuffleDeterministic(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3), vm.IntVal(4), vm.IntVal(5)})

	RandomSeed([]vm.Value{vm.IntVal(777)}, h)
	s1, err := RandomShuffle([]vm.Value{arr}, h)
	if err != nil {
		t.Fatal(err)
	}

	RandomSeed([]vm.Value{vm.IntVal(777)}, h)
	s2, err := RandomShuffle([]vm.Value{arr}, h)
	if err != nil {
		t.Fatal(err)
	}

	e1 := getArray(s1, h)
	e2 := getArray(s2, h)
	for i := range e1 {
		if e1[i].AsInt() != e2[i].AsInt() {
			t.Errorf("deterministic shuffle mismatch at %d: %d vs %d", i, e1[i].AsInt(), e2[i].AsInt())
		}
	}
}

func TestRandomChoiceSingleElement(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(99)})
	for i := 0; i < 10; i++ {
		v, err := RandomChoice([]vm.Value{arr}, h)
		if err != nil {
			t.Fatal(err)
		}
		if v.AsInt() != 99 {
			t.Errorf("choice([99]) = %d, want 99", v.AsInt())
		}
	}
}

// ---------------------------------------------------------------------------
// Additional array_ops.go edge case coverage
// ---------------------------------------------------------------------------

func TestArraySumNonNumericError(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.BoolVal(true)})
	_, err := ArraySum([]vm.Value{arr}, h)
	if err == nil {
		t.Error("expected error for non-numeric element in sum")
	}
}

func TestArrayChunkZeroSizeError(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2)})
	_, err := ArrayChunk([]vm.Value{arr, vm.IntVal(0)}, h)
	if err == nil {
		t.Error("expected error for chunk size 0")
	}
	_, err = ArrayChunk([]vm.Value{arr, vm.IntVal(-1)}, h)
	if err == nil {
		t.Error("expected error for negative chunk size")
	}
}

func TestArraySliceNegativeIndices(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(10), vm.IntVal(20), vm.IntVal(30), vm.IntVal(40), vm.IntVal(50)})

	// Negative start: -2 means index 3.
	v, err := ArraySlice([]vm.Value{arr, vm.IntVal(-2), vm.IntVal(5)}, h)
	if err != nil {
		t.Fatal(err)
	}
	elems := getArray(v, h)
	if len(elems) != 2 || elems[0].AsInt() != 40 || elems[1].AsInt() != 50 {
		t.Errorf("slice(-2,5) = %v, want [40,50]", elems)
	}

	// Negative end: -1 means index 4.
	v, _ = ArraySlice([]vm.Value{arr, vm.IntVal(0), vm.IntVal(-1)}, h)
	elems = getArray(v, h)
	if len(elems) != 4 {
		t.Errorf("slice(0,-1) length = %d, want 4", len(elems))
	}

	// Both negative.
	v, _ = ArraySlice([]vm.Value{arr, vm.IntVal(-3), vm.IntVal(-1)}, h)
	elems = getArray(v, h)
	if len(elems) != 2 || elems[0].AsInt() != 30 || elems[1].AsInt() != 40 {
		t.Errorf("slice(-3,-1) = %v, want [30,40]", elems)
	}
}

func TestArrayChunkEvenSplit(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3), vm.IntVal(4)})
	v, err := ArrayChunk([]vm.Value{arr, vm.IntVal(2)}, h)
	if err != nil {
		t.Fatal(err)
	}
	chunks := getArray(v, h)
	if len(chunks) != 2 {
		t.Fatalf("chunk(2) on [1,2,3,4] = %d chunks, want 2", len(chunks))
	}
	c0 := getArray(chunks[0], h)
	c1 := getArray(chunks[1], h)
	if len(c0) != 2 || c0[0].AsInt() != 1 || c0[1].AsInt() != 2 {
		t.Errorf("chunk[0] = %v, want [1,2]", c0)
	}
	if len(c1) != 2 || c1[0].AsInt() != 3 || c1[1].AsInt() != 4 {
		t.Errorf("chunk[1] = %v, want [3,4]", c1)
	}

	// Empty array chunk.
	empty := allocArr(h, []vm.Value{})
	v, _ = ArrayChunk([]vm.Value{empty, vm.IntVal(3)}, h)
	chunks = getArray(v, h)
	if len(chunks) != 0 {
		t.Errorf("chunk empty = %d chunks, want 0", len(chunks))
	}
}

func TestArraySumMixedIntFloat(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.FloatVal(2.5), vm.IntVal(3)})
	v, err := ArraySum([]vm.Value{arr}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.Tag != vm.TagFloat {
		t.Fatalf("mixed sum tag = %d, want Float", v.Tag)
	}
	if v.AsFloat() != 6.5 {
		t.Errorf("mixed sum = %v, want 6.5", v.AsFloat())
	}
}

func TestArrayMinMaxSingleElement(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(42)})

	v, err := ArrayMin([]vm.Value{arr}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 42 {
		t.Errorf("min([42]) = %d, want 42", v.AsInt())
	}

	v, err = ArrayMax([]vm.Value{arr}, h)
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInt() != 42 {
		t.Errorf("max([42]) = %d, want 42", v.AsInt())
	}
}

func TestArrayTakeDropNegative(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(1), vm.IntVal(2), vm.IntVal(3)})

	// Take with negative n returns empty.
	v, err := ArrayTake([]vm.Value{arr, vm.IntVal(-1)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if len(getArray(v, h)) != 0 {
		t.Error("take(-1) should return empty")
	}

	// Drop with negative n returns full array.
	v, err = ArrayDrop([]vm.Value{arr, vm.IntVal(-1)}, h)
	if err != nil {
		t.Fatal(err)
	}
	if len(getArray(v, h)) != 3 {
		t.Errorf("drop(-1) length = %d, want 3", len(getArray(v, h)))
	}
}

func TestArrayUniqueAllSame(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{vm.IntVal(7), vm.IntVal(7), vm.IntVal(7)})
	v, err := ArrayUnique([]vm.Value{arr}, h)
	if err != nil {
		t.Fatal(err)
	}
	elems := getArray(v, h)
	if len(elems) != 1 || elems[0].AsInt() != 7 {
		t.Errorf("unique([7,7,7]) = %v, want [7]", elems)
	}
}

func TestArrayJoinEmptySeparator(t *testing.T) {
	h := newHeap()
	arr := allocArr(h, []vm.Value{allocStr(h, "a"), allocStr(h, "b"), allocStr(h, "c")})
	v, err := ArrayJoin([]vm.Value{arr, allocStr(h, "")}, h)
	if err != nil {
		t.Fatal(err)
	}
	if getString(v, h) != "abc" {
		t.Errorf("join with empty sep = %q, want %q", getString(v, h), "abc")
	}
}
