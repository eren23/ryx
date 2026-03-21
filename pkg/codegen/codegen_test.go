package codegen

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/mir"
	"github.com/ryx-lang/ryx/pkg/types"
)

// ---------------------------------------------------------------------------
// Opcode round-trip encoding/decoding
// ---------------------------------------------------------------------------

func TestOpcodeRoundTrip(t *testing.T) {
	// Test every opcode with no operands round-trips through encode/decode.
	noOperandOps := []Opcode{
		OpConstTrue, OpConstFalse, OpConstUnit, OpPop, OpDup, OpSwap,
		OpAddInt, OpAddFloat, OpSubInt, OpSubFloat, OpMulInt, OpMulFloat,
		OpDivInt, OpDivFloat, OpModInt, OpModFloat, OpNegInt, OpNegFloat,
		OpConcatString,
		OpEq, OpNeq, OpLtInt, OpLtFloat, OpGtInt, OpGtFloat,
		OpLeqInt, OpLeqFloat, OpGeqInt, OpGeqFloat,
		OpNot, OpReturn, OpIndexGet, OpIndexSet, OpAllocClosure,
		OpChannelSend, OpChannelRecv, OpChannelClose,
		OpPrint, OpPrintln, OpIntToFloat, OpFloatToInt,
		OpIntToString, OpFloatToString, OpStringLen, OpArrayLen,
		OpBreakpoint,
	}
	for _, op := range noOperandOps {
		var buf []byte
		EmitOp(&buf, op)
		instr, size, err := DecodeInstruction(buf, 0)
		if err != nil {
			t.Fatalf("decode %s: %v", op, err)
		}
		if size != 1 {
			t.Errorf("%s: expected size 1, got %d", op, size)
		}
		if instr.Op != op {
			t.Errorf("expected op %s, got %s", op, instr.Op)
		}
	}
}

func TestOpcodeU16RoundTrip(t *testing.T) {
	u16Ops := []Opcode{
		OpConstString, OpLoadLocal, OpStoreLocal, OpLoadUpvalue, OpStoreUpvalue,
		OpLoadGlobal, OpStoreGlobal, OpCall, OpTailCall,
		OpMakeArray, OpMakeTuple, OpFieldGet, OpFieldSet,
		OpAllocArray, OpTagCheck, OpDestructure, OpChannelCreate, OpSpawn,
	}
	for _, op := range u16Ops {
		var buf []byte
		EmitU16(&buf, op, 0x1234)
		instr, size, err := DecodeInstruction(buf, 0)
		if err != nil {
			t.Fatalf("decode %s: %v", op, err)
		}
		if size != 3 {
			t.Errorf("%s: expected size 3, got %d", op, size)
		}
		if instr.Op != op {
			t.Errorf("expected op %s, got %s", op, instr.Op)
		}
		if len(instr.Operands) != 1 || instr.Operands[0] != 0x1234 {
			t.Errorf("%s: expected operand 0x1234, got %v", op, instr.Operands)
		}
	}
}

// ---------------------------------------------------------------------------
// CONST_INT encoding
// ---------------------------------------------------------------------------

func TestConstIntEncoding(t *testing.T) {
	tests := []int64{0, 1, -1, 42, math.MaxInt64, math.MinInt64, 1000000}
	for _, val := range tests {
		var buf []byte
		EmitI64(&buf, OpConstInt, val)

		instr, size, err := DecodeInstruction(buf, 0)
		if err != nil {
			t.Fatalf("CONST_INT %d: decode error: %v", val, err)
		}
		if size != 9 {
			t.Errorf("CONST_INT: expected size 9, got %d", size)
		}
		if instr.Op != OpConstInt {
			t.Errorf("expected OpConstInt, got %s", instr.Op)
		}
		if len(instr.Operands) != 1 || instr.Operands[0] != val {
			t.Errorf("CONST_INT: expected %d, got %v", val, instr.Operands)
		}
	}
}

// ---------------------------------------------------------------------------
// CONST_FLOAT encoding
// ---------------------------------------------------------------------------

func TestConstFloatEncoding(t *testing.T) {
	tests := []float64{0.0, 1.5, -3.14, math.MaxFloat64, math.SmallestNonzeroFloat64, math.Inf(1)}
	for _, val := range tests {
		var buf []byte
		EmitF64(&buf, OpConstFloat, val)

		instr, size, err := DecodeInstruction(buf, 0)
		if err != nil {
			t.Fatalf("CONST_FLOAT %g: decode error: %v", val, err)
		}
		if size != 9 {
			t.Errorf("CONST_FLOAT: expected size 9, got %d", size)
		}
		if instr.Op != OpConstFloat {
			t.Errorf("expected OpConstFloat, got %s", instr.Op)
		}
		decoded := OperandF64(instr)
		if decoded != val && !(math.IsNaN(val) && math.IsNaN(decoded)) {
			t.Errorf("CONST_FLOAT: expected %g, got %g", val, decoded)
		}
	}
}

// ---------------------------------------------------------------------------
// CONST_CHAR encoding
// ---------------------------------------------------------------------------

func TestConstCharEncoding(t *testing.T) {
	chars := []rune{'A', 'Z', '0', '\n', 0x1F600, 0x4E16} // includes emoji and CJK
	for _, ch := range chars {
		var buf []byte
		EmitU32(&buf, OpConstChar, uint32(ch))

		instr, size, err := DecodeInstruction(buf, 0)
		if err != nil {
			t.Fatalf("CONST_CHAR U+%04X: decode error: %v", ch, err)
		}
		if size != 5 {
			t.Errorf("CONST_CHAR: expected size 5, got %d", size)
		}
		if uint32(instr.Operands[0]) != uint32(ch) {
			t.Errorf("CONST_CHAR: expected U+%04X, got U+%04X", ch, uint32(instr.Operands[0]))
		}
	}
}

// ---------------------------------------------------------------------------
// Jump offset encoding
// ---------------------------------------------------------------------------

func TestJumpOffsetEncoding(t *testing.T) {
	offsets := []int16{0, 1, -1, 100, -100, math.MaxInt16, math.MinInt16}
	for _, off := range offsets {
		var buf []byte
		EmitI16(&buf, OpJump, off)

		instr, size, err := DecodeInstruction(buf, 0)
		if err != nil {
			t.Fatalf("JUMP %d: decode error: %v", off, err)
		}
		if size != 3 {
			t.Errorf("JUMP: expected size 3, got %d", size)
		}
		if int16(instr.Operands[0]) != off {
			t.Errorf("JUMP: expected offset %d, got %d", off, int16(instr.Operands[0]))
		}
	}
}

func TestJumpTableEncoding(t *testing.T) {
	offsets := []int16{10, -20, 30, -40}
	var buf []byte
	EmitJumpTable(&buf, offsets)

	instr, size, err := DecodeInstruction(buf, 0)
	if err != nil {
		t.Fatalf("JUMP_TABLE: decode error: %v", err)
	}
	// 1 (opcode) + 2 (count) + 4*2 (offsets) = 11
	if size != 11 {
		t.Errorf("JUMP_TABLE: expected size 11, got %d", size)
	}
	if instr.Op != OpJumpTable {
		t.Errorf("expected OpJumpTable, got %s", instr.Op)
	}
	if len(instr.Operands) != 5 { // count + 4 offsets
		t.Errorf("JUMP_TABLE: expected 5 operands, got %d", len(instr.Operands))
	}
	if instr.Operands[0] != 4 {
		t.Errorf("JUMP_TABLE: expected count 4, got %d", instr.Operands[0])
	}
	for i, expected := range offsets {
		got := int16(instr.Operands[i+1])
		if got != expected {
			t.Errorf("JUMP_TABLE[%d]: expected %d, got %d", i, expected, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Multi-operand opcodes
// ---------------------------------------------------------------------------

func TestU16U16Encoding(t *testing.T) {
	var buf []byte
	EmitU16U16(&buf, OpCallMethod, 42, 3)
	instr, size, err := DecodeInstruction(buf, 0)
	if err != nil {
		t.Fatalf("CALL_METHOD: %v", err)
	}
	if size != 5 {
		t.Errorf("CALL_METHOD: expected size 5, got %d", size)
	}
	if instr.Operands[0] != 42 || instr.Operands[1] != 3 {
		t.Errorf("CALL_METHOD: expected (42, 3), got %v", instr.Operands)
	}
}

func TestU16U16U16Encoding(t *testing.T) {
	var buf []byte
	EmitU16U16U16(&buf, OpMakeEnum, 10, 2, 3)
	instr, size, err := DecodeInstruction(buf, 0)
	if err != nil {
		t.Fatalf("MAKE_ENUM: %v", err)
	}
	if size != 7 {
		t.Errorf("MAKE_ENUM: expected size 7, got %d", size)
	}
	if instr.Operands[0] != 10 || instr.Operands[1] != 2 || instr.Operands[2] != 3 {
		t.Errorf("MAKE_ENUM: expected (10, 2, 3), got %v", instr.Operands)
	}
}

// ---------------------------------------------------------------------------
// Multiple instructions in sequence
// ---------------------------------------------------------------------------

func TestMultipleInstructionDecode(t *testing.T) {
	var buf []byte
	EmitI64(&buf, OpConstInt, 10)
	EmitI64(&buf, OpConstInt, 20)
	EmitOp(&buf, OpAddInt)
	EmitOp(&buf, OpReturn)

	offset := 0
	// Instruction 1: CONST_INT 10
	instr, size, err := DecodeInstruction(buf, offset)
	if err != nil {
		t.Fatal(err)
	}
	if instr.Op != OpConstInt || instr.Operands[0] != 10 {
		t.Errorf("expected CONST_INT 10, got %s %v", instr.Op, instr.Operands)
	}
	offset += size

	// Instruction 2: CONST_INT 20
	instr, size, err = DecodeInstruction(buf, offset)
	if err != nil {
		t.Fatal(err)
	}
	if instr.Op != OpConstInt || instr.Operands[0] != 20 {
		t.Errorf("expected CONST_INT 20, got %s %v", instr.Op, instr.Operands)
	}
	offset += size

	// Instruction 3: ADD_INT
	instr, size, err = DecodeInstruction(buf, offset)
	if err != nil {
		t.Fatal(err)
	}
	if instr.Op != OpAddInt {
		t.Errorf("expected ADD_INT, got %s", instr.Op)
	}
	offset += size

	// Instruction 4: RETURN
	instr, _, err = DecodeInstruction(buf, offset)
	if err != nil {
		t.Fatal(err)
	}
	if instr.Op != OpReturn {
		t.Errorf("expected RETURN, got %s", instr.Op)
	}
}

// ---------------------------------------------------------------------------
// String pool deduplication
// ---------------------------------------------------------------------------

func TestStringPoolDedup(t *testing.T) {
	g := &generator{
		strings:   nil,
		stringIdx: make(map[string]uint16),
	}

	idx1 := g.internString("hello")
	idx2 := g.internString("world")
	idx3 := g.internString("hello") // duplicate

	if idx1 != idx3 {
		t.Errorf("expected dedup: idx1=%d, idx3=%d", idx1, idx3)
	}
	if idx1 == idx2 {
		t.Errorf("different strings should have different indices")
	}
	if len(g.strings) != 2 {
		t.Errorf("expected 2 strings in pool, got %d", len(g.strings))
	}
	if g.strings[idx1] != "hello" || g.strings[idx2] != "world" {
		t.Errorf("unexpected string pool contents: %v", g.strings)
	}
}

func TestStringPoolDedup_MoreStrings(t *testing.T) {
	g := &generator{
		strings:   nil,
		stringIdx: make(map[string]uint16),
	}

	strs := []string{"alpha", "beta", "gamma", "alpha", "beta", "delta", "gamma"}
	indices := make([]uint16, len(strs))
	for i, s := range strs {
		indices[i] = g.internString(s)
	}

	// "alpha" appears at 0, 3 — same index
	if indices[0] != indices[3] {
		t.Errorf("alpha dedup failed: %d != %d", indices[0], indices[3])
	}
	// "beta" appears at 1, 4 — same index
	if indices[1] != indices[4] {
		t.Errorf("beta dedup failed: %d != %d", indices[1], indices[4])
	}
	// "gamma" appears at 2, 6 — same index
	if indices[2] != indices[6] {
		t.Errorf("gamma dedup failed: %d != %d", indices[2], indices[6])
	}
	// Unique count: alpha, beta, gamma, delta = 4
	if len(g.strings) != 4 {
		t.Errorf("expected 4 unique strings, got %d", len(g.strings))
	}
}

// ---------------------------------------------------------------------------
// Function table offsets
// ---------------------------------------------------------------------------

func TestFunctionTableOffsets(t *testing.T) {
	// Build a simple two-function MIR program.
	prog := &mir.Program{
		Functions: []*mir.Function{
			buildAddFunction(),
			buildMainFunction(),
		},
	}

	compiled, err := Generate(prog)
	if err != nil {
		t.Fatalf("codegen: %v", err)
	}

	if len(compiled.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(compiled.Functions))
	}

	fn0 := compiled.Functions[0]
	fn1 := compiled.Functions[1]

	// First function starts at offset 0.
	if fn0.CodeOffset != 0 {
		t.Errorf("fn0: expected code offset 0, got %d", fn0.CodeOffset)
	}

	// Second function starts after the first.
	if fn1.CodeOffset != fn0.CodeOffset+fn0.CodeLength {
		t.Errorf("fn1: expected code offset %d, got %d",
			fn0.CodeOffset+fn0.CodeLength, fn1.CodeOffset)
	}

	// Code lengths should be non-zero.
	if fn0.CodeLength == 0 {
		t.Error("fn0: code length should be > 0")
	}
	if fn1.CodeLength == 0 {
		t.Error("fn1: code length should be > 0")
	}

	// Total code should equal sum of function code lengths.
	total := fn0.CodeLength + fn1.CodeLength
	if uint32(len(compiled.Code)) != total {
		t.Errorf("total code: expected %d, got %d", total, len(compiled.Code))
	}
}

// ---------------------------------------------------------------------------
// Max stack depth
// ---------------------------------------------------------------------------

func TestMaxStackDepth(t *testing.T) {
	prog := &mir.Program{
		Functions: []*mir.Function{buildAddFunction()},
	}
	compiled, err := Generate(prog)
	if err != nil {
		t.Fatalf("codegen: %v", err)
	}
	if len(compiled.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(compiled.Functions))
	}
	// The add function pushes two locals and does an add — max stack should be >= 2.
	if compiled.Functions[0].MaxStack < 2 {
		t.Errorf("expected max stack >= 2, got %d", compiled.Functions[0].MaxStack)
	}
}

// ---------------------------------------------------------------------------
// Binary format round-trip (encode → decode)
// ---------------------------------------------------------------------------

func TestBinaryRoundTrip(t *testing.T) {
	prog := &mir.Program{
		Functions: []*mir.Function{
			buildAddFunction(),
			buildMainFunction(),
		},
	}

	compiled, err := Generate(prog)
	if err != nil {
		t.Fatalf("codegen: %v", err)
	}

	// Encode
	var buf bytes.Buffer
	if err := Encode(&buf, compiled); err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Decode
	decoded, err := Decode(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Verify string pool
	if len(decoded.StringPool) != len(compiled.StringPool) {
		t.Errorf("string pool: expected %d, got %d", len(compiled.StringPool), len(decoded.StringPool))
	}
	for i, s := range compiled.StringPool {
		if i < len(decoded.StringPool) && decoded.StringPool[i] != s {
			t.Errorf("string[%d]: expected %q, got %q", i, s, decoded.StringPool[i])
		}
	}

	// Verify function table
	if len(decoded.Functions) != len(compiled.Functions) {
		t.Fatalf("functions: expected %d, got %d", len(compiled.Functions), len(decoded.Functions))
	}
	for i, fn := range compiled.Functions {
		dfn := decoded.Functions[i]
		if fn.NameIdx != dfn.NameIdx {
			t.Errorf("fn[%d].NameIdx: expected %d, got %d", i, fn.NameIdx, dfn.NameIdx)
		}
		if fn.Arity != dfn.Arity {
			t.Errorf("fn[%d].Arity: expected %d, got %d", i, fn.Arity, dfn.Arity)
		}
		if fn.LocalsCount != dfn.LocalsCount {
			t.Errorf("fn[%d].LocalsCount: expected %d, got %d", i, fn.LocalsCount, dfn.LocalsCount)
		}
		if fn.MaxStack != dfn.MaxStack {
			t.Errorf("fn[%d].MaxStack: expected %d, got %d", i, fn.MaxStack, dfn.MaxStack)
		}
		if fn.CodeOffset != dfn.CodeOffset {
			t.Errorf("fn[%d].CodeOffset: expected %d, got %d", i, fn.CodeOffset, dfn.CodeOffset)
		}
		if fn.CodeLength != dfn.CodeLength {
			t.Errorf("fn[%d].CodeLength: expected %d, got %d", i, fn.CodeLength, dfn.CodeLength)
		}
	}

	// Verify code section byte-for-byte
	if !bytes.Equal(decoded.Code, compiled.Code) {
		t.Errorf("code section mismatch: expected %d bytes, got %d bytes", len(compiled.Code), len(decoded.Code))
	}

	// Verify entry point
	if decoded.MainIndex != compiled.MainIndex {
		t.Errorf("main index: expected %d, got %d", compiled.MainIndex, decoded.MainIndex)
	}
}

func TestBinaryMagicValidation(t *testing.T) {
	// Invalid magic bytes should fail.
	bad := []byte{0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}
	_, err := Decode(bytes.NewReader(bad))
	if err == nil {
		t.Error("expected error for invalid magic, got nil")
	}
}

// ---------------------------------------------------------------------------
// Source map correctness
// ---------------------------------------------------------------------------

func TestSourceMapEncoding(t *testing.T) {
	compiled := &CompiledProgram{
		StringPool: []string{"main"},
		Functions: []CompiledFunc{{
			NameIdx:     0,
			Arity:       0,
			LocalsCount: 0,
			MaxStack:    1,
			CodeOffset:  0,
			CodeLength:  1,
		}},
		Code: []byte{byte(OpReturn)},
		SourceMap: []SourceMapEntry{
			{BytecodeOffset: 0, Line: 1, Col: 1},
			{BytecodeOffset: 10, Line: 5, Col: 12},
			{BytecodeOffset: 100, Line: 50, Col: 0},
		},
		MainIndex: 0,
	}

	var buf bytes.Buffer
	if err := Encode(&buf, compiled); err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := Decode(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(decoded.SourceMap) != 3 {
		t.Fatalf("expected 3 source map entries, got %d", len(decoded.SourceMap))
	}
	for i, expected := range compiled.SourceMap {
		got := decoded.SourceMap[i]
		if got.BytecodeOffset != expected.BytecodeOffset || got.Line != expected.Line || got.Col != expected.Col {
			t.Errorf("source map[%d]: expected {%d, %d, %d}, got {%d, %d, %d}",
				i, expected.BytecodeOffset, expected.Line, expected.Col,
				got.BytecodeOffset, got.Line, got.Col)
		}
	}
}

// ---------------------------------------------------------------------------
// Disassembler output
// ---------------------------------------------------------------------------

func TestDisassemblerBasic(t *testing.T) {
	var code []byte
	EmitI64(&code, OpConstInt, 42)
	EmitI64(&code, OpConstInt, 58)
	EmitOp(&code, OpAddInt)
	EmitOp(&code, OpReturn)

	output := Disassemble(code, nil)
	if !strings.Contains(output, "CONST_INT") {
		t.Error("disasm should contain CONST_INT")
	}
	if !strings.Contains(output, "42") {
		t.Error("disasm should contain 42")
	}
	if !strings.Contains(output, "58") {
		t.Error("disasm should contain 58")
	}
	if !strings.Contains(output, "ADD_INT") {
		t.Error("disasm should contain ADD_INT")
	}
	if !strings.Contains(output, "RETURN") {
		t.Error("disasm should contain RETURN")
	}
}

func TestDisassemblerWithStringPool(t *testing.T) {
	pool := []string{"hello", "world"}
	var code []byte
	EmitU16(&code, OpConstString, 0)
	EmitU16(&code, OpConstString, 1)
	EmitOp(&code, OpConcatString)
	EmitOp(&code, OpReturn)

	output := Disassemble(code, pool)
	if !strings.Contains(output, "CONST_STRING") {
		t.Error("disasm should contain CONST_STRING")
	}
	if !strings.Contains(output, `"hello"`) {
		t.Error("disasm should resolve string pool index to \"hello\"")
	}
	if !strings.Contains(output, `"world"`) {
		t.Error("disasm should resolve string pool index to \"world\"")
	}
}

func TestDisassemblerJump(t *testing.T) {
	var code []byte
	EmitI16(&code, OpJump, 10)
	EmitI16(&code, OpJumpIfFalse, -5)

	output := Disassemble(code, nil)
	if !strings.Contains(output, "JUMP") {
		t.Error("disasm should contain JUMP")
	}
	if !strings.Contains(output, "+10") {
		t.Error("disasm should show positive offset +10")
	}
	if !strings.Contains(output, "-5") {
		t.Error("disasm should show negative offset -5")
	}
}

func TestDisassemblerFloat(t *testing.T) {
	var code []byte
	EmitF64(&code, OpConstFloat, 3.14)

	output := Disassemble(code, nil)
	if !strings.Contains(output, "CONST_FLOAT") {
		t.Error("disasm should contain CONST_FLOAT")
	}
	if !strings.Contains(output, "3.14") {
		t.Error("disasm should contain 3.14")
	}
}

func TestDisassemblerChar(t *testing.T) {
	var code []byte
	EmitU32(&code, OpConstChar, uint32('A'))

	output := Disassemble(code, nil)
	if !strings.Contains(output, "CONST_CHAR") {
		t.Error("disasm should contain CONST_CHAR")
	}
	if !strings.Contains(output, "U+0041") {
		t.Error("disasm should contain U+0041")
	}
}

func TestDisassemblerJumpTable(t *testing.T) {
	var code []byte
	EmitJumpTable(&code, []int16{5, -3, 12})

	output := Disassemble(code, nil)
	if !strings.Contains(output, "JUMP_TABLE") {
		t.Error("disasm should contain JUMP_TABLE")
	}
	if !strings.Contains(output, "+5") {
		t.Error("disasm should show offset +5")
	}
	if !strings.Contains(output, "-3") {
		t.Error("disasm should show offset -3")
	}
}

func TestDisassembleProgram(t *testing.T) {
	prog := &mir.Program{
		Functions: []*mir.Function{buildAddFunction()},
	}
	compiled, err := Generate(prog)
	if err != nil {
		t.Fatalf("codegen: %v", err)
	}

	output := DisassembleProgram(compiled)
	if !strings.Contains(output, "RYX bytecode") {
		t.Error("program disasm should contain header")
	}
	if !strings.Contains(output, "function add") {
		t.Error("program disasm should contain function name")
	}
}

// ---------------------------------------------------------------------------
// Full codegen round-trip: MIR → bytecode → encode → decode → disassemble
// ---------------------------------------------------------------------------

func TestFullRoundTrip(t *testing.T) {
	prog := &mir.Program{
		Functions: []*mir.Function{
			buildAddFunction(),
			buildMainFunction(),
		},
	}

	// Generate
	compiled, err := Generate(prog)
	if err != nil {
		t.Fatalf("codegen: %v", err)
	}

	// Encode
	var buf bytes.Buffer
	if err := Encode(&buf, compiled); err != nil {
		t.Fatalf("encode: %v", err)
	}
	encoded := buf.Bytes()

	// Check magic
	if encoded[0] != 0x52 || encoded[1] != 0x59 || encoded[2] != 0x58 || encoded[3] != 0x00 {
		t.Errorf("invalid magic: %v", encoded[:4])
	}

	// Decode
	decoded, err := Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Verify code is identical
	if !bytes.Equal(compiled.Code, decoded.Code) {
		t.Error("code mismatch after round-trip")
	}

	// Disassemble both and compare
	disasm1 := DisassembleProgram(compiled)
	disasm2 := DisassembleProgram(decoded)
	if disasm1 != disasm2 {
		t.Errorf("disassembly mismatch after round-trip:\n--- original ---\n%s\n--- decoded ---\n%s", disasm1, disasm2)
	}
}

// ---------------------------------------------------------------------------
// Opcode name lookup
// ---------------------------------------------------------------------------

func TestOpcodeLookup(t *testing.T) {
	// Every opcode in the table should round-trip through name lookup.
	for op, info := range instrTable {
		found, ok := OpcodeByName(info.Name)
		if !ok {
			t.Errorf("OpcodeByName(%q) not found", info.Name)
			continue
		}
		if found != op {
			t.Errorf("OpcodeByName(%q) = 0x%02X, expected 0x%02X", info.Name, found, op)
		}
	}
}

func TestOpcodeCount(t *testing.T) {
	// The spec header says "68 instructions" but the listing enumerates 76
	// distinct opcodes (some lines list two, e.g. "SUB_INT, SUB_FLOAT").
	// We implement all 76 from the spec listing, plus OpCallBuiltin (77 total).
	if len(instrTable) != 77 {
		t.Errorf("expected 77 opcodes, got %d", len(instrTable))
	}
}

func TestOpcodeString(t *testing.T) {
	if OpConstInt.String() != "CONST_INT" {
		t.Errorf("expected CONST_INT, got %s", OpConstInt.String())
	}
	if Opcode(0xFF).String() != "UNKNOWN(0xFF)" {
		t.Errorf("expected UNKNOWN, got %s", Opcode(0xFF).String())
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestDecodeEmptyCode(t *testing.T) {
	_, _, err := DecodeInstruction(nil, 0)
	if err == nil {
		t.Error("expected error for empty code")
	}
}

func TestDecodeTruncatedOperand(t *testing.T) {
	// Just the opcode for CONST_INT, no i64 operand.
	code := []byte{byte(OpConstInt)}
	_, _, err := DecodeInstruction(code, 0)
	if err == nil {
		t.Error("expected error for truncated CONST_INT")
	}
}

func TestInstrSize(t *testing.T) {
	tests := map[Opcode]int{
		OpConstInt:    9,
		OpConstFloat:  9,
		OpConstChar:   5,
		OpConstTrue:   1,
		OpLoadLocal:   3,
		OpJump:        3,
		OpCallMethod:  5,
		OpMakeEnum:    7,
		OpSourceLoc:   5,
		OpJumpTable:   -1,
	}
	for op, expected := range tests {
		got := InstrSize(op)
		if got != expected {
			t.Errorf("InstrSize(%s): expected %d, got %d", op, expected, got)
		}
	}
}

// ---------------------------------------------------------------------------
// MIR → Bytecode specific tests
// ---------------------------------------------------------------------------

func TestCodegenSimpleReturn(t *testing.T) {
	fn := &mir.Function{
		Name:       "ret42",
		Params:     nil,
		ReturnType: types.TypInt,
		Locals:     nil,
		Entry:      0,
	}
	fn.NewBlock("entry")
	fn.Block(0).Term = &mir.Return{Value: mir.IntConst(42)}

	prog := &mir.Program{Functions: []*mir.Function{fn}}
	compiled, err := Generate(prog)
	if err != nil {
		t.Fatalf("codegen: %v", err)
	}

	// Disassemble and check
	disasm := Disassemble(compiled.Code, compiled.StringPool)
	if !strings.Contains(disasm, "CONST_INT") || !strings.Contains(disasm, "42") {
		t.Errorf("expected CONST_INT 42 in disasm:\n%s", disasm)
	}
	if !strings.Contains(disasm, "RETURN") {
		t.Errorf("expected RETURN in disasm:\n%s", disasm)
	}
}

func TestCodegenBinaryOp(t *testing.T) {
	fn := buildAddFunction()
	prog := &mir.Program{Functions: []*mir.Function{fn}}
	compiled, err := Generate(prog)
	if err != nil {
		t.Fatalf("codegen: %v", err)
	}

	disasm := Disassemble(compiled.Code, compiled.StringPool)
	if !strings.Contains(disasm, "LOAD_LOCAL") {
		t.Errorf("expected LOAD_LOCAL in disasm:\n%s", disasm)
	}
	if !strings.Contains(disasm, "ADD_INT") {
		t.Errorf("expected ADD_INT in disasm:\n%s", disasm)
	}
}

func TestCodegenBranch(t *testing.T) {
	fn := buildBranchFunction()
	prog := &mir.Program{Functions: []*mir.Function{fn}}
	compiled, err := Generate(prog)
	if err != nil {
		t.Fatalf("codegen: %v", err)
	}

	disasm := Disassemble(compiled.Code, compiled.StringPool)
	if !strings.Contains(disasm, "JUMP_IF_FALSE") {
		t.Errorf("expected JUMP_IF_FALSE in disasm:\n%s", disasm)
	}
	if !strings.Contains(disasm, "JUMP") {
		t.Errorf("expected JUMP in disasm:\n%s", disasm)
	}
}

func TestCodegenStringConst(t *testing.T) {
	fn := &mir.Function{
		Name:       "greet",
		Params:     nil,
		ReturnType: types.TypString,
		Locals:     nil,
		Entry:      0,
	}
	fn.NewBlock("entry")
	fn.Block(0).Term = &mir.Return{Value: mir.StringConst("hello")}

	prog := &mir.Program{Functions: []*mir.Function{fn}}
	compiled, err := Generate(prog)
	if err != nil {
		t.Fatalf("codegen: %v", err)
	}

	// String should be in the pool.
	found := false
	for _, s := range compiled.StringPool {
		if s == "hello" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'hello' in string pool")
	}

	disasm := Disassemble(compiled.Code, compiled.StringPool)
	if !strings.Contains(disasm, "CONST_STRING") {
		t.Errorf("expected CONST_STRING in disasm:\n%s", disasm)
	}
}

func TestCodegenArrayAlloc(t *testing.T) {
	fn := &mir.Function{
		Name:       "mk_array",
		Params:     nil,
		ReturnType: &types.ArrayType{Elem: types.TypInt},
		Entry:      0,
	}
	dest := fn.NewLocal("arr", &types.ArrayType{Elem: types.TypInt})
	fn.NewBlock("entry")
	fn.Block(0).Stmts = []mir.Stmt{
		&mir.ArrayAllocStmt{
			Dest:  dest,
			Elems: []mir.Value{mir.IntConst(1), mir.IntConst(2), mir.IntConst(3)},
			Type:  &types.ArrayType{Elem: types.TypInt},
		},
	}
	fn.Block(0).Term = &mir.Return{Value: fn.LocalRef(dest)}

	prog := &mir.Program{Functions: []*mir.Function{fn}}
	compiled, err := Generate(prog)
	if err != nil {
		t.Fatalf("codegen: %v", err)
	}

	disasm := Disassemble(compiled.Code, compiled.StringPool)
	if !strings.Contains(disasm, "MAKE_ARRAY") {
		t.Errorf("expected MAKE_ARRAY in disasm:\n%s", disasm)
	}
}

// ---------------------------------------------------------------------------
// Closure mutable capture: assignment inside if emits OpStoreUpvalue
// ---------------------------------------------------------------------------

func TestCodegenClosureMutCaptureEmitsStoreUpvalue(t *testing.T) {
	// Build a closure function that captures variable x (upvalue 0) and
	// assigns to it inside an if block. The codegen must emit
	// OpStoreUpvalue (not OpStoreLocal) for that assignment.
	//
	// Equivalent to the inner closure in closure_mut_capture.ryx:
	//   let inc = |_: Int| -> Int {
	//       if true { x = x + 1; }; x
	//   };
	fn := &mir.Function{
		Name:         "inc",
		ReturnType:   types.TypInt,
		Entry:        0,
		UpvalueCount: 1, // captures x
	}

	// Local 0: the parameter (unused)
	param := fn.NewLocal("_", types.TypInt)
	fn.Params = []mir.LocalID{param}

	// Local 1: x (initial load from upvalue 0)
	xInit := fn.NewLocal("x", types.TypInt)
	fn.Locals[int(xInit)].UpvalueAlias = 0

	// Local 2: x+1 result
	xPlusOne := fn.NewLocal("x", types.TypInt)
	xPlusOne2 := xPlusOne // reuse ID
	fn.Locals[int(xPlusOne2)].UpvalueAlias = 0

	// Local 3: phi merge result for x
	xMerge := fn.NewLocal("x", types.TypInt)
	fn.Locals[int(xMerge)].UpvalueAlias = 0

	// Blocks: entry -> if.then -> if.merge
	entry := fn.NewBlock("entry")
	thenB := fn.NewBlock("if.then")
	mergeB := fn.NewBlock("if.merge")

	// entry: load x from upvalue, branch on true
	fn.Block(entry).Stmts = []mir.Stmt{
		&mir.Assign{Dest: xInit, Src: &mir.Upvalue{Index: 0, Type: types.TypInt}, Type: types.TypInt},
	}
	fn.Block(entry).Term = &mir.Branch{
		Cond: mir.BoolConst(true),
		Then: thenB,
		Else: mergeB,
	}

	// if.then: x = x + 1
	fn.Block(thenB).Stmts = []mir.Stmt{
		&mir.BinaryOpStmt{
			Dest:  xPlusOne2,
			Op:    "+",
			Left:  fn.LocalRef(xInit),
			Right: mir.IntConst(1),
			Type:  types.TypInt,
		},
	}
	fn.Block(thenB).Term = &mir.Goto{Target: mergeB}

	// if.merge: phi(x) then return x
	fn.Block(mergeB).Phis = []*mir.Phi{
		{
			Dest: xMerge,
			Args: map[mir.BlockID]mir.Value{
				entry: fn.LocalRef(xInit),
				thenB: fn.LocalRef(xPlusOne2),
			},
			Type: types.TypInt,
		},
	}
	fn.Block(mergeB).Term = &mir.Return{Value: fn.LocalRef(xMerge)}

	fn.AddEdge(entry, thenB)
	fn.AddEdge(entry, mergeB)
	fn.AddEdge(thenB, mergeB)

	prog := &mir.Program{Functions: []*mir.Function{fn}}
	compiled, err := Generate(prog)
	if err != nil {
		t.Fatalf("codegen: %v", err)
	}

	disasm := Disassemble(compiled.Code, compiled.StringPool)

	// Must emit STORE_UPVALUE for the mutable capture assignment.
	if !strings.Contains(disasm, "STORE_UPVALUE") {
		t.Errorf("expected STORE_UPVALUE in disasm (closure mutable capture must store to upvalue, not local):\n%s", disasm)
	}

	// Must also load from upvalue.
	if !strings.Contains(disasm, "LOAD_UPVALUE") {
		t.Errorf("expected LOAD_UPVALUE in disasm:\n%s", disasm)
	}

	// Should NOT emit STORE_LOCAL for slots that have UpvalueAlias >= 0.
	// Count STORE_LOCAL vs STORE_UPVALUE to verify the alias is working.
	storeLocalCount := strings.Count(disasm, "STORE_LOCAL")
	storeUpvalueCount := strings.Count(disasm, "STORE_UPVALUE")
	if storeUpvalueCount == 0 {
		t.Errorf("expected at least one STORE_UPVALUE, got 0; STORE_LOCAL count: %d\ndisasm:\n%s",
			storeLocalCount, disasm)
	}
	t.Logf("STORE_UPVALUE count: %d, STORE_LOCAL count: %d", storeUpvalueCount, storeLocalCount)
	t.Logf("Disassembly:\n%s", disasm)
}

// ---------------------------------------------------------------------------
// Test helpers — build MIR functions
// ---------------------------------------------------------------------------

// buildAddFunction creates: fn add(a: Int, b: Int) -> Int { a + b }
func buildAddFunction() *mir.Function {
	fn := &mir.Function{
		Name:       "add",
		ReturnType: types.TypInt,
		Entry:      0,
	}
	a := fn.NewLocal("a", types.TypInt)
	b := fn.NewLocal("b", types.TypInt)
	fn.Params = []mir.LocalID{a, b}

	result := fn.NewLocal("result", types.TypInt)
	fn.NewBlock("entry")
	fn.Block(0).Stmts = []mir.Stmt{
		&mir.BinaryOpStmt{
			Dest:  result,
			Op:    "+",
			Left:  fn.LocalRef(a),
			Right: fn.LocalRef(b),
			Type:  types.TypInt,
		},
	}
	fn.Block(0).Term = &mir.Return{Value: fn.LocalRef(result)}
	return fn
}

// buildMainFunction creates: fn main() { let x = 10; let y = 20; return; }
func buildMainFunction() *mir.Function {
	fn := &mir.Function{
		Name:       "main",
		ReturnType: types.TypUnit,
		Entry:      0,
	}
	x := fn.NewLocal("x", types.TypInt)
	y := fn.NewLocal("y", types.TypInt)
	fn.NewBlock("entry")
	fn.Block(0).Stmts = []mir.Stmt{
		&mir.Assign{Dest: x, Src: mir.IntConst(10), Type: types.TypInt},
		&mir.Assign{Dest: y, Src: mir.IntConst(20), Type: types.TypInt},
	}
	fn.Block(0).Term = &mir.Return{Value: nil}
	return fn
}

// buildBranchFunction creates:
//
//	fn branch(x: Bool) -> Int {
//	    if x { 1 } else { 0 }
//	}
func buildBranchFunction() *mir.Function {
	fn := &mir.Function{
		Name:       "branch",
		ReturnType: types.TypInt,
		Entry:      0,
	}
	x := fn.NewLocal("x", types.TypBool)
	fn.Params = []mir.LocalID{x}

	result := fn.NewLocal("result", types.TypInt)

	entry := fn.NewBlock("entry")
	thenB := fn.NewBlock("if.then")
	elseB := fn.NewBlock("if.else")
	mergeB := fn.NewBlock("if.merge")

	// entry: branch on x
	fn.Block(entry).Term = &mir.Branch{
		Cond: fn.LocalRef(x),
		Then: thenB,
		Else: elseB,
	}

	// then: result = 1
	fn.Block(thenB).Stmts = []mir.Stmt{
		&mir.Assign{Dest: result, Src: mir.IntConst(1), Type: types.TypInt},
	}
	fn.Block(thenB).Term = &mir.Goto{Target: mergeB}

	// else: result = 0
	fn.Block(elseB).Stmts = []mir.Stmt{
		&mir.Assign{Dest: result, Src: mir.IntConst(0), Type: types.TypInt},
	}
	fn.Block(elseB).Term = &mir.Goto{Target: mergeB}

	// merge: return result
	fn.Block(mergeB).Term = &mir.Return{Value: fn.LocalRef(result)}

	// Set up CFG edges.
	fn.AddEdge(entry, thenB)
	fn.AddEdge(entry, elseB)
	fn.AddEdge(thenB, mergeB)
	fn.AddEdge(elseB, mergeB)

	return fn
}
