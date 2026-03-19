package codegen

import (
	"encoding/binary"
	"fmt"
	"math"
)

// ---------------------------------------------------------------------------
// Opcodes — 68 bytecode instructions
// ---------------------------------------------------------------------------

// Opcode is a single-byte instruction identifier.
type Opcode byte

const (
	// Stack operations
	OpConstInt    Opcode = 0x01 // operand: i64
	OpConstFloat  Opcode = 0x02 // operand: f64
	OpConstTrue   Opcode = 0x03
	OpConstFalse  Opcode = 0x04
	OpConstUnit   Opcode = 0x05
	OpConstString Opcode = 0x06 // operand: u16 idx
	OpConstChar   Opcode = 0x07 // operand: u32
	OpPop         Opcode = 0x08
	OpDup         Opcode = 0x09
	OpSwap        Opcode = 0x0A

	// Local variable access
	OpLoadLocal    Opcode = 0x10 // operand: u16 slot
	OpStoreLocal   Opcode = 0x11 // operand: u16 slot
	OpLoadUpvalue  Opcode = 0x12 // operand: u16 idx
	OpStoreUpvalue Opcode = 0x13 // operand: u16 idx

	// Global access
	OpLoadGlobal  Opcode = 0x14 // operand: u16 idx
	OpStoreGlobal Opcode = 0x15 // operand: u16 idx

	// Arithmetic
	OpAddInt    Opcode = 0x20
	OpAddFloat  Opcode = 0x21
	OpSubInt    Opcode = 0x22
	OpSubFloat  Opcode = 0x23
	OpMulInt    Opcode = 0x24
	OpMulFloat  Opcode = 0x25
	OpDivInt    Opcode = 0x26
	OpDivFloat  Opcode = 0x27
	OpModInt    Opcode = 0x28
	OpModFloat  Opcode = 0x29
	OpNegInt    Opcode = 0x2A
	OpNegFloat  Opcode = 0x2B

	// String
	OpConcatString Opcode = 0x2C

	// Comparison
	OpEq       Opcode = 0x30
	OpNeq      Opcode = 0x31
	OpLtInt    Opcode = 0x32
	OpLtFloat  Opcode = 0x33
	OpGtInt    Opcode = 0x34
	OpGtFloat  Opcode = 0x35
	OpLeqInt   Opcode = 0x36
	OpLeqFloat Opcode = 0x37
	OpGeqInt   Opcode = 0x38
	OpGeqFloat Opcode = 0x39

	// Logical
	OpNot Opcode = 0x3A

	// Control flow
	OpJump        Opcode = 0x40 // operand: i16 offset
	OpJumpIfTrue  Opcode = 0x41 // operand: i16 offset
	OpJumpIfFalse Opcode = 0x42 // operand: i16 offset
	OpJumpTable   Opcode = 0x43 // operand: u16 count + count × i16 offsets

	// Functions
	OpCall        Opcode = 0x50 // operand: u16 arg_count
	OpCallMethod  Opcode = 0x51 // operands: u16 name_idx, u16 arg_count
	OpTailCall    Opcode = 0x52 // operand: u16 arg_count
	OpReturn      Opcode = 0x53
	OpMakeClosure Opcode = 0x54 // operands: u16 fn_idx, u16 upvalue_count

	// Data structures
	OpMakeArray  Opcode = 0x60 // operand: u16 count
	OpMakeTuple  Opcode = 0x61 // operand: u16 count
	OpMakeStruct Opcode = 0x62 // operands: u16 type_idx, u16 field_count
	OpMakeEnum   Opcode = 0x63 // operands: u16 type_idx, u16 variant_idx, u16 field_count
	OpIndexGet   Opcode = 0x64
	OpIndexSet   Opcode = 0x65
	OpFieldGet   Opcode = 0x66 // operand: u16 field_idx
	OpFieldSet   Opcode = 0x67 // operand: u16 field_idx

	// Heap / GC
	OpAllocArray   Opcode = 0x70 // operand: u16 count
	OpAllocClosure Opcode = 0x71

	// Pattern matching
	OpTagCheck    Opcode = 0x80 // operand: u16 variant_idx
	OpDestructure Opcode = 0x81 // operand: u16 count

	// Concurrency
	OpChannelCreate Opcode = 0x90 // operand: u16 cap
	OpChannelSend   Opcode = 0x91
	OpChannelRecv   Opcode = 0x92
	OpChannelClose  Opcode = 0x93
	OpSpawn         Opcode = 0x94 // operand: u16 fn_idx

	// Built-ins
	OpPrint         Opcode = 0xA0
	OpPrintln       Opcode = 0xA1
	OpIntToFloat    Opcode = 0xA2
	OpFloatToInt    Opcode = 0xA3
	OpIntToString   Opcode = 0xA4
	OpFloatToString Opcode = 0xA5
	OpStringLen     Opcode = 0xA6
	OpArrayLen      Opcode = 0xA7
	OpCallBuiltin   Opcode = 0xA8 // operands: u16 name_idx, u16 arg_count

	// Debug
	OpBreakpoint Opcode = 0xF0
	OpSourceLoc  Opcode = 0xF1 // operands: u16 line, u16 col (spec says u32 line, u16 col but we use u16 for consistency in instruction encoding)
)

// ---------------------------------------------------------------------------
// Opcode metadata
// ---------------------------------------------------------------------------

// OperandKind describes how to decode an instruction's operands.
type OperandKind int

const (
	OpNone      OperandKind = iota // no operands
	OpI64                          // i64 immediate
	OpF64                          // f64 immediate
	OpU32                          // u32 immediate (char)
	OpU16                          // single u16
	OpI16                          // single i16 (jump offset)
	OpU16U16                       // two u16 values
	OpU16U16U16                    // three u16 values
	OpJumpTab                      // u16 count + count × i16
	OpU16U16Src                    // source loc: u16 line, u16 col (same encoding as U16U16 but semantic difference)
)

// InstrInfo holds metadata about an opcode.
type InstrInfo struct {
	Name     string
	Operands OperandKind
}

// instrTable maps every opcode to its metadata.
var instrTable = map[Opcode]InstrInfo{
	// Stack operations
	OpConstInt:    {"CONST_INT", OpI64},
	OpConstFloat:  {"CONST_FLOAT", OpF64},
	OpConstTrue:   {"CONST_TRUE", OpNone},
	OpConstFalse:  {"CONST_FALSE", OpNone},
	OpConstUnit:   {"CONST_UNIT", OpNone},
	OpConstString: {"CONST_STRING", OpU16},
	OpConstChar:   {"CONST_CHAR", OpU32},
	OpPop:         {"POP", OpNone},
	OpDup:         {"DUP", OpNone},
	OpSwap:        {"SWAP", OpNone},

	// Local/Upvalue/Global
	OpLoadLocal:    {"LOAD_LOCAL", OpU16},
	OpStoreLocal:   {"STORE_LOCAL", OpU16},
	OpLoadUpvalue:  {"LOAD_UPVALUE", OpU16},
	OpStoreUpvalue: {"STORE_UPVALUE", OpU16},
	OpLoadGlobal:   {"LOAD_GLOBAL", OpU16},
	OpStoreGlobal:  {"STORE_GLOBAL", OpU16},

	// Arithmetic
	OpAddInt:    {"ADD_INT", OpNone},
	OpAddFloat:  {"ADD_FLOAT", OpNone},
	OpSubInt:    {"SUB_INT", OpNone},
	OpSubFloat:  {"SUB_FLOAT", OpNone},
	OpMulInt:    {"MUL_INT", OpNone},
	OpMulFloat:  {"MUL_FLOAT", OpNone},
	OpDivInt:    {"DIV_INT", OpNone},
	OpDivFloat:  {"DIV_FLOAT", OpNone},
	OpModInt:    {"MOD_INT", OpNone},
	OpModFloat:  {"MOD_FLOAT", OpNone},
	OpNegInt:    {"NEG_INT", OpNone},
	OpNegFloat:  {"NEG_FLOAT", OpNone},
	OpConcatString: {"CONCAT_STRING", OpNone},

	// Comparison
	OpEq:       {"EQ", OpNone},
	OpNeq:      {"NEQ", OpNone},
	OpLtInt:    {"LT_INT", OpNone},
	OpLtFloat:  {"LT_FLOAT", OpNone},
	OpGtInt:    {"GT_INT", OpNone},
	OpGtFloat:  {"GT_FLOAT", OpNone},
	OpLeqInt:   {"LEQ_INT", OpNone},
	OpLeqFloat: {"LEQ_FLOAT", OpNone},
	OpGeqInt:   {"GEQ_INT", OpNone},
	OpGeqFloat: {"GEQ_FLOAT", OpNone},

	// Logical
	OpNot: {"NOT", OpNone},

	// Control flow
	OpJump:        {"JUMP", OpI16},
	OpJumpIfTrue:  {"JUMP_IF_TRUE", OpI16},
	OpJumpIfFalse: {"JUMP_IF_FALSE", OpI16},
	OpJumpTable:   {"JUMP_TABLE", OpJumpTab},

	// Functions
	OpCall:        {"CALL", OpU16},
	OpCallMethod:  {"CALL_METHOD", OpU16U16},
	OpTailCall:    {"TAIL_CALL", OpU16},
	OpReturn:      {"RETURN", OpNone},
	OpMakeClosure: {"MAKE_CLOSURE", OpU16U16},

	// Data structures
	OpMakeArray:  {"MAKE_ARRAY", OpU16},
	OpMakeTuple:  {"MAKE_TUPLE", OpU16},
	OpMakeStruct: {"MAKE_STRUCT", OpU16U16},
	OpMakeEnum:   {"MAKE_ENUM", OpU16U16U16},
	OpIndexGet:   {"INDEX_GET", OpNone},
	OpIndexSet:   {"INDEX_SET", OpNone},
	OpFieldGet:   {"FIELD_GET", OpU16},
	OpFieldSet:   {"FIELD_SET", OpU16},

	// Heap/GC
	OpAllocArray:   {"ALLOC_ARRAY", OpU16},
	OpAllocClosure: {"ALLOC_CLOSURE", OpNone},

	// Pattern matching
	OpTagCheck:    {"TAG_CHECK", OpU16},
	OpDestructure: {"DESTRUCTURE", OpU16},

	// Concurrency
	OpChannelCreate: {"CHANNEL_CREATE", OpU16},
	OpChannelSend:   {"CHANNEL_SEND", OpNone},
	OpChannelRecv:   {"CHANNEL_RECV", OpNone},
	OpChannelClose:  {"CHANNEL_CLOSE", OpNone},
	OpSpawn:         {"SPAWN", OpU16},

	// Built-ins
	OpPrint:         {"PRINT", OpNone},
	OpPrintln:       {"PRINTLN", OpNone},
	OpIntToFloat:    {"INT_TO_FLOAT", OpNone},
	OpFloatToInt:    {"FLOAT_TO_INT", OpNone},
	OpIntToString:   {"INT_TO_STRING", OpNone},
	OpFloatToString: {"FLOAT_TO_STRING", OpNone},
	OpStringLen:     {"STRING_LEN", OpNone},
	OpArrayLen:      {"ARRAY_LEN", OpNone},
	OpCallBuiltin:   {"CALL_BUILTIN", OpU16U16},

	// Debug
	OpBreakpoint: {"BREAKPOINT", OpNone},
	OpSourceLoc:  {"SOURCE_LOC", OpU16U16Src},
}

// LookupOpcode returns the InstrInfo for a given opcode, or false if unknown.
func LookupOpcode(op Opcode) (InstrInfo, bool) {
	info, ok := instrTable[op]
	return info, ok
}

// OpcodeByName returns the opcode for a given instruction name, or false.
func OpcodeByName(name string) (Opcode, bool) {
	for op, info := range instrTable {
		if info.Name == name {
			return op, true
		}
	}
	return 0, false
}

func (op Opcode) String() string {
	if info, ok := instrTable[op]; ok {
		return info.Name
	}
	return fmt.Sprintf("UNKNOWN(0x%02X)", byte(op))
}

// ---------------------------------------------------------------------------
// Instruction — decoded representation
// ---------------------------------------------------------------------------

// Instruction is a decoded bytecode instruction with its operands.
type Instruction struct {
	Op       Opcode
	Operands []int64 // decoded operand values (widths depend on OperandKind)
}

// InstrSize returns the byte size of an encoded instruction (opcode + operands).
func InstrSize(op Opcode) int {
	info, ok := instrTable[op]
	if !ok {
		return 1
	}
	switch info.Operands {
	case OpNone:
		return 1
	case OpI64, OpF64:
		return 1 + 8
	case OpU32:
		return 1 + 4
	case OpU16, OpI16:
		return 1 + 2
	case OpU16U16, OpU16U16Src:
		return 1 + 4
	case OpU16U16U16:
		return 1 + 6
	case OpJumpTab:
		return -1 // variable length; caller must inspect operands
	default:
		return 1
	}
}

// ---------------------------------------------------------------------------
// Instruction encoding helpers
// ---------------------------------------------------------------------------

// EmitOp writes a no-operand instruction.
func EmitOp(buf *[]byte, op Opcode) {
	*buf = append(*buf, byte(op))
}

// EmitI64 writes an opcode followed by an i64 operand (little-endian).
func EmitI64(buf *[]byte, op Opcode, val int64) {
	*buf = append(*buf, byte(op))
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(val))
	*buf = append(*buf, b[:]...)
}

// EmitF64 writes an opcode followed by an f64 operand (little-endian).
func EmitF64(buf *[]byte, op Opcode, val float64) {
	*buf = append(*buf, byte(op))
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(val))
	*buf = append(*buf, b[:]...)
}

// EmitU32 writes an opcode followed by a u32 operand (little-endian).
func EmitU32(buf *[]byte, op Opcode, val uint32) {
	*buf = append(*buf, byte(op))
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], val)
	*buf = append(*buf, b[:]...)
}

// EmitU16 writes an opcode followed by a single u16 operand.
func EmitU16(buf *[]byte, op Opcode, val uint16) {
	*buf = append(*buf, byte(op))
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], val)
	*buf = append(*buf, b[:]...)
}

// EmitI16 writes an opcode followed by a single i16 operand.
func EmitI16(buf *[]byte, op Opcode, val int16) {
	*buf = append(*buf, byte(op))
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], uint16(val))
	*buf = append(*buf, b[:]...)
}

// EmitU16U16 writes an opcode followed by two u16 operands.
func EmitU16U16(buf *[]byte, op Opcode, a, b_ uint16) {
	*buf = append(*buf, byte(op))
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], a)
	*buf = append(*buf, b[:]...)
	binary.LittleEndian.PutUint16(b[:], b_)
	*buf = append(*buf, b[:]...)
}

// EmitU16U16U16 writes an opcode followed by three u16 operands.
func EmitU16U16U16(buf *[]byte, op Opcode, a, b_, c uint16) {
	*buf = append(*buf, byte(op))
	var bx [2]byte
	binary.LittleEndian.PutUint16(bx[:], a)
	*buf = append(*buf, bx[:]...)
	binary.LittleEndian.PutUint16(bx[:], b_)
	*buf = append(*buf, bx[:]...)
	binary.LittleEndian.PutUint16(bx[:], c)
	*buf = append(*buf, bx[:]...)
}

// EmitJumpTable writes JUMP_TABLE: opcode, u16 count, then count × i16 offsets.
func EmitJumpTable(buf *[]byte, offsets []int16) {
	*buf = append(*buf, byte(OpJumpTable))
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], uint16(len(offsets)))
	*buf = append(*buf, b[:]...)
	for _, off := range offsets {
		binary.LittleEndian.PutUint16(b[:], uint16(off))
		*buf = append(*buf, b[:]...)
	}
}

// ---------------------------------------------------------------------------
// Instruction decoding
// ---------------------------------------------------------------------------

// DecodeInstruction decodes one instruction from code at the given offset.
// Returns the instruction and the number of bytes consumed.
func DecodeInstruction(code []byte, offset int) (Instruction, int, error) {
	if offset >= len(code) {
		return Instruction{}, 0, fmt.Errorf("offset %d beyond code length %d", offset, len(code))
	}
	op := Opcode(code[offset])
	info, ok := instrTable[op]
	if !ok {
		return Instruction{Op: op}, 1, nil
	}

	pos := offset + 1
	var operands []int64

	switch info.Operands {
	case OpNone:
		// no operands

	case OpI64:
		if pos+8 > len(code) {
			return Instruction{}, 0, fmt.Errorf("truncated i64 at offset %d", offset)
		}
		v := int64(binary.LittleEndian.Uint64(code[pos : pos+8]))
		operands = append(operands, v)
		pos += 8

	case OpF64:
		if pos+8 > len(code) {
			return Instruction{}, 0, fmt.Errorf("truncated f64 at offset %d", offset)
		}
		bits := binary.LittleEndian.Uint64(code[pos : pos+8])
		operands = append(operands, int64(bits))
		pos += 8

	case OpU32:
		if pos+4 > len(code) {
			return Instruction{}, 0, fmt.Errorf("truncated u32 at offset %d", offset)
		}
		v := binary.LittleEndian.Uint32(code[pos : pos+4])
		operands = append(operands, int64(v))
		pos += 4

	case OpU16:
		if pos+2 > len(code) {
			return Instruction{}, 0, fmt.Errorf("truncated u16 at offset %d", offset)
		}
		v := binary.LittleEndian.Uint16(code[pos : pos+2])
		operands = append(operands, int64(v))
		pos += 2

	case OpI16:
		if pos+2 > len(code) {
			return Instruction{}, 0, fmt.Errorf("truncated i16 at offset %d", offset)
		}
		v := int16(binary.LittleEndian.Uint16(code[pos : pos+2]))
		operands = append(operands, int64(v))
		pos += 2

	case OpU16U16, OpU16U16Src:
		if pos+4 > len(code) {
			return Instruction{}, 0, fmt.Errorf("truncated u16u16 at offset %d", offset)
		}
		a := binary.LittleEndian.Uint16(code[pos : pos+2])
		b := binary.LittleEndian.Uint16(code[pos+2 : pos+4])
		operands = append(operands, int64(a), int64(b))
		pos += 4

	case OpU16U16U16:
		if pos+6 > len(code) {
			return Instruction{}, 0, fmt.Errorf("truncated u16u16u16 at offset %d", offset)
		}
		a := binary.LittleEndian.Uint16(code[pos : pos+2])
		b := binary.LittleEndian.Uint16(code[pos+2 : pos+4])
		c := binary.LittleEndian.Uint16(code[pos+4 : pos+6])
		operands = append(operands, int64(a), int64(b), int64(c))
		pos += 6

	case OpJumpTab:
		if pos+2 > len(code) {
			return Instruction{}, 0, fmt.Errorf("truncated jump table count at offset %d", offset)
		}
		count := binary.LittleEndian.Uint16(code[pos : pos+2])
		operands = append(operands, int64(count))
		pos += 2
		if pos+int(count)*2 > len(code) {
			return Instruction{}, 0, fmt.Errorf("truncated jump table at offset %d", offset)
		}
		for i := 0; i < int(count); i++ {
			off := int16(binary.LittleEndian.Uint16(code[pos : pos+2]))
			operands = append(operands, int64(off))
			pos += 2
		}
	}

	return Instruction{Op: op, Operands: operands}, pos - offset, nil
}

// OperandF64 extracts the float64 from an OpConstFloat instruction's operand.
func OperandF64(instr Instruction) float64 {
	if len(instr.Operands) == 0 {
		return 0
	}
	return math.Float64frombits(uint64(instr.Operands[0]))
}
