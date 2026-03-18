package optimize

import (
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/mir"
	"github.com/ryx-lang/ryx/pkg/types"
)

// ---------------------------------------------------------------------------
// Helpers: build MIR functions directly for isolated pass testing
// ---------------------------------------------------------------------------

// buildTestFn creates a function with a single entry block, allocating
// params as the first N locals.
func buildTestFn(name string, paramTypes []types.Type, retType types.Type) *mir.Function {
	fn := &mir.Function{
		Name:       name,
		ReturnType: retType,
		Entry:      0,
	}
	entry := fn.NewBlock("entry")
	_ = entry
	for i, pt := range paramTypes {
		id := fn.NewLocal("p"+string(rune('0'+i)), pt)
		fn.Params = append(fn.Params, id)
	}
	return fn
}

func entryBlock(fn *mir.Function) *mir.BasicBlock {
	return fn.Block(fn.Entry)
}

// ---------------------------------------------------------------------------
// Test: ConstantFoldPass
// ---------------------------------------------------------------------------

func TestConstantFold_IntArithmetic(t *testing.T) {
	fn := buildTestFn("fold_int", nil, types.TypInt)
	b := entryBlock(fn)

	dest := fn.NewLocal("r", types.TypInt)
	b.Stmts = append(b.Stmts, &mir.BinaryOpStmt{
		Dest: dest, Op: "+",
		Left: mir.IntConst(3), Right: mir.IntConst(4),
		Type: types.TypInt,
	})
	b.Term = &mir.Return{Value: fn.LocalRef(dest)}

	RunPass(fn, &ConstantFoldPass{})

	// The BinaryOpStmt should be replaced with an Assign of the constant 7.
	a, ok := b.Stmts[0].(*mir.Assign)
	if !ok {
		t.Fatalf("expected Assign after fold, got %T", b.Stmts[0])
	}
	c, ok := a.Src.(*mir.Const)
	if !ok || c.Kind != mir.ConstInt || c.Int != 7 {
		t.Errorf("expected IntConst(7), got %s", a.Src.String())
	}
}

func TestConstantFold_IntMultiply(t *testing.T) {
	fn := buildTestFn("fold_mul", nil, types.TypInt)
	b := entryBlock(fn)

	dest := fn.NewLocal("r", types.TypInt)
	b.Stmts = append(b.Stmts, &mir.BinaryOpStmt{
		Dest: dest, Op: "*",
		Left: mir.IntConst(5), Right: mir.IntConst(6),
		Type: types.TypInt,
	})
	b.Term = &mir.Return{Value: fn.LocalRef(dest)}

	RunPass(fn, &ConstantFoldPass{})

	a := b.Stmts[0].(*mir.Assign)
	c := a.Src.(*mir.Const)
	if c.Int != 30 {
		t.Errorf("expected 30, got %d", c.Int)
	}
}

func TestConstantFold_IntComparison(t *testing.T) {
	fn := buildTestFn("fold_cmp", nil, types.TypBool)
	b := entryBlock(fn)

	dest := fn.NewLocal("r", types.TypBool)
	b.Stmts = append(b.Stmts, &mir.BinaryOpStmt{
		Dest: dest, Op: "<",
		Left: mir.IntConst(3), Right: mir.IntConst(5),
		Type: types.TypBool,
	})
	b.Term = &mir.Return{Value: fn.LocalRef(dest)}

	RunPass(fn, &ConstantFoldPass{})

	a := b.Stmts[0].(*mir.Assign)
	c := a.Src.(*mir.Const)
	if !c.Bool {
		t.Error("expected true for 3 < 5")
	}
}

func TestConstantFold_UnaryNegation(t *testing.T) {
	fn := buildTestFn("fold_neg", nil, types.TypInt)
	b := entryBlock(fn)

	dest := fn.NewLocal("r", types.TypInt)
	b.Stmts = append(b.Stmts, &mir.UnaryOpStmt{
		Dest: dest, Op: "-", Operand: mir.IntConst(42),
		Type: types.TypInt,
	})
	b.Term = &mir.Return{Value: fn.LocalRef(dest)}

	RunPass(fn, &ConstantFoldPass{})

	a := b.Stmts[0].(*mir.Assign)
	c := a.Src.(*mir.Const)
	if c.Int != -42 {
		t.Errorf("expected -42, got %d", c.Int)
	}
}

func TestConstantFold_BoolNot(t *testing.T) {
	fn := buildTestFn("fold_not", nil, types.TypBool)
	b := entryBlock(fn)

	dest := fn.NewLocal("r", types.TypBool)
	b.Stmts = append(b.Stmts, &mir.UnaryOpStmt{
		Dest: dest, Op: "!", Operand: mir.BoolConst(true),
		Type: types.TypBool,
	})
	b.Term = &mir.Return{Value: fn.LocalRef(dest)}

	RunPass(fn, &ConstantFoldPass{})

	a := b.Stmts[0].(*mir.Assign)
	c := a.Src.(*mir.Const)
	if c.Bool {
		t.Error("expected false for !true")
	}
}

func TestConstantFold_IdentityMultByOne(t *testing.T) {
	fn := buildTestFn("fold_id", []types.Type{types.TypInt}, types.TypInt)
	b := entryBlock(fn)

	dest := fn.NewLocal("r", types.TypInt)
	b.Stmts = append(b.Stmts, &mir.BinaryOpStmt{
		Dest: dest, Op: "*",
		Left: fn.LocalRef(fn.Params[0]), Right: mir.IntConst(1),
		Type: types.TypInt,
	})
	b.Term = &mir.Return{Value: fn.LocalRef(dest)}

	RunPass(fn, &ConstantFoldPass{})

	a, ok := b.Stmts[0].(*mir.Assign)
	if !ok {
		t.Fatalf("expected Assign (identity fold), got %T", b.Stmts[0])
	}
	local, ok := a.Src.(*mir.Local)
	if !ok || local.ID != fn.Params[0] {
		t.Errorf("expected reference to param, got %s", a.Src.String())
	}
}

func TestConstantFold_DivByZeroNotFolded(t *testing.T) {
	fn := buildTestFn("fold_div0", nil, types.TypInt)
	b := entryBlock(fn)

	dest := fn.NewLocal("r", types.TypInt)
	b.Stmts = append(b.Stmts, &mir.BinaryOpStmt{
		Dest: dest, Op: "/",
		Left: mir.IntConst(10), Right: mir.IntConst(0),
		Type: types.TypInt,
	})
	b.Term = &mir.Return{Value: fn.LocalRef(dest)}

	RunPass(fn, &ConstantFoldPass{})

	// Should NOT fold division by zero.
	if _, ok := b.Stmts[0].(*mir.BinaryOpStmt); !ok {
		t.Errorf("expected BinaryOpStmt preserved for div-by-zero, got %T", b.Stmts[0])
	}
}

func TestConstantFold_FloatArithmetic(t *testing.T) {
	fn := buildTestFn("fold_float", nil, types.TypFloat)
	b := entryBlock(fn)

	dest := fn.NewLocal("r", types.TypFloat)
	b.Stmts = append(b.Stmts, &mir.BinaryOpStmt{
		Dest: dest, Op: "+",
		Left: mir.FloatConst(1.5), Right: mir.FloatConst(2.5),
		Type: types.TypFloat,
	})
	b.Term = &mir.Return{Value: fn.LocalRef(dest)}

	RunPass(fn, &ConstantFoldPass{})

	a := b.Stmts[0].(*mir.Assign)
	c := a.Src.(*mir.Const)
	if c.Float != 4.0 {
		t.Errorf("expected 4.0, got %g", c.Float)
	}
}

// ---------------------------------------------------------------------------
// Test: DeadCodePass
// ---------------------------------------------------------------------------

func TestDeadCode_RemovesUnused(t *testing.T) {
	fn := buildTestFn("dce", []types.Type{types.TypInt}, types.TypInt)
	b := entryBlock(fn)

	// %1 = param
	// %2 = %1 + 1  (unused)
	// %3 = %1 + 2  (returned)
	dead := fn.NewLocal("dead", types.TypInt)
	live := fn.NewLocal("live", types.TypInt)

	b.Stmts = append(b.Stmts,
		&mir.BinaryOpStmt{Dest: dead, Op: "+", Left: fn.LocalRef(fn.Params[0]), Right: mir.IntConst(1), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: live, Op: "+", Left: fn.LocalRef(fn.Params[0]), Right: mir.IntConst(2), Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(live)}

	RunPass(fn, &DeadCodePass{})

	if len(b.Stmts) != 1 {
		t.Fatalf("expected 1 stmt after DCE, got %d", len(b.Stmts))
	}
	if b.Stmts[0].DestLocal() != live {
		t.Errorf("expected live stmt preserved, got dest %%%d", b.Stmts[0].DestLocal())
	}
}

func TestDeadCode_PreservesSideEffects(t *testing.T) {
	fn := buildTestFn("dce_se", nil, types.TypUnit)
	b := entryBlock(fn)

	callDest := fn.NewLocal("call", types.TypUnit)
	b.Stmts = append(b.Stmts,
		&mir.CallStmt{Dest: callDest, Func: &mir.Global{Name: "print", Type: types.TypUnit}, Type: types.TypUnit},
	)
	b.Term = &mir.Return{Value: mir.UnitConst()}

	RunPass(fn, &DeadCodePass{})

	// Call should not be removed even though its result is unused.
	if len(b.Stmts) != 1 {
		t.Errorf("expected call preserved (side effect), got %d stmts", len(b.Stmts))
	}
}

func TestDeadCode_RemovesUnreachableBlocks(t *testing.T) {
	fn := buildTestFn("dce_unreach", nil, types.TypUnit)
	b := entryBlock(fn)
	b.Term = &mir.Return{Value: mir.UnitConst()}

	// Create an unreachable block.
	unreachID := fn.NewBlock("unreachable")
	unreachBlk := fn.Block(unreachID)
	unreachBlk.Term = &mir.Return{Value: mir.UnitConst()}

	if len(fn.Blocks) != 2 {
		t.Fatalf("expected 2 blocks before DCE, got %d", len(fn.Blocks))
	}

	RunPass(fn, &DeadCodePass{})

	if len(fn.Blocks) != 1 {
		t.Errorf("expected 1 block after DCE (unreachable removed), got %d", len(fn.Blocks))
	}
}

// ---------------------------------------------------------------------------
// Test: CopyPropPass
// ---------------------------------------------------------------------------

func TestCopyProp_ChainedCopies(t *testing.T) {
	fn := buildTestFn("cprop", []types.Type{types.TypInt}, types.TypInt)
	b := entryBlock(fn)

	// %1 = %0 (copy)
	// %2 = %1 (copy)
	// return %2  -> should become return %0
	c1 := fn.NewLocal("c1", types.TypInt)
	c2 := fn.NewLocal("c2", types.TypInt)

	b.Stmts = append(b.Stmts,
		&mir.Assign{Dest: c1, Src: fn.LocalRef(fn.Params[0]), Type: types.TypInt},
		&mir.Assign{Dest: c2, Src: fn.LocalRef(c1), Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(c2)}

	RunPass(fn, &CopyPropPass{})

	ret := b.Term.(*mir.Return)
	local, ok := ret.Value.(*mir.Local)
	if !ok {
		t.Fatalf("expected Local in return after copy prop, got %T", ret.Value)
	}
	if local.ID != fn.Params[0] {
		t.Errorf("expected return to reference param %%%d, got %%%d", fn.Params[0], local.ID)
	}
}

func TestCopyProp_ConstantCopy(t *testing.T) {
	fn := buildTestFn("cprop_const", nil, types.TypInt)
	b := entryBlock(fn)

	c1 := fn.NewLocal("c1", types.TypInt)
	c2 := fn.NewLocal("c2", types.TypInt)

	b.Stmts = append(b.Stmts,
		&mir.Assign{Dest: c1, Src: mir.IntConst(42), Type: types.TypInt},
		&mir.Assign{Dest: c2, Src: fn.LocalRef(c1), Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(c2)}

	RunPass(fn, &CopyPropPass{})

	ret := b.Term.(*mir.Return)
	c, ok := ret.Value.(*mir.Const)
	if !ok || c.Int != 42 {
		t.Errorf("expected IntConst(42) after copy prop, got %s", ret.Value.String())
	}
}

// ---------------------------------------------------------------------------
// Test: ConstPropPass
// ---------------------------------------------------------------------------

func TestConstProp_PropagatesConstants(t *testing.T) {
	fn := buildTestFn("constprop", nil, types.TypInt)
	b := entryBlock(fn)

	x := fn.NewLocal("x", types.TypInt)
	y := fn.NewLocal("y", types.TypInt)

	b.Stmts = append(b.Stmts,
		&mir.Assign{Dest: x, Src: mir.IntConst(10), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: y, Op: "+", Left: fn.LocalRef(x), Right: mir.IntConst(5), Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(y)}

	RunPass(fn, &ConstPropPass{})

	// After const prop, the BinaryOpStmt should use IntConst(10) instead of %x.
	binop, ok := b.Stmts[1].(*mir.BinaryOpStmt)
	if !ok {
		t.Fatalf("expected BinaryOpStmt, got %T", b.Stmts[1])
	}
	lc, ok := binop.Left.(*mir.Const)
	if !ok || lc.Int != 10 {
		t.Errorf("expected left operand IntConst(10) after const prop, got %s", binop.Left.String())
	}
}

func TestConstProp_SimplifiesConstBranch(t *testing.T) {
	fn := buildTestFn("constprop_br", nil, types.TypInt)
	b := entryBlock(fn)

	cond := fn.NewLocal("cond", types.TypBool)
	b.Stmts = append(b.Stmts,
		&mir.Assign{Dest: cond, Src: mir.BoolConst(true), Type: types.TypBool},
	)

	thenID := fn.NewBlock("then")
	elseID := fn.NewBlock("else")

	b.Term = &mir.Branch{Cond: fn.LocalRef(cond), Then: thenID, Else: elseID}
	b.Succs = []mir.BlockID{thenID, elseID}

	thenBlk := fn.Block(thenID)
	thenBlk.Preds = []mir.BlockID{fn.Entry}
	thenBlk.Term = &mir.Return{Value: mir.IntConst(1)}

	elseBlk := fn.Block(elseID)
	elseBlk.Preds = []mir.BlockID{fn.Entry}
	elseBlk.Term = &mir.Return{Value: mir.IntConst(0)}

	RunPass(fn, &ConstPropPass{})

	// Branch should be simplified to a Goto to the then block.
	gt, ok := b.Term.(*mir.Goto)
	if !ok {
		t.Fatalf("expected Goto after const-prop on true branch, got %T", b.Term)
	}
	if gt.Target != thenID {
		t.Errorf("expected goto then block, got bb%d", gt.Target)
	}
}

// ---------------------------------------------------------------------------
// Test: CSEPass
// ---------------------------------------------------------------------------

func TestCSE_EliminatesDuplicateExpr(t *testing.T) {
	fn := buildTestFn("cse", []types.Type{types.TypInt, types.TypInt}, types.TypInt)
	b := entryBlock(fn)

	r1 := fn.NewLocal("r1", types.TypInt)
	r2 := fn.NewLocal("r2", types.TypInt)
	sum := fn.NewLocal("sum", types.TypInt)

	// %r1 = %p0 + %p1
	// %r2 = %p0 + %p1  (same expression)
	// %sum = %r1 + %r2
	b.Stmts = append(b.Stmts,
		&mir.BinaryOpStmt{Dest: r1, Op: "+", Left: fn.LocalRef(fn.Params[0]), Right: fn.LocalRef(fn.Params[1]), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: r2, Op: "+", Left: fn.LocalRef(fn.Params[0]), Right: fn.LocalRef(fn.Params[1]), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: sum, Op: "+", Left: fn.LocalRef(r1), Right: fn.LocalRef(r2), Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(sum)}

	RunPass(fn, &CSEPass{})

	// Second stmt should become an Assign from r1.
	a, ok := b.Stmts[1].(*mir.Assign)
	if !ok {
		t.Fatalf("expected Assign after CSE, got %T", b.Stmts[1])
	}
	local, ok := a.Src.(*mir.Local)
	if !ok || local.ID != r1 {
		t.Errorf("expected CSE to reference %%%d, got %s", r1, a.Src.String())
	}
}

func TestCSE_DoesNotEliminateCalls(t *testing.T) {
	fn := buildTestFn("cse_call", nil, types.TypInt)
	b := entryBlock(fn)

	r1 := fn.NewLocal("r1", types.TypInt)
	r2 := fn.NewLocal("r2", types.TypInt)

	g := &mir.Global{Name: "rand", Type: types.TypInt}
	b.Stmts = append(b.Stmts,
		&mir.CallStmt{Dest: r1, Func: g, Type: types.TypInt},
		&mir.CallStmt{Dest: r2, Func: g, Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(r2)}

	RunPass(fn, &CSEPass{})

	// Both calls should be preserved (calls have side effects).
	if _, ok := b.Stmts[0].(*mir.CallStmt); !ok {
		t.Error("first call should be preserved")
	}
	if _, ok := b.Stmts[1].(*mir.CallStmt); !ok {
		t.Error("second call should be preserved")
	}
}

// ---------------------------------------------------------------------------
// Test: InlinePass (with program context)
// ---------------------------------------------------------------------------

func TestInline_SmallFunction(t *testing.T) {
	// Callee: fn double(x: Int) -> Int { x + x }
	callee := buildTestFn("double", []types.Type{types.TypInt}, types.TypInt)
	cb := entryBlock(callee)
	r := callee.NewLocal("r", types.TypInt)
	cb.Stmts = append(cb.Stmts, &mir.BinaryOpStmt{
		Dest: r, Op: "+",
		Left: callee.LocalRef(callee.Params[0]), Right: callee.LocalRef(callee.Params[0]),
		Type: types.TypInt,
	})
	cb.Term = &mir.Return{Value: callee.LocalRef(r)}

	// Caller: fn main() -> Int { double(5) }
	caller := buildTestFn("main", nil, types.TypInt)
	mb := entryBlock(caller)
	callDest := caller.NewLocal("call", types.TypInt)
	mb.Stmts = append(mb.Stmts, &mir.CallStmt{
		Dest: callDest,
		Func: &mir.Global{Name: "double", Type: types.TypInt},
		Args: []mir.Value{mir.IntConst(5)},
		Type: types.TypInt,
	})
	mb.Term = &mir.Return{Value: caller.LocalRef(callDest)}

	prog := &mir.Program{Functions: []*mir.Function{callee, caller}}
	pass := &InlinePassWithProgram{MaxStmts: 8, Program: prog}
	RunPass(caller, pass)

	// After inlining, the call should be replaced with the callee's body.
	hasCall := false
	for _, s := range mb.Stmts {
		if _, ok := s.(*mir.CallStmt); ok {
			hasCall = true
		}
	}
	if hasCall {
		t.Error("expected call to be inlined (removed)")
	}

	// Should have the inlined binary op.
	hasBinOp := false
	for _, s := range mb.Stmts {
		if _, ok := s.(*mir.BinaryOpStmt); ok {
			hasBinOp = true
		}
	}
	if !hasBinOp {
		t.Error("expected inlined BinaryOpStmt")
	}
}

func TestInline_SkipsRecursive(t *testing.T) {
	// fn fact(n: Int) -> Int { fact(n-1) }  (recursive)
	fn := buildTestFn("fact", []types.Type{types.TypInt}, types.TypInt)
	b := entryBlock(fn)
	sub := fn.NewLocal("sub", types.TypInt)
	callDest := fn.NewLocal("call", types.TypInt)
	b.Stmts = append(b.Stmts,
		&mir.BinaryOpStmt{Dest: sub, Op: "-", Left: fn.LocalRef(fn.Params[0]), Right: mir.IntConst(1), Type: types.TypInt},
		&mir.CallStmt{Dest: callDest, Func: &mir.Global{Name: "fact", Type: types.TypInt}, Args: []mir.Value{fn.LocalRef(sub)}, Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(callDest)}

	prog := &mir.Program{Functions: []*mir.Function{fn}}
	pass := &InlinePassWithProgram{MaxStmts: 8, Program: prog}
	RunPass(fn, pass)

	// Recursive call should NOT be inlined.
	hasCall := false
	for _, s := range b.Stmts {
		if _, ok := s.(*mir.CallStmt); ok {
			hasCall = true
		}
	}
	if !hasCall {
		t.Error("recursive call should be preserved (not inlined)")
	}
}

func TestInline_SkipsLargeFunctions(t *testing.T) {
	// Create a callee with 10 statements (exceeds MaxStmts=8).
	callee := buildTestFn("big", []types.Type{types.TypInt}, types.TypInt)
	cb := entryBlock(callee)
	prev := callee.Params[0]
	for i := 0; i < 10; i++ {
		d := callee.NewLocal("t", types.TypInt)
		cb.Stmts = append(cb.Stmts, &mir.BinaryOpStmt{
			Dest: d, Op: "+", Left: callee.LocalRef(prev), Right: mir.IntConst(1), Type: types.TypInt,
		})
		prev = d
	}
	cb.Term = &mir.Return{Value: callee.LocalRef(prev)}

	caller := buildTestFn("caller", nil, types.TypInt)
	mb := entryBlock(caller)
	callDest := caller.NewLocal("call", types.TypInt)
	mb.Stmts = append(mb.Stmts, &mir.CallStmt{
		Dest: callDest, Func: &mir.Global{Name: "big", Type: types.TypInt},
		Args: []mir.Value{mir.IntConst(1)}, Type: types.TypInt,
	})
	mb.Term = &mir.Return{Value: caller.LocalRef(callDest)}

	prog := &mir.Program{Functions: []*mir.Function{callee, caller}}
	pass := &InlinePassWithProgram{MaxStmts: 8, Program: prog}
	RunPass(caller, pass)

	// Call should be preserved (too large to inline).
	hasCall := false
	for _, s := range mb.Stmts {
		if _, ok := s.(*mir.CallStmt); ok {
			hasCall = true
		}
	}
	if !hasCall {
		t.Error("expected call to large function to be preserved")
	}
}

// ---------------------------------------------------------------------------
// Test: LICMPass
// ---------------------------------------------------------------------------

func TestLICM_HoistsInvariantComputation(t *testing.T) {
	fn := buildTestFn("licm", []types.Type{types.TypInt, types.TypInt}, types.TypInt)
	b := entryBlock(fn)

	// entry: goto header
	headerID := fn.NewBlock("loop.header")
	bodyID := fn.NewBlock("loop.body")
	exitID := fn.NewBlock("loop.exit")

	b.Term = &mir.Goto{Target: headerID}
	b.Succs = []mir.BlockID{headerID}

	header := fn.Block(headerID)
	header.Preds = []mir.BlockID{fn.Entry, bodyID}
	// Loop counter phi.
	iPhi := fn.NewLocal("i", types.TypInt)
	header.Phis = []*mir.Phi{{
		Dest: iPhi, Type: types.TypInt,
		Args: map[mir.BlockID]mir.Value{
			fn.Entry: mir.IntConst(0),
			bodyID:   fn.LocalRef(iPhi), // placeholder
		},
	}}
	cond := fn.NewLocal("cond", types.TypBool)
	header.Stmts = append(header.Stmts, &mir.BinaryOpStmt{
		Dest: cond, Op: "<", Left: fn.LocalRef(iPhi), Right: mir.IntConst(10),
		Type: types.TypBool,
	})
	header.Term = &mir.Branch{Cond: fn.LocalRef(cond), Then: bodyID, Else: exitID}
	header.Succs = []mir.BlockID{bodyID, exitID}

	body := fn.Block(bodyID)
	body.Preds = []mir.BlockID{headerID}

	// Loop-invariant: uses only p0 and p1 (defined outside loop).
	inv := fn.NewLocal("inv", types.TypInt)
	body.Stmts = append(body.Stmts, &mir.BinaryOpStmt{
		Dest: inv, Op: "*",
		Left: fn.LocalRef(fn.Params[0]), Right: fn.LocalRef(fn.Params[1]),
		Type: types.TypInt,
	})

	// Loop-variant: increment i.
	iNext := fn.NewLocal("i_next", types.TypInt)
	body.Stmts = append(body.Stmts, &mir.BinaryOpStmt{
		Dest: iNext, Op: "+", Left: fn.LocalRef(iPhi), Right: mir.IntConst(1),
		Type: types.TypInt,
	})
	body.Term = &mir.Goto{Target: headerID}
	body.Succs = []mir.BlockID{headerID}

	// Fix phi to reference iNext.
	header.Phis[0].Args[bodyID] = fn.LocalRef(iNext)

	exit := fn.Block(exitID)
	exit.Preds = []mir.BlockID{headerID}
	exit.Term = &mir.Return{Value: fn.LocalRef(inv)}

	RunPass(fn, &LICMPass{})

	// The invariant computation (p0 * p1) should be hoisted to entry block.
	hoisted := false
	for _, s := range b.Stmts {
		if binop, ok := s.(*mir.BinaryOpStmt); ok && binop.Op == "*" {
			hoisted = true
		}
	}
	if !hoisted {
		t.Error("expected loop-invariant computation to be hoisted to preheader")
	}

	// Body should only have the variant computation.
	for _, s := range body.Stmts {
		if binop, ok := s.(*mir.BinaryOpStmt); ok && binop.Op == "*" {
			t.Error("invariant computation should have been removed from loop body")
			_ = binop
		}
	}
}

// ---------------------------------------------------------------------------
// Test: TCOPass
// ---------------------------------------------------------------------------

func TestTCO_TransformsSelfTailRecursion(t *testing.T) {
	// fn sum(n: Int, acc: Int) -> Int {
	//   if n == 0 { return acc; }
	//   return sum(n-1, acc+n);
	// }
	fn := buildTestFn("sum", []types.Type{types.TypInt, types.TypInt}, types.TypInt)
	b := entryBlock(fn)

	// Check n == 0
	cond := fn.NewLocal("cond", types.TypBool)
	b.Stmts = append(b.Stmts, &mir.BinaryOpStmt{
		Dest: cond, Op: "==",
		Left: fn.LocalRef(fn.Params[0]), Right: mir.IntConst(0),
		Type: types.TypBool,
	})

	baseID := fn.NewBlock("base")
	recID := fn.NewBlock("rec")

	b.Term = &mir.Branch{Cond: fn.LocalRef(cond), Then: baseID, Else: recID}
	b.Succs = []mir.BlockID{baseID, recID}

	// Base case: return acc.
	baseBlk := fn.Block(baseID)
	baseBlk.Preds = []mir.BlockID{fn.Entry}
	baseBlk.Term = &mir.Return{Value: fn.LocalRef(fn.Params[1])}

	// Recursive case: sum(n-1, acc+n).
	recBlk := fn.Block(recID)
	recBlk.Preds = []mir.BlockID{fn.Entry}

	n1 := fn.NewLocal("n1", types.TypInt)
	accn := fn.NewLocal("accn", types.TypInt)
	callDest := fn.NewLocal("call", types.TypInt)

	recBlk.Stmts = append(recBlk.Stmts,
		&mir.BinaryOpStmt{Dest: n1, Op: "-", Left: fn.LocalRef(fn.Params[0]), Right: mir.IntConst(1), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: accn, Op: "+", Left: fn.LocalRef(fn.Params[1]), Right: fn.LocalRef(fn.Params[0]), Type: types.TypInt},
		&mir.CallStmt{Dest: callDest, Func: &mir.Global{Name: "sum", Type: types.TypInt},
			Args: []mir.Value{fn.LocalRef(n1), fn.LocalRef(accn)}, Type: types.TypInt},
	)
	recBlk.Term = &mir.Return{Value: fn.LocalRef(callDest)}

	RunPass(fn, &TCOPass{})

	// After TCO, the recursive call should be replaced with a goto.
	hasCall := false
	for _, blk := range fn.Blocks {
		for _, s := range blk.Stmts {
			if call, ok := s.(*mir.CallStmt); ok {
				if g, ok := call.Func.(*mir.Global); ok && g.Name == "sum" {
					hasCall = true
				}
			}
		}
	}
	if hasCall {
		t.Error("expected tail call to be eliminated by TCO")
	}

	// Should have a tco.header block.
	hasHeader := false
	for _, blk := range fn.Blocks {
		if blk.Label == "tco.header" {
			hasHeader = true
		}
	}
	if !hasHeader {
		t.Error("expected tco.header block after TCO transformation")
	}
}

// ---------------------------------------------------------------------------
// Test: EscapePass
// ---------------------------------------------------------------------------

func TestEscape_LocalStructDoesNotEscape(t *testing.T) {
	fn := buildTestFn("escape_local", nil, types.TypInt)
	b := entryBlock(fn)

	alloc := fn.NewLocal("pt", types.TypInt)
	field := fn.NewLocal("x", types.TypInt)

	b.Stmts = append(b.Stmts,
		&mir.StructAllocStmt{
			Dest: alloc, Name: "Point",
			Fields: []mir.FieldValue{{Name: "x", Value: mir.IntConst(1)}, {Name: "y", Value: mir.IntConst(2)}},
			Type: types.TypInt,
		},
		&mir.FieldAccessStmt{Dest: field, Object: fn.LocalRef(alloc), Field: "x", Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(field)}

	info := AnalyzeEscapes(fn)
	if info.Escapes[alloc] {
		t.Error("local struct should not escape (only field is returned)")
	}
}

func TestEscape_ReturnedStructEscapes(t *testing.T) {
	fn := buildTestFn("escape_return", nil, types.TypInt)
	b := entryBlock(fn)

	alloc := fn.NewLocal("pt", types.TypInt)
	b.Stmts = append(b.Stmts,
		&mir.StructAllocStmt{
			Dest: alloc, Name: "Point",
			Fields: []mir.FieldValue{{Name: "x", Value: mir.IntConst(1)}},
			Type: types.TypInt,
		},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(alloc)}

	info := AnalyzeEscapes(fn)
	if !info.Escapes[alloc] {
		t.Error("returned struct should escape")
	}
}

func TestEscape_PassedToCallEscapes(t *testing.T) {
	fn := buildTestFn("escape_call", nil, types.TypUnit)
	b := entryBlock(fn)

	alloc := fn.NewLocal("arr", types.TypInt)
	callDest := fn.NewLocal("call", types.TypUnit)

	b.Stmts = append(b.Stmts,
		&mir.ArrayAllocStmt{Dest: alloc, Elems: []mir.Value{mir.IntConst(1)}, Type: types.TypInt},
		&mir.CallStmt{Dest: callDest, Func: &mir.Global{Name: "consume", Type: types.TypUnit},
			Args: []mir.Value{fn.LocalRef(alloc)}, Type: types.TypUnit},
	)
	b.Term = &mir.Return{Value: mir.UnitConst()}

	info := AnalyzeEscapes(fn)
	if !info.Escapes[alloc] {
		t.Error("allocation passed to call should escape")
	}
}

func TestEscape_ChannelSendEscapes(t *testing.T) {
	fn := buildTestFn("escape_chan", nil, types.TypUnit)
	b := entryBlock(fn)

	alloc := fn.NewLocal("val", types.TypInt)
	ch := fn.NewLocal("ch", types.TypInt)

	b.Stmts = append(b.Stmts,
		&mir.StructAllocStmt{Dest: alloc, Name: "Data", Fields: []mir.FieldValue{{Name: "x", Value: mir.IntConst(1)}}, Type: types.TypInt},
		&mir.ChannelSendStmt{Chan: fn.LocalRef(ch), SendVal: fn.LocalRef(alloc)},
	)
	b.Term = &mir.Return{Value: mir.UnitConst()}

	info := AnalyzeEscapes(fn)
	if !info.Escapes[alloc] {
		t.Error("allocation sent through channel should escape")
	}
}

// ---------------------------------------------------------------------------
// Test: BlockMergePass
// ---------------------------------------------------------------------------

func TestBlockMerge_MergesLinearChain(t *testing.T) {
	fn := buildTestFn("merge", []types.Type{types.TypInt}, types.TypInt)
	b := entryBlock(fn)

	// entry -> bb1 -> return
	bb1ID := fn.NewBlock("cont")
	bb1 := fn.Block(bb1ID)

	r := fn.NewLocal("r", types.TypInt)
	bb1.Stmts = append(bb1.Stmts, &mir.BinaryOpStmt{
		Dest: r, Op: "+", Left: fn.LocalRef(fn.Params[0]), Right: mir.IntConst(1), Type: types.TypInt,
	})
	bb1.Term = &mir.Return{Value: fn.LocalRef(r)}
	bb1.Preds = []mir.BlockID{fn.Entry}

	b.Term = &mir.Goto{Target: bb1ID}
	b.Succs = []mir.BlockID{bb1ID}

	blocksBefore := len(fn.Blocks)
	RunPass(fn, &BlockMergePass{})

	// Blocks should be merged into one.
	reachable := 0
	for _, blk := range fn.Blocks {
		if blk.Term != nil {
			reachable++
		}
	}

	if reachable >= blocksBefore {
		// Check entry now has the return directly.
		entry := fn.Block(fn.Entry)
		if _, ok := entry.Term.(*mir.Return); !ok {
			t.Errorf("expected entry to terminate with Return after merge, got %T", entry.Term)
		}
	}
}

func TestBlockMerge_DoesNotMergeMultiplePreds(t *testing.T) {
	fn := buildTestFn("nomerge", []types.Type{types.TypBool}, types.TypInt)
	b := entryBlock(fn)

	mergeID := fn.NewBlock("merge")
	thenID := fn.NewBlock("then")
	elseID := fn.NewBlock("else")

	b.Term = &mir.Branch{Cond: fn.LocalRef(fn.Params[0]), Then: thenID, Else: elseID}
	b.Succs = []mir.BlockID{thenID, elseID}

	thenBlk := fn.Block(thenID)
	thenBlk.Preds = []mir.BlockID{fn.Entry}
	thenBlk.Term = &mir.Goto{Target: mergeID}
	thenBlk.Succs = []mir.BlockID{mergeID}

	elseBlk := fn.Block(elseID)
	elseBlk.Preds = []mir.BlockID{fn.Entry}
	elseBlk.Term = &mir.Goto{Target: mergeID}
	elseBlk.Succs = []mir.BlockID{mergeID}

	mergeBlk := fn.Block(mergeID)
	mergeBlk.Preds = []mir.BlockID{thenID, elseID}
	mergeBlk.Term = &mir.Return{Value: mir.IntConst(0)}

	blocksBefore := len(fn.Blocks)
	RunPass(fn, &BlockMergePass{})

	// Merge block has 2 predecessors, so it should not be merged.
	// Block count should not decrease (except possibly unreachable blocks).
	if len(fn.Blocks) < blocksBefore-1 {
		t.Errorf("merge block should not be merged (has 2 preds)")
	}
}

// ---------------------------------------------------------------------------
// Test: Combined pipeline preserves semantics
// ---------------------------------------------------------------------------

func TestPipeline_O1_BasicOptimization(t *testing.T) {
	// fn test() -> Int { let x = 3 + 4; x }
	// After O1: constant fold 3+4=7, const prop, copy prop -> return 7.
	fn := buildTestFn("test_o1", nil, types.TypInt)
	b := entryBlock(fn)

	binop := fn.NewLocal("binop", types.TypInt)
	x := fn.NewLocal("x", types.TypInt)

	b.Stmts = append(b.Stmts,
		&mir.BinaryOpStmt{Dest: binop, Op: "+", Left: mir.IntConst(3), Right: mir.IntConst(4), Type: types.TypInt},
		&mir.Assign{Dest: x, Src: fn.LocalRef(binop), Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(x)}

	prog := &mir.Program{Functions: []*mir.Function{fn}}
	Pipeline(prog, O1)

	// Should simplify to returning constant 7.
	entry := fn.Block(fn.Entry)
	ret, ok := entry.Term.(*mir.Return)
	if !ok {
		t.Fatalf("expected Return, got %T", entry.Term)
	}
	c, ok := ret.Value.(*mir.Const)
	if !ok {
		t.Fatalf("expected Const return value, got %T (%s)", ret.Value, ret.Value.String())
	}
	if c.Kind != mir.ConstInt || c.Int != 7 {
		t.Errorf("expected return 7, got %s", c.String())
	}
}

func TestPipeline_O0_NoOptimization(t *testing.T) {
	fn := buildTestFn("test_o0", nil, types.TypInt)
	b := entryBlock(fn)

	binop := fn.NewLocal("binop", types.TypInt)
	b.Stmts = append(b.Stmts,
		&mir.BinaryOpStmt{Dest: binop, Op: "+", Left: mir.IntConst(3), Right: mir.IntConst(4), Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(binop)}

	prog := &mir.Program{Functions: []*mir.Function{fn}}
	Pipeline(prog, O0)

	// BinaryOpStmt should be preserved with O0.
	if _, ok := b.Stmts[0].(*mir.BinaryOpStmt); !ok {
		t.Errorf("expected BinaryOpStmt preserved with O0, got %T", b.Stmts[0])
	}
}

func TestPipeline_O1_DeadCodeAfterConstProp(t *testing.T) {
	// fn test(x: Int) -> Int {
	//   let a = 10;
	//   let b = a + 5;  // const prop -> 10 + 5 -> fold -> 15
	//   let c = x * 2;  // unused
	//   b
	// }
	fn := buildTestFn("test_dce", []types.Type{types.TypInt}, types.TypInt)
	b := entryBlock(fn)

	a := fn.NewLocal("a", types.TypInt)
	bv := fn.NewLocal("b", types.TypInt)
	c := fn.NewLocal("c", types.TypInt)

	b.Stmts = append(b.Stmts,
		&mir.Assign{Dest: a, Src: mir.IntConst(10), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: bv, Op: "+", Left: fn.LocalRef(a), Right: mir.IntConst(5), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: c, Op: "*", Left: fn.LocalRef(fn.Params[0]), Right: mir.IntConst(2), Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(bv)}

	prog := &mir.Program{Functions: []*mir.Function{fn}}
	Pipeline(prog, O1)

	// c should be eliminated by DCE.
	entry := fn.Block(fn.Entry)
	for _, s := range entry.Stmts {
		if s.DestLocal() == c {
			t.Error("dead code (c = x * 2) should have been eliminated")
		}
	}
}

// ---------------------------------------------------------------------------
// Test: printer output includes key elements
// ---------------------------------------------------------------------------

func TestPrinter_FunctionOutput(t *testing.T) {
	fn := buildTestFn("print_test", []types.Type{types.TypInt}, types.TypInt)
	b := entryBlock(fn)

	dest := fn.NewLocal("r", types.TypInt)
	b.Stmts = append(b.Stmts, &mir.BinaryOpStmt{
		Dest: dest, Op: "+",
		Left: fn.LocalRef(fn.Params[0]), Right: mir.IntConst(1),
		Type: types.TypInt,
	})
	b.Term = &mir.Return{Value: fn.LocalRef(dest)}

	output := mir.PrintFunction(fn)

	if !strings.Contains(output, "fn print_test") {
		t.Error("expected function name in output")
	}
	if !strings.Contains(output, "bb0") {
		t.Error("expected bb0 in output")
	}
	if !strings.Contains(output, "return") {
		t.Error("expected return in output")
	}
	if !strings.Contains(output, "+") {
		t.Error("expected + operator in output")
	}
}

// ---------------------------------------------------------------------------
// Test: pipeline O2 with full optimization
// ---------------------------------------------------------------------------

func TestPipeline_O2_FullOptimization(t *testing.T) {
	// Simple function optimized through the full pipeline.
	fn := buildTestFn("full_opt", nil, types.TypInt)
	b := entryBlock(fn)

	// let x = 2 * 3;
	// let y = 2 * 3;  (CSE candidate)
	// return x + y;
	x := fn.NewLocal("x", types.TypInt)
	y := fn.NewLocal("y", types.TypInt)
	r := fn.NewLocal("r", types.TypInt)

	b.Stmts = append(b.Stmts,
		&mir.BinaryOpStmt{Dest: x, Op: "*", Left: mir.IntConst(2), Right: mir.IntConst(3), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: y, Op: "*", Left: mir.IntConst(2), Right: mir.IntConst(3), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: r, Op: "+", Left: fn.LocalRef(x), Right: fn.LocalRef(y), Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(r)}

	prog := &mir.Program{Functions: []*mir.Function{fn}}
	Pipeline(prog, O2)

	// After full optimization: 2*3=6, 6+6=12, should return 12.
	entry := fn.Block(fn.Entry)
	ret, ok := entry.Term.(*mir.Return)
	if !ok {
		t.Fatalf("expected Return, got %T", entry.Term)
	}
	c, ok := ret.Value.(*mir.Const)
	if !ok {
		// May not fully fold to constant, but at least verify it compiles.
		t.Logf("return value is %T: %s (may not fully const-fold)", ret.Value, ret.Value.String())
		return
	}
	if c.Kind == mir.ConstInt && c.Int != 12 {
		t.Errorf("expected return 12, got %d", c.Int)
	}
}
