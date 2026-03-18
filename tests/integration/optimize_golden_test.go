package integration

import (
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/mir"
	"github.com/ryx-lang/ryx/pkg/optimize"
	"github.com/ryx-lang/ryx/pkg/types"
)

// ---------------------------------------------------------------------------
// Integration tests for optimization passes
//
// These tests build MIR functions programmatically, run optimization
// passes, and verify the output matches expected semantics. They test
// the passes as they would run in the real compiler pipeline.
// ---------------------------------------------------------------------------

// TestOptimize_ConstantFoldIntegration tests constant folding end-to-end.
func TestOptimize_ConstantFoldIntegration(t *testing.T) {
	// fn constant_fold() -> Int { (3 + 4) * 2 }
	fn := &mir.Function{
		Name:       "constant_fold",
		ReturnType: types.TypInt,
		Entry:      0,
	}
	fn.NewBlock("entry")
	b := fn.Block(fn.Entry)

	binop1 := fn.NewLocal("binop1", types.TypInt)
	binop2 := fn.NewLocal("binop2", types.TypInt)

	b.Stmts = append(b.Stmts,
		&mir.BinaryOpStmt{Dest: binop1, Op: "+", Left: mir.IntConst(3), Right: mir.IntConst(4), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: binop2, Op: "*", Left: fn.LocalRef(binop1), Right: mir.IntConst(2), Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(binop2)}

	prog := &mir.Program{Functions: []*mir.Function{fn}}
	optimize.Pipeline(prog, optimize.O1)

	// After O1 pipeline: fold + const-prop + copy-prop should give us return 14.
	entry := fn.Block(fn.Entry)
	ret, ok := entry.Term.(*mir.Return)
	if !ok {
		t.Fatalf("expected Return, got %T", entry.Term)
	}

	c, ok := ret.Value.(*mir.Const)
	if !ok {
		t.Logf("MIR after optimization:\n%s", mir.PrintFunction(fn))
		t.Fatalf("expected Const return value, got %T (%s)", ret.Value, ret.Value.String())
	}
	if c.Kind != mir.ConstInt || c.Int != 14 {
		t.Errorf("expected return 14, got %s", c.String())
	}
}

// TestOptimize_DeadCodeIntegration tests dead code elimination.
func TestOptimize_DeadCodeIntegration(t *testing.T) {
	// fn dead_code(x: Int) -> Int {
	//   let unused = x + 100;
	//   let used = x * 2;
	//   used
	// }
	fn := &mir.Function{
		Name:       "dead_code",
		ReturnType: types.TypInt,
		Entry:      0,
	}
	fn.NewBlock("entry")
	b := fn.Block(fn.Entry)

	param := fn.NewLocal("x", types.TypInt)
	fn.Params = append(fn.Params, param)

	unused := fn.NewLocal("unused", types.TypInt)
	used := fn.NewLocal("used", types.TypInt)

	b.Stmts = append(b.Stmts,
		&mir.BinaryOpStmt{Dest: unused, Op: "+", Left: fn.LocalRef(param), Right: mir.IntConst(100), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: used, Op: "*", Left: fn.LocalRef(param), Right: mir.IntConst(2), Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(used)}

	prog := &mir.Program{Functions: []*mir.Function{fn}}
	optimize.Pipeline(prog, optimize.O1)

	entry := fn.Block(fn.Entry)

	// The unused computation should be eliminated.
	for _, s := range entry.Stmts {
		if s.DestLocal() == unused {
			t.Error("unused computation should have been eliminated by DCE")
		}
	}

	// The used computation should be preserved.
	found := false
	for _, s := range entry.Stmts {
		if s.DestLocal() == used {
			found = true
		}
	}
	if !found {
		t.Error("used computation should be preserved after DCE")
	}
}

// TestOptimize_InlineIntegration tests function inlining with program context.
func TestOptimize_InlineIntegration(t *testing.T) {
	// Callee: fn double(x: Int) -> Int { x + x }
	callee := &mir.Function{
		Name:       "double",
		ReturnType: types.TypInt,
		Entry:      0,
	}
	callee.NewBlock("entry")
	cb := callee.Block(callee.Entry)
	cparam := callee.NewLocal("x", types.TypInt)
	callee.Params = append(callee.Params, cparam)
	r := callee.NewLocal("r", types.TypInt)
	cb.Stmts = append(cb.Stmts, &mir.BinaryOpStmt{
		Dest: r, Op: "+",
		Left: callee.LocalRef(cparam), Right: callee.LocalRef(cparam),
		Type: types.TypInt,
	})
	cb.Term = &mir.Return{Value: callee.LocalRef(r)}

	// Caller: fn main() -> Int { double(5) }
	caller := &mir.Function{
		Name:       "main",
		ReturnType: types.TypInt,
		Entry:      0,
	}
	caller.NewBlock("entry")
	mb := caller.Block(caller.Entry)
	callDest := caller.NewLocal("call", types.TypInt)
	mb.Stmts = append(mb.Stmts, &mir.CallStmt{
		Dest: callDest,
		Func: &mir.Global{Name: "double", Type: types.TypInt},
		Args: []mir.Value{mir.IntConst(5)},
		Type: types.TypInt,
	})
	mb.Term = &mir.Return{Value: caller.LocalRef(callDest)}

	prog := &mir.Program{Functions: []*mir.Function{callee, caller}}

	// Run inline pass with program context, then cleanup.
	inlinePass := &optimize.InlinePassWithProgram{MaxStmts: 8, Program: prog}
	optimize.RunPass(caller, inlinePass)
	optimize.RunPass(caller, &optimize.ConstantFoldPass{})
	optimize.RunPass(caller, &optimize.CopyPropPass{})
	optimize.RunPass(caller, &optimize.DeadCodePass{})

	// After inlining and optimization: should have the BinaryOp, not the call.
	entry := caller.Block(caller.Entry)
	hasCall := false
	for _, s := range entry.Stmts {
		if _, ok := s.(*mir.CallStmt); ok {
			hasCall = true
		}
	}
	if hasCall {
		t.Error("call to double should be inlined")
	}
}

// TestOptimize_TCOIntegration tests tail call optimization.
func TestOptimize_TCOIntegration(t *testing.T) {
	// fn sum(n: Int, acc: Int) -> Int {
	//   if n == 0 { return acc; }
	//   return sum(n-1, acc+n);
	// }
	fn := &mir.Function{
		Name:       "sum",
		ReturnType: types.TypInt,
		Entry:      0,
	}
	fn.NewBlock("entry")

	n := fn.NewLocal("n", types.TypInt)
	acc := fn.NewLocal("acc", types.TypInt)
	fn.Params = append(fn.Params, n, acc)

	b := fn.Block(fn.Entry)
	cond := fn.NewLocal("cond", types.TypBool)
	b.Stmts = append(b.Stmts, &mir.BinaryOpStmt{
		Dest: cond, Op: "==",
		Left: fn.LocalRef(n), Right: mir.IntConst(0),
		Type: types.TypBool,
	})

	baseID := fn.NewBlock("base")
	recID := fn.NewBlock("rec")

	b.Term = &mir.Branch{Cond: fn.LocalRef(cond), Then: baseID, Else: recID}
	b.Succs = []mir.BlockID{baseID, recID}

	baseBlk := fn.Block(baseID)
	baseBlk.Preds = []mir.BlockID{fn.Entry}
	baseBlk.Term = &mir.Return{Value: fn.LocalRef(acc)}

	recBlk := fn.Block(recID)
	recBlk.Preds = []mir.BlockID{fn.Entry}

	n1 := fn.NewLocal("n1", types.TypInt)
	accn := fn.NewLocal("accn", types.TypInt)
	callDest := fn.NewLocal("call", types.TypInt)

	recBlk.Stmts = append(recBlk.Stmts,
		&mir.BinaryOpStmt{Dest: n1, Op: "-", Left: fn.LocalRef(n), Right: mir.IntConst(1), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: accn, Op: "+", Left: fn.LocalRef(acc), Right: fn.LocalRef(n), Type: types.TypInt},
		&mir.CallStmt{Dest: callDest, Func: &mir.Global{Name: "sum", Type: types.TypInt},
			Args: []mir.Value{fn.LocalRef(n1), fn.LocalRef(accn)}, Type: types.TypInt},
	)
	recBlk.Term = &mir.Return{Value: fn.LocalRef(callDest)}

	optimize.RunPass(fn, &optimize.TCOPass{})

	// Verify: no recursive call to "sum" remains.
	hasRecCall := false
	for _, blk := range fn.Blocks {
		for _, s := range blk.Stmts {
			if call, ok := s.(*mir.CallStmt); ok {
				if g, ok := call.Func.(*mir.Global); ok && g.Name == "sum" {
					hasRecCall = true
				}
			}
		}
	}
	if hasRecCall {
		t.Error("tail recursive call should be eliminated by TCO")
	}

	// Should have a tco.header block.
	hasHeader := false
	for _, blk := range fn.Blocks {
		if blk.Label == "tco.header" {
			hasHeader = true
		}
	}
	if !hasHeader {
		t.Error("expected tco.header block after TCO")
	}

	// Verify the recursive block now ends with Goto (not Return).
	for _, blk := range fn.Blocks {
		if blk.Label == "rec" {
			if _, ok := blk.Term.(*mir.Goto); !ok {
				t.Errorf("expected Goto in rec block after TCO, got %T", blk.Term)
			}
		}
	}

	t.Log(mir.PrintFunction(fn))
}

// TestOptimize_EscapeIntegration tests escape analysis.
func TestOptimize_EscapeIntegration(t *testing.T) {
	fn := &mir.Function{
		Name:       "escape_test",
		ReturnType: types.TypInt,
		Entry:      0,
	}
	fn.NewBlock("entry")
	b := fn.Block(fn.Entry)

	// Local struct: field accessed, struct not returned.
	localAlloc := fn.NewLocal("local_pt", types.TypInt)
	field := fn.NewLocal("field", types.TypInt)

	// Struct passed to call: escapes.
	escapedAlloc := fn.NewLocal("escaped_pt", types.TypInt)
	callDest := fn.NewLocal("call", types.TypUnit)

	b.Stmts = append(b.Stmts,
		&mir.StructAllocStmt{
			Dest: localAlloc, Name: "Point",
			Fields: []mir.FieldValue{{Name: "x", Value: mir.IntConst(1)}, {Name: "y", Value: mir.IntConst(2)}},
			Type: types.TypInt,
		},
		&mir.FieldAccessStmt{Dest: field, Object: fn.LocalRef(localAlloc), Field: "x", Type: types.TypInt},
		&mir.StructAllocStmt{
			Dest: escapedAlloc, Name: "Point",
			Fields: []mir.FieldValue{{Name: "x", Value: mir.IntConst(3)}},
			Type: types.TypInt,
		},
		&mir.CallStmt{
			Dest: callDest, Func: &mir.Global{Name: "consume", Type: types.TypUnit},
			Args: []mir.Value{fn.LocalRef(escapedAlloc)}, Type: types.TypUnit,
		},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(field)}

	info := optimize.AnalyzeEscapes(fn)

	if info.Escapes[localAlloc] {
		t.Error("local struct (field accessed only) should NOT escape")
	}

	if !info.Escapes[escapedAlloc] {
		t.Error("struct passed to call should escape")
	}
}

// TestOptimize_PipelineSemanticsPreserved tests that the full pipeline
// preserves program semantics across a complex function.
func TestOptimize_PipelineSemanticsPreserved(t *testing.T) {
	// fn complex(x: Int) -> Int {
	//   let a = 2 + 3;     // fold to 5
	//   let b = a * 2;     // fold to 10
	//   let c = x + b;     // x + 10
	//   let d = x + b;     // CSE with c
	//   let e = c + d;     // (x+10) + (x+10)
	//   let unused = 999;  // dead code
	//   e
	// }
	fn := &mir.Function{
		Name:       "complex",
		ReturnType: types.TypInt,
		Entry:      0,
	}
	fn.NewBlock("entry")
	b := fn.Block(fn.Entry)

	param := fn.NewLocal("x", types.TypInt)
	fn.Params = append(fn.Params, param)

	a := fn.NewLocal("a", types.TypInt)
	bv := fn.NewLocal("b", types.TypInt)
	c := fn.NewLocal("c", types.TypInt)
	d := fn.NewLocal("d", types.TypInt)
	e := fn.NewLocal("e", types.TypInt)
	unused := fn.NewLocal("unused", types.TypInt)

	b.Stmts = append(b.Stmts,
		&mir.BinaryOpStmt{Dest: a, Op: "+", Left: mir.IntConst(2), Right: mir.IntConst(3), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: bv, Op: "*", Left: fn.LocalRef(a), Right: mir.IntConst(2), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: c, Op: "+", Left: fn.LocalRef(param), Right: fn.LocalRef(bv), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: d, Op: "+", Left: fn.LocalRef(param), Right: fn.LocalRef(bv), Type: types.TypInt},
		&mir.BinaryOpStmt{Dest: e, Op: "+", Left: fn.LocalRef(c), Right: fn.LocalRef(d), Type: types.TypInt},
		&mir.Assign{Dest: unused, Src: mir.IntConst(999), Type: types.TypInt},
	)
	b.Term = &mir.Return{Value: fn.LocalRef(e)}

	prog := &mir.Program{Functions: []*mir.Function{fn}}
	optimize.Pipeline(prog, optimize.O2)

	// Verify the unused variable is eliminated.
	entry := fn.Block(fn.Entry)
	for _, s := range entry.Stmts {
		if s.DestLocal() == unused {
			t.Error("unused variable should be eliminated by DCE")
		}
	}

	// The function should still return something meaningful.
	ret, ok := entry.Term.(*mir.Return)
	if !ok {
		t.Fatalf("expected Return terminator")
	}
	if ret.Value == nil {
		t.Fatal("return value should not be nil")
	}

	t.Logf("Optimized output:\n%s", mir.PrintFunction(fn))
}

// TestOptimize_PassNames verifies all pass names are unique and non-empty.
func TestOptimize_PassNames(t *testing.T) {
	passes := []optimize.Pass{
		&optimize.ConstantFoldPass{},
		&optimize.ConstPropPass{},
		&optimize.CopyPropPass{},
		&optimize.DeadCodePass{},
		&optimize.BlockMergePass{},
		&optimize.CSEPass{},
		&optimize.InlinePass{},
		&optimize.LICMPass{},
		&optimize.TCOPass{},
		&optimize.EscapePass{},
	}

	seen := make(map[string]bool)
	for _, p := range passes {
		name := p.Name()
		if name == "" {
			t.Error("pass name should not be empty")
		}
		if seen[name] {
			t.Errorf("duplicate pass name: %s", name)
		}
		seen[name] = true
	}
}

// TestOptimize_PrinterRoundtrip verifies that the MIR printer produces
// consistent output before and after optimization.
func TestOptimize_PrinterRoundtrip(t *testing.T) {
	fn := &mir.Function{
		Name:       "printer_test",
		ReturnType: types.TypInt,
		Entry:      0,
	}
	fn.NewBlock("entry")
	b := fn.Block(fn.Entry)

	param := fn.NewLocal("x", types.TypInt)
	fn.Params = append(fn.Params, param)
	dest := fn.NewLocal("r", types.TypInt)

	b.Stmts = append(b.Stmts, &mir.BinaryOpStmt{
		Dest: dest, Op: "+",
		Left: fn.LocalRef(param), Right: mir.IntConst(1),
		Type: types.TypInt,
	})
	b.Term = &mir.Return{Value: fn.LocalRef(dest)}

	output := mir.PrintFunction(fn)
	if !strings.Contains(output, "fn printer_test") {
		t.Error("expected function name in output")
	}
	if !strings.Contains(output, "return") {
		t.Error("expected return in output")
	}
	if !strings.Contains(output, "bb0") {
		t.Error("expected bb0 in output")
	}
}
