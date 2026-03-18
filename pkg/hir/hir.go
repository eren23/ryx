package hir

import (
	"fmt"
	"strings"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/types"
)

// ---------------------------------------------------------------------------
// Program (root)
// ---------------------------------------------------------------------------

// Program is the top-level HIR node. All types are fully resolved (no type
// variables), generic functions have been monomorphized, syntactic sugars
// desugared, and pattern matches compiled to decision trees.
type Program struct {
	Functions []*Function
	Structs   []*StructDef
	Enums     []*EnumDef
}

// ---------------------------------------------------------------------------
// Top-level definitions
// ---------------------------------------------------------------------------

// Function is a concrete (monomorphic) function.
type Function struct {
	Name       string
	Params     []Param
	ReturnType types.Type
	Body       *Block
	Span       diagnostic.Span
}

// Param is a function parameter with a resolved type.
type Param struct {
	Name string
	Type types.Type
}

// StructDef is a concrete struct definition.
type StructDef struct {
	Name       string
	Fields     []FieldDef
	Span       diagnostic.Span
}

// FieldDef is a struct field with a resolved type.
type FieldDef struct {
	Name   string
	Type   types.Type
	Public bool
}

// EnumDef is a concrete enum definition.
type EnumDef struct {
	Name     string
	Variants []VariantDef
	Span     diagnostic.Span
}

// VariantDef is an enum variant with resolved field types.
type VariantDef struct {
	Name   string
	Fields []types.Type
}

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

// Stmt is the interface for HIR statements.
type Stmt interface {
	stmtSpan() diagnostic.Span
	hirStmtTag()
}

// LetStmt is a variable binding.
type LetStmt struct {
	Name    string
	Type    types.Type
	Value   Expr
	Mutable bool
	Span    diagnostic.Span
}

func (s *LetStmt) stmtSpan() diagnostic.Span { return s.Span }
func (*LetStmt) hirStmtTag()                  {}

// ExprStmt wraps an expression used as a statement.
type ExprStmt struct {
	Expr Expr
	Span diagnostic.Span
}

func (s *ExprStmt) stmtSpan() diagnostic.Span { return s.Span }
func (*ExprStmt) hirStmtTag()                  {}

// ReturnStmt is a return statement.
type ReturnStmt struct {
	Value Expr // nil for bare return
	Span  diagnostic.Span
}

func (s *ReturnStmt) stmtSpan() diagnostic.Span { return s.Span }
func (*ReturnStmt) hirStmtTag()                  {}

// ---------------------------------------------------------------------------
// Expressions — all carry their resolved type
// ---------------------------------------------------------------------------

// Expr is the interface for HIR expressions. Every expression carries its
// fully resolved type (no type variables).
type Expr interface {
	ExprType() types.Type
	ExprSpan() diagnostic.Span
	hirExprTag()
}

type exprBase struct {
	Typ  types.Type
	Span diagnostic.Span
}

func (e exprBase) ExprType() types.Type       { return e.Typ }
func (e exprBase) ExprSpan() diagnostic.Span  { return e.Span }
func (exprBase) hirExprTag()                  {}
func (e *exprBase) setType(t types.Type)      { e.Typ = t }

// SetExprType sets the type on an Expr's embedded exprBase.
// This is intended for use by tests outside the hir package.
func SetExprType(e Expr, t types.Type) {
	type typeSetter interface {
		setType(types.Type)
	}
	if s, ok := e.(typeSetter); ok {
		s.setType(t)
	}
}

// ---------------------------------------------------------------------------
// Literal expressions
// ---------------------------------------------------------------------------

type IntLit struct {
	exprBase
	Value string
}

type FloatLit struct {
	exprBase
	Value string
}

type StringLit struct {
	exprBase
	Value string
}

type CharLit struct {
	exprBase
	Value string
}

type BoolLit struct {
	exprBase
	Value bool
}

type UnitLit struct {
	exprBase
}

// ---------------------------------------------------------------------------
// Variable / identifier references
// ---------------------------------------------------------------------------

// VarRef refers to a resolved variable.
type VarRef struct {
	exprBase
	Name string
}

// PathRef refers to a module-qualified path (e.g. Type::method).
type PathRef struct {
	exprBase
	Segments []string
}

// ---------------------------------------------------------------------------
// Compound expressions
// ---------------------------------------------------------------------------

// Block is a sequence of statements with an optional trailing expression.
type Block struct {
	exprBase
	Stmts        []Stmt
	TrailingExpr Expr // nil when block yields Unit
}

// IfExpr is a conditional expression.
type IfExpr struct {
	exprBase
	Cond Expr
	Then *Block
	Else Expr // *Block or *IfExpr; nil when no else
}

// WhileExpr is a while-loop expression (also used for desugared for-in-range).
type WhileExpr struct {
	exprBase
	Cond Expr
	Body *Block
}

// LoopExpr is an infinite loop (also used for desugared for-in-channel).
type LoopExpr struct {
	exprBase
	Body *Block
}

// ---------------------------------------------------------------------------
// Call expressions
// ---------------------------------------------------------------------------

// Call is a direct function call (function name or expression).
type Call struct {
	exprBase
	Func Expr
	Args []Expr
}

// StaticCall is a desugared method call: Type::method(receiver, args...).
type StaticCall struct {
	exprBase
	TypeName string
	Method   string
	Args     []Expr // first arg is the receiver
}

// ---------------------------------------------------------------------------
// Operators
// ---------------------------------------------------------------------------

// BinaryOp is a binary operation (arithmetic, comparison, logical).
// The ++ (concat) and |> (pipe) operators never appear here — they are
// desugared during lowering.
type BinaryOp struct {
	exprBase
	Op    string
	Left  Expr
	Right Expr
}

// UnaryOp is a unary operation.
type UnaryOp struct {
	exprBase
	Op      string
	Operand Expr
}

// ---------------------------------------------------------------------------
// Aggregate expressions
// ---------------------------------------------------------------------------

// FieldAccess is struct field access.
type FieldAccess struct {
	exprBase
	Object Expr
	Field  string
}

// Index is array/slice indexing.
type Index struct {
	exprBase
	Object Expr
	Idx    Expr
}

// ArrayLiteral is an array literal.
type ArrayLiteral struct {
	exprBase
	Elems []Expr
}

// TupleLiteral is a tuple literal.
type TupleLiteral struct {
	exprBase
	Elems []Expr
}

// StructLiteral is a struct literal.
type StructLiteral struct {
	exprBase
	Name   string
	Fields []FieldInit
}

// FieldInit is a struct field initializer.
type FieldInit struct {
	Name  string
	Value Expr
}

// ---------------------------------------------------------------------------
// [CLAUDE-FIX] Assignment expressions
// ---------------------------------------------------------------------------

// Assign is a variable assignment: name = value.
type Assign struct {
	exprBase
	Name  string
	Value Expr
}

// FieldAssign is a field assignment: object.field = value.
type FieldAssign struct {
	exprBase
	Object Expr
	Field  string
	Value  Expr
}

// IndexAssign is an index assignment: object[index] = value.
type IndexAssign struct {
	exprBase
	Object Expr
	Index  Expr
	Value  Expr
}

// ---------------------------------------------------------------------------
// Enum construction
// ---------------------------------------------------------------------------

// [CLAUDE-FIX] EnumConstruct represents an enum variant constructor call
type EnumConstruct struct {
	exprBase
	EnumName string
	Variant  string
	Args     []Expr
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

// ChannelCreate creates a channel.
type ChannelCreate struct {
	exprBase
	ElemType types.Type
	BufSize  Expr // nil for unbuffered
}

// Spawn spawns a concurrent fiber.
type Spawn struct {
	exprBase
	Body *Block
}

// ---------------------------------------------------------------------------
// Control flow
// ---------------------------------------------------------------------------

// BreakExpr is a break statement.
type BreakExpr struct{ exprBase }

// ContinueExpr is a continue statement.
type ContinueExpr struct{ exprBase }

// ReturnExpr is a return expression.
type ReturnExpr struct {
	exprBase
	Value Expr
}

// Cast is a type cast (as).
type Cast struct {
	exprBase
	Expr   Expr
	Target types.Type
}

// ---------------------------------------------------------------------------
// Closures / Lambdas
// ---------------------------------------------------------------------------

// Lambda is a closure with identified captured variables.
type Lambda struct {
	exprBase
	Params   []Param
	Body     Expr
	Captures []Capture
}

// Capture describes a variable captured by a closure. In later stages,
// captured variables become fields on the closure struct.
type Capture struct {
	Name string
	Type types.Type
}

// ---------------------------------------------------------------------------
// Pattern match — compiled to decision trees
// ---------------------------------------------------------------------------

// MatchExpr is a compiled pattern match. The Decision field is populated
// by the match compiler (match_compile.go).
type MatchExpr struct {
	exprBase
	Scrutinee Expr
	Decision  Decision
	// Arms retained for reference during compilation; nil after match compile.
	Arms []*MatchArm
}

// MatchArm is a pre-compilation match arm (pattern + optional guard + body).
type MatchArm struct {
	Pattern Pattern
	Guard   Expr
	Body    Expr
}

// ---------------------------------------------------------------------------
// Decision tree (compiled pattern match)
// ---------------------------------------------------------------------------

// Decision is a node in a pattern match decision tree.
type Decision interface {
	hirDecisionTag()
}

// DecisionLeaf executes a match arm body after binding pattern variables.
type DecisionLeaf struct {
	Bindings []Binding
	Body     Expr
}

func (*DecisionLeaf) hirDecisionTag() {}

// Binding maps a pattern variable name to the sub-expression it is bound to.
type Binding struct {
	Name string
	Expr Expr
	Type types.Type
}

// DecisionSwitch tests the scrutinee against a set of constructors.
type DecisionSwitch struct {
	Scrutinee Expr
	Cases     []*DecisionCase
	Default   Decision // nil if exhaustive
}

func (*DecisionSwitch) hirDecisionTag() {}

// DecisionCase is one arm of a decision switch.
type DecisionCase struct {
	Constructor string   // variant name or literal value
	ArgNames    []string // bound variable names for constructor fields
	Body        Decision
}

// DecisionGuard checks a guard clause and branches.
type DecisionGuard struct {
	Condition Expr
	Then      Decision
	Else      Decision
}

func (*DecisionGuard) hirDecisionTag() {}

// DecisionFail represents an unreachable branch (match failure). Should never
// be reached at runtime in well-typed programs.
type DecisionFail struct{}

func (*DecisionFail) hirDecisionTag() {}

// ---------------------------------------------------------------------------
// Patterns (used before match compilation)
// ---------------------------------------------------------------------------

// Pattern is a pattern in a match arm (pre-compilation representation).
type Pattern interface {
	hirPatternTag()
	PatternSpan() diagnostic.Span
}

type WildcardPat struct{ Span diagnostic.Span }
type BindingPat struct {
	Name string
	Span diagnostic.Span
}
type LiteralPat struct {
	Value Expr
	Span  diagnostic.Span
}
type TuplePat struct {
	Elems []Pattern
	Span  diagnostic.Span
}
type VariantPat struct {
	Name   string
	Fields []Pattern
	Span   diagnostic.Span
}
type StructPat struct {
	Name   string
	Fields []StructPatField
	Span   diagnostic.Span
}
type OrPat struct {
	Alts []Pattern
	Span diagnostic.Span
}
type RangePat struct {
	Start     Expr
	End       Expr
	Inclusive bool
	Span      diagnostic.Span
}

type StructPatField struct {
	Name    string
	Pattern Pattern
}

func (*WildcardPat) hirPatternTag() {}
func (*BindingPat) hirPatternTag()  {}
func (*LiteralPat) hirPatternTag()  {}
func (*TuplePat) hirPatternTag()    {}
func (*VariantPat) hirPatternTag()  {}
func (*StructPat) hirPatternTag()   {}
func (*OrPat) hirPatternTag()       {}
func (*RangePat) hirPatternTag()    {}

func (p *WildcardPat) PatternSpan() diagnostic.Span { return p.Span }
func (p *BindingPat) PatternSpan() diagnostic.Span  { return p.Span }
func (p *LiteralPat) PatternSpan() diagnostic.Span  { return p.Span }
func (p *TuplePat) PatternSpan() diagnostic.Span    { return p.Span }
func (p *VariantPat) PatternSpan() diagnostic.Span  { return p.Span }
func (p *StructPat) PatternSpan() diagnostic.Span   { return p.Span }
func (p *OrPat) PatternSpan() diagnostic.Span       { return p.Span }
func (p *RangePat) PatternSpan() diagnostic.Span    { return p.Span }

// ---------------------------------------------------------------------------
// Debug formatting
// ---------------------------------------------------------------------------

// FormatExpr returns a human-readable string for an HIR expression (for tests/debugging).
func FormatExpr(e Expr) string {
	if e == nil {
		return "<nil>"
	}
	switch ex := e.(type) {
	case *IntLit:
		return ex.Value
	case *FloatLit:
		return ex.Value
	case *StringLit:
		return fmt.Sprintf("%q", ex.Value)
	case *CharLit:
		return fmt.Sprintf("'%s'", ex.Value)
	case *BoolLit:
		if ex.Value {
			return "true"
		}
		return "false"
	case *UnitLit:
		return "()"
	case *VarRef:
		return ex.Name
	case *PathRef:
		return strings.Join(ex.Segments, "::")
	case *Call:
		args := formatExprs(ex.Args)
		return fmt.Sprintf("%s(%s)", FormatExpr(ex.Func), strings.Join(args, ", "))
	case *StaticCall:
		args := formatExprs(ex.Args)
		return fmt.Sprintf("%s::%s(%s)", ex.TypeName, ex.Method, strings.Join(args, ", "))
	case *BinaryOp:
		return fmt.Sprintf("(%s %s %s)", FormatExpr(ex.Left), ex.Op, FormatExpr(ex.Right))
	case *UnaryOp:
		return fmt.Sprintf("(%s%s)", ex.Op, FormatExpr(ex.Operand))
	case *FieldAccess:
		return fmt.Sprintf("%s.%s", FormatExpr(ex.Object), ex.Field)
	case *Index:
		return fmt.Sprintf("%s[%s]", FormatExpr(ex.Object), FormatExpr(ex.Idx))
	case *WhileExpr:
		return fmt.Sprintf("while %s { ... }", FormatExpr(ex.Cond))
	case *LoopExpr:
		return "loop { ... }"
	case *IfExpr:
		return fmt.Sprintf("if %s { ... }", FormatExpr(ex.Cond))
	case *MatchExpr:
		return fmt.Sprintf("match %s { ... }", FormatExpr(ex.Scrutinee))
	case *Lambda:
		return "lambda { ... }"
	case *Cast:
		return fmt.Sprintf("(%s as %s)", FormatExpr(ex.Expr), ex.Target.String())
	case *EnumConstruct: // [CLAUDE-FIX]
		args := formatExprs(ex.Args)
		return fmt.Sprintf("%s::%s(%s)", ex.EnumName, ex.Variant, strings.Join(args, ", "))
	case *Block:
		return "{ ... }"
	default:
		return fmt.Sprintf("<%T>", e)
	}
}

func formatExprs(exprs []Expr) []string {
	out := make([]string, len(exprs))
	for i, e := range exprs {
		out[i] = FormatExpr(e)
	}
	return out
}
