package parser

import (
	"testing"

	"github.com/ryx-lang/ryx/pkg/lexer"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseOK(t *testing.T, src string) *ParseResult {
	t.Helper()
	res := Parse(src, 0)
	if res.HasErrors() {
		for _, e := range res.Errors {
			t.Errorf("parse error: %s", e.Message)
		}
		t.FailNow()
	}
	return res
}

func parseExprOK(t *testing.T, src string) Expr {
	t.Helper()
	expr, errs := ParseExpr(src, 0)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("parse error: %s", e.Message)
		}
		t.FailNow()
	}
	if expr == nil {
		t.Fatal("expected expression, got nil")
	}
	return expr
}

func expectErrors(t *testing.T, src string, minErrors int) *ParseResult {
	t.Helper()
	res := Parse(src, 0)
	if len(res.Errors) < minErrors {
		t.Errorf("expected at least %d errors, got %d", minErrors, len(res.Errors))
	}
	return res
}

func assertBinary(t *testing.T, expr Expr, op lexer.TokenType) *BinaryExpr {
	t.Helper()
	be, ok := expr.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != op {
		t.Fatalf("expected op %s, got %s", op, be.Op)
	}
	return be
}

func assertIdent(t *testing.T, expr Expr, name string) {
	t.Helper()
	id, ok := expr.(*Ident)
	if !ok {
		t.Fatalf("expected Ident, got %T", expr)
	}
	if id.Name != name {
		t.Fatalf("expected name %q, got %q", name, id.Name)
	}
}

func assertIntLit(t *testing.T, expr Expr, val string) {
	t.Helper()
	lit, ok := expr.(*IntLit)
	if !ok {
		t.Fatalf("expected IntLit, got %T", expr)
	}
	if lit.Value != val {
		t.Fatalf("expected value %q, got %q", val, lit.Value)
	}
}

// ---------------------------------------------------------------------------
// Literal expressions
// ---------------------------------------------------------------------------

func TestParseIntLit(t *testing.T) {
	expr := parseExprOK(t, "42")
	assertIntLit(t, expr, "42")
}

func TestParseHexLit(t *testing.T) {
	expr := parseExprOK(t, "0xFF")
	assertIntLit(t, expr, "0xFF")
}

func TestParseFloatLit(t *testing.T) {
	expr := parseExprOK(t, "3.14")
	fl, ok := expr.(*FloatLit)
	if !ok {
		t.Fatalf("expected FloatLit, got %T", expr)
	}
	if fl.Value != "3.14" {
		t.Fatalf("expected 3.14, got %s", fl.Value)
	}
}

func TestParseStringLit(t *testing.T) {
	expr := parseExprOK(t, `"hello"`)
	sl, ok := expr.(*StringLit)
	if !ok {
		t.Fatalf("expected StringLit, got %T", expr)
	}
	// Lexer stores raw value including quotes.
	if sl.Value != `"hello"` {
		t.Fatalf("expected %q, got %q", `"hello"`, sl.Value)
	}
}

func TestParseCharLit(t *testing.T) {
	expr := parseExprOK(t, "'a'")
	cl, ok := expr.(*CharLit)
	if !ok {
		t.Fatalf("expected CharLit, got %T", expr)
	}
	// Lexer stores raw value including quotes.
	if cl.Value != "'a'" {
		t.Fatalf("expected %q, got %q", "'a'", cl.Value)
	}
}

func TestParseBoolLit(t *testing.T) {
	expr := parseExprOK(t, "true")
	bl, ok := expr.(*BoolLit)
	if !ok {
		t.Fatalf("expected BoolLit, got %T", expr)
	}
	if !bl.Value {
		t.Fatal("expected true")
	}
}

func TestParseIdent(t *testing.T) {
	expr := parseExprOK(t, "foo")
	assertIdent(t, expr, "foo")
}

func TestParseSelf(t *testing.T) {
	expr := parseExprOK(t, "self")
	if _, ok := expr.(*SelfExpr); !ok {
		t.Fatalf("expected SelfExpr, got %T", expr)
	}
}

// ---------------------------------------------------------------------------
// Unary expressions
// ---------------------------------------------------------------------------

func TestParseUnaryNot(t *testing.T) {
	expr := parseExprOK(t, "!x")
	ue, ok := expr.(*UnaryExpr)
	if !ok {
		t.Fatalf("expected UnaryExpr, got %T", expr)
	}
	if ue.Op != lexer.NOT {
		t.Fatalf("expected NOT, got %s", ue.Op)
	}
	assertIdent(t, ue.Operand, "x")
}

func TestParseUnaryNeg(t *testing.T) {
	expr := parseExprOK(t, "-42")
	ue, ok := expr.(*UnaryExpr)
	if !ok {
		t.Fatalf("expected UnaryExpr, got %T", expr)
	}
	if ue.Op != lexer.MINUS {
		t.Fatalf("expected MINUS, got %s", ue.Op)
	}
	assertIntLit(t, ue.Operand, "42")
}

func TestParseDoubleNeg(t *testing.T) {
	// --x → -(-(x))
	expr := parseExprOK(t, "- -x")
	outer, ok := expr.(*UnaryExpr)
	if !ok {
		t.Fatalf("expected UnaryExpr, got %T", expr)
	}
	inner, ok := outer.Operand.(*UnaryExpr)
	if !ok {
		t.Fatalf("expected inner UnaryExpr, got %T", outer.Operand)
	}
	assertIdent(t, inner.Operand, "x")
}

// ---------------------------------------------------------------------------
// Binary expressions — precedence
// ---------------------------------------------------------------------------

func TestPrecAddMul(t *testing.T) {
	// 1 + 2 * 3  →  Add(1, Mul(2, 3))
	expr := parseExprOK(t, "1 + 2 * 3")
	add := assertBinary(t, expr, lexer.PLUS)
	assertIntLit(t, add.Left, "1")
	mul := assertBinary(t, add.Right, lexer.STAR)
	assertIntLit(t, mul.Left, "2")
	assertIntLit(t, mul.Right, "3")
}

func TestPrecMulAdd(t *testing.T) {
	// 1 * 2 + 3  →  Add(Mul(1, 2), 3)
	expr := parseExprOK(t, "1 * 2 + 3")
	add := assertBinary(t, expr, lexer.PLUS)
	mul := assertBinary(t, add.Left, lexer.STAR)
	assertIntLit(t, mul.Left, "1")
	assertIntLit(t, mul.Right, "2")
	assertIntLit(t, add.Right, "3")
}

func TestPrecSubLeftAssoc(t *testing.T) {
	// 1 - 2 - 3  →  Sub(Sub(1, 2), 3)
	expr := parseExprOK(t, "1 - 2 - 3")
	outer := assertBinary(t, expr, lexer.MINUS)
	inner := assertBinary(t, outer.Left, lexer.MINUS)
	assertIntLit(t, inner.Left, "1")
	assertIntLit(t, inner.Right, "2")
	assertIntLit(t, outer.Right, "3")
}

func TestPrecAssignRightAssoc(t *testing.T) {
	// a = b = c  →  Assign(a, Assign(b, c))
	expr := parseExprOK(t, "a = b = c")
	outer := assertBinary(t, expr, lexer.ASSIGN)
	assertIdent(t, outer.Left, "a")
	inner := assertBinary(t, outer.Right, lexer.ASSIGN)
	assertIdent(t, inner.Left, "b")
	assertIdent(t, inner.Right, "c")
}

func TestPrecComparison(t *testing.T) {
	// a < b == c  →  Eq(Lt(a, b), c)
	// Wait, equality has lower prec than comparison.
	// So: (a < b) == c
	expr := parseExprOK(t, "a < b == c")
	eq := assertBinary(t, expr, lexer.EQ)
	lt := assertBinary(t, eq.Left, lexer.LT)
	assertIdent(t, lt.Left, "a")
	assertIdent(t, lt.Right, "b")
	assertIdent(t, eq.Right, "c")
}

func TestPrecLogical(t *testing.T) {
	// a || b && c  →  Or(a, And(b, c))
	expr := parseExprOK(t, "a || b && c")
	or := assertBinary(t, expr, lexer.OR)
	assertIdent(t, or.Left, "a")
	and := assertBinary(t, or.Right, lexer.AND)
	assertIdent(t, and.Left, "b")
	assertIdent(t, and.Right, "c")
}

func TestPrecPipe(t *testing.T) {
	// a |> b |> c  →  Pipe(Pipe(a, b), c)
	expr := parseExprOK(t, "a |> b |> c")
	outer := assertBinary(t, expr, lexer.PIPE)
	inner := assertBinary(t, outer.Left, lexer.PIPE)
	assertIdent(t, inner.Left, "a")
	assertIdent(t, inner.Right, "b")
	assertIdent(t, outer.Right, "c")
}

func TestPrecConcat(t *testing.T) {
	expr := parseExprOK(t, `"a" ++ "b"`)
	cc := assertBinary(t, expr, lexer.CONCAT)
	if _, ok := cc.Left.(*StringLit); !ok {
		t.Fatalf("expected StringLit, got %T", cc.Left)
	}
}

func TestPrecRange(t *testing.T) {
	expr := parseExprOK(t, "0..10")
	rng := assertBinary(t, expr, lexer.RANGE)
	assertIntLit(t, rng.Left, "0")
	assertIntLit(t, rng.Right, "10")
}

func TestPrecRangeInclusive(t *testing.T) {
	expr := parseExprOK(t, "0..=10")
	rng := assertBinary(t, expr, lexer.RANGE_INCLUSIVE)
	assertIntLit(t, rng.Left, "0")
	assertIntLit(t, rng.Right, "10")
}

func TestPrecAll(t *testing.T) {
	// Test multiple precedence levels in one expression.
	// a || b && c == d < e + f * g
	// Parses as: Or(a, And(b, Eq(c, Lt(d, Add(e, Mul(f, g))))))
	expr := parseExprOK(t, "a || b && c == d < e + f * g")
	or := assertBinary(t, expr, lexer.OR)
	assertIdent(t, or.Left, "a")
	and := assertBinary(t, or.Right, lexer.AND)
	assertIdent(t, and.Left, "b")
	eq := assertBinary(t, and.Right, lexer.EQ)
	assertIdent(t, eq.Left, "c")
	lt := assertBinary(t, eq.Right, lexer.LT)
	assertIdent(t, lt.Left, "d")
	add := assertBinary(t, lt.Right, lexer.PLUS)
	assertIdent(t, add.Left, "e")
	mul := assertBinary(t, add.Right, lexer.STAR)
	assertIdent(t, mul.Left, "f")
	assertIdent(t, mul.Right, "g")
}

func TestPrecAssignPipe(t *testing.T) {
	// x = a |> b  →  Assign(x, Pipe(a, b))
	expr := parseExprOK(t, "x = a |> b")
	assign := assertBinary(t, expr, lexer.ASSIGN)
	assertIdent(t, assign.Left, "x")
	pipe := assertBinary(t, assign.Right, lexer.PIPE)
	assertIdent(t, pipe.Left, "a")
	assertIdent(t, pipe.Right, "b")
}

func TestPrecDivMod(t *testing.T) {
	// a / b % c → Mod(Div(a, b), c)
	expr := parseExprOK(t, "a / b % c")
	mod := assertBinary(t, expr, lexer.PERCENT)
	div := assertBinary(t, mod.Left, lexer.SLASH)
	assertIdent(t, div.Left, "a")
	assertIdent(t, div.Right, "b")
	assertIdent(t, mod.Right, "c")
}

// ---------------------------------------------------------------------------
// Call, field, index expressions
// ---------------------------------------------------------------------------

func TestParseCall(t *testing.T) {
	expr := parseExprOK(t, "foo(1, 2)")
	ce, ok := expr.(*CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", expr)
	}
	assertIdent(t, ce.Func, "foo")
	if len(ce.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(ce.Args))
	}
}

func TestParseCallNoArgs(t *testing.T) {
	expr := parseExprOK(t, "foo()")
	ce, ok := expr.(*CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", expr)
	}
	if len(ce.Args) != 0 {
		t.Fatalf("expected 0 args, got %d", len(ce.Args))
	}
}

func TestParseCallTrailingComma(t *testing.T) {
	expr := parseExprOK(t, "foo(1, 2,)")
	ce, ok := expr.(*CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", expr)
	}
	if len(ce.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(ce.Args))
	}
}

func TestParseFieldAccess(t *testing.T) {
	expr := parseExprOK(t, "a.b")
	fe, ok := expr.(*FieldExpr)
	if !ok {
		t.Fatalf("expected FieldExpr, got %T", expr)
	}
	assertIdent(t, fe.Object, "a")
	if fe.Field != "b" {
		t.Fatalf("expected field b, got %s", fe.Field)
	}
}

func TestParseChainedFieldCall(t *testing.T) {
	// a.b(c).d → FieldExpr(CallExpr(FieldExpr(a, b), c), d)
	expr := parseExprOK(t, "a.b(c).d")
	outerField, ok := expr.(*FieldExpr)
	if !ok {
		t.Fatalf("expected FieldExpr, got %T", expr)
	}
	if outerField.Field != "d" {
		t.Fatalf("expected field d, got %s", outerField.Field)
	}
	call, ok := outerField.Object.(*CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", outerField.Object)
	}
	innerField, ok := call.Func.(*FieldExpr)
	if !ok {
		t.Fatalf("expected FieldExpr, got %T", call.Func)
	}
	assertIdent(t, innerField.Object, "a")
	if innerField.Field != "b" {
		t.Fatalf("expected field b, got %s", innerField.Field)
	}
}

func TestParseIndex(t *testing.T) {
	expr := parseExprOK(t, "a[0]")
	ie, ok := expr.(*IndexExpr)
	if !ok {
		t.Fatalf("expected IndexExpr, got %T", expr)
	}
	assertIdent(t, ie.Object, "a")
	assertIntLit(t, ie.Index, "0")
}

func TestParseChainedIndex(t *testing.T) {
	// a[0][1] → IndexExpr(IndexExpr(a, 0), 1)
	expr := parseExprOK(t, "a[0][1]")
	outer, ok := expr.(*IndexExpr)
	if !ok {
		t.Fatalf("expected IndexExpr, got %T", expr)
	}
	assertIntLit(t, outer.Index, "1")
	inner, ok := outer.Object.(*IndexExpr)
	if !ok {
		t.Fatalf("expected IndexExpr, got %T", outer.Object)
	}
	assertIdent(t, inner.Object, "a")
}

// ---------------------------------------------------------------------------
// Group and Tuple
// ---------------------------------------------------------------------------

func TestParseGroupExpr(t *testing.T) {
	expr := parseExprOK(t, "(42)")
	ge, ok := expr.(*GroupExpr)
	if !ok {
		t.Fatalf("expected GroupExpr, got %T", expr)
	}
	assertIntLit(t, ge.Inner, "42")
}

func TestParseTupleExpr(t *testing.T) {
	expr := parseExprOK(t, "(1, 2, 3)")
	te, ok := expr.(*TupleLit)
	if !ok {
		t.Fatalf("expected TupleLit, got %T", expr)
	}
	if len(te.Elems) != 3 {
		t.Fatalf("expected 3 elems, got %d", len(te.Elems))
	}
}

func TestParseEmptyTuple(t *testing.T) {
	expr := parseExprOK(t, "()")
	te, ok := expr.(*TupleLit)
	if !ok {
		t.Fatalf("expected TupleLit, got %T", expr)
	}
	if len(te.Elems) != 0 {
		t.Fatalf("expected 0 elems, got %d", len(te.Elems))
	}
}

// ---------------------------------------------------------------------------
// Array literal
// ---------------------------------------------------------------------------

func TestParseArrayLit(t *testing.T) {
	expr := parseExprOK(t, "[1, 2, 3]")
	al, ok := expr.(*ArrayLit)
	if !ok {
		t.Fatalf("expected ArrayLit, got %T", expr)
	}
	if len(al.Elems) != 3 {
		t.Fatalf("expected 3 elems, got %d", len(al.Elems))
	}
}

func TestParseEmptyArray(t *testing.T) {
	expr := parseExprOK(t, "[]")
	al, ok := expr.(*ArrayLit)
	if !ok {
		t.Fatalf("expected ArrayLit, got %T", expr)
	}
	if len(al.Elems) != 0 {
		t.Fatalf("expected 0 elems, got %d", len(al.Elems))
	}
}

// ---------------------------------------------------------------------------
// Block expression
// ---------------------------------------------------------------------------

func TestParseBlockExpr(t *testing.T) {
	expr := parseExprOK(t, "{ 42 }")
	bl, ok := expr.(*Block)
	if !ok {
		t.Fatalf("expected Block, got %T", expr)
	}
	if bl.TrailingExpr == nil {
		t.Fatal("expected trailing expr")
	}
	assertIntLit(t, bl.TrailingExpr, "42")
}

func TestParseBlockWithStatements(t *testing.T) {
	expr := parseExprOK(t, "{ let x = 1; x }")
	bl, ok := expr.(*Block)
	if !ok {
		t.Fatalf("expected Block, got %T", expr)
	}
	if len(bl.Stmts) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(bl.Stmts))
	}
	if bl.TrailingExpr == nil {
		t.Fatal("expected trailing expr")
	}
	assertIdent(t, bl.TrailingExpr, "x")
}

func TestParseBlockNoTrailing(t *testing.T) {
	expr := parseExprOK(t, "{ foo(); }")
	bl, ok := expr.(*Block)
	if !ok {
		t.Fatalf("expected Block, got %T", expr)
	}
	if len(bl.Stmts) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(bl.Stmts))
	}
	if bl.TrailingExpr != nil {
		t.Fatal("expected no trailing expr")
	}
}

// ---------------------------------------------------------------------------
// If expression
// ---------------------------------------------------------------------------

func TestParseIfExpr(t *testing.T) {
	expr := parseExprOK(t, "if x { 1 } else { 2 }")
	ie, ok := expr.(*IfExpr)
	if !ok {
		t.Fatalf("expected IfExpr, got %T", expr)
	}
	assertIdent(t, ie.Cond, "x")
	if ie.Then.TrailingExpr == nil {
		t.Fatal("expected trailing in then")
	}
	assertIntLit(t, ie.Then.TrailingExpr, "1")
	elseBlk, ok := ie.Else.(*Block)
	if !ok {
		t.Fatalf("expected Block in else, got %T", ie.Else)
	}
	assertIntLit(t, elseBlk.TrailingExpr, "2")
}

func TestParseIfElseIf(t *testing.T) {
	expr := parseExprOK(t, "if a { 1 } else if b { 2 } else { 3 }")
	ie, ok := expr.(*IfExpr)
	if !ok {
		t.Fatalf("expected IfExpr, got %T", expr)
	}
	elseIf, ok := ie.Else.(*IfExpr)
	if !ok {
		t.Fatalf("expected IfExpr in else, got %T", ie.Else)
	}
	assertIdent(t, elseIf.Cond, "b")
}

func TestParseIfNoElse(t *testing.T) {
	expr := parseExprOK(t, "if x { 1 }")
	ie, ok := expr.(*IfExpr)
	if !ok {
		t.Fatalf("expected IfExpr, got %T", expr)
	}
	if ie.Else != nil {
		t.Fatal("expected no else")
	}
}

// ---------------------------------------------------------------------------
// Match expression
// ---------------------------------------------------------------------------

func TestParseMatchExpr(t *testing.T) {
	src := `match x {
		1 => a,
		2 => b,
	}`
	expr := parseExprOK(t, src)
	me, ok := expr.(*MatchExpr)
	if !ok {
		t.Fatalf("expected MatchExpr, got %T", expr)
	}
	if len(me.Arms) != 2 {
		t.Fatalf("expected 2 arms, got %d", len(me.Arms))
	}
}

func TestParseMatchWithGuard(t *testing.T) {
	src := `match x {
		n if n > 0 => n,
		_ => 0,
	}`
	expr := parseExprOK(t, src)
	me, ok := expr.(*MatchExpr)
	if !ok {
		t.Fatalf("expected MatchExpr, got %T", expr)
	}
	if me.Arms[0].Guard == nil {
		t.Fatal("expected guard on first arm")
	}
	if me.Arms[1].Guard != nil {
		t.Fatal("expected no guard on second arm")
	}
}

func TestParseMatchVariantPattern(t *testing.T) {
	src := `match opt {
		Some(v) => v,
		None => 0,
	}`
	expr := parseExprOK(t, src)
	me := expr.(*MatchExpr)
	vp, ok := me.Arms[0].Pattern.(*VariantPat)
	if !ok {
		t.Fatalf("expected VariantPat, got %T", me.Arms[0].Pattern)
	}
	if vp.Name != "Some" {
		t.Fatalf("expected Some, got %s", vp.Name)
	}
	if len(vp.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(vp.Fields))
	}
}

func TestParseMatchTuplePattern(t *testing.T) {
	src := `match pair {
		(a, b) => a,
	}`
	expr := parseExprOK(t, src)
	me := expr.(*MatchExpr)
	tp, ok := me.Arms[0].Pattern.(*TuplePat)
	if !ok {
		t.Fatalf("expected TuplePat, got %T", me.Arms[0].Pattern)
	}
	if len(tp.Elems) != 2 {
		t.Fatalf("expected 2 elems, got %d", len(tp.Elems))
	}
}

func TestParseMatchRangePattern(t *testing.T) {
	src := `match n { 1..=10 => a, _ => b }`
	expr := parseExprOK(t, src)
	me := expr.(*MatchExpr)
	rp, ok := me.Arms[0].Pattern.(*RangePat)
	if !ok {
		t.Fatalf("expected RangePat, got %T", me.Arms[0].Pattern)
	}
	if !rp.Inclusive {
		t.Fatal("expected inclusive range")
	}
}

// ---------------------------------------------------------------------------
// For / While / Loop
// ---------------------------------------------------------------------------

func TestParseForExpr(t *testing.T) {
	expr := parseExprOK(t, "for i in 0..10 { x }")
	fe, ok := expr.(*ForExpr)
	if !ok {
		t.Fatalf("expected ForExpr, got %T", expr)
	}
	if fe.Binding != "i" {
		t.Fatalf("expected binding i, got %s", fe.Binding)
	}
}

func TestParseWhileExpr(t *testing.T) {
	expr := parseExprOK(t, "while x { y; }")
	we, ok := expr.(*WhileExpr)
	if !ok {
		t.Fatalf("expected WhileExpr, got %T", expr)
	}
	assertIdent(t, we.Cond, "x")
}

func TestParseLoopExpr(t *testing.T) {
	expr := parseExprOK(t, "loop { break; }")
	le, ok := expr.(*LoopExpr)
	if !ok {
		t.Fatalf("expected LoopExpr, got %T", expr)
	}
	if len(le.Body.Stmts) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(le.Body.Stmts))
	}
}

// ---------------------------------------------------------------------------
// Spawn / Channel
// ---------------------------------------------------------------------------

func TestParseSpawnExpr(t *testing.T) {
	expr := parseExprOK(t, "spawn { foo(); }")
	se, ok := expr.(*SpawnExpr)
	if !ok {
		t.Fatalf("expected SpawnExpr, got %T", expr)
	}
	if len(se.Body.Stmts) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(se.Body.Stmts))
	}
}

func TestParseChannelExpr(t *testing.T) {
	expr := parseExprOK(t, "channel<Int>(10)")
	ce, ok := expr.(*ChannelExpr)
	if !ok {
		t.Fatalf("expected ChannelExpr, got %T", expr)
	}
	nt, ok := ce.ElemType.(*NamedType)
	if !ok {
		t.Fatalf("expected NamedType, got %T", ce.ElemType)
	}
	if nt.Name != "Int" {
		t.Fatalf("expected Int, got %s", nt.Name)
	}
	assertIntLit(t, ce.Size, "10")
}

// ---------------------------------------------------------------------------
// Break / Continue / Return expressions
// ---------------------------------------------------------------------------

func TestParseBreak(t *testing.T) {
	expr := parseExprOK(t, "break")
	if _, ok := expr.(*BreakExpr); !ok {
		t.Fatalf("expected BreakExpr, got %T", expr)
	}
}

func TestParseContinue(t *testing.T) {
	expr := parseExprOK(t, "continue")
	if _, ok := expr.(*ContinueExpr); !ok {
		t.Fatalf("expected ContinueExpr, got %T", expr)
	}
}

// ---------------------------------------------------------------------------
// Struct literal
// ---------------------------------------------------------------------------

func TestParseStructLit(t *testing.T) {
	expr := parseExprOK(t, `Point { x: 1, y: 2 }`)
	sl, ok := expr.(*StructLit)
	if !ok {
		t.Fatalf("expected StructLit, got %T", expr)
	}
	if sl.Name != "Point" {
		t.Fatalf("expected Point, got %s", sl.Name)
	}
	if len(sl.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(sl.Fields))
	}
}

func TestParseStructLitEmpty(t *testing.T) {
	expr := parseExprOK(t, `Unit { }`)
	sl, ok := expr.(*StructLit)
	if !ok {
		t.Fatalf("expected StructLit, got %T", expr)
	}
	if len(sl.Fields) != 0 {
		t.Fatalf("expected 0 fields, got %d", len(sl.Fields))
	}
}

// ---------------------------------------------------------------------------
// Lambda expressions
// ---------------------------------------------------------------------------

func TestParseLambdaExpr(t *testing.T) {
	expr := parseExprOK(t, "|x: Int| x + 1")
	le, ok := expr.(*LambdaExpr)
	if !ok {
		t.Fatalf("expected LambdaExpr, got %T", expr)
	}
	if len(le.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(le.Params))
	}
	if le.Params[0].Name != "x" {
		t.Fatalf("expected param x, got %s", le.Params[0].Name)
	}
}

func TestParseLambdaWithReturn(t *testing.T) {
	expr := parseExprOK(t, "|x: Int| -> Int { x + 1 }")
	le, ok := expr.(*LambdaExpr)
	if !ok {
		t.Fatalf("expected LambdaExpr, got %T", expr)
	}
	if le.ReturnType == nil {
		t.Fatal("expected return type")
	}
}

func TestParseLambdaMultiParam(t *testing.T) {
	expr := parseExprOK(t, "|a, b| a + b")
	le, ok := expr.(*LambdaExpr)
	if !ok {
		t.Fatalf("expected LambdaExpr, got %T", expr)
	}
	if len(le.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(le.Params))
	}
}

func TestParseZeroParamLambda(t *testing.T) {
	expr := parseExprOK(t, "|| 42")
	le, ok := expr.(*LambdaExpr)
	if !ok {
		t.Fatalf("expected LambdaExpr, got %T", expr)
	}
	if len(le.Params) != 0 {
		t.Fatalf("expected 0 params, got %d", len(le.Params))
	}
}

// ---------------------------------------------------------------------------
// As expression
// ---------------------------------------------------------------------------

func TestParseAsExpr(t *testing.T) {
	expr := parseExprOK(t, "x as Int")
	ae, ok := expr.(*AsExpr)
	if !ok {
		t.Fatalf("expected AsExpr, got %T", expr)
	}
	assertIdent(t, ae.Expr, "x")
}

// ---------------------------------------------------------------------------
// Path expression
// ---------------------------------------------------------------------------

func TestParsePathExpr(t *testing.T) {
	expr := parseExprOK(t, "std::collections::HashMap")
	pe, ok := expr.(*PathExpr)
	if !ok {
		t.Fatalf("expected PathExpr, got %T", expr)
	}
	if len(pe.Segments) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(pe.Segments))
	}
	if pe.Segments[0] != "std" || pe.Segments[2] != "HashMap" {
		t.Fatalf("unexpected segments: %v", pe.Segments)
	}
}

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

func TestParseLetStmt(t *testing.T) {
	res := parseOK(t, "fn main() { let x = 42; }")
	fn := res.Program.Items[0].(*FnDef)
	ls, ok := fn.Body.Stmts[0].(*LetStmt)
	if !ok {
		t.Fatalf("expected LetStmt, got %T", fn.Body.Stmts[0])
	}
	if ls.Name != "x" {
		t.Fatalf("expected x, got %s", ls.Name)
	}
	if ls.Mutable {
		t.Fatal("expected immutable")
	}
}

func TestParseLetMut(t *testing.T) {
	res := parseOK(t, "fn main() { let mut x = 0; }")
	fn := res.Program.Items[0].(*FnDef)
	ls := fn.Body.Stmts[0].(*LetStmt)
	if !ls.Mutable {
		t.Fatal("expected mutable")
	}
}

func TestParseLetWithType(t *testing.T) {
	res := parseOK(t, "fn main() { let x: Int = 42; }")
	fn := res.Program.Items[0].(*FnDef)
	ls := fn.Body.Stmts[0].(*LetStmt)
	if ls.Type == nil {
		t.Fatal("expected type annotation")
	}
	nt, ok := ls.Type.(*NamedType)
	if !ok {
		t.Fatalf("expected NamedType, got %T", ls.Type)
	}
	if nt.Name != "Int" {
		t.Fatalf("expected Int, got %s", nt.Name)
	}
}

func TestParseReturnStmt(t *testing.T) {
	res := parseOK(t, "fn main() { return 42; }")
	fn := res.Program.Items[0].(*FnDef)
	rs, ok := fn.Body.Stmts[0].(*ReturnStmt)
	if !ok {
		t.Fatalf("expected ReturnStmt, got %T", fn.Body.Stmts[0])
	}
	assertIntLit(t, rs.Value, "42")
}

func TestParseReturnBare(t *testing.T) {
	res := parseOK(t, "fn main() { return; }")
	fn := res.Program.Items[0].(*FnDef)
	rs := fn.Body.Stmts[0].(*ReturnStmt)
	if rs.Value != nil {
		t.Fatal("expected nil value for bare return")
	}
}

func TestParseExprStmt(t *testing.T) {
	res := parseOK(t, "fn main() { foo(); }")
	fn := res.Program.Items[0].(*FnDef)
	es, ok := fn.Body.Stmts[0].(*ExprStmt)
	if !ok {
		t.Fatalf("expected ExprStmt, got %T", fn.Body.Stmts[0])
	}
	if _, ok := es.Expr.(*CallExpr); !ok {
		t.Fatalf("expected CallExpr, got %T", es.Expr)
	}
}

// ---------------------------------------------------------------------------
// Function definitions
// ---------------------------------------------------------------------------

func TestParseFnDef(t *testing.T) {
	res := parseOK(t, "fn add(a: Int, b: Int) -> Int { a + b }")
	fn := res.Program.Items[0].(*FnDef)
	if fn.Name != "add" {
		t.Fatalf("expected add, got %s", fn.Name)
	}
	if len(fn.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(fn.Params))
	}
	if fn.ReturnType == nil {
		t.Fatal("expected return type")
	}
}

func TestParseFnDefNoReturn(t *testing.T) {
	res := parseOK(t, "fn greet(name: String) { println(name); }")
	fn := res.Program.Items[0].(*FnDef)
	if fn.ReturnType != nil {
		t.Fatal("expected no return type")
	}
}

func TestParsePubFn(t *testing.T) {
	res := parseOK(t, "pub fn main() { }")
	fn := res.Program.Items[0].(*FnDef)
	if !fn.Public {
		t.Fatal("expected public")
	}
}

func TestParseFnGeneric(t *testing.T) {
	res := parseOK(t, "fn id<T>(x: T) -> T { x }")
	fn := res.Program.Items[0].(*FnDef)
	if fn.GenParams == nil {
		t.Fatal("expected generic params")
	}
	if len(fn.GenParams.Params) != 1 {
		t.Fatalf("expected 1 generic param, got %d", len(fn.GenParams.Params))
	}
	if fn.GenParams.Params[0].Name != "T" {
		t.Fatalf("expected T, got %s", fn.GenParams.Params[0].Name)
	}
}

func TestParseFnGenericBounds(t *testing.T) {
	res := parseOK(t, "fn sum<T: Summable>(a: T, b: T) -> T { a + b }")
	fn := res.Program.Items[0].(*FnDef)
	gp := fn.GenParams.Params[0]
	if len(gp.Bounds) != 1 {
		t.Fatalf("expected 1 bound, got %d", len(gp.Bounds))
	}
}

func TestParseFnSelfParam(t *testing.T) {
	// Simulate a method-like signature.
	res := parseOK(t, "fn method(self, x: Int) -> Int { x }")
	fn := res.Program.Items[0].(*FnDef)
	if fn.Params[0].Name != "self" {
		t.Fatalf("expected self param, got %s", fn.Params[0].Name)
	}
}

// ---------------------------------------------------------------------------
// Type definitions (ADT)
// ---------------------------------------------------------------------------

func TestParseTypeDef(t *testing.T) {
	src := `type Option<T> {
		Some(T),
		None,
	}`
	res := parseOK(t, src)
	td := res.Program.Items[0].(*TypeDef)
	if td.Name != "Option" {
		t.Fatalf("expected Option, got %s", td.Name)
	}
	if len(td.Variants) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(td.Variants))
	}
	if td.Variants[0].Name != "Some" {
		t.Fatalf("expected Some, got %s", td.Variants[0].Name)
	}
	if len(td.Variants[0].Fields) != 1 {
		t.Fatalf("expected 1 field on Some, got %d", len(td.Variants[0].Fields))
	}
	if td.Variants[1].Name != "None" {
		t.Fatalf("expected None, got %s", td.Variants[1].Name)
	}
}

func TestParseTypeDefMultiField(t *testing.T) {
	src := `type Tree<T> {
		Leaf(T),
		Node(Tree<T>, T, Tree<T>),
	}`
	res := parseOK(t, src)
	td := res.Program.Items[0].(*TypeDef)
	if len(td.Variants[1].Fields) != 3 {
		t.Fatalf("expected 3 fields on Node, got %d", len(td.Variants[1].Fields))
	}
}

// ---------------------------------------------------------------------------
// Struct definitions
// ---------------------------------------------------------------------------

func TestParseStructDef(t *testing.T) {
	src := `struct Point {
		x: Float,
		y: Float,
	}`
	res := parseOK(t, src)
	sd := res.Program.Items[0].(*StructDef)
	if sd.Name != "Point" {
		t.Fatalf("expected Point, got %s", sd.Name)
	}
	if len(sd.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(sd.Fields))
	}
}

func TestParseStructGeneric(t *testing.T) {
	src := `struct Pair<A, B> { first: A, second: B }`
	res := parseOK(t, src)
	sd := res.Program.Items[0].(*StructDef)
	if sd.GenParams == nil || len(sd.GenParams.Params) != 2 {
		t.Fatal("expected 2 generic params")
	}
}

// ---------------------------------------------------------------------------
// Trait definitions
// ---------------------------------------------------------------------------

func TestParseTraitDef(t *testing.T) {
	src := `trait Summable {
		fn zero() -> Self;
		fn add(self, other: Self) -> Self;
	}`
	res := parseOK(t, src)
	td := res.Program.Items[0].(*TraitDef)
	if td.Name != "Summable" {
		t.Fatalf("expected Summable, got %s", td.Name)
	}
	if len(td.Methods) != 2 {
		t.Fatalf("expected 2 methods, got %d", len(td.Methods))
	}
	if td.Methods[0].Name != "zero" {
		t.Fatalf("expected zero, got %s", td.Methods[0].Name)
	}
	if td.Methods[0].Body != nil {
		t.Fatal("expected nil body for trait method signature")
	}
}

// ---------------------------------------------------------------------------
// Impl blocks
// ---------------------------------------------------------------------------

func TestParseImplBlock(t *testing.T) {
	src := `impl Summable for Int {
		fn zero() -> Int { 0 }
		fn add(self, other: Int) -> Int { self + other }
	}`
	res := parseOK(t, src)
	ib := res.Program.Items[0].(*ImplBlock)
	if ib.TraitName != "Summable" {
		t.Fatalf("expected Summable, got %s", ib.TraitName)
	}
	if len(ib.Methods) != 2 {
		t.Fatalf("expected 2 methods, got %d", len(ib.Methods))
	}
}

func TestParseImplInherent(t *testing.T) {
	src := `impl Point {
		fn new(x: Float, y: Float) -> Point {
			Point { x: x, y: y }
		}
	}`
	res := parseOK(t, src)
	ib := res.Program.Items[0].(*ImplBlock)
	if ib.TraitName != "" {
		t.Fatalf("expected empty trait name, got %s", ib.TraitName)
	}
	if len(ib.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(ib.Methods))
	}
}

func TestParseImplGeneric(t *testing.T) {
	src := `impl<T: Summable> Summable for Option<T> {
		fn zero() -> Option<T> { None }
		fn add(self, other: Option<T>) -> Option<T> { self }
	}`
	res := parseOK(t, src)
	ib := res.Program.Items[0].(*ImplBlock)
	if ib.GenParams == nil || len(ib.GenParams.Params) != 1 {
		t.Fatal("expected 1 generic param on impl")
	}
}

// ---------------------------------------------------------------------------
// Import / Module
// ---------------------------------------------------------------------------

func TestParseImport(t *testing.T) {
	res := parseOK(t, "import std::io;")
	id := res.Program.Items[0].(*ImportDecl)
	if len(id.Path) != 2 || id.Path[0] != "std" || id.Path[1] != "io" {
		t.Fatalf("unexpected path: %v", id.Path)
	}
}

func TestParseImportAlias(t *testing.T) {
	res := parseOK(t, "import std::collections as col;")
	id := res.Program.Items[0].(*ImportDecl)
	if id.Alias != "col" {
		t.Fatalf("expected alias col, got %s", id.Alias)
	}
}

func TestParseModule(t *testing.T) {
	res := parseOK(t, "module mymod;")
	md := res.Program.Items[0].(*ModuleDecl)
	if md.Name != "mymod" {
		t.Fatalf("expected mymod, got %s", md.Name)
	}
}

// ---------------------------------------------------------------------------
// Type expressions
// ---------------------------------------------------------------------------

func TestParseNamedType(t *testing.T) {
	res := parseOK(t, "fn f(x: Int) { }")
	fn := res.Program.Items[0].(*FnDef)
	nt, ok := fn.Params[0].Type.(*NamedType)
	if !ok {
		t.Fatalf("expected NamedType, got %T", fn.Params[0].Type)
	}
	if nt.Name != "Int" {
		t.Fatalf("expected Int, got %s", nt.Name)
	}
}

func TestParseGenericType(t *testing.T) {
	res := parseOK(t, "fn f(x: Option<Int>) { }")
	fn := res.Program.Items[0].(*FnDef)
	nt := fn.Params[0].Type.(*NamedType)
	if nt.Name != "Option" {
		t.Fatalf("expected Option, got %s", nt.Name)
	}
	if len(nt.GenArgs) != 1 {
		t.Fatalf("expected 1 generic arg, got %d", len(nt.GenArgs))
	}
}

func TestParseFnType(t *testing.T) {
	res := parseOK(t, "fn f(cb: (Int, Int) -> Bool) { }")
	fn := res.Program.Items[0].(*FnDef)
	ft, ok := fn.Params[0].Type.(*FnType)
	if !ok {
		t.Fatalf("expected FnType, got %T", fn.Params[0].Type)
	}
	if len(ft.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(ft.Params))
	}
}

func TestParseTupleType(t *testing.T) {
	res := parseOK(t, "fn f(x: (Int, String)) { }")
	fn := res.Program.Items[0].(*FnDef)
	tt, ok := fn.Params[0].Type.(*TupleType)
	if !ok {
		// Could be interpreted as fn type without ->.
		// Check if it's a TupleType or something else.
		t.Fatalf("expected TupleType, got %T", fn.Params[0].Type)
	}
	if len(tt.Elems) != 2 {
		t.Fatalf("expected 2 elems, got %d", len(tt.Elems))
	}
}

func TestParseArrayType(t *testing.T) {
	res := parseOK(t, "fn f(x: [Int]) { }")
	fn := res.Program.Items[0].(*FnDef)
	at, ok := fn.Params[0].Type.(*ArrayType)
	if !ok {
		t.Fatalf("expected ArrayType, got %T", fn.Params[0].Type)
	}
	if at.Elem.(*NamedType).Name != "Int" {
		t.Fatalf("expected Int, got %s", at.Elem.(*NamedType).Name)
	}
}

func TestParseSelfType(t *testing.T) {
	src := `trait Foo { fn bar() -> Self; }`
	res := parseOK(t, src)
	td := res.Program.Items[0].(*TraitDef)
	_, ok := td.Methods[0].ReturnType.(*SelfType)
	if !ok {
		t.Fatalf("expected SelfType, got %T", td.Methods[0].ReturnType)
	}
}

// ---------------------------------------------------------------------------
// Span preservation
// ---------------------------------------------------------------------------

func TestSpanPreservation(t *testing.T) {
	src := "42"
	expr, errs := ParseExpr(src, 0)
	if len(errs) > 0 {
		t.FailNow()
	}
	span := expr.Span()
	if span.Start != 0 || span.End != 2 {
		t.Errorf("expected span [0..2), got [%d..%d)", span.Start, span.End)
	}
}

func TestSpanBinaryExpr(t *testing.T) {
	src := "1 + 2"
	expr, errs := ParseExpr(src, 0)
	if len(errs) > 0 {
		t.FailNow()
	}
	span := expr.Span()
	if span.Start != 0 || span.End != 5 {
		t.Errorf("expected span [0..5), got [%d..%d)", span.Start, span.End)
	}
}

func TestSpanFnDef(t *testing.T) {
	src := "fn main() { }"
	res := parseOK(t, src)
	fn := res.Program.Items[0].(*FnDef)
	span := fn.Span()
	if span.Start != 0 || span.End != len(src) {
		t.Errorf("expected span [0..%d), got [%d..%d)", len(src), span.Start, span.End)
	}
}

// ---------------------------------------------------------------------------
// Error recovery
// ---------------------------------------------------------------------------

func TestErrorRecoveryMissingSemicolon(t *testing.T) {
	// Missing semicolons before } should auto-insert.
	res := Parse("fn main() { let x = 42 }", 0)
	// Should parse successfully with auto-inserted semicolon before }.
	if res.HasErrors() {
		t.Logf("got %d errors (some may be expected)", len(res.Errors))
	}
	fn := res.Program.Items[0].(*FnDef)
	if fn.Body == nil {
		t.Fatal("expected body")
	}
}

func TestErrorRecoveryUnexpectedToken(t *testing.T) {
	// Missing closing paren — parser should produce an error.
	res := expectErrors(t, "fn main( { }", 1)
	if res.Program == nil {
		t.Fatal("expected program even with errors")
	}
}

func TestErrorRecoverySyncOnFn(t *testing.T) {
	// After error in first fn, parser should recover and parse second fn.
	src := `fn bad( { }
fn good() { }`
	res := Parse(src, 0)
	if !res.HasErrors() {
		t.Fatal("expected errors")
	}
	// Should have parsed at least one item.
	if len(res.Program.Items) == 0 {
		t.Fatal("expected at least one item after recovery")
	}
}

func TestErrorMaxErrors(t *testing.T) {
	// Generate many errors.
	src := ""
	for i := 0; i < 30; i++ {
		src += "@@@ "
	}
	src += "fn main() { }"
	res := Parse(src, 0)
	if len(res.Errors) > maxErrors {
		t.Errorf("expected at most %d errors, got %d", maxErrors, len(res.Errors))
	}
}

// ---------------------------------------------------------------------------
// Multiple items
// ---------------------------------------------------------------------------

func TestParseMultipleItems(t *testing.T) {
	src := `
fn foo() -> Int { 1 }
fn bar() -> Int { 2 }
struct Point { x: Float, y: Float }
`
	res := parseOK(t, src)
	if len(res.Program.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(res.Program.Items))
	}
}

// ---------------------------------------------------------------------------
// Complex expressions
// ---------------------------------------------------------------------------

func TestParseComplexExpr(t *testing.T) {
	// a.b(c + d).e[0]
	expr := parseExprOK(t, "a.b(c + d).e[0]")
	idx, ok := expr.(*IndexExpr)
	if !ok {
		t.Fatalf("expected IndexExpr, got %T", expr)
	}
	fe, ok := idx.Object.(*FieldExpr)
	if !ok {
		t.Fatalf("expected FieldExpr, got %T", idx.Object)
	}
	if fe.Field != "e" {
		t.Fatalf("expected field e, got %s", fe.Field)
	}
	call, ok := fe.Object.(*CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", fe.Object)
	}
	if len(call.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(call.Args))
	}
}

func TestParseNestedBlocks(t *testing.T) {
	src := `fn main() {
		let x = {
			let y = 1;
			y + 1
		};
	}`
	res := parseOK(t, src)
	fn := res.Program.Items[0].(*FnDef)
	ls := fn.Body.Stmts[0].(*LetStmt)
	blk, ok := ls.Value.(*Block)
	if !ok {
		t.Fatalf("expected Block, got %T", ls.Value)
	}
	if blk.TrailingExpr == nil {
		t.Fatal("expected trailing expr in inner block")
	}
}

func TestParseSampleProgram(t *testing.T) {
	src := `
type Tree<T> {
	Leaf(T),
	Node(Tree<T>, T, Tree<T>),
}

trait Summable {
	fn zero() -> Self;
	fn add(self, other: Self) -> Self;
}

impl Summable for Int {
	fn zero() -> Int { 0 }
	fn add(self, other: Int) -> Int { self + other }
}

fn sum_tree<T: Summable>(tree: Tree<T>) -> T {
	match tree {
		Leaf(val) => val,
		Node(left, val, right) => {
			let l = sum_tree(left);
			let r = sum_tree(right);
			l.add(val).add(r)
		}
	}
}

fn main() {
	let tree = Node(
		Node(Leaf(1), 2, Leaf(3)),
		4,
		Node(Leaf(5), 6, Leaf(7)),
	);
	let total = sum_tree(tree);
	println(total);
}
`
	res := parseOK(t, src)
	if len(res.Program.Items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(res.Program.Items))
	}
}

// ---------------------------------------------------------------------------
// Concurrency full example
// ---------------------------------------------------------------------------

func TestParseConcurrencyExample(t *testing.T) {
	src := `fn main() {
		let ch = channel<Int>(10);
		spawn {
			ch.send(42);
			ch.close();
		};
		for val in ch {
			println(val);
		};
	}`
	res := parseOK(t, src)
	fn := res.Program.Items[0].(*FnDef)
	if len(fn.Body.Stmts) < 3 {
		t.Fatalf("expected at least 3 stmts, got %d", len(fn.Body.Stmts))
	}
}

// ---------------------------------------------------------------------------
// Pattern: struct pattern
// ---------------------------------------------------------------------------

func TestParseStructPattern(t *testing.T) {
	src := `match p { Point { x: a, y: b } => a + b }`
	expr := parseExprOK(t, src)
	me := expr.(*MatchExpr)
	sp, ok := me.Arms[0].Pattern.(*StructPat)
	if !ok {
		t.Fatalf("expected StructPat, got %T", me.Arms[0].Pattern)
	}
	if sp.Name != "Point" {
		t.Fatalf("expected Point, got %s", sp.Name)
	}
	if len(sp.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(sp.Fields))
	}
}

// ---------------------------------------------------------------------------
// Pattern: negative literal
// ---------------------------------------------------------------------------

func TestParseNegativeLiteralPattern(t *testing.T) {
	src := `match n { -1 => a, _ => b }`
	expr := parseExprOK(t, src)
	me := expr.(*MatchExpr)
	lp, ok := me.Arms[0].Pattern.(*LiteralPat)
	if !ok {
		t.Fatalf("expected LiteralPat, got %T", me.Arms[0].Pattern)
	}
	il, ok := lp.Value.(*IntLit)
	if !ok {
		t.Fatalf("expected IntLit, got %T", lp.Value)
	}
	if il.Value != "-1" {
		t.Fatalf("expected -1, got %s", il.Value)
	}
}

// ---------------------------------------------------------------------------
// Pattern: wildcard
// ---------------------------------------------------------------------------

func TestParseWildcardPattern(t *testing.T) {
	src := `match x { _ => 0 }`
	expr := parseExprOK(t, src)
	me := expr.(*MatchExpr)
	_, ok := me.Arms[0].Pattern.(*WildcardPat)
	if !ok {
		t.Fatalf("expected WildcardPat, got %T", me.Arms[0].Pattern)
	}
}

// ---------------------------------------------------------------------------
// FnType expression with fn keyword
// ---------------------------------------------------------------------------

func TestParseFnTypeKeyword(t *testing.T) {
	res := parseOK(t, "fn f(cb: fn(Int) -> Bool) { }")
	fn := res.Program.Items[0].(*FnDef)
	ft, ok := fn.Params[0].Type.(*FnType)
	if !ok {
		t.Fatalf("expected FnType, got %T", fn.Params[0].Type)
	}
	if len(ft.Params) != 1 {
		t.Fatalf("expected 1 param type, got %d", len(ft.Params))
	}
}

// ---------------------------------------------------------------------------
// Generic bounds with multiple constraints
// ---------------------------------------------------------------------------

func TestParseMultipleBounds(t *testing.T) {
	res := parseOK(t, "fn f<T: A + B + C>(x: T) { }")
	fn := res.Program.Items[0].(*FnDef)
	gp := fn.GenParams.Params[0]
	if len(gp.Bounds) != 3 {
		t.Fatalf("expected 3 bounds, got %d", len(gp.Bounds))
	}
}

// ---------------------------------------------------------------------------
// Trailing comma in various positions
// ---------------------------------------------------------------------------

func TestTrailingCommaParams(t *testing.T) {
	res := parseOK(t, "fn f(a: Int, b: Int,) { }")
	fn := res.Program.Items[0].(*FnDef)
	if len(fn.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(fn.Params))
	}
}

func TestTrailingCommaVariants(t *testing.T) {
	src := `type Color { Red, Green, Blue, }`
	res := parseOK(t, src)
	td := res.Program.Items[0].(*TypeDef)
	if len(td.Variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(td.Variants))
	}
}

func TestTrailingCommaFields(t *testing.T) {
	src := `struct Point { x: Float, y: Float, }`
	res := parseOK(t, src)
	sd := res.Program.Items[0].(*StructDef)
	if len(sd.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(sd.Fields))
	}
}

func TestTrailingCommaMatchArms(t *testing.T) {
	src := `match x { 1 => a, 2 => b, }`
	expr := parseExprOK(t, src)
	me := expr.(*MatchExpr)
	if len(me.Arms) != 2 {
		t.Fatalf("expected 2 arms, got %d", len(me.Arms))
	}
}

// ---------------------------------------------------------------------------
// Or-patterns
// ---------------------------------------------------------------------------

func TestParseOrPattern(t *testing.T) {
	src := `match x { 1 | 2 | 3 => true, _ => false }`
	expr := parseExprOK(t, src)
	me := expr.(*MatchExpr)
	op, ok := me.Arms[0].Pattern.(*OrPat)
	if !ok {
		t.Fatalf("expected OrPat, got %T", me.Arms[0].Pattern)
	}
	if len(op.Alts) != 3 {
		t.Fatalf("expected 3 alternatives, got %d", len(op.Alts))
	}
}

func TestParseOrPatternTwoAlts(t *testing.T) {
	src := `match x { true | false => 1 }`
	expr := parseExprOK(t, src)
	me := expr.(*MatchExpr)
	op, ok := me.Arms[0].Pattern.(*OrPat)
	if !ok {
		t.Fatalf("expected OrPat, got %T", me.Arms[0].Pattern)
	}
	if len(op.Alts) != 2 {
		t.Fatalf("expected 2 alternatives, got %d", len(op.Alts))
	}
}

func TestParseOrPatternWithGuard(t *testing.T) {
	src := `match x { 1 | 2 if x > 0 => x, _ => 0 }`
	expr := parseExprOK(t, src)
	me := expr.(*MatchExpr)
	if _, ok := me.Arms[0].Pattern.(*OrPat); !ok {
		t.Fatalf("expected OrPat, got %T", me.Arms[0].Pattern)
	}
	if me.Arms[0].Guard == nil {
		t.Fatal("expected guard on first arm")
	}
}

// ---------------------------------------------------------------------------
// Lambda edge cases
// ---------------------------------------------------------------------------

func TestParseLambdaWithBlock(t *testing.T) {
	expr := parseExprOK(t, "|x| { let y = x + 1; y }")
	le, ok := expr.(*LambdaExpr)
	if !ok {
		t.Fatalf("expected LambdaExpr, got %T", expr)
	}
	if _, ok := le.Body.(*Block); !ok {
		t.Fatalf("expected Block body, got %T", le.Body)
	}
}

func TestParseZeroParamLambdaWithReturn(t *testing.T) {
	expr := parseExprOK(t, "|| -> Int { 42 }")
	le, ok := expr.(*LambdaExpr)
	if !ok {
		t.Fatalf("expected LambdaExpr, got %T", expr)
	}
	if le.ReturnType == nil {
		t.Fatal("expected return type")
	}
}

// ---------------------------------------------------------------------------
// Error recovery — additional tests
// ---------------------------------------------------------------------------

func TestErrorPubOnImpl(t *testing.T) {
	expectErrors(t, "pub impl Foo {}", 1)
}

func TestErrorPubOnImport(t *testing.T) {
	expectErrors(t, "pub import foo;", 1)
}

func TestErrorPubOnModule(t *testing.T) {
	expectErrors(t, "pub module foo;", 1)
}

func TestErrorAutoInsertSemicolonBeforeBrace(t *testing.T) {
	// Semicolon auto-insert before }
	res := parseOK(t, "fn main() { let x = 1 }")
	fn := res.Program.Items[0].(*FnDef)
	if len(fn.Body.Stmts) < 1 {
		t.Fatal("expected at least 1 stmt")
	}
}

func TestParseReturnExpr(t *testing.T) {
	expr := parseExprOK(t, "return 42")
	re, ok := expr.(*ReturnExpr)
	if !ok {
		t.Fatalf("expected ReturnExpr, got %T", expr)
	}
	if re.Value == nil {
		t.Fatal("expected return value")
	}
}

func TestParseBoolPatterns(t *testing.T) {
	src := `match b { true => 1, false => 0 }`
	expr := parseExprOK(t, src)
	me := expr.(*MatchExpr)
	lp0, ok := me.Arms[0].Pattern.(*LiteralPat)
	if !ok {
		t.Fatalf("expected LiteralPat, got %T", me.Arms[0].Pattern)
	}
	bl := lp0.Value.(*BoolLit)
	if !bl.Value {
		t.Fatal("expected true")
	}
}

func TestParseStringPattern(t *testing.T) {
	src := `match s { "hello" => 1, _ => 0 }`
	expr := parseExprOK(t, src)
	me := expr.(*MatchExpr)
	lp, ok := me.Arms[0].Pattern.(*LiteralPat)
	if !ok {
		t.Fatalf("expected LiteralPat, got %T", me.Arms[0].Pattern)
	}
	if _, ok := lp.Value.(*StringLit); !ok {
		t.Fatalf("expected StringLit, got %T", lp.Value)
	}
}

func TestParseChannelNoTypeArg(t *testing.T) {
	expr := parseExprOK(t, "channel()")
	ch, ok := expr.(*ChannelExpr)
	if !ok {
		t.Fatalf("expected ChannelExpr, got %T", expr)
	}
	if ch.ElemType != nil {
		t.Fatal("expected nil element type")
	}
}

func TestParsePipeChain(t *testing.T) {
	// a |> b |> c → (a |> b) |> c
	expr := parseExprOK(t, "a |> b |> c")
	top := assertBinary(t, expr, lexer.PIPE)
	assertBinary(t, top.Left, lexer.PIPE)
	assertIdent(t, top.Right, "c")
}

func TestParseImplPubMethod(t *testing.T) {
	src := `impl Foo { pub fn bar(self) {} }`
	res := parseOK(t, src)
	impl := res.Program.Items[0].(*ImplBlock)
	if !impl.Methods[0].Public {
		t.Fatal("expected public method")
	}
}

func TestParseTraitDefaultMethod(t *testing.T) {
	src := `trait Greet { fn greet(self) -> String { "hello" } }`
	res := parseOK(t, src)
	td := res.Program.Items[0].(*TraitDef)
	if td.Methods[0].Body == nil {
		t.Fatal("expected body for default method")
	}
}

func TestParseStructFieldVisibility(t *testing.T) {
	src := `struct Foo { pub name: String, age: Int }`
	res := parseOK(t, src)
	sd := res.Program.Items[0].(*StructDef)
	if !sd.Fields[0].Public {
		t.Fatal("expected first field to be public")
	}
	if sd.Fields[1].Public {
		t.Fatal("expected second field to be private")
	}
}
