package parser

import (
	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/lexer"
)

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

// Node is the interface implemented by all AST nodes.
type Node interface {
	Span() diagnostic.Span
	nodeTag()
}

// Expr represents an expression node.
type Expr interface {
	Node
	exprTag()
}

// Stmt represents a statement node.
type Stmt interface {
	Node
	stmtTag()
}

// Item represents a top-level declaration node.
type Item interface {
	Node
	itemTag()
}

// Pattern represents a pattern-match node.
type Pattern interface {
	Node
	patternTag()
}

// TypeExpr represents a type-expression node.
type TypeExpr interface {
	Node
	typeExprTag()
}

// ---------------------------------------------------------------------------
// Base embedding helpers (avoid 40+ copies of the same one-liner methods)
// ---------------------------------------------------------------------------

type nodeBase struct{ pos diagnostic.Span }

func (n nodeBase) Span() diagnostic.Span { return n.pos }
func (nodeBase) nodeTag()                {}

// ---------------------------------------------------------------------------
// Program (root)
// ---------------------------------------------------------------------------

type Program struct {
	nodeBase
	Items []Item
}

// ---------------------------------------------------------------------------
// Items
// ---------------------------------------------------------------------------

type FnDef struct {
	nodeBase
	Public     bool
	Name       string
	GenParams  *GenericParams
	Params     []*Param
	ReturnType TypeExpr // nil when omitted
	Body       *Block   // nil for trait method signatures
}

func (*FnDef) itemTag() {}

type TypeDef struct {
	nodeBase
	Public    bool
	Name      string
	GenParams *GenericParams
	Variants  []*Variant
}

func (*TypeDef) itemTag() {}

type StructDef struct {
	nodeBase
	Public    bool
	Name      string
	GenParams *GenericParams
	Fields    []*Field
}

func (*StructDef) itemTag() {}

type TraitDef struct {
	nodeBase
	Public    bool
	Name      string
	GenParams *GenericParams
	Methods   []*FnDef
}

func (*TraitDef) itemTag() {}

type ImplBlock struct {
	nodeBase
	GenParams  *GenericParams
	TraitName  string   // "" for inherent impls
	TraitGens  []TypeExpr // generic args on trait
	TargetType TypeExpr
	Methods    []*FnDef
}

func (*ImplBlock) itemTag() {}

type ImportDecl struct {
	nodeBase
	Path  []string
	Alias string // "" when no alias
}

func (*ImportDecl) itemTag() {}

type ModuleDecl struct {
	nodeBase
	Name string
}

func (*ModuleDecl) itemTag() {}

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

type LetStmt struct {
	nodeBase
	Mutable bool
	Name    string
	Type    TypeExpr // nil when omitted
	Value   Expr     // nil when omitted (let x: Int;)
}

func (*LetStmt) stmtTag() {}

type ExprStmt struct {
	nodeBase
	Expr Expr
}

func (*ExprStmt) stmtTag() {}

type ReturnStmt struct {
	nodeBase
	Value Expr // nil for bare `return;`
}

func (*ReturnStmt) stmtTag() {}

// ---------------------------------------------------------------------------
// Expressions
// ---------------------------------------------------------------------------

// Block is both a statement container and an expression (last expr is value).
type Block struct {
	nodeBase
	Stmts        []Stmt
	TrailingExpr Expr // nil when block ends with `;` or is empty
}

func (*Block) exprTag() {}

type IntLit struct {
	nodeBase
	Value string
}

func (*IntLit) exprTag() {}

type FloatLit struct {
	nodeBase
	Value string
}

func (*FloatLit) exprTag() {}

type StringLit struct {
	nodeBase
	Value string
}

func (*StringLit) exprTag() {}

type CharLit struct {
	nodeBase
	Value string
}

func (*CharLit) exprTag() {}

type BoolLit struct {
	nodeBase
	Value bool
}

func (*BoolLit) exprTag() {}

type Ident struct {
	nodeBase
	Name string
}

func (*Ident) exprTag() {}

type SelfExpr struct{ nodeBase }

func (*SelfExpr) exprTag() {}

type BinaryExpr struct {
	nodeBase
	Left  Expr
	Op    lexer.TokenType
	Right Expr
}

func (*BinaryExpr) exprTag() {}

type UnaryExpr struct {
	nodeBase
	Op      lexer.TokenType
	Operand Expr
}

func (*UnaryExpr) exprTag() {}

type CallExpr struct {
	nodeBase
	Func Expr
	Args []Expr
}

func (*CallExpr) exprTag() {}

type FieldExpr struct {
	nodeBase
	Object Expr
	Field  string
}

func (*FieldExpr) exprTag() {}

type IndexExpr struct {
	nodeBase
	Object Expr
	Index  Expr
}

func (*IndexExpr) exprTag() {}

type IfExpr struct {
	nodeBase
	Cond Expr
	Then *Block
	Else Expr // *Block or *IfExpr; nil when no else
}

func (*IfExpr) exprTag() {}

type MatchExpr struct {
	nodeBase
	Scrutinee Expr
	Arms      []*MatchArm
}

func (*MatchExpr) exprTag() {}

type ForExpr struct {
	nodeBase
	Binding string
	Iter    Expr
	Body    *Block
}

func (*ForExpr) exprTag() {}

type WhileExpr struct {
	nodeBase
	Cond Expr
	Body *Block
}

func (*WhileExpr) exprTag() {}

type LoopExpr struct {
	nodeBase
	Body *Block
}

func (*LoopExpr) exprTag() {}

type LambdaExpr struct {
	nodeBase
	Params     []*Param
	ReturnType TypeExpr // nil when omitted
	Body       Expr
}

func (*LambdaExpr) exprTag() {}

type SpawnExpr struct {
	nodeBase
	Body *Block
}

func (*SpawnExpr) exprTag() {}

type ChannelExpr struct {
	nodeBase
	ElemType TypeExpr
	Size     Expr // nil when unbuffered
}

func (*ChannelExpr) exprTag() {}

type ArrayLit struct {
	nodeBase
	Elems []Expr
}

func (*ArrayLit) exprTag() {}

type TupleLit struct {
	nodeBase
	Elems []Expr
}

func (*TupleLit) exprTag() {}

type StructLit struct {
	nodeBase
	Name   string
	Fields []*FieldInit
}

func (*StructLit) exprTag() {}

type GroupExpr struct {
	nodeBase
	Inner Expr
}

func (*GroupExpr) exprTag() {}

type BreakExpr struct{ nodeBase }

func (*BreakExpr) exprTag() {}

type ContinueExpr struct{ nodeBase }

func (*ContinueExpr) exprTag() {}

type ReturnExpr struct {
	nodeBase
	Value Expr // nil for bare return
}

func (*ReturnExpr) exprTag() {}

type PathExpr struct {
	nodeBase
	Segments []string
}

func (*PathExpr) exprTag() {}

type AsExpr struct {
	nodeBase
	Expr Expr
	Type TypeExpr
}

func (*AsExpr) exprTag() {}

// ---------------------------------------------------------------------------
// Patterns
// ---------------------------------------------------------------------------

type WildcardPat struct{ nodeBase }

func (*WildcardPat) patternTag() {}

type BindingPat struct {
	nodeBase
	Name string
}

func (*BindingPat) patternTag() {}

type LiteralPat struct {
	nodeBase
	Value Expr // IntLit, FloatLit, StringLit, CharLit, BoolLit
}

func (*LiteralPat) patternTag() {}

type TuplePat struct {
	nodeBase
	Elems []Pattern
}

func (*TuplePat) patternTag() {}

type VariantPat struct {
	nodeBase
	Name   string
	Fields []Pattern
}

func (*VariantPat) patternTag() {}

type StructPat struct {
	nodeBase
	Name   string
	Fields []*StructPatField
}

func (*StructPat) patternTag() {}

type OrPat struct {
	nodeBase
	Alts []Pattern
}

func (*OrPat) patternTag() {}

type RangePat struct {
	nodeBase
	Start     Expr // IntLit or CharLit
	End       Expr
	Inclusive bool
}

func (*RangePat) patternTag() {}

// ---------------------------------------------------------------------------
// Type Expressions
// ---------------------------------------------------------------------------

type NamedType struct {
	nodeBase
	Name    string
	GenArgs []TypeExpr
}

func (*NamedType) typeExprTag() {}

type FnType struct {
	nodeBase
	Params []TypeExpr
	Return TypeExpr
}

func (*FnType) typeExprTag() {}

type TupleType struct {
	nodeBase
	Elems []TypeExpr
}

func (*TupleType) typeExprTag() {}

type ArrayType struct {
	nodeBase
	Elem TypeExpr
}

func (*ArrayType) typeExprTag() {}

type SliceType struct {
	nodeBase
	Elem TypeExpr
}

func (*SliceType) typeExprTag() {}

type ReferenceType struct {
	nodeBase
	Elem TypeExpr
}

func (*ReferenceType) typeExprTag() {}

type SelfType struct{ nodeBase }

func (*SelfType) typeExprTag() {}

// [CLAUDE-FIX] Add ChannelType AST node for channel<T> in type positions
type ChannelType struct {
	nodeBase
	Elem TypeExpr
}

func (*ChannelType) typeExprTag() {}

// ---------------------------------------------------------------------------
// Helper / sub-node types
// ---------------------------------------------------------------------------

type GenericParams struct {
	nodeBase
	Params []*GenericParam
}

type GenericParam struct {
	nodeBase
	Name   string
	Bounds []TypeExpr // trait bounds: T: A + B
}

type Param struct {
	nodeBase
	Name string
	Type TypeExpr // nil when inferred
}

type Field struct {
	nodeBase
	Public bool
	Name   string
	Type   TypeExpr
}

type Variant struct {
	nodeBase
	Name   string
	Fields []TypeExpr // associated data types
}

type MatchArm struct {
	nodeBase
	Pattern Pattern
	Guard   Expr // nil when absent
	Body    Expr
}

type FieldInit struct {
	nodeBase
	Name  string
	Value Expr
}

type StructPatField struct {
	nodeBase
	Name    string
	Pattern Pattern // nil → bind to same name
}
