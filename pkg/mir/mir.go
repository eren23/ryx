package mir

import (
	"fmt"

	"github.com/ryx-lang/ryx/pkg/types"
)

// ---------------------------------------------------------------------------
// Identifiers
// ---------------------------------------------------------------------------

// BlockID identifies a basic block within a function.
type BlockID int

// LocalID identifies an SSA local variable within a function.
type LocalID int

// NoLocal is the sentinel for statements that produce no value.
const NoLocal LocalID = -1

// ---------------------------------------------------------------------------
// Values — SSA operands
// ---------------------------------------------------------------------------

// Value is an SSA operand: a local, constant, or global reference.
type Value interface {
	mirValueTag()
	ValueType() types.Type
	String() string
}

// Local references an SSA local variable.
type Local struct {
	ID   LocalID
	Type types.Type
}

func (*Local) mirValueTag()            {}
func (v *Local) ValueType() types.Type { return v.Type }
func (v *Local) String() string        { return fmt.Sprintf("%%%d", v.ID) }

// Const is a compile-time constant value.
type Const struct {
	Kind  ConstKind
	Int   int64
	Float float64
	Str   string
	Bool  bool
	Type  types.Type
}

func (*Const) mirValueTag()            {}
func (v *Const) ValueType() types.Type { return v.Type }

func (v *Const) String() string {
	switch v.Kind {
	case ConstInt:
		return fmt.Sprintf("%d", v.Int)
	case ConstFloat:
		return fmt.Sprintf("%g", v.Float)
	case ConstString:
		return fmt.Sprintf("%q", v.Str)
	case ConstChar:
		return fmt.Sprintf("'%s'", v.Str)
	case ConstBool:
		if v.Bool {
			return "true"
		}
		return "false"
	case ConstUnit:
		return "()"
	default:
		return "<const?>"
	}
}

// ConstKind discriminates Const variants.
type ConstKind int

const (
	ConstInt ConstKind = iota
	ConstFloat
	ConstString
	ConstChar
	ConstBool
	ConstUnit
)

// Global references a top-level function or static symbol.
type Global struct {
	Name string
	Type types.Type
}

func (*Global) mirValueTag()            {}
func (v *Global) ValueType() types.Type { return v.Type }
func (v *Global) String() string        { return fmt.Sprintf("@%s", v.Name) }

// Upvalue references a captured variable in a closure function.
type Upvalue struct {
	Index int
	Type  types.Type
}

func (*Upvalue) mirValueTag()            {}
func (v *Upvalue) ValueType() types.Type { return v.Type }
func (v *Upvalue) String() string        { return fmt.Sprintf("upval(%d)", v.Index) }

// ---------------------------------------------------------------------------
// Convenience constructors
// ---------------------------------------------------------------------------

func IntConst(v int64) *Const {
	return &Const{Kind: ConstInt, Int: v, Type: types.TypInt}
}

func FloatConst(v float64) *Const {
	return &Const{Kind: ConstFloat, Float: v, Type: types.TypFloat}
}

func BoolConst(v bool) *Const {
	return &Const{Kind: ConstBool, Bool: v, Type: types.TypBool}
}

func StringConst(v string) *Const {
	return &Const{Kind: ConstString, Str: v, Type: types.TypString}
}

func UnitConst() *Const {
	return &Const{Kind: ConstUnit, Type: types.TypUnit}
}

// ---------------------------------------------------------------------------
// Phi node
// ---------------------------------------------------------------------------

// Phi merges values from different predecessor blocks at a join point.
type Phi struct {
	Dest LocalID
	Type types.Type
	Args map[BlockID]Value // predecessor block -> incoming value
}

// ---------------------------------------------------------------------------
// Statements — SSA instructions that produce a value
// ---------------------------------------------------------------------------

// Stmt is an SSA instruction within a basic block.
type Stmt interface {
	mirStmtTag()
	DestLocal() LocalID // returns NoLocal if no destination
	StmtType() types.Type
}

// Assign copies a value: dest = src.
type Assign struct {
	Dest LocalID
	Src  Value
	Type types.Type
}

func (*Assign) mirStmtTag()              {}
func (s *Assign) DestLocal() LocalID     { return s.Dest }
func (s *Assign) StmtType() types.Type   { return s.Type }

// CallStmt is a function call: dest = func(args...).
type CallStmt struct {
	Dest LocalID
	Func Value
	Args []Value
	Type types.Type
}

func (*CallStmt) mirStmtTag()              {}
func (s *CallStmt) DestLocal() LocalID     { return s.Dest }
func (s *CallStmt) StmtType() types.Type   { return s.Type }

// BinaryOpStmt: dest = left op right.
type BinaryOpStmt struct {
	Dest  LocalID
	Op    string
	Left  Value
	Right Value
	Type  types.Type
}

func (*BinaryOpStmt) mirStmtTag()              {}
func (s *BinaryOpStmt) DestLocal() LocalID     { return s.Dest }
func (s *BinaryOpStmt) StmtType() types.Type   { return s.Type }

// UnaryOpStmt: dest = op operand.
type UnaryOpStmt struct {
	Dest    LocalID
	Op      string
	Operand Value
	Type    types.Type
}

func (*UnaryOpStmt) mirStmtTag()              {}
func (s *UnaryOpStmt) DestLocal() LocalID     { return s.Dest }
func (s *UnaryOpStmt) StmtType() types.Type   { return s.Type }

// FieldAccessStmt: dest = object.field.
type FieldAccessStmt struct {
	Dest   LocalID
	Object Value
	Field  string
	Type   types.Type
}

func (*FieldAccessStmt) mirStmtTag()              {}
func (s *FieldAccessStmt) DestLocal() LocalID     { return s.Dest }
func (s *FieldAccessStmt) StmtType() types.Type   { return s.Type }

// IndexAccessStmt: dest = object[index].
type IndexAccessStmt struct {
	Dest   LocalID
	Object Value
	Index  Value
	Type   types.Type
}

func (*IndexAccessStmt) mirStmtTag()              {}
func (s *IndexAccessStmt) DestLocal() LocalID     { return s.Dest }
func (s *IndexAccessStmt) StmtType() types.Type   { return s.Type }

// [CLAUDE-FIX] FieldSetStmt: object.field = value.
type FieldSetStmt struct {
	Object Value
	Field  string
	Value  Value
}

func (*FieldSetStmt) mirStmtTag()              {}
func (s *FieldSetStmt) DestLocal() LocalID     { return NoLocal }
func (s *FieldSetStmt) StmtType() types.Type   { return types.TypUnit }

// [CLAUDE-FIX] IndexSetStmt: object[index] = value.
type IndexSetStmt struct {
	Object Value
	Index  Value
	Value  Value
}

func (*IndexSetStmt) mirStmtTag()              {}
func (s *IndexSetStmt) DestLocal() LocalID     { return NoLocal }
func (s *IndexSetStmt) StmtType() types.Type   { return types.TypUnit }

// ArrayAllocStmt: dest = [elem0, elem1, ...].
type ArrayAllocStmt struct {
	Dest  LocalID
	Elems []Value
	Type  types.Type
}

func (*ArrayAllocStmt) mirStmtTag()              {}
func (s *ArrayAllocStmt) DestLocal() LocalID     { return s.Dest }
func (s *ArrayAllocStmt) StmtType() types.Type   { return s.Type }

// StructAllocStmt: dest = StructName { fields... }.
type StructAllocStmt struct {
	Dest   LocalID
	Name   string
	Fields []FieldValue
	Type   types.Type
}

func (*StructAllocStmt) mirStmtTag()              {}
func (s *StructAllocStmt) DestLocal() LocalID     { return s.Dest }
func (s *StructAllocStmt) StmtType() types.Type   { return s.Type }

// FieldValue is a named field initializer.
type FieldValue struct {
	Name  string
	Value Value
}

// EnumAllocStmt: dest = EnumName::Variant(args...).
type EnumAllocStmt struct {
	Dest     LocalID
	EnumName string
	Variant  string
	Args     []Value
	Type     types.Type
}

func (*EnumAllocStmt) mirStmtTag()              {}
func (s *EnumAllocStmt) DestLocal() LocalID     { return s.Dest }
func (s *EnumAllocStmt) StmtType() types.Type   { return s.Type }

// ClosureAllocStmt: dest = closure(funcName, captures...).
type ClosureAllocStmt struct {
	Dest     LocalID
	FuncName string
	Captures []Value
	Type     types.Type
}

func (*ClosureAllocStmt) mirStmtTag()              {}
func (s *ClosureAllocStmt) DestLocal() LocalID     { return s.Dest }
func (s *ClosureAllocStmt) StmtType() types.Type   { return s.Type }

// ChannelCreateStmt: dest = channel<ElemType>(bufSize).
type ChannelCreateStmt struct {
	Dest     LocalID
	ElemType types.Type
	BufSize  Value // nil for unbuffered
	Type     types.Type
}

func (*ChannelCreateStmt) mirStmtTag()              {}
func (s *ChannelCreateStmt) DestLocal() LocalID     { return s.Dest }
func (s *ChannelCreateStmt) StmtType() types.Type   { return s.Type }

// ChannelSendStmt: chan <- value. No destination (side effect).
type ChannelSendStmt struct {
	Chan    Value
	SendVal Value
}

func (*ChannelSendStmt) mirStmtTag()              {}
func (s *ChannelSendStmt) DestLocal() LocalID     { return NoLocal }
func (s *ChannelSendStmt) StmtType() types.Type   { return types.TypUnit }

// [CLAUDE-FIX] ChannelRecvStmt: dest = <- chan.
type ChannelRecvStmt struct {
	Dest LocalID
	Chan Value
	Type types.Type
}

func (*ChannelRecvStmt) mirStmtTag()              {}
func (s *ChannelRecvStmt) DestLocal() LocalID     { return s.Dest }
func (s *ChannelRecvStmt) StmtType() types.Type   { return s.Type }

// [CLAUDE-FIX] ChannelCloseStmt: close(chan).
type ChannelCloseStmt struct {
	Chan Value
}

func (*ChannelCloseStmt) mirStmtTag()              {}
func (s *ChannelCloseStmt) DestLocal() LocalID     { return NoLocal }
func (s *ChannelCloseStmt) StmtType() types.Type   { return types.TypUnit }

// SpawnStmt: dest = spawn func(args...).
type SpawnStmt struct {
	Dest LocalID
	Func Value
	Args []Value
	Type types.Type
}

func (*SpawnStmt) mirStmtTag()              {}
func (s *SpawnStmt) DestLocal() LocalID     { return s.Dest }
func (s *SpawnStmt) StmtType() types.Type   { return s.Type }

// CastStmt: dest = src as TargetType.
type CastStmt struct {
	Dest   LocalID
	Src    Value
	Target types.Type
	Type   types.Type
}

func (*CastStmt) mirStmtTag()              {}
func (s *CastStmt) DestLocal() LocalID     { return s.Dest }
func (s *CastStmt) StmtType() types.Type   { return s.Type }

// [CLAUDE-FIX] TagCheckStmt: dest = (scrutinee variant == variantName) : Bool
type TagCheckStmt struct {
	Dest     LocalID
	Object   Value
	EnumName string
	Variant  string
	Type     types.Type
}

func (*TagCheckStmt) mirStmtTag()              {}
func (s *TagCheckStmt) DestLocal() LocalID     { return s.Dest }
func (s *TagCheckStmt) StmtType() types.Type   { return s.Type }

// ---------------------------------------------------------------------------
// Terminators — control flow
// ---------------------------------------------------------------------------

// Terminator ends a basic block by transferring control.
type Terminator interface {
	mirTermTag()
	Successors() []BlockID
}

// Goto unconditionally jumps to a target block.
type Goto struct {
	Target BlockID
}

func (*Goto) mirTermTag()              {}
func (t *Goto) Successors() []BlockID  { return []BlockID{t.Target} }

// Branch conditionally jumps based on a boolean value.
type Branch struct {
	Cond Value
	Then BlockID
	Else BlockID
}

func (*Branch) mirTermTag()              {}
func (t *Branch) Successors() []BlockID  { return []BlockID{t.Then, t.Else} }

// Switch dispatches on an integer/enum discriminant.
type Switch struct {
	Scrutinee Value
	Cases     []SwitchCase
	Default   BlockID
}

func (*Switch) mirTermTag() {}
func (t *Switch) Successors() []BlockID {
	out := make([]BlockID, 0, len(t.Cases)+1)
	for _, c := range t.Cases {
		out = append(out, c.Target)
	}
	out = append(out, t.Default)
	return out
}

// SwitchCase is a single case in a switch terminator.
type SwitchCase struct {
	Value  Value
	Target BlockID
}

// Return exits the function.
type Return struct {
	Value Value // nil for unit return
}

func (*Return) mirTermTag()              {}
func (t *Return) Successors() []BlockID  { return nil }

// Unreachable marks control flow that should never be reached.
type Unreachable struct{}

func (*Unreachable) mirTermTag()              {}
func (t *Unreachable) Successors() []BlockID  { return nil }

// ---------------------------------------------------------------------------
// Basic Block
// ---------------------------------------------------------------------------

// BasicBlock is a sequence of phi nodes and statements terminated by
// a control-flow instruction.
type BasicBlock struct {
	ID    BlockID
	Label string // human-readable label (e.g. "entry", "if.then")
	Phis  []*Phi
	Stmts []Stmt
	Term  Terminator
	Preds []BlockID
	Succs []BlockID
}

// ---------------------------------------------------------------------------
// Local variable definition
// ---------------------------------------------------------------------------

// LocalDef describes an SSA local variable.
type LocalDef struct {
	ID      LocalID
	Name    string // debug name (original source name or generated)
	Type    types.Type
	Mutable bool // original source variable was declared mutable
}

// ---------------------------------------------------------------------------
// Function
// ---------------------------------------------------------------------------

// Function is a MIR function in SSA form with a control-flow graph.
type Function struct {
	Name         string
	Params       []LocalID
	ReturnType   types.Type
	Locals       []*LocalDef
	Blocks       []*BasicBlock
	Entry        BlockID
	UpvalueCount int // number of upvalues (captured variables) for closure functions
}

// Block returns the basic block with the given ID.
func (f *Function) Block(id BlockID) *BasicBlock {
	return f.Blocks[int(id)]
}

// NewLocal allocates a fresh SSA local with the given name and type.
func (f *Function) NewLocal(name string, typ types.Type) LocalID {
	id := LocalID(len(f.Locals))
	f.Locals = append(f.Locals, &LocalDef{
		ID:   id,
		Name: name,
		Type: typ,
	})
	return id
}

// NewBlock allocates a fresh basic block with the given label.
func (f *Function) NewBlock(label string) BlockID {
	id := BlockID(len(f.Blocks))
	f.Blocks = append(f.Blocks, &BasicBlock{
		ID:    id,
		Label: label,
	})
	return id
}

// AddEdge records a CFG edge from src to dst.
func (f *Function) AddEdge(src, dst BlockID) {
	srcB := f.Block(src)
	dstB := f.Block(dst)
	srcB.Succs = append(srcB.Succs, dst)
	dstB.Preds = append(dstB.Preds, src)
}

// LocalRef creates a Value referencing the given local.
func (f *Function) LocalRef(id LocalID) *Local {
	def := f.Locals[int(id)]
	return &Local{ID: id, Type: def.Type}
}

// ---------------------------------------------------------------------------
// Program
// ---------------------------------------------------------------------------

// Program is the top-level MIR representation.
type Program struct {
	Functions []*Function
	Structs   []*StructDef
	Enums     []*EnumDef
}

// StructDef mirrors the HIR struct definition.
type StructDef struct {
	Name   string
	Fields []FieldDef
}

// FieldDef is a struct field.
type FieldDef struct {
	Name string
	Type types.Type
}

// EnumDef mirrors the HIR enum definition.
type EnumDef struct {
	Name     string
	Variants []VariantDef
}

// VariantDef is an enum variant.
type VariantDef struct {
	Name   string
	Fields []types.Type
}

// ---------------------------------------------------------------------------
// Escape annotation (filled by escape analysis)
// ---------------------------------------------------------------------------

// EscapeInfo records whether an allocation escapes to the heap.
type EscapeInfo struct {
	Escapes map[LocalID]bool // true = heap-allocated
}
