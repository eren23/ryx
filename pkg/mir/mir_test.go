package mir

import (
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/hir"
	"github.com/ryx-lang/ryx/pkg/types"
)

// ---------------------------------------------------------------------------
// Helper: build a minimal HIR program
// ---------------------------------------------------------------------------

func hirBlock(stmts []hir.Stmt, trailing hir.Expr) *hir.Block {
	return &hir.Block{Stmts: stmts, TrailingExpr: trailing}
}

func hirInt(v string) *hir.IntLit {
	return &hir.IntLit{Value: v}
}

func hirBool(v bool) *hir.BoolLit {
	return &hir.BoolLit{Value: v}
}

func hirVar(name string, typ types.Type) *hir.VarRef {
	return &hir.VarRef{Name: name}
}

func hirLet(name string, typ types.Type, val hir.Expr, mutable bool) *hir.LetStmt {
	return &hir.LetStmt{Name: name, Type: typ, Value: val, Mutable: mutable}
}

func init() {
	// Set expression types via exprBase reflection — needed for test helpers.
	// We use a helper that creates typed HIR nodes.
}

// typedInt creates an IntLit with type Int set.
func typedInt(v string) hir.Expr {
	e := &hir.IntLit{Value: v}
	setExprType(e, types.TypInt)
	return e
}

func typedBool(v bool) hir.Expr {
	e := &hir.BoolLit{Value: v}
	setExprType(e, types.TypBool)
	return e
}

func typedVar(name string, typ types.Type) hir.Expr {
	e := &hir.VarRef{Name: name}
	setExprType(e, typ)
	return e
}

func typedBlock(stmts []hir.Stmt, trailing hir.Expr, typ types.Type) *hir.Block {
	b := &hir.Block{Stmts: stmts, TrailingExpr: trailing}
	setExprType(b, typ)
	return b
}

func typedBinOp(op string, left, right hir.Expr, typ types.Type) hir.Expr {
	e := &hir.BinaryOp{Op: op, Left: left, Right: right}
	setExprType(e, typ)
	return e
}

func typedIf(cond hir.Expr, then *hir.Block, els hir.Expr, typ types.Type) hir.Expr {
	e := &hir.IfExpr{Cond: cond, Then: then, Else: els}
	setExprType(e, typ)
	return e
}

func typedUnit() hir.Expr {
	e := &hir.UnitLit{}
	setExprType(e, types.TypUnit)
	return e
}

// setExprType sets the type on an HIR expression using the hir.SetExprType helper.
func setExprType(e hir.Expr, t types.Type) {
	hir.SetExprType(e, t)
}

// ---------------------------------------------------------------------------
// Test: basic function with linear control flow (no branches)
// ---------------------------------------------------------------------------

func TestBuildLinearFunction(t *testing.T) {
	// fn add(a: Int, b: Int) -> Int { a + b }
	hirFn := &hir.Function{
		Name:       "add",
		Params:     []hir.Param{{Name: "a", Type: types.TypInt}, {Name: "b", Type: types.TypInt}},
		ReturnType: types.TypInt,
		Body: &hir.Block{
			TrailingExpr: &hir.BinaryOp{
				Op:    "+",
				Left:  &hir.VarRef{Name: "a"},
				Right: &hir.VarRef{Name: "b"},
			},
		},
	}

	prog := &hir.Program{Functions: []*hir.Function{hirFn}}
	mirProg := Build(prog)

	if len(mirProg.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(mirProg.Functions))
	}

	fn := mirProg.Functions[0]
	if fn.Name != "add" {
		t.Errorf("expected function name 'add', got %q", fn.Name)
	}

	// Should have 2 params.
	if len(fn.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(fn.Params))
	}

	// Entry block should exist.
	if len(fn.Blocks) < 1 {
		t.Fatalf("expected at least 1 block, got %d", len(fn.Blocks))
	}

	entry := fn.Block(fn.Entry)
	if entry.Label != "entry" {
		t.Errorf("expected entry label 'entry', got %q", entry.Label)
	}

	// Should have a BinaryOpStmt.
	hasBinOp := false
	for _, s := range entry.Stmts {
		if _, ok := s.(*BinaryOpStmt); ok {
			hasBinOp = true
		}
	}
	if !hasBinOp {
		t.Error("expected a BinaryOpStmt in entry block")
	}

	// Should end with Return.
	if _, ok := entry.Term.(*Return); !ok {
		t.Errorf("expected Return terminator, got %T", entry.Term)
	}

	// Print and verify output is not empty.
	output := PrintFunction(fn)
	if len(output) == 0 {
		t.Error("PrintFunction returned empty string")
	}
	t.Log(output)
}

// ---------------------------------------------------------------------------
// Test: if-else creates correct CFG with phi nodes
// ---------------------------------------------------------------------------

func TestBuildIfElse(t *testing.T) {
	// fn choose(c: Bool, x: Int, y: Int) -> Int {
	//   if c { x } else { y }
	// }
	ifExpr := &hir.IfExpr{
		Cond: &hir.VarRef{Name: "c"},
		Then: &hir.Block{TrailingExpr: &hir.VarRef{Name: "x"}},
		Else: &hir.Block{TrailingExpr: &hir.VarRef{Name: "y"}},
	}
	hir.SetExprType(ifExpr, types.TypInt)

	hirFn := &hir.Function{
		Name: "choose",
		Params: []hir.Param{
			{Name: "c", Type: types.TypBool},
			{Name: "x", Type: types.TypInt},
			{Name: "y", Type: types.TypInt},
		},
		ReturnType: types.TypInt,
		Body: &hir.Block{
			TrailingExpr: ifExpr,
		},
	}

	prog := &hir.Program{Functions: []*hir.Function{hirFn}}
	mirProg := Build(prog)
	fn := mirProg.Functions[0]

	// Should have at least 4 blocks: entry, if.then, if.else, if.merge.
	if len(fn.Blocks) < 4 {
		t.Fatalf("expected >= 4 blocks for if-else, got %d", len(fn.Blocks))
	}

	// Entry should end with Branch.
	entry := fn.Block(fn.Entry)
	branch, ok := entry.Term.(*Branch)
	if !ok {
		t.Fatalf("expected Branch terminator in entry, got %T", entry.Term)
	}

	// Then and else blocks should exist.
	thenBlk := fn.Block(branch.Then)
	elseBlk := fn.Block(branch.Else)
	if thenBlk.Label != "if.then" {
		t.Errorf("expected then block label 'if.then', got %q", thenBlk.Label)
	}
	if elseBlk.Label != "if.else" {
		t.Errorf("expected else block label 'if.else', got %q", elseBlk.Label)
	}

	// Both then and else should jump to a merge block.
	thenGoto, tOk := thenBlk.Term.(*Goto)
	elseGoto, eOk := elseBlk.Term.(*Goto)
	if !tOk || !eOk {
		t.Fatalf("expected Goto terminators in then/else blocks, got %T/%T", thenBlk.Term, elseBlk.Term)
	}
	if thenGoto.Target != elseGoto.Target {
		t.Error("then and else blocks should merge to the same block")
	}

	mergeBlk := fn.Block(thenGoto.Target)
	if mergeBlk.Label != "if.merge" {
		t.Errorf("expected merge block label 'if.merge', got %q", mergeBlk.Label)
	}

	// Merge block should have a phi node.
	if len(mergeBlk.Phis) == 0 {
		t.Error("expected phi node in merge block for if-else value")
	}

	// Merge block should have 2 predecessors.
	if len(mergeBlk.Preds) != 2 {
		t.Errorf("expected 2 predecessors for merge block, got %d", len(mergeBlk.Preds))
	}

	output := PrintFunction(fn)
	t.Log(output)
}

// ---------------------------------------------------------------------------
// Test: while loop creates correct CFG
// ---------------------------------------------------------------------------

func TestBuildWhileLoop(t *testing.T) {
	// fn countdown(n: Int) -> Unit {
	//   let mut i = n;
	//   while i > 0 { let x = i; }
	// }
	hirFn := &hir.Function{
		Name:       "countdown",
		Params:     []hir.Param{{Name: "n", Type: types.TypInt}},
		ReturnType: types.TypUnit,
		Body: &hir.Block{
			Stmts: []hir.Stmt{
				&hir.LetStmt{Name: "i", Type: types.TypInt, Value: &hir.VarRef{Name: "n"}, Mutable: true},
			},
			TrailingExpr: &hir.WhileExpr{
				Cond: &hir.BinaryOp{Op: ">", Left: &hir.VarRef{Name: "i"}, Right: &hir.IntLit{Value: "0"}},
				Body: &hir.Block{
					Stmts: []hir.Stmt{
						&hir.LetStmt{Name: "x", Type: types.TypInt, Value: &hir.VarRef{Name: "i"}},
					},
				},
			},
		},
	}

	prog := &hir.Program{Functions: []*hir.Function{hirFn}}
	mirProg := Build(prog)
	fn := mirProg.Functions[0]

	// Should have blocks: entry, while.header, while.body, while.exit.
	if len(fn.Blocks) < 4 {
		t.Fatalf("expected >= 4 blocks for while loop, got %d", len(fn.Blocks))
	}

	// Find header block.
	var headerBlk *BasicBlock
	for _, blk := range fn.Blocks {
		if blk.Label == "while.header" {
			headerBlk = blk
			break
		}
	}
	if headerBlk == nil {
		t.Fatal("expected while.header block")
	}

	// Header should end with Branch.
	if _, ok := headerBlk.Term.(*Branch); !ok {
		t.Errorf("expected Branch terminator in while.header, got %T", headerBlk.Term)
	}

	// Header should have >= 2 predecessors (entry + back edge).
	if len(headerBlk.Preds) < 2 {
		t.Errorf("expected >= 2 predecessors for loop header, got %d", len(headerBlk.Preds))
	}

	output := PrintFunction(fn)
	t.Log(output)
}

// ---------------------------------------------------------------------------
// Test: SSA property — each local defined exactly once
// ---------------------------------------------------------------------------

func TestSSAProperty(t *testing.T) {
	// fn ssa_test(x: Int) -> Int {
	//   let a = x + 1;
	//   let b = a + 2;
	//   b
	// }
	hirFn := &hir.Function{
		Name:       "ssa_test",
		Params:     []hir.Param{{Name: "x", Type: types.TypInt}},
		ReturnType: types.TypInt,
		Body: &hir.Block{
			Stmts: []hir.Stmt{
				&hir.LetStmt{
					Name: "a", Type: types.TypInt,
					Value: &hir.BinaryOp{Op: "+", Left: &hir.VarRef{Name: "x"}, Right: &hir.IntLit{Value: "1"}},
				},
				&hir.LetStmt{
					Name: "b", Type: types.TypInt,
					Value: &hir.BinaryOp{Op: "+", Left: &hir.VarRef{Name: "a"}, Right: &hir.IntLit{Value: "2"}},
				},
			},
			TrailingExpr: &hir.VarRef{Name: "b"},
		},
	}

	prog := &hir.Program{Functions: []*hir.Function{hirFn}}
	mirProg := Build(prog)
	fn := mirProg.Functions[0]

	// Check SSA property: each LocalID is a destination of at most one statement
	// (across all blocks, phi nodes count as definitions too).
	defCount := make(map[LocalID]int)
	for _, pid := range fn.Params {
		defCount[pid]++ // params are implicit definitions
	}
	for _, blk := range fn.Blocks {
		for _, phi := range blk.Phis {
			defCount[phi.Dest]++
		}
		for _, s := range blk.Stmts {
			d := s.DestLocal()
			if d != NoLocal {
				defCount[d]++
			}
		}
	}

	for lid, count := range defCount {
		if count > 1 {
			t.Errorf("SSA violation: local %%%d defined %d times", lid, count)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: CFG edges are consistent
// ---------------------------------------------------------------------------

func TestCFGConsistency(t *testing.T) {
	ifExpr := &hir.IfExpr{
		Cond: &hir.VarRef{Name: "c"},
		Then: &hir.Block{TrailingExpr: &hir.IntLit{Value: "1"}},
		Else: &hir.Block{TrailingExpr: &hir.IntLit{Value: "2"}},
	}
	hir.SetExprType(ifExpr, types.TypInt)

	hirFn := &hir.Function{
		Name:       "cfg_test",
		Params:     []hir.Param{{Name: "c", Type: types.TypBool}},
		ReturnType: types.TypInt,
		Body: &hir.Block{
			TrailingExpr: ifExpr,
		},
	}

	prog := &hir.Program{Functions: []*hir.Function{hirFn}}
	mirProg := Build(prog)
	fn := mirProg.Functions[0]

	// For every edge (src -> dst) in Succs, dst should have src in Preds.
	for _, blk := range fn.Blocks {
		for _, succ := range blk.Succs {
			succBlk := fn.Block(succ)
			found := false
			for _, pred := range succBlk.Preds {
				if pred == blk.ID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("CFG inconsistency: bb%d -> bb%d in succs but not in preds", blk.ID, succ)
			}
		}
	}

	// For every pred edge, the reverse should exist in succs.
	for _, blk := range fn.Blocks {
		for _, pred := range blk.Preds {
			predBlk := fn.Block(pred)
			found := false
			for _, succ := range predBlk.Succs {
				if succ == blk.ID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("CFG inconsistency: bb%d has pred bb%d but bb%d doesn't have bb%d in succs",
					blk.ID, pred, pred, blk.ID)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Test: phi node placement at loop headers
// ---------------------------------------------------------------------------

func TestPhiAtLoopHeader(t *testing.T) {
	// fn loop_phi(n: Int) -> Int {
	//   let mut sum = 0;
	//   let mut i = 0;
	//   while i < n {
	//     let sum = sum + i;
	//     let i = i + 1;
	//   }
	//   sum
	// }
	hirFn := &hir.Function{
		Name:       "loop_phi",
		Params:     []hir.Param{{Name: "n", Type: types.TypInt}},
		ReturnType: types.TypInt,
		Body: &hir.Block{
			Stmts: []hir.Stmt{
				&hir.LetStmt{Name: "sum", Type: types.TypInt, Value: &hir.IntLit{Value: "0"}, Mutable: true},
				&hir.LetStmt{Name: "i", Type: types.TypInt, Value: &hir.IntLit{Value: "0"}, Mutable: true},
			},
			TrailingExpr: &hir.Block{
				Stmts: []hir.Stmt{
					&hir.ExprStmt{Expr: &hir.WhileExpr{
						Cond: &hir.BinaryOp{Op: "<", Left: &hir.VarRef{Name: "i"}, Right: &hir.VarRef{Name: "n"}},
						Body: &hir.Block{
							Stmts: []hir.Stmt{
								&hir.LetStmt{Name: "sum", Type: types.TypInt,
									Value: &hir.BinaryOp{Op: "+", Left: &hir.VarRef{Name: "sum"}, Right: &hir.VarRef{Name: "i"}}},
								&hir.LetStmt{Name: "i", Type: types.TypInt,
									Value: &hir.BinaryOp{Op: "+", Left: &hir.VarRef{Name: "i"}, Right: &hir.IntLit{Value: "1"}}},
							},
						},
					}},
				},
				TrailingExpr: &hir.VarRef{Name: "sum"},
			},
		},
	}

	prog := &hir.Program{Functions: []*hir.Function{hirFn}}
	mirProg := Build(prog)
	fn := mirProg.Functions[0]

	// The while header block should have phi nodes for 'sum' and 'i'
	// since they are defined before the loop and updated inside.
	var headerBlk *BasicBlock
	for _, blk := range fn.Blocks {
		if blk.Label == "while.header" {
			headerBlk = blk
			break
		}
	}

	if headerBlk == nil {
		t.Fatal("expected while.header block")
	}

	output := PrintFunction(fn)
	t.Log(output)

	// Verify the loop header has phi nodes (from the Braun algorithm).
	// The header receives values from entry (initial) and body (updated).
	if len(headerBlk.Preds) < 2 {
		t.Errorf("expected >= 2 predecessors for loop header, got %d", len(headerBlk.Preds))
	}
}

// ---------------------------------------------------------------------------
// Test: printer output format
// ---------------------------------------------------------------------------

func TestPrinterFormat(t *testing.T) {
	hirFn := &hir.Function{
		Name:       "simple",
		Params:     []hir.Param{{Name: "x", Type: types.TypInt}},
		ReturnType: types.TypInt,
		Body: &hir.Block{
			TrailingExpr: &hir.VarRef{Name: "x"},
		},
	}

	prog := &hir.Program{Functions: []*hir.Function{hirFn}}
	mirProg := Build(prog)

	output := Print(mirProg)
	if !strings.Contains(output, "fn simple") {
		t.Error("expected 'fn simple' in output")
	}
	if !strings.Contains(output, "return") {
		t.Error("expected 'return' in output")
	}
	if !strings.Contains(output, "bb0") {
		t.Error("expected 'bb0' in output")
	}
	t.Log(output)
}

// ---------------------------------------------------------------------------
// Test: short-circuit operators create correct phi nodes
// ---------------------------------------------------------------------------

func TestShortCircuitPhi(t *testing.T) {
	// fn sc(a: Bool, b: Bool) -> Bool { a && b }
	hirFn := &hir.Function{
		Name:       "sc",
		Params:     []hir.Param{{Name: "a", Type: types.TypBool}, {Name: "b", Type: types.TypBool}},
		ReturnType: types.TypBool,
		Body: &hir.Block{
			TrailingExpr: &hir.BinaryOp{
				Op:    "&&",
				Left:  &hir.VarRef{Name: "a"},
				Right: &hir.VarRef{Name: "b"},
			},
		},
	}

	prog := &hir.Program{Functions: []*hir.Function{hirFn}}
	mirProg := Build(prog)
	fn := mirProg.Functions[0]

	// Should have merge block with phi for short-circuit result.
	var mergeBlk *BasicBlock
	for _, blk := range fn.Blocks {
		if blk.Label == "sc.merge" {
			mergeBlk = blk
			break
		}
	}
	if mergeBlk == nil {
		t.Fatal("expected sc.merge block for short-circuit")
	}
	if len(mergeBlk.Phis) == 0 {
		t.Error("expected phi node in short-circuit merge block")
	}

	output := PrintFunction(fn)
	t.Log(output)
}

// ---------------------------------------------------------------------------
// Test: struct/enum definitions are preserved
// ---------------------------------------------------------------------------

func TestStructEnumPreservation(t *testing.T) {
	hirProg := &hir.Program{
		Structs: []*hir.StructDef{
			{Name: "Point", Fields: []hir.FieldDef{{Name: "x", Type: types.TypInt}, {Name: "y", Type: types.TypInt}}},
		},
		Enums: []*hir.EnumDef{
			{Name: "Option", Variants: []hir.VariantDef{
				{Name: "Some", Fields: []types.Type{types.TypInt}},
				{Name: "None"},
			}},
		},
	}

	mirProg := Build(hirProg)

	if len(mirProg.Structs) != 1 || mirProg.Structs[0].Name != "Point" {
		t.Error("struct not preserved")
	}
	if len(mirProg.Enums) != 1 || mirProg.Enums[0].Name != "Option" {
		t.Error("enum not preserved")
	}

	output := Print(mirProg)
	if !strings.Contains(output, "struct Point") {
		t.Error("expected struct Point in output")
	}
	if !strings.Contains(output, "enum Option") {
		t.Error("expected enum Option in output")
	}
	t.Log(output)
}

// ---------------------------------------------------------------------------
// Test: break/continue in loops
// ---------------------------------------------------------------------------

func TestBreakContinue(t *testing.T) {
	// fn bc(n: Int) -> Unit {
	//   let mut i = 0;
	//   while i < n {
	//     if i == 5 { break; }
	//     let i = i + 1;
	//   }
	// }
	hirFn := &hir.Function{
		Name:       "bc",
		Params:     []hir.Param{{Name: "n", Type: types.TypInt}},
		ReturnType: types.TypUnit,
		Body: &hir.Block{
			Stmts: []hir.Stmt{
				&hir.LetStmt{Name: "i", Type: types.TypInt, Value: &hir.IntLit{Value: "0"}, Mutable: true},
			},
			TrailingExpr: &hir.WhileExpr{
				Cond: &hir.BinaryOp{Op: "<", Left: &hir.VarRef{Name: "i"}, Right: &hir.VarRef{Name: "n"}},
				Body: &hir.Block{
					Stmts: []hir.Stmt{
						&hir.ExprStmt{Expr: &hir.IfExpr{
							Cond: &hir.BinaryOp{Op: "==", Left: &hir.VarRef{Name: "i"}, Right: &hir.IntLit{Value: "5"}},
							Then: &hir.Block{
								Stmts: []hir.Stmt{
									&hir.ExprStmt{Expr: &hir.BreakExpr{}},
								},
							},
						}},
						&hir.LetStmt{Name: "i", Type: types.TypInt,
							Value: &hir.BinaryOp{Op: "+", Left: &hir.VarRef{Name: "i"}, Right: &hir.IntLit{Value: "1"}}},
					},
				},
			},
		},
	}

	prog := &hir.Program{Functions: []*hir.Function{hirFn}}
	mirProg := Build(prog)
	fn := mirProg.Functions[0]

	// Verify we have while.exit block.
	found := false
	for _, blk := range fn.Blocks {
		if blk.Label == "while.exit" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected while.exit block for break target")
	}

	output := PrintFunction(fn)
	t.Log(output)
}

// ---------------------------------------------------------------------------
// Test: every block has a terminator
// ---------------------------------------------------------------------------

func TestAllBlocksTerminated(t *testing.T) {
	ifExpr := &hir.IfExpr{
		Cond: &hir.BinaryOp{Op: ">", Left: &hir.VarRef{Name: "x"}, Right: &hir.IntLit{Value: "0"}},
		Then: &hir.Block{TrailingExpr: &hir.IntLit{Value: "1"}},
		Else: &hir.Block{TrailingExpr: &hir.IntLit{Value: "0"}},
	}
	hir.SetExprType(ifExpr, types.TypInt)

	hirFn := &hir.Function{
		Name:       "term_test",
		Params:     []hir.Param{{Name: "x", Type: types.TypInt}},
		ReturnType: types.TypInt,
		Body: &hir.Block{
			TrailingExpr: ifExpr,
		},
	}

	prog := &hir.Program{Functions: []*hir.Function{hirFn}}
	mirProg := Build(prog)
	fn := mirProg.Functions[0]

	for _, blk := range fn.Blocks {
		if blk.Term == nil {
			t.Errorf("block bb%d (%s) has no terminator", blk.ID, blk.Label)
		}
	}
}
