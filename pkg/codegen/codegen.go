package codegen

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/ryx-lang/ryx/pkg/mir"
	"github.com/ryx-lang/ryx/pkg/types"
)

// ---------------------------------------------------------------------------
// Compiled output
// ---------------------------------------------------------------------------

// CompiledProgram is the output of code generation, ready for binary encoding.
type CompiledProgram struct {
	StringPool []string          // deduplicated string constants
	TypePool   []TypeDescriptor  // type descriptors
	Functions  []CompiledFunc    // compiled function table
	Code       []byte            // concatenated bytecode for all functions
	SourceMap  []SourceMapEntry  // source location annotations
	MainIndex  uint32            // index of "main" function in Functions
}

// CompiledFunc holds metadata for one function in the function table.
type CompiledFunc struct {
	NameIdx        uint32
	Arity          uint16
	LocalsCount    uint16
	UpvalueCount   uint16
	MaxStack       uint16
	CodeOffset     uint32
	CodeLength     uint32
	SourceMapOffset uint32
}

// TypeDescriptor encodes a type in the type pool.
type TypeDescriptor struct {
	Tag     TypeTag
	NameIdx uint32   // for struct/enum: string pool index
	Fields  []uint32 // field type indices or field name indices
}

// TypeTag discriminates types in the type pool.
type TypeTag byte

const (
	TypeTagInt     TypeTag = 0x01
	TypeTagFloat   TypeTag = 0x02
	TypeTagBool    TypeTag = 0x03
	TypeTagChar    TypeTag = 0x04
	TypeTagString  TypeTag = 0x05
	TypeTagUnit    TypeTag = 0x06
	TypeTagArray   TypeTag = 0x07
	TypeTagTuple   TypeTag = 0x08
	TypeTagFn      TypeTag = 0x09
	TypeTagStruct  TypeTag = 0x0A
	TypeTagEnum    TypeTag = 0x0B
	TypeTagChannel TypeTag = 0x0C
)

// SourceMapEntry maps a bytecode offset to a source line/column.
type SourceMapEntry struct {
	BytecodeOffset uint32
	Line           uint32
	Col            uint16
}

// ---------------------------------------------------------------------------
// Code generator
// ---------------------------------------------------------------------------

type generator struct {
	prog *mir.Program

	// Output accumulators
	code      []byte
	sourceMap []SourceMapEntry

	// String pool with deduplication
	strings    []string
	stringIdx  map[string]uint16

	// Type pool
	typePool []TypeDescriptor

	// Function table
	funcs []CompiledFunc

	// Function name -> index in Functions table
	funcIndex map[string]uint16

	// Per-function state (reset for each function)
	curFunc      *mir.Function
	localSlots   map[mir.LocalID]uint16 // MIR local -> bytecode local slot
	blockOffsets map[mir.BlockID]int     // block -> code offset (filled during emission)
	patches      []jumpPatch            // forward jumps to patch
	stackDepth   int                    // current simulated stack depth
	maxStack     int                    // max stack depth seen
}

type jumpPatch struct {
	instrOffset int        // offset of the i16 operand in code
	targetBlock mir.BlockID
}

// builtinOpcodes maps built-in function names to their corresponding opcodes.
// Calls to these functions are emitted as single opcodes rather than OpCall.
var builtinOpcodes = map[string]Opcode{
	"println":         OpPrintln,
	"print":           OpPrint,
	"int_to_float":    OpIntToFloat,
	"float_to_int":    OpFloatToInt,
	"int_to_string":   OpIntToString,
	"float_to_string": OpFloatToString,
	"string_len":      OpStringLen,
	"array_len":       OpArrayLen,
	"String::concat":  OpConcatString, // [CLAUDE-FIX] String concat desugared from ++ operator
}

// Generate compiles a MIR program to bytecode.
func Generate(prog *mir.Program) (*CompiledProgram, error) {
	g := &generator{
		prog:      prog,
		strings:   nil,
		stringIdx: make(map[string]uint16),
		funcIndex: make(map[string]uint16),
	}

	// Register all function names in the function table first (for forward references).
	for i, fn := range prog.Functions {
		g.funcIndex[fn.Name] = uint16(i)
		g.internString(fn.Name)
	}

	// Register struct/enum type names.
	for _, sd := range prog.Structs {
		g.internString(sd.Name)
		for _, f := range sd.Fields {
			g.internString(f.Name)
		}
	}
	for _, ed := range prog.Enums {
		g.internString(ed.Name)
		for _, v := range ed.Variants {
			g.internString(v.Name)
		}
	}

	// Compile each function.
	for _, fn := range prog.Functions {
		if err := g.compileFunction(fn); err != nil {
			return nil, fmt.Errorf("codegen: function %s: %w", fn.Name, err)
		}
	}

	// Find main function index.
	mainIdx, ok := g.funcIndex["main"]
	if !ok {
		// If no main, default to 0.
		mainIdx = 0
	}

	return &CompiledProgram{
		StringPool: g.strings,
		TypePool:   g.typePool,
		Functions:  g.funcs,
		Code:       g.code,
		SourceMap:  g.sourceMap,
		MainIndex:  uint32(mainIdx),
	}, nil
}

// ---------------------------------------------------------------------------
// String pool
// ---------------------------------------------------------------------------

func (g *generator) internString(s string) uint16 {
	if idx, ok := g.stringIdx[s]; ok {
		return idx
	}
	idx := uint16(len(g.strings))
	g.strings = append(g.strings, s)
	g.stringIdx[s] = idx
	return idx
}

// ---------------------------------------------------------------------------
// Function compilation
// ---------------------------------------------------------------------------

func (g *generator) compileFunction(fn *mir.Function) error {
	g.curFunc = fn
	g.localSlots = make(map[mir.LocalID]uint16, len(fn.Locals))
	g.blockOffsets = make(map[mir.BlockID]int, len(fn.Blocks))
	g.patches = nil
	g.stackDepth = 0
	g.maxStack = 0

	// Assign local slots: parameters first, then remaining locals.
	slot := uint16(0)
	for _, pid := range fn.Params {
		g.localSlots[pid] = slot
		slot++
	}
	for _, ld := range fn.Locals {
		if _, ok := g.localSlots[ld.ID]; !ok {
			g.localSlots[ld.ID] = slot
			slot++
		}
	}

	codeStart := len(g.code)
	smStart := len(g.sourceMap)

	// Emit blocks in order (entry block first, then remaining).
	order := g.blockOrder(fn)
	for _, bid := range order {
		g.blockOffsets[bid] = len(g.code)
		bb := fn.Block(bid)
		if err := g.emitBlock(bb); err != nil {
			return err
		}
	}

	// Patch forward jumps.
	for _, p := range g.patches {
		targetOff, ok := g.blockOffsets[p.targetBlock]
		if !ok {
			return fmt.Errorf("unknown block %d", p.targetBlock)
		}
		// Relative offset from the instruction AFTER the jump operand.
		// The i16 operand is at p.instrOffset, and the next instruction is at p.instrOffset+2.
		rel := targetOff - (p.instrOffset + 2)
		if rel < math.MinInt16 || rel > math.MaxInt16 {
			return fmt.Errorf("jump offset %d out of i16 range", rel)
		}
		g.code[p.instrOffset] = byte(uint16(int16(rel)))
		g.code[p.instrOffset+1] = byte(uint16(int16(rel)) >> 8)
	}

	codeLen := len(g.code) - codeStart
	nameIdx := g.internString(fn.Name)

	cf := CompiledFunc{
		NameIdx:         uint32(nameIdx),
		Arity:           uint16(len(fn.Params)),
		LocalsCount:     slot,
		UpvalueCount:    uint16(fn.UpvalueCount),
		MaxStack:        uint16(g.maxStack),
		CodeOffset:      uint32(codeStart),
		CodeLength:      uint32(codeLen),
		SourceMapOffset: uint32(smStart),
	}
	g.funcs = append(g.funcs, cf)
	return nil
}

// blockOrder returns a linearized order of blocks, starting with the entry block.
func (g *generator) blockOrder(fn *mir.Function) []mir.BlockID {
	visited := make(map[mir.BlockID]bool, len(fn.Blocks))
	order := make([]mir.BlockID, 0, len(fn.Blocks))

	var walk func(bid mir.BlockID)
	walk = func(bid mir.BlockID) {
		if visited[bid] {
			return
		}
		visited[bid] = true
		order = append(order, bid)
		bb := fn.Block(bid)
		if bb.Term != nil {
			for _, succ := range bb.Term.Successors() {
				walk(succ)
			}
		}
	}
	walk(fn.Entry)

	// Include any unreachable blocks.
	for _, bb := range fn.Blocks {
		if !visited[bb.ID] {
			order = append(order, bb.ID)
			visited[bb.ID] = true
		}
	}
	return order
}

// ---------------------------------------------------------------------------
// Block emission
// ---------------------------------------------------------------------------

func (g *generator) emitBlock(bb *mir.BasicBlock) error {
	// Emit phi nodes: for each phi, the incoming values were already stored
	// by the predecessor's terminator. In a stack-based VM, we handle phis
	// by having predecessors store to the phi's destination local.
	// (Phi resolution is handled at branch sites in emitTerminator.)

	for _, stmt := range bb.Stmts {
		if err := g.emitStmt(stmt); err != nil {
			return err
		}
	}

	if bb.Term != nil {
		return g.emitTerminator(bb)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Statement emission
// ---------------------------------------------------------------------------

func (g *generator) emitStmt(stmt mir.Stmt) error {
	switch s := stmt.(type) {
	case *mir.Assign:
		g.emitValue(s.Src)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.BinaryOpStmt:
		g.emitValue(s.Left)
		g.emitValue(s.Right)
		// [CLAUDE-FIX] When result type is unresolved, infer from operand types
		opType := s.Type
		if !isFloatType(opType) && !isIntType(opType) && !isStringType(opType) {
			if isFloatType(s.Left.ValueType()) || isFloatType(s.Right.ValueType()) {
				opType = types.TypFloat
			}
		}
		g.emitBinaryOp(s.Op, opType)
		g.adjustStack(-1) // two consumed, one produced (net -1 from the two pushes)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.UnaryOpStmt:
		g.emitValue(s.Operand)
		g.emitUnaryOp(s.Op, s.Type)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.CallStmt:
		// Check if this is a call to a built-in function that maps to an opcode.
		if glob, ok := s.Func.(*mir.Global); ok {
			if op, isBuiltin := builtinOpcodes[glob.Name]; isBuiltin {
				for _, arg := range s.Args {
					g.emitValue(arg)
				}
				EmitOp(&g.code, op)
				// All builtins consume args and push one result (VM ensures this).
				g.adjustStack(-len(s.Args) + 1)
				if s.Dest != mir.NoLocal {
					g.emitStoreLocal(s.Dest)
				}
				break
			}
		}
		// Regular function call: push function reference, then args.
		g.emitValue(s.Func)
		for _, arg := range s.Args {
			g.emitValue(arg)
		}
		EmitU16(&g.code, OpCall, uint16(len(s.Args)))
		// Call consumes func + args, pushes result.
		g.adjustStack(-(len(s.Args) + 1) + 1)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.CastStmt:
		g.emitValue(s.Src)
		g.emitCast(s.Src.ValueType(), s.Target)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.FieldAccessStmt:
		g.emitValue(s.Object)
		fieldIdx := g.resolveFieldIndex(s.Object.ValueType(), s.Field)
		EmitU16(&g.code, OpFieldGet, fieldIdx)
		// Consumes struct, pushes field value (net 0 from the push).
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.IndexAccessStmt:
		g.emitValue(s.Object)
		g.emitValue(s.Index)
		EmitOp(&g.code, OpIndexGet)
		g.adjustStack(-1) // consumes object+index, pushes element
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.ArrayAllocStmt:
		for _, elem := range s.Elems {
			g.emitValue(elem)
		}
		EmitU16(&g.code, OpMakeArray, uint16(len(s.Elems)))
		g.adjustStack(-(len(s.Elems)) + 1)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.StructAllocStmt:
		for _, fv := range s.Fields {
			g.emitValue(fv.Value)
		}
		typeIdx := g.internString(s.Name)
		EmitU16U16(&g.code, OpMakeStruct, typeIdx, uint16(len(s.Fields)))
		g.adjustStack(-(len(s.Fields)) + 1)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.FieldSetStmt: // [CLAUDE-FIX]
		g.emitValue(s.Object)
		g.emitValue(s.Value)
		fieldIdx := g.resolveFieldIndex(s.Object.ValueType(), s.Field)
		EmitU16(&g.code, OpFieldSet, fieldIdx)
		g.adjustStack(-2)

	case *mir.IndexSetStmt: // [CLAUDE-FIX]
		g.emitValue(s.Object)
		g.emitValue(s.Index)
		g.emitValue(s.Value)
		EmitOp(&g.code, OpIndexSet)
		g.adjustStack(-3)

	case *mir.TagCheckStmt: // [CLAUDE-FIX] Emit tag check for match dispatch
		g.emitValue(s.Object)
		variantIdx := g.resolveVariantIndex(s.EnumName, s.Variant)
		EmitU16(&g.code, OpTagCheck, variantIdx)
		// TagCheck peeks the enum (leaves it) and pushes a bool: stack = [..., enum, bool]
		g.adjustStack(1)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest) // pop bool into dest: stack = [..., enum]
		}
		// Pop the enum value that TagCheck left via peek
		EmitOp(&g.code, OpPop)
		g.adjustStack(-1)

	case *mir.EnumAllocStmt:
		for _, arg := range s.Args {
			g.emitValue(arg)
		}
		typeIdx := g.internString(s.EnumName)
		variantIdx := g.resolveVariantIndex(s.EnumName, s.Variant)
		EmitU16U16U16(&g.code, OpMakeEnum, typeIdx, variantIdx, uint16(len(s.Args)))
		g.adjustStack(-(len(s.Args)) + 1)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.ClosureAllocStmt:
		for _, cap := range s.Captures {
			g.emitValue(cap)
		}
		fnIdx, ok := g.funcIndex[s.FuncName]
		if !ok {
			return fmt.Errorf("unknown function %s in closure", s.FuncName)
		}
		EmitU16U16(&g.code, OpMakeClosure, fnIdx, uint16(len(s.Captures)))
		g.adjustStack(-(len(s.Captures)) + 1)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.ChannelCreateStmt:
		if s.BufSize != nil {
			g.emitValue(s.BufSize)
		} else {
			EmitI64(&g.code, OpConstInt, 0)
			g.adjustStack(1)
		}
		// BufSize is on stack; we convert to u16 operand.
		// Actually CHANNEL_CREATE takes a u16 operand. For dynamic buf sizes,
		// we'd need a different approach. For now, we handle const buf sizes.
		// Pop the buf size we pushed and use CHANNEL_CREATE with operand 0.
		EmitOp(&g.code, OpPop)
		g.adjustStack(-1)
		cap := uint16(0)
		if s.BufSize != nil {
			if c, ok := s.BufSize.(*mir.Const); ok && c.Kind == mir.ConstInt {
				cap = uint16(c.Int)
			}
		}
		EmitU16(&g.code, OpChannelCreate, cap)
		g.adjustStack(1)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.ChannelSendStmt:
		g.emitValue(s.Chan)
		g.emitValue(s.SendVal)
		EmitOp(&g.code, OpChannelSend)
		g.adjustStack(-2)

	case *mir.ChannelRecvStmt: // [CLAUDE-FIX]
		g.emitValue(s.Chan)
		EmitOp(&g.code, OpChannelRecv)
		// recv pops channel, pushes received value (net 0)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	case *mir.ChannelCloseStmt: // [CLAUDE-FIX]
		g.emitValue(s.Chan)
		EmitOp(&g.code, OpChannelClose)
		g.adjustStack(-1)

	case *mir.SpawnStmt:
		fnIdx := uint16(0)
		if gv, ok := s.Func.(*mir.Global); ok {
			if idx, found := g.funcIndex[gv.Name]; found {
				fnIdx = idx
			}
		}
		// Push args for the spawned function.
		for _, arg := range s.Args {
			g.emitValue(arg)
		}
		EmitU16(&g.code, OpSpawn, fnIdx)
		g.adjustStack(-(len(s.Args)) + 1)
		if s.Dest != mir.NoLocal {
			g.emitStoreLocal(s.Dest)
		}

	default:
		return fmt.Errorf("unsupported statement type %T", stmt)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Terminator emission
// ---------------------------------------------------------------------------

func (g *generator) emitTerminator(bb *mir.BasicBlock) error {
	switch t := bb.Term.(type) {
	case *mir.Goto:
		g.emitPhiMoves(bb.ID, t.Target)
		g.emitJump(OpJump, t.Target)

	case *mir.Branch:
		g.emitValue(t.Cond)
		// Strategy: JUMP_IF_FALSE to else phi moves, fall through to then.
		// We emit a raw placeholder (not block-targeted) so the false branch
		// lands on the inline else phi moves rather than skipping them.
		EmitI16(&g.code, OpJumpIfFalse, 0)
		elsePatchOff := len(g.code) - 2
		g.adjustStack(-1) // condition consumed
		g.emitPhiMoves(bb.ID, t.Then)
		g.emitJump(OpJump, t.Then)
		// Patch JUMP_IF_FALSE to land here (else phi moves).
		elseOff := len(g.code)
		rel := elseOff - (elsePatchOff + 2)
		g.code[elsePatchOff] = byte(uint16(int16(rel)))
		g.code[elsePatchOff+1] = byte(uint16(int16(rel)) >> 8)
		g.emitPhiMoves(bb.ID, t.Else)
		g.emitJump(OpJump, t.Else)

	case *mir.Switch:
		g.emitValue(t.Scrutinee)
		// Emit as a series of comparisons + conditional jumps.
		for _, sc := range t.Cases {
			EmitOp(&g.code, OpDup)
			g.adjustStack(1)
			g.emitValue(sc.Value)
			EmitOp(&g.code, OpEq)
			g.adjustStack(-1)
			caseJumpOff := g.emitJumpPlaceholder(OpJumpIfTrue, sc.Target)
			_ = caseJumpOff
			g.adjustStack(-1)
		}
		EmitOp(&g.code, OpPop)
		g.adjustStack(-1)
		g.emitPhiMoves(bb.ID, t.Default)
		g.emitJump(OpJump, t.Default)

	case *mir.Return:
		if t.Value != nil {
			g.emitValue(t.Value)
		} else {
			EmitOp(&g.code, OpConstUnit)
			g.adjustStack(1)
		}
		EmitOp(&g.code, OpReturn)
		g.adjustStack(-1)

	case *mir.Unreachable:
		// Emit a breakpoint as a trap for unreachable code.
		EmitOp(&g.code, OpBreakpoint)
	}
	return nil
}

// emitPhiMoves stores values into phi destination locals for edges from src to dst.
// Uses parallel move semantics: all source values are pushed first, then stored
// in reverse order. This prevents the "lost copy" problem where storing one phi
// dest overwrites a value needed by a subsequent phi arg.
func (g *generator) emitPhiMoves(src mir.BlockID, dst mir.BlockID) {
	dstBlock := g.curFunc.Block(dst)
	var slots []uint16
	for _, phi := range dstBlock.Phis {
		if val, ok := phi.Args[src]; ok {
			g.emitValue(val)
			slots = append(slots, g.localSlots[phi.Dest])
		}
	}
	// Store in reverse order (LIFO) to match stack push order.
	for i := len(slots) - 1; i >= 0; i-- {
		EmitU16(&g.code, OpStoreLocal, slots[i])
		g.adjustStack(-1)
	}
}

// ---------------------------------------------------------------------------
// Jump helpers
// ---------------------------------------------------------------------------

func (g *generator) emitJump(op Opcode, target mir.BlockID) {
	if off, ok := g.blockOffsets[target]; ok {
		// Backward jump — we know the offset.
		rel := off - (len(g.code) + 3) // +3 = opcode + i16
		EmitI16(&g.code, op, int16(rel))
	} else {
		// Forward jump — record patch.
		EmitI16(&g.code, op, 0)
		g.patches = append(g.patches, jumpPatch{
			instrOffset: len(g.code) - 2,
			targetBlock: target,
		})
	}
}

func (g *generator) emitJumpPlaceholder(op Opcode, target mir.BlockID) int {
	EmitI16(&g.code, op, 0)
	patchOff := len(g.code) - 2
	g.patches = append(g.patches, jumpPatch{
		instrOffset: patchOff,
		targetBlock: target,
	})
	return patchOff
}

// ---------------------------------------------------------------------------
// Value emission (push onto stack)
// ---------------------------------------------------------------------------

func (g *generator) emitValue(v mir.Value) {
	switch val := v.(type) {
	case *mir.Local:
		slot := g.localSlots[val.ID]
		EmitU16(&g.code, OpLoadLocal, slot)
		g.adjustStack(1)

	case *mir.Const:
		switch val.Kind {
		case mir.ConstInt:
			EmitI64(&g.code, OpConstInt, val.Int)
		case mir.ConstFloat:
			EmitF64(&g.code, OpConstFloat, val.Float)
		case mir.ConstBool:
			if val.Bool {
				EmitOp(&g.code, OpConstTrue)
			} else {
				EmitOp(&g.code, OpConstFalse)
			}
		case mir.ConstString:
			idx := g.internString(val.Str)
			EmitU16(&g.code, OpConstString, idx)
		case mir.ConstChar:
			// Char is stored as string in MIR with surrounding quotes (e.g. "'x'").
			// Strip quotes and handle escape sequences to extract the rune.
			s := val.Str
			if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
				s = s[1 : len(s)-1]
			}
			r := rune(0)
			if len(s) > 0 && s[0] == '\\' && len(s) > 1 {
				switch s[1] {
				case 'n':
					r = '\n'
				case 't':
					r = '\t'
				case 'r':
					r = '\r'
				case '\\':
					r = '\\'
				case '\'':
					r = '\''
				case '0':
					r = 0
				default:
					r = rune(s[1])
				}
			} else {
				for _, c := range s {
					r = c
					break
				}
			}
			EmitU32(&g.code, OpConstChar, uint32(r))
		case mir.ConstUnit:
			EmitOp(&g.code, OpConstUnit)
		}
		g.adjustStack(1)

	case *mir.Global:
		if fnIdx, ok := g.funcIndex[val.Name]; ok {
			EmitU16(&g.code, OpLoadGlobal, fnIdx)
		} else {
			idx := g.internString(val.Name)
			EmitU16(&g.code, OpLoadGlobal, idx)
		}
		g.adjustStack(1)

	case *mir.Upvalue:
		EmitU16(&g.code, OpLoadUpvalue, uint16(val.Index))
		g.adjustStack(1)
	}
}

func (g *generator) emitStoreLocal(dest mir.LocalID) {
	slot := g.localSlots[dest]
	EmitU16(&g.code, OpStoreLocal, slot)
	g.adjustStack(-1)
}

// ---------------------------------------------------------------------------
// Binary/Unary op emission
// ---------------------------------------------------------------------------

func (g *generator) emitBinaryOp(op string, typ types.Type) {
	isFloat := isFloatType(typ)
	isStr := isStringType(typ)

	switch op {
	case "+":
		if isFloat {
			EmitOp(&g.code, OpAddFloat)
		} else if isStr {
			EmitOp(&g.code, OpConcatString)
		} else {
			EmitOp(&g.code, OpAddInt)
		}
	case "-":
		if isFloat {
			EmitOp(&g.code, OpSubFloat)
		} else {
			EmitOp(&g.code, OpSubInt)
		}
	case "*":
		if isFloat {
			EmitOp(&g.code, OpMulFloat)
		} else {
			EmitOp(&g.code, OpMulInt)
		}
	case "/":
		if isFloat {
			EmitOp(&g.code, OpDivFloat)
		} else {
			EmitOp(&g.code, OpDivInt)
		}
	case "%":
		if isFloat {
			EmitOp(&g.code, OpModFloat)
		} else {
			EmitOp(&g.code, OpModInt)
		}
	case "++":
		EmitOp(&g.code, OpConcatString)
	case "==":
		EmitOp(&g.code, OpEq)
	case "!=":
		EmitOp(&g.code, OpNeq)
	case "<":
		if isFloat {
			EmitOp(&g.code, OpLtFloat)
		} else {
			EmitOp(&g.code, OpLtInt)
		}
	case ">":
		if isFloat {
			EmitOp(&g.code, OpGtFloat)
		} else {
			EmitOp(&g.code, OpGtInt)
		}
	case "<=":
		if isFloat {
			EmitOp(&g.code, OpLeqFloat)
		} else {
			EmitOp(&g.code, OpLeqInt)
		}
	case ">=":
		if isFloat {
			EmitOp(&g.code, OpGeqFloat)
		} else {
			EmitOp(&g.code, OpGeqInt)
		}
	}
}

func (g *generator) emitUnaryOp(op string, typ types.Type) {
	switch op {
	case "-":
		if isFloatType(typ) {
			EmitOp(&g.code, OpNegFloat)
		} else {
			EmitOp(&g.code, OpNegInt)
		}
	case "!":
		EmitOp(&g.code, OpNot)
	}
}

func (g *generator) emitCast(srcType, dstType types.Type) {
	switch {
	case isIntType(srcType) && isFloatType(dstType):
		EmitOp(&g.code, OpIntToFloat)
	case isFloatType(srcType) && isIntType(dstType):
		EmitOp(&g.code, OpFloatToInt)
	case isIntType(srcType) && isStringType(dstType):
		EmitOp(&g.code, OpIntToString)
	case isFloatType(srcType) && isStringType(dstType):
		EmitOp(&g.code, OpFloatToString)
	}
}

// ---------------------------------------------------------------------------
// Field / variant resolution
// ---------------------------------------------------------------------------

func (g *generator) resolveFieldIndex(objType types.Type, field string) uint16 {
	switch t := objType.(type) {
	case *types.StructType:
		for i, name := range t.FieldOrder {
			if name == field {
				return uint16(i)
			}
		}
	}
	// [CLAUDE-FIX] Handle enum variant field access: "_N" or "Variant.N" format
	if strings.HasPrefix(field, "_") {
		if fieldIdx, err := strconv.Atoi(field[1:]); err == nil {
			return uint16(fieldIdx)
		}
	}
	if idx := strings.IndexByte(field, '.'); idx >= 0 {
		if fieldIdx, err := strconv.Atoi(field[idx+1:]); err == nil {
			return uint16(fieldIdx)
		}
	}
	// For struct defs in the MIR program, search there.
	for _, sd := range g.prog.Structs {
		for i, f := range sd.Fields {
			if f.Name == field {
				return uint16(i)
			}
		}
	}
	return 0
}

func (g *generator) resolveVariantIndex(enumName, variant string) uint16 {
	for _, ed := range g.prog.Enums {
		if ed.Name == enumName {
			for i, v := range ed.Variants {
				if v.Name == variant {
					return uint16(i)
				}
			}
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Stack depth tracking
// ---------------------------------------------------------------------------

func (g *generator) adjustStack(delta int) {
	g.stackDepth += delta
	if g.stackDepth > g.maxStack {
		g.maxStack = g.stackDepth
	}
	if g.stackDepth < 0 {
		g.stackDepth = 0
	}
}

// ---------------------------------------------------------------------------
// Type helpers
// ---------------------------------------------------------------------------

func isIntType(t types.Type) bool {
	_, ok := t.(*types.IntType)
	return ok
}

func isFloatType(t types.Type) bool {
	_, ok := t.(*types.FloatType)
	return ok
}

func isStringType(t types.Type) bool {
	_, ok := t.(*types.StringType)
	return ok
}

func isBoolType(t types.Type) bool {
	_, ok := t.(*types.BoolType)
	return ok
}
