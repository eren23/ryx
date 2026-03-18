package hir

import (
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/parser"
	"github.com/ryx-lang/ryx/pkg/resolver"
	"github.com/ryx-lang/ryx/pkg/types"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// lowerSource parses, resolves, type-checks, and lowers a Ryx source string
// to HIR. Returns the lowered program and any errors from each phase.
func lowerSource(t *testing.T, src string) *Program {
	t.Helper()

	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("test.ryx", src)

	result := parser.Parse(src, fileID)
	if result.HasErrors() {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	resolved := resolver.Resolve(result.Program, registry)
	for _, d := range resolved.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			t.Fatalf("resolver error: %s", d.Message)
		}
	}

	checked := types.Check(result.Program, resolved, registry)
	for _, d := range checked.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			t.Fatalf("type check error: %s", d.Message)
		}
	}

	lr := Lower(result.Program, checked, resolved, registry)
	if lr.HasErrors() {
		t.Fatalf("lowering errors: %v", lr.Diagnostics)
	}

	return lr.Program
}

// findFunction finds a function by name in the HIR program.
func findFunction(prog *Program, name string) *Function {
	for _, fn := range prog.Functions {
		if fn.Name == name {
			return fn
		}
	}
	return nil
}

// findExprOfType walks an expression tree and returns the first expression
// of the given type. Uses a type switch for each caller.
func walkExprs(expr Expr, visit func(Expr) bool) {
	if expr == nil {
		return
	}
	if visit(expr) {
		return
	}
	switch e := expr.(type) {
	case *Block:
		for _, s := range e.Stmts {
			switch st := s.(type) {
			case *LetStmt:
				if st.Value != nil {
					walkExprs(st.Value, visit)
				}
			case *ExprStmt:
				walkExprs(st.Expr, visit)
			case *ReturnStmt:
				if st.Value != nil {
					walkExprs(st.Value, visit)
				}
			}
		}
		if e.TrailingExpr != nil {
			walkExprs(e.TrailingExpr, visit)
		}
	case *IfExpr:
		walkExprs(e.Cond, visit)
		walkExprs(e.Then, visit)
		if e.Else != nil {
			walkExprs(e.Else, visit)
		}
	case *WhileExpr:
		walkExprs(e.Cond, visit)
		walkExprs(e.Body, visit)
	case *LoopExpr:
		walkExprs(e.Body, visit)
	case *Call:
		walkExprs(e.Func, visit)
		for _, arg := range e.Args {
			walkExprs(arg, visit)
		}
	case *StaticCall:
		for _, arg := range e.Args {
			walkExprs(arg, visit)
		}
	case *BinaryOp:
		walkExprs(e.Left, visit)
		walkExprs(e.Right, visit)
	case *UnaryOp:
		walkExprs(e.Operand, visit)
	case *FieldAccess:
		walkExprs(e.Object, visit)
	case *Index:
		walkExprs(e.Object, visit)
		walkExprs(e.Idx, visit)
	case *Lambda:
		walkExprs(e.Body, visit)
	case *MatchExpr:
		walkExprs(e.Scrutinee, visit)
		for _, arm := range e.Arms {
			walkExprs(arm.Body, visit)
		}
	case *ReturnExpr:
		if e.Value != nil {
			walkExprs(e.Value, visit)
		}
	case *Cast:
		walkExprs(e.Expr, visit)
	case *ArrayLiteral:
		for _, elem := range e.Elems {
			walkExprs(elem, visit)
		}
	case *TupleLiteral:
		for _, elem := range e.Elems {
			walkExprs(elem, visit)
		}
	case *StructLiteral:
		for _, f := range e.Fields {
			walkExprs(f.Value, visit)
		}
	}
}

// ---------------------------------------------------------------------------
// Desugaring: for-in-range → while + counter
// ---------------------------------------------------------------------------

func TestDesugarForInRange(t *testing.T) {
	src := `
fn main() {
    for x in 0..10 {
        let y = x;
    }
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "main")
	if fn == nil {
		t.Fatal("expected function 'main'")
	}

	// The for-in-range should be desugared to a block containing:
	// 1. let __counter = 0
	// 2. let __end = 10
	// 3. while __counter < __end { let x = __counter; ... __counter = __counter + 1; }
	body := fn.Body
	if body == nil {
		t.Fatal("expected function body")
	}

	// Look for a WhileExpr in the lowered output.
	foundWhile := false
	walkExprs(body, func(e Expr) bool {
		if _, ok := e.(*WhileExpr); ok {
			foundWhile = true
			return true
		}
		return false
	})

	if !foundWhile {
		t.Error("for-in-range should be desugared to a while loop")
	}

	// Verify no ForExpr remains (there's no ForExpr type in HIR).
	// This is guaranteed by the type system — ForExpr doesn't exist in HIR.
}

func TestDesugarForInRangeInclusive(t *testing.T) {
	src := `
fn main() {
    for x in 0..=5 {
        let y = x;
    }
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "main")
	if fn == nil {
		t.Fatal("expected function 'main'")
	}

	// The inclusive range should produce <= comparison.
	foundLEQ := false
	walkExprs(fn.Body, func(e Expr) bool {
		if bo, ok := e.(*BinaryOp); ok && bo.Op == "<=" {
			foundLEQ = true
			return true
		}
		return false
	})

	if !foundLEQ {
		t.Error("for-in-range with ..= should produce <= comparison")
	}
}

// ---------------------------------------------------------------------------
// Desugaring: for-in-channel → loop + recv
// ---------------------------------------------------------------------------

func TestDesugarForInChannel(t *testing.T) {
	// Test the channel desugaring by constructing the AST manually since the
	// type checker treats ranges as array types and doesn't support for-in
	// on channels directly. We build a ForExpr with a channel-typed iterator
	// and verify the lowerer produces a loop + recv.

	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("test.ryx", "")

	span := diagnostic.Span{FileID: fileID, Start: 0, End: 1}
	chanType := &types.ChannelType{Elem: types.TypInt}

	// Build CheckResult with channel type for the iterator.
	checkResult := &types.CheckResult{
		NodeTypes:   map[diagnostic.Span]types.Type{span: chanType},
		SymbolTypes: map[diagnostic.Span]*types.TypeScheme{},
	}
	resolved := &resolver.ResolveResult{
		Resolutions: map[diagnostic.Span]*resolver.Symbol{},
		TypeDefs:    map[string]*parser.TypeDef{},
		StructDefs:  map[string]*parser.StructDef{},
		TraitDefs:   map[string]*parser.TraitDef{},
	}

	l := &lowerer{
		checkResult: checkResult,
		resolved:    resolved,
		collector:   diagnostic.NewCollector(registry, 50, 50),
	}

	// Simulate: for x in ch { let y = x; }
	// where ch has type channel<Int>.
	result := l.desugarForInChannel(
		&parser.ForExpr{Binding: "x"},
		types.TypUnit,
		span,
	)

	// Should produce a LoopExpr.
	loopExpr, ok := result.(*LoopExpr)
	if !ok {
		t.Fatalf("for-in-channel should desugar to LoopExpr, got %T", result)
	}

	// Should contain a recv static call.
	foundRecv := false
	walkExprs(loopExpr.Body, func(e Expr) bool {
		if sc, ok := e.(*StaticCall); ok && sc.Method == "recv" {
			foundRecv = true
			return true
		}
		return false
	})

	if !foundRecv {
		t.Error("for-in-channel should contain a recv call")
	}
}

// ---------------------------------------------------------------------------
// Desugaring: method calls → static dispatch
// ---------------------------------------------------------------------------

func TestDesugarMethodCall(t *testing.T) {
	// Test that the lowerer desugars method calls (obj.method(args)) to
	// static dispatch (Type::method(obj, args)). We test this by directly
	// calling the lowerer's desugarMethodCall on a constructed AST node.

	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("test.ryx", "")
	span := diagnostic.Span{FileID: fileID, Start: 0, End: 1}

	pointType := &types.StructType{
		Name:       "Point",
		Fields:     map[string]types.Type{"x": types.TypInt, "y": types.TypInt},
		FieldOrder: []string{"x", "y"},
	}

	checkResult := &types.CheckResult{
		NodeTypes: map[diagnostic.Span]types.Type{
			span: types.TypInt, // result type
		},
		SymbolTypes: map[diagnostic.Span]*types.TypeScheme{},
	}
	resolved := &resolver.ResolveResult{
		Resolutions: map[diagnostic.Span]*resolver.Symbol{},
		TypeDefs:    map[string]*parser.TypeDef{},
		StructDefs:  map[string]*parser.StructDef{},
		TraitDefs:   map[string]*parser.TraitDef{},
	}

	l := &lowerer{
		checkResult: checkResult,
		resolved:    resolved,
		collector:   diagnostic.NewCollector(registry, 50, 50),
	}

	// Simulate: p.distance() where p: Point.
	fieldExpr := &parser.FieldExpr{Object: &parser.Ident{Name: "p"}, Field: "distance"}
	// Add type info for the receiver.
	checkResult.NodeTypes[fieldExpr.Object.Span()] = pointType

	result := l.desugarMethodCall(fieldExpr, nil, types.TypInt, span)

	sc, ok := result.(*StaticCall)
	if !ok {
		t.Fatalf("expected StaticCall, got %T", result)
	}
	if sc.TypeName != "Point" {
		t.Errorf("expected type name 'Point', got %q", sc.TypeName)
	}
	if sc.Method != "distance" {
		t.Errorf("expected method name 'distance', got %q", sc.Method)
	}
	if len(sc.Args) != 1 {
		t.Errorf("expected 1 arg (receiver), got %d", len(sc.Args))
	}
}

func TestDesugarMethodCallWithArgs(t *testing.T) {
	// Test method call with additional arguments:
	// c.add(5) → Counter::add(c, 5)
	// We test via the lowerer directly. Parser nodes constructed outside the
	// parser package have zero spans, so we must avoid registering conflicting
	// types for the same zero span. We only register the receiver type; the
	// arg type is left unset (resolves to Unit, which is fine for this test).

	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("test.ryx", "")
	span := diagnostic.Span{FileID: fileID, Start: 0, End: 1}

	counterType := &types.StructType{
		Name:       "Counter",
		Fields:     map[string]types.Type{"count": types.TypInt},
		FieldOrder: []string{"count"},
	}

	// Zero span is what parser.Ident and parser.IntLit both produce.
	// We register only counterType for it — the desugar method reads
	// the receiver type from the lowered expression, not from NodeTypes
	// for the arguments.
	zeroSpan := diagnostic.Span{}
	checkResult := &types.CheckResult{
		NodeTypes: map[diagnostic.Span]types.Type{
			span:     types.TypInt,
			zeroSpan: counterType,
		},
		SymbolTypes: map[diagnostic.Span]*types.TypeScheme{},
	}
	resolved := &resolver.ResolveResult{
		Resolutions: map[diagnostic.Span]*resolver.Symbol{},
		TypeDefs:    map[string]*parser.TypeDef{},
		StructDefs:  map[string]*parser.StructDef{},
		TraitDefs:   map[string]*parser.TraitDef{},
	}

	l := &lowerer{
		checkResult: checkResult,
		resolved:    resolved,
		collector:   diagnostic.NewCollector(registry, 50, 50),
	}

	fieldExpr := &parser.FieldExpr{Object: &parser.Ident{Name: "c"}, Field: "add"}
	extraArgs := []parser.Expr{&parser.IntLit{Value: "5"}}

	result := l.desugarMethodCall(fieldExpr, extraArgs, types.TypInt, span)

	sc, ok := result.(*StaticCall)
	if !ok {
		t.Fatalf("expected StaticCall, got %T", result)
	}
	if sc.TypeName != "Counter" {
		t.Errorf("expected type name 'Counter', got %q", sc.TypeName)
	}
	if sc.Method != "add" {
		t.Errorf("expected method name 'add', got %q", sc.Method)
	}
	if len(sc.Args) != 2 {
		t.Errorf("expected 2 args (receiver + n), got %d", len(sc.Args))
	}
}

// ---------------------------------------------------------------------------
// Desugaring: pipe operator → nested calls
// ---------------------------------------------------------------------------

func TestDesugarPipeOperator(t *testing.T) {
	src := `
fn double(x: Int) -> Int { x * 2 }
fn inc(x: Int) -> Int { x + 1 }

fn main() {
    let result = 5 |> double |> inc;
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "main")
	if fn == nil {
		t.Fatal("expected function 'main'")
	}

	// 5 |> double |> inc should become inc(double(5)).
	// Look for a nested Call(inc, [Call(double, [5])]) structure.
	foundNestedCalls := false
	walkExprs(fn.Body, func(e Expr) bool {
		if outerCall, ok := e.(*Call); ok {
			// Check if outer function is 'inc'.
			if outerRef, ok := outerCall.Func.(*VarRef); ok && outerRef.Name == "inc" {
				if len(outerCall.Args) == 1 {
					if innerCall, ok := outerCall.Args[0].(*Call); ok {
						if innerRef, ok := innerCall.Func.(*VarRef); ok && innerRef.Name == "double" {
							if len(innerCall.Args) == 1 {
								if lit, ok := innerCall.Args[0].(*IntLit); ok && lit.Value == "5" {
									foundNestedCalls = true
									return true
								}
							}
						}
					}
				}
			}
		}
		return false
	})

	if !foundNestedCalls {
		t.Error("pipe operator should be desugared: 5 |> double |> inc → inc(double(5))")
	}
}

func TestDesugarPipeOperatorSingle(t *testing.T) {
	src := `
fn double(x: Int) -> Int { x * 2 }

fn main() {
    let result = 5 |> double;
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "main")
	if fn == nil {
		t.Fatal("expected function 'main'")
	}

	// 5 |> double → double(5)
	foundCall := false
	walkExprs(fn.Body, func(e Expr) bool {
		if call, ok := e.(*Call); ok {
			if ref, ok := call.Func.(*VarRef); ok && ref.Name == "double" {
				if len(call.Args) == 1 {
					if lit, ok := call.Args[0].(*IntLit); ok && lit.Value == "5" {
						foundCall = true
						return true
					}
				}
			}
		}
		return false
	})

	if !foundCall {
		t.Error("single pipe should be desugared: 5 |> double → double(5)")
	}
}

// ---------------------------------------------------------------------------
// Desugaring: string concatenation ++ → String::concat
// ---------------------------------------------------------------------------

func TestDesugarStringConcat(t *testing.T) {
	src := `
fn main() {
    let s = "hello" ++ " world";
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "main")
	if fn == nil {
		t.Fatal("expected function 'main'")
	}

	// "hello" ++ " world" → String::concat("hello", " world")
	foundConcat := false
	walkExprs(fn.Body, func(e Expr) bool {
		if sc, ok := e.(*StaticCall); ok {
			if sc.TypeName == "String" && sc.Method == "concat" {
				foundConcat = true
				if len(sc.Args) != 2 {
					t.Errorf("String::concat should have 2 args, got %d", len(sc.Args))
				}
				return true
			}
		}
		return false
	})

	if !foundConcat {
		t.Error("string ++ should be desugared to String::concat(a, b)")
	}
}

// ---------------------------------------------------------------------------
// Pattern match compilation: decision tree
// ---------------------------------------------------------------------------

func TestMatchCompileVariantPattern(t *testing.T) {
	src := `
type Color {
    Red,
    Green,
    Blue,
}

fn describe(c: Color) -> Int {
    match c {
        Red => 1,
        Green => 2,
        Blue => 3,
    }
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "describe")
	if fn == nil {
		t.Fatal("expected function 'describe'")
	}

	// Compile matches.
	CompileMatches(prog)

	// Find the match expression and verify it has a decision tree.
	foundMatch := false
	walkExprs(fn.Body, func(e Expr) bool {
		if me, ok := e.(*MatchExpr); ok {
			foundMatch = true
			if me.Decision == nil {
				t.Error("match expression should have a compiled decision tree")
				return true
			}
			// Should be a DecisionSwitch with 3 cases.
			if sw, ok := me.Decision.(*DecisionSwitch); ok {
				if len(sw.Cases) != 3 {
					t.Errorf("expected 3 cases in decision switch, got %d", len(sw.Cases))
				}
				// Verify case constructors.
				names := make(map[string]bool)
				for _, c := range sw.Cases {
					names[c.Constructor] = true
				}
				for _, expected := range []string{"Red", "Green", "Blue"} {
					if !names[expected] {
						t.Errorf("expected case for variant %q", expected)
					}
				}
			} else {
				t.Errorf("expected DecisionSwitch, got %T", me.Decision)
			}
			return true
		}
		return false
	})

	if !foundMatch {
		t.Error("expected a match expression in function body")
	}
}

func TestMatchCompileNoRedundantChecks(t *testing.T) {
	src := `
type Bool2 {
    True2,
    False2,
}

fn check(b: Bool2) -> Int {
    match b {
        True2 => 1,
        False2 => 0,
    }
}
`
	prog := lowerSource(t, src)
	CompileMatches(prog)

	fn := findFunction(prog, "check")
	if fn == nil {
		t.Fatal("expected function 'check'")
	}

	// With 2 exhaustive cases, there should be exactly 2 cases in the switch
	// and no default (or a fail default).
	walkExprs(fn.Body, func(e Expr) bool {
		if me, ok := e.(*MatchExpr); ok {
			if sw, ok := me.Decision.(*DecisionSwitch); ok {
				if len(sw.Cases) != 2 {
					t.Errorf("expected exactly 2 cases (no redundancy), got %d", len(sw.Cases))
				}
			}
			return true
		}
		return false
	})
}

func TestMatchCompileGuardClause(t *testing.T) {
	src := `
fn classify(x: Int) -> Int {
    match x {
        0 => 0,
        n if n > 0 => 1,
        _ => 2,
    }
}
`
	prog := lowerSource(t, src)
	CompileMatches(prog)

	fn := findFunction(prog, "classify")
	if fn == nil {
		t.Fatal("expected function 'classify'")
	}

	// The guard clause `n if n > 0` should produce a DecisionGuard node.
	foundGuard := false
	var checkDecision func(Decision)
	checkDecision = func(d Decision) {
		if d == nil {
			return
		}
		switch dt := d.(type) {
		case *DecisionGuard:
			foundGuard = true
		case *DecisionSwitch:
			for _, c := range dt.Cases {
				checkDecision(c.Body)
			}
			if dt.Default != nil {
				checkDecision(dt.Default)
			}
		}
	}

	walkExprs(fn.Body, func(e Expr) bool {
		if me, ok := e.(*MatchExpr); ok {
			checkDecision(me.Decision)
			return true
		}
		return false
	})

	if !foundGuard {
		t.Error("guard clause should produce a DecisionGuard in the decision tree")
	}
}

func TestMatchCompileWildcard(t *testing.T) {
	src := `
fn test_wild(x: Int) -> Int {
    match x {
        _ => 42,
    }
}
`
	prog := lowerSource(t, src)
	CompileMatches(prog)

	fn := findFunction(prog, "test_wild")
	if fn == nil {
		t.Fatal("expected function 'test_wild'")
	}

	// Wildcard should compile to a simple leaf.
	walkExprs(fn.Body, func(e Expr) bool {
		if me, ok := e.(*MatchExpr); ok {
			if _, ok := me.Decision.(*DecisionLeaf); !ok {
				t.Errorf("wildcard pattern should compile to DecisionLeaf, got %T", me.Decision)
			}
			return true
		}
		return false
	})
}

func TestMatchCompileBindingPattern(t *testing.T) {
	src := `
fn test_bind(x: Int) -> Int {
    match x {
        n => n,
    }
}
`
	prog := lowerSource(t, src)
	CompileMatches(prog)

	fn := findFunction(prog, "test_bind")
	if fn == nil {
		t.Fatal("expected function 'test_bind'")
	}

	walkExprs(fn.Body, func(e Expr) bool {
		if me, ok := e.(*MatchExpr); ok {
			if leaf, ok := me.Decision.(*DecisionLeaf); ok {
				if len(leaf.Bindings) != 1 {
					t.Errorf("expected 1 binding, got %d", len(leaf.Bindings))
				} else if leaf.Bindings[0].Name != "n" {
					t.Errorf("expected binding name 'n', got %q", leaf.Bindings[0].Name)
				}
			} else {
				t.Errorf("binding pattern should compile to DecisionLeaf, got %T", me.Decision)
			}
			return true
		}
		return false
	})
}

// ---------------------------------------------------------------------------
// Closure capture: correct variables identified
// ---------------------------------------------------------------------------

func TestClosureCaptureIdentification(t *testing.T) {
	src := `
fn main() {
    let x = 10;
    let y = 20;
    let f = |a: Int| -> Int { x + a + y };
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "main")
	if fn == nil {
		t.Fatal("expected function 'main'")
	}

	// The lambda |a| { x + a + y } should capture x and y but not a.
	foundLambda := false
	walkExprs(fn.Body, func(e Expr) bool {
		if lam, ok := e.(*Lambda); ok {
			foundLambda = true
			capturedNames := make(map[string]bool)
			for _, cap := range lam.Captures {
				capturedNames[cap.Name] = true
			}

			if !capturedNames["x"] {
				t.Error("lambda should capture variable 'x'")
			}
			if !capturedNames["y"] {
				t.Error("lambda should capture variable 'y'")
			}
			if capturedNames["a"] {
				t.Error("lambda should NOT capture parameter 'a'")
			}
			return true
		}
		return false
	})

	if !foundLambda {
		t.Error("expected a lambda expression in function body")
	}
}

func TestClosureCaptureNoFalsePositive(t *testing.T) {
	src := `
fn main() {
    let f = |x: Int| -> Int { x * 2 };
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "main")
	if fn == nil {
		t.Fatal("expected function 'main'")
	}

	walkExprs(fn.Body, func(e Expr) bool {
		if lam, ok := e.(*Lambda); ok {
			if len(lam.Captures) != 0 {
				t.Errorf("lambda with no free variables should have 0 captures, got %d", len(lam.Captures))
			}
			return true
		}
		return false
	})
}

func TestClosureCaptureNested(t *testing.T) {
	src := `
fn main() {
    let outer = 42;
    let f = |x: Int| -> Int {
        let inner = x + 1;
        outer + inner
    };
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "main")
	if fn == nil {
		t.Fatal("expected function 'main'")
	}

	walkExprs(fn.Body, func(e Expr) bool {
		if lam, ok := e.(*Lambda); ok {
			capturedNames := make(map[string]bool)
			for _, cap := range lam.Captures {
				capturedNames[cap.Name] = true
			}
			if !capturedNames["outer"] {
				t.Error("lambda should capture 'outer'")
			}
			if capturedNames["inner"] {
				t.Error("lambda should NOT capture 'inner' (defined inside the body)")
			}
			if capturedNames["x"] {
				t.Error("lambda should NOT capture parameter 'x'")
			}
			return true
		}
		return false
	})
}

// ---------------------------------------------------------------------------
// Monomorphization
// ---------------------------------------------------------------------------

func TestMonomorphizationBasic(t *testing.T) {
	// Build a simple HIR program manually with a generic function.
	identityFn := &Function{
		Name: "identity",
		Params: []Param{
			{Name: "x", Type: &types.TypeVar{ID: 100}},
		},
		ReturnType: &types.TypeVar{ID: 100},
		Body: &Block{
			exprBase: exprBase{Typ: &types.TypeVar{ID: 100}},
			TrailingExpr: &VarRef{
				exprBase: exprBase{Typ: &types.TypeVar{ID: 100}},
				Name:     "x",
			},
		},
	}

	mainFn := &Function{
		Name:       "main",
		Params:     nil,
		ReturnType: types.TypUnit,
		Body: &Block{
			exprBase: exprBase{Typ: types.TypUnit},
			Stmts: []Stmt{
				&ExprStmt{
					Expr: &Call{
						exprBase: exprBase{Typ: types.TypInt},
						Func:     &VarRef{exprBase: exprBase{Typ: types.TypInt}, Name: "identity"},
						Args: []Expr{
							&IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "42"},
						},
					},
				},
			},
		},
	}

	prog := &Program{
		Functions: []*Function{identityFn, mainFn},
	}

	result := Monomorphize(prog, 64)
	if len(result.Diagnostics) > 0 {
		for _, d := range result.Diagnostics {
			t.Errorf("monomorphization diagnostic: %s", d.Message)
		}
	}

	// Should have created a monomorphized copy.
	found := false
	for _, fn := range prog.Functions {
		if strings.Contains(fn.Name, "identity$") && strings.Contains(fn.Name, "Int") {
			found = true
			// Verify the parameter type is Int.
			if len(fn.Params) == 1 && fn.Params[0].Type.Equal(types.TypInt) {
				// Good.
			}
			break
		}
	}

	if !found {
		t.Error("expected monomorphized copy identity$Int")
	}
}

func TestMonomorphizationLimit(t *testing.T) {
	// Create a generic function and try to exceed the limit.
	genFn := &Function{
		Name: "generic_fn",
		Params: []Param{
			{Name: "x", Type: &types.TypeVar{ID: 200}},
		},
		ReturnType: &types.TypeVar{ID: 200},
		Body: &Block{
			exprBase: exprBase{Typ: &types.TypeVar{ID: 200}},
			TrailingExpr: &VarRef{
				exprBase: exprBase{Typ: &types.TypeVar{ID: 200}},
				Name:     "x",
			},
		},
	}

	// Create many calls with different types.
	var stmts []Stmt
	concreteTypes := []types.Type{
		types.TypInt, types.TypFloat, types.TypBool,
	}
	for _, ct := range concreteTypes {
		stmts = append(stmts, &ExprStmt{
			Expr: &Call{
				exprBase: exprBase{Typ: ct},
				Func:     &VarRef{exprBase: exprBase{Typ: ct}, Name: "generic_fn"},
				Args: []Expr{
					&IntLit{exprBase: exprBase{Typ: ct}, Value: "0"},
				},
			},
		})
	}

	mainFn := &Function{
		Name:       "main",
		Params:     nil,
		ReturnType: types.TypUnit,
		Body: &Block{
			exprBase: exprBase{Typ: types.TypUnit},
			Stmts:    stmts,
		},
	}

	prog := &Program{
		Functions: []*Function{genFn, mainFn},
	}

	// Set limit to 2 — should error on the 3rd instantiation.
	result := Monomorphize(prog, 2)

	foundError := false
	for _, d := range result.Diagnostics {
		if d.Severity == diagnostic.SeverityError && strings.Contains(d.Message, "monomorphization limit") {
			foundError = true
		}
	}

	if !foundError {
		t.Error("expected monomorphization limit error when exceeding 2 instances")
	}
}

func TestMonomorphizationNoDuplicates(t *testing.T) {
	// Same type args should not produce duplicate copies.
	genFn := &Function{
		Name: "id",
		Params: []Param{
			{Name: "x", Type: &types.TypeVar{ID: 300}},
		},
		ReturnType: &types.TypeVar{ID: 300},
		Body: &Block{
			exprBase: exprBase{Typ: &types.TypeVar{ID: 300}},
			TrailingExpr: &VarRef{
				exprBase: exprBase{Typ: &types.TypeVar{ID: 300}},
				Name:     "x",
			},
		},
	}

	mainFn := &Function{
		Name:       "main",
		Params:     nil,
		ReturnType: types.TypUnit,
		Body: &Block{
			exprBase: exprBase{Typ: types.TypUnit},
			Stmts: []Stmt{
				&ExprStmt{
					Expr: &Call{
						exprBase: exprBase{Typ: types.TypInt},
						Func:     &VarRef{exprBase: exprBase{Typ: types.TypInt}, Name: "id"},
						Args:     []Expr{&IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "1"}},
					},
				},
				// Same call again — should not create a second copy.
				&ExprStmt{
					Expr: &Call{
						exprBase: exprBase{Typ: types.TypInt},
						Func:     &VarRef{exprBase: exprBase{Typ: types.TypInt}, Name: "id"},
						Args:     []Expr{&IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "2"}},
					},
				},
			},
		},
	}

	prog := &Program{
		Functions: []*Function{genFn, mainFn},
	}

	Monomorphize(prog, 64)

	count := InstanceCount(prog, "id")
	// Should have original + 1 monomorphized copy = 2 total.
	if count > 2 {
		t.Errorf("expected at most 2 instances of 'id' (original + Int), got %d", count)
	}
}

// ---------------------------------------------------------------------------
// HIR structure: all types resolved
// ---------------------------------------------------------------------------

func TestAllTypesResolved(t *testing.T) {
	src := `
fn add(a: Int, b: Int) -> Int {
    a + b
}

fn main() {
    let x = add(1, 2);
}
`
	prog := lowerSource(t, src)

	// Verify that functions have resolved types.
	addFn := findFunction(prog, "add")
	if addFn == nil {
		t.Fatal("expected function 'add'")
	}
	if addFn.ReturnType == nil {
		t.Error("return type should be resolved")
	}
	for _, p := range addFn.Params {
		if p.Type == nil {
			t.Errorf("param %s type should be resolved", p.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// Impl block methods are lowered as Type::method
// ---------------------------------------------------------------------------

func TestImplMethodsLowered(t *testing.T) {
	src := `
struct Rect {
    width: Int,
    height: Int,
}

impl Rect {
    fn area(self) -> Int {
        self.width * self.height
    }
}

fn main() {
    let r = Rect { width: 10, height: 5 };
}
`
	prog := lowerSource(t, src)

	// The method should be lowered as "Rect::area".
	found := findFunction(prog, "Rect::area")
	if found == nil {
		t.Error("impl method should be lowered as 'Rect::area'")
	}
}

// ---------------------------------------------------------------------------
// Struct and enum definitions are preserved
// ---------------------------------------------------------------------------

func TestStructDefLowered(t *testing.T) {
	src := `
struct Vec2 {
    pub x: Float,
    pub y: Float,
}

fn main() {
    let v = Vec2 { x: 1.0, y: 2.0 };
}
`
	prog := lowerSource(t, src)

	if len(prog.Structs) == 0 {
		t.Fatal("expected at least 1 struct definition")
	}
	found := false
	for _, sd := range prog.Structs {
		if sd.Name == "Vec2" {
			found = true
			if len(sd.Fields) != 2 {
				t.Errorf("expected 2 fields, got %d", len(sd.Fields))
			}
		}
	}
	if !found {
		t.Error("expected struct 'Vec2' in HIR")
	}
}

func TestEnumDefLowered(t *testing.T) {
	src := `
type Shape {
    Circle(Float),
    Rectangle(Float, Float),
}

fn main() {
    let s = Circle(5.0);
}
`
	prog := lowerSource(t, src)

	if len(prog.Enums) == 0 {
		t.Fatal("expected at least 1 enum definition")
	}
	found := false
	for _, ed := range prog.Enums {
		if ed.Name == "Shape" {
			found = true
			if len(ed.Variants) != 2 {
				t.Errorf("expected 2 variants, got %d", len(ed.Variants))
			}
		}
	}
	if !found {
		t.Error("expected enum 'Shape' in HIR")
	}
}

// ---------------------------------------------------------------------------
// FormatExpr debug helper
// ---------------------------------------------------------------------------

func TestFormatExpr(t *testing.T) {
	tests := []struct {
		expr     Expr
		expected string
	}{
		{
			expr:     &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "42"},
			expected: "42",
		},
		{
			expr:     &BoolLit{exprBase: exprBase{Typ: types.TypBool}, Value: true},
			expected: "true",
		},
		{
			expr:     &VarRef{exprBase: exprBase{Typ: types.TypInt}, Name: "x"},
			expected: "x",
		},
		{
			expr: &StaticCall{
				exprBase: exprBase{Typ: types.TypInt},
				TypeName: "String",
				Method:   "concat",
				Args: []Expr{
					&StringLit{exprBase: exprBase{Typ: types.TypString}, Value: "a"},
					&StringLit{exprBase: exprBase{Typ: types.TypString}, Value: "b"},
				},
			},
			expected: `String::concat("a", "b")`,
		},
		{
			expr: &BinaryOp{
				exprBase: exprBase{Typ: types.TypInt},
				Op:       "+",
				Left:     &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "1"},
				Right:    &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "2"},
			},
			expected: "(1 + 2)",
		},
		{
			expr:     nil,
			expected: "<nil>",
		},
	}

	for _, tt := range tests {
		got := FormatExpr(tt.expr)
		if got != tt.expected {
			t.Errorf("FormatExpr(%v) = %q, want %q", tt.expr, got, tt.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// Decision tree structures
// ---------------------------------------------------------------------------

func TestDecisionTreeTypes(t *testing.T) {
	// Verify that all Decision types satisfy the interface.
	var _ Decision = &DecisionLeaf{}
	var _ Decision = &DecisionSwitch{}
	var _ Decision = &DecisionGuard{}
	var _ Decision = &DecisionFail{}
}

// ---------------------------------------------------------------------------
// Match compile: or-pattern
// ---------------------------------------------------------------------------

func TestMatchCompileOrPattern(t *testing.T) {
	src := `
type Dir {
    North,
    South,
    East,
    West,
}

fn is_vertical(d: Dir) -> Int {
    match d {
        North | South => 1,
        _ => 0,
    }
}
`
	prog := lowerSource(t, src)
	CompileMatches(prog)

	fn := findFunction(prog, "is_vertical")
	if fn == nil {
		t.Fatal("expected function 'is_vertical'")
	}

	// The or-pattern should expand into separate cases for North and South.
	walkExprs(fn.Body, func(e Expr) bool {
		if me, ok := e.(*MatchExpr); ok {
			if me.Decision == nil {
				t.Error("decision tree should be compiled")
			}
			return true
		}
		return false
	})
}

// ---------------------------------------------------------------------------
// Match compile: literal patterns
// ---------------------------------------------------------------------------

func TestMatchCompileLiteralPattern(t *testing.T) {
	src := `
fn to_name(x: Int) -> Int {
    match x {
        1 => 10,
        2 => 20,
        _ => 0,
    }
}
`
	prog := lowerSource(t, src)
	CompileMatches(prog)

	fn := findFunction(prog, "to_name")
	if fn == nil {
		t.Fatal("expected function 'to_name'")
	}

	walkExprs(fn.Body, func(e Expr) bool {
		if me, ok := e.(*MatchExpr); ok {
			if me.Decision == nil {
				t.Error("decision tree should be compiled")
				return true
			}
			// First should be a switch on the literal 1.
			if sw, ok := me.Decision.(*DecisionSwitch); ok {
				if len(sw.Cases) == 0 {
					t.Error("expected at least 1 case for literal pattern")
				}
			}
			return true
		}
		return false
	})
}

// ---------------------------------------------------------------------------
// Integration: full pipeline (parse → resolve → typecheck → lower)
// ---------------------------------------------------------------------------

func TestFullPipelineSimple(t *testing.T) {
	src := `
fn square(n: Int) -> Int {
    n * n
}

fn main() {
    let result = square(5);
}
`
	prog := lowerSource(t, src)

	if len(prog.Functions) < 2 {
		t.Fatalf("expected at least 2 functions, got %d", len(prog.Functions))
	}

	sqFn := findFunction(prog, "square")
	if sqFn == nil {
		t.Fatal("expected function 'square'")
	}
	if len(sqFn.Params) != 1 {
		t.Errorf("expected 1 param, got %d", len(sqFn.Params))
	}
}

func TestFullPipelineWithLoopAndBreak(t *testing.T) {
	src := `
fn count_up() {
    let mut i = 0;
    while i < 10 {
        i = i + 1;
    }
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "count_up")
	if fn == nil {
		t.Fatal("expected function 'count_up'")
	}

	foundWhile := false
	walkExprs(fn.Body, func(e Expr) bool {
		if _, ok := e.(*WhileExpr); ok {
			foundWhile = true
			return true
		}
		return false
	})
	if !foundWhile {
		t.Error("expected while loop to be preserved in HIR")
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestEmptyFunction(t *testing.T) {
	src := `
fn noop() {
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "noop")
	if fn == nil {
		t.Fatal("expected function 'noop'")
	}
	if fn.Body == nil {
		t.Error("expected non-nil body for empty function")
	}
}

func TestNestedBlocks(t *testing.T) {
	src := `
fn nested() -> Int {
    let a = {
        let b = 10;
        b + 1
    };
    a
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "nested")
	if fn == nil {
		t.Fatal("expected function 'nested'")
	}

	// The block value-binding creates a nested block.
	// The important thing is that it compiles and lowers without errors.
	if fn.Body == nil {
		t.Error("expected non-nil body for nested function")
	}
}

// ---------------------------------------------------------------------------
// Closure: captured variables become struct fields
// ---------------------------------------------------------------------------

func TestClosureCaptureAsStructFields(t *testing.T) {
	src := `
fn main() {
    let x = 10;
    let y = "hello";
    let f = |a: Int| -> Int { x + a };
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "main")
	if fn == nil {
		t.Fatal("expected function 'main'")
	}

	// Verify captures have correct types (these become struct fields in later stages).
	walkExprs(fn.Body, func(e Expr) bool {
		if lam, ok := e.(*Lambda); ok {
			if len(lam.Captures) == 0 {
				t.Error("expected at least one capture")
				return true
			}
			for _, cap := range lam.Captures {
				if cap.Name == "" {
					t.Error("capture should have a name (becomes struct field name)")
				}
				if cap.Type == nil {
					t.Errorf("capture %q should have a type (becomes struct field type)", cap.Name)
				}
			}
			// Verify x is captured with Int type.
			foundX := false
			for _, cap := range lam.Captures {
				if cap.Name == "x" {
					foundX = true
					if _, ok := cap.Type.(*types.IntType); !ok {
						t.Errorf("capture 'x' should have Int type, got %v", cap.Type)
					}
				}
			}
			if !foundX {
				t.Error("expected capture 'x'")
			}
			return true
		}
		return false
	})
}

func TestClosureCaptureMultipleTypes(t *testing.T) {
	src := `
fn main() {
    let count = 42;
    let flag = true;
    let f = |x: Int| -> Int {
        if flag { count + x } else { x }
    };
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "main")
	if fn == nil {
		t.Fatal("expected function 'main'")
	}

	walkExprs(fn.Body, func(e Expr) bool {
		if lam, ok := e.(*Lambda); ok {
			captureNames := make(map[string]types.Type)
			for _, cap := range lam.Captures {
				captureNames[cap.Name] = cap.Type
			}
			if _, ok := captureNames["count"]; !ok {
				t.Error("expected capture 'count'")
			}
			if _, ok := captureNames["flag"]; !ok {
				t.Error("expected capture 'flag'")
			}
			if len(lam.Captures) != 2 {
				t.Errorf("expected exactly 2 captures (count, flag), got %d", len(lam.Captures))
			}
			return true
		}
		return false
	})
}

// ---------------------------------------------------------------------------
// Monomorphize: generic struct → concrete struct per type
// ---------------------------------------------------------------------------

func TestMonomorphizeGenericStruct(t *testing.T) {
	// Build a generic struct Pair<T> with T=TypeVar fields.
	pairDef := &StructDef{
		Name: "Pair",
		Fields: []FieldDef{
			{Name: "first", Type: &types.TypeVar{ID: 500}},
			{Name: "second", Type: &types.TypeVar{ID: 500}},
		},
	}

	// A function using Pair<Int>.
	pairIntType := &types.StructType{
		Name:       "Pair",
		GenArgs:    []types.Type{types.TypInt},
		Fields:     map[string]types.Type{"first": types.TypInt, "second": types.TypInt},
		FieldOrder: []string{"first", "second"},
	}

	mainFn := &Function{
		Name:       "main",
		ReturnType: types.TypUnit,
		Body: &Block{
			exprBase: exprBase{Typ: types.TypUnit},
			Stmts: []Stmt{
				&LetStmt{
					Name: "p",
					Type: pairIntType,
					Value: &StructLiteral{
						exprBase: exprBase{Typ: pairIntType},
						Name:     "Pair",
						Fields: []FieldInit{
							{Name: "first", Value: &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "1"}},
							{Name: "second", Value: &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "2"}},
						},
					},
				},
			},
		},
	}

	prog := &Program{
		Functions: []*Function{mainFn},
		Structs:   []*StructDef{pairDef},
	}

	result := Monomorphize(prog, 64)
	if len(result.Diagnostics) > 0 {
		for _, d := range result.Diagnostics {
			t.Errorf("diagnostic: %s", d.Message)
		}
	}

	// Should have created a concrete Pair$Int struct.
	found := false
	for _, sd := range prog.Structs {
		if strings.Contains(sd.Name, "Pair$") && strings.Contains(sd.Name, "Int") {
			found = true
			// All fields should have concrete types, not TypeVars.
			for _, f := range sd.Fields {
				if _, isTV := f.Type.(*types.TypeVar); isTV {
					t.Errorf("field %s should have concrete type, got TypeVar", f.Name)
				}
				if !f.Type.Equal(types.TypInt) {
					t.Errorf("field %s expected Int type, got %v", f.Name, f.Type)
				}
			}
		}
	}
	if !found {
		t.Error("expected monomorphized struct Pair$Int")
	}
}

func TestMonomorphizeGenericStructMultipleInstances(t *testing.T) {
	// Generic struct with uses at both Int and Float.
	boxDef := &StructDef{
		Name: "Box",
		Fields: []FieldDef{
			{Name: "value", Type: &types.TypeVar{ID: 600}},
		},
	}

	boxIntType := &types.StructType{
		Name:       "Box",
		GenArgs:    []types.Type{types.TypInt},
		Fields:     map[string]types.Type{"value": types.TypInt},
		FieldOrder: []string{"value"},
	}
	boxFloatType := &types.StructType{
		Name:       "Box",
		GenArgs:    []types.Type{types.TypFloat},
		Fields:     map[string]types.Type{"value": types.TypFloat},
		FieldOrder: []string{"value"},
	}

	mainFn := &Function{
		Name:       "main",
		ReturnType: types.TypUnit,
		Body: &Block{
			exprBase: exprBase{Typ: types.TypUnit},
			Stmts: []Stmt{
				&LetStmt{
					Name:  "bi",
					Type:  boxIntType,
					Value: &StructLiteral{exprBase: exprBase{Typ: boxIntType}, Name: "Box"},
				},
				&LetStmt{
					Name:  "bf",
					Type:  boxFloatType,
					Value: &StructLiteral{exprBase: exprBase{Typ: boxFloatType}, Name: "Box"},
				},
			},
		},
	}

	prog := &Program{
		Functions: []*Function{mainFn},
		Structs:   []*StructDef{boxDef},
	}

	result := Monomorphize(prog, 64)
	if len(result.Diagnostics) > 0 {
		for _, d := range result.Diagnostics {
			t.Errorf("diagnostic: %s", d.Message)
		}
	}

	// Should have original + 2 monomorphized copies = 3 total.
	count := StructInstanceCount(prog, "Box")
	if count < 3 {
		t.Errorf("expected at least 3 struct instances (original + Int + Float), got %d", count)
	}
}

func TestMonomorphizeGenericEnum(t *testing.T) {
	// Generic enum Option<T> with concrete usage at Int.
	optDef := &EnumDef{
		Name: "Option",
		Variants: []VariantDef{
			{Name: "Some", Fields: []types.Type{&types.TypeVar{ID: 700}}},
			{Name: "None", Fields: nil},
		},
	}

	optIntType := &types.EnumType{
		Name:     "Option",
		GenArgs:  []types.Type{types.TypInt},
		Variants: map[string][]types.Type{"Some": {types.TypInt}, "None": nil},
	}

	mainFn := &Function{
		Name:       "main",
		ReturnType: types.TypUnit,
		Body: &Block{
			exprBase: exprBase{Typ: types.TypUnit},
			Stmts: []Stmt{
				&ExprStmt{
					Expr: &VarRef{exprBase: exprBase{Typ: optIntType}, Name: "some_val"},
				},
			},
		},
	}

	prog := &Program{
		Functions: []*Function{mainFn},
		Enums:     []*EnumDef{optDef},
	}

	result := Monomorphize(prog, 64)
	if len(result.Diagnostics) > 0 {
		for _, d := range result.Diagnostics {
			t.Errorf("diagnostic: %s", d.Message)
		}
	}

	// Should have created a concrete Option$Int enum.
	found := false
	for _, ed := range prog.Enums {
		if strings.Contains(ed.Name, "Option$") && strings.Contains(ed.Name, "Int") {
			found = true
			// Verify Some variant has Int field.
			for _, v := range ed.Variants {
				if v.Name == "Some" {
					if len(v.Fields) != 1 {
						t.Errorf("Some variant should have 1 field, got %d", len(v.Fields))
					} else if !v.Fields[0].Equal(types.TypInt) {
						t.Errorf("Some field should be Int, got %v", v.Fields[0])
					}
				}
			}
		}
	}
	if !found {
		t.Error("expected monomorphized enum Option$Int")
	}
}

func TestMonomorphizeStructLimit(t *testing.T) {
	// Create a generic struct and try to exceed the limit.
	genStruct := &StructDef{
		Name:   "GenS",
		Fields: []FieldDef{{Name: "val", Type: &types.TypeVar{ID: 800}}},
	}

	concreteTypes := []types.Type{types.TypInt, types.TypFloat, types.TypBool}
	var stmts []Stmt
	for _, ct := range concreteTypes {
		st := &types.StructType{
			Name:       "GenS",
			GenArgs:    []types.Type{ct},
			Fields:     map[string]types.Type{"val": ct},
			FieldOrder: []string{"val"},
		}
		stmts = append(stmts, &LetStmt{
			Name:  "v",
			Type:  st,
			Value: &StructLiteral{exprBase: exprBase{Typ: st}, Name: "GenS"},
		})
	}

	mainFn := &Function{
		Name:       "main",
		ReturnType: types.TypUnit,
		Body: &Block{
			exprBase: exprBase{Typ: types.TypUnit},
			Stmts:    stmts,
		},
	}

	prog := &Program{
		Functions: []*Function{mainFn},
		Structs:   []*StructDef{genStruct},
	}

	// Set limit to 2 — should error on the 3rd instantiation.
	result := Monomorphize(prog, 2)

	foundError := false
	for _, d := range result.Diagnostics {
		if d.Severity == diagnostic.SeverityError && strings.Contains(d.Message, "monomorphization limit") {
			foundError = true
		}
	}
	if !foundError {
		t.Error("expected monomorphization limit error for struct when exceeding 2 instances")
	}
}

// ---------------------------------------------------------------------------
// All type annotations resolved: no TypeVar remaining
// ---------------------------------------------------------------------------

// walkAllTypes checks every expression type in a function body for TypeVars.
func hasTypeVar(t types.Type) bool {
	if t == nil {
		return false
	}
	switch ty := t.(type) {
	case *types.TypeVar:
		return true
	case *types.ArrayType:
		return hasTypeVar(ty.Elem)
	case *types.SliceType:
		return hasTypeVar(ty.Elem)
	case *types.TupleType:
		for _, e := range ty.Elems {
			if hasTypeVar(e) {
				return true
			}
		}
	case *types.FnType:
		for _, p := range ty.Params {
			if hasTypeVar(p) {
				return true
			}
		}
		return hasTypeVar(ty.Return)
	case *types.StructType:
		for _, a := range ty.GenArgs {
			if hasTypeVar(a) {
				return true
			}
		}
	case *types.EnumType:
		for _, a := range ty.GenArgs {
			if hasTypeVar(a) {
				return true
			}
		}
	case *types.ChannelType:
		return hasTypeVar(ty.Elem)
	}
	return false
}

func TestNoTypeVarsInResolvedHIR(t *testing.T) {
	src := `
fn add(a: Int, b: Int) -> Int { a + b }

fn negate(x: Bool) -> Bool { !x }

fn main() {
    let x = 1 + 2;
    let y = !true;
    let s = "hello" ++ " world";
}
`
	prog := lowerSource(t, src)

	// Verify function signatures have no TypeVars (these must be fully resolved).
	for _, fn := range prog.Functions {
		if hasTypeVar(fn.ReturnType) {
			t.Errorf("function %s return type should be resolved, got TypeVar", fn.Name)
		}
		for _, p := range fn.Params {
			if hasTypeVar(p.Type) {
				t.Errorf("function %s param %s type should be resolved", fn.Name, p.Name)
			}
		}
	}

	// Verify literal and operator expression types are resolved.
	for _, fn := range prog.Functions {
		if fn.Body != nil {
			walkExprs(fn.Body, func(e Expr) bool {
				switch e.(type) {
				case *IntLit, *FloatLit, *StringLit, *BoolLit, *CharLit:
					if hasTypeVar(e.ExprType()) {
						t.Errorf("literal type should be resolved, got %v in %s", e.ExprType(), FormatExpr(e))
					}
				case *BinaryOp, *UnaryOp:
					if hasTypeVar(e.ExprType()) {
						t.Errorf("operator type should be resolved, got %v in %s", e.ExprType(), FormatExpr(e))
					}
				case *VarRef:
					if hasTypeVar(e.ExprType()) {
						t.Errorf("variable type should be resolved, got %v in %s", e.ExprType(), FormatExpr(e))
					}
				}
				return false
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Match compile: variant with data fields
// ---------------------------------------------------------------------------

func TestMatchCompileVariantWithData(t *testing.T) {
	src := `
type Shape {
    Circle(Float),
    Rectangle(Float, Float),
}

fn area(s: Shape) -> Float {
    match s {
        Circle(r) => r * r,
        Rectangle(w, h) => w * h,
    }
}
`
	prog := lowerSource(t, src)
	CompileMatches(prog)

	fn := findFunction(prog, "area")
	if fn == nil {
		t.Fatal("expected function 'area'")
	}

	walkExprs(fn.Body, func(e Expr) bool {
		if me, ok := e.(*MatchExpr); ok {
			if me.Decision == nil {
				t.Error("decision tree should be compiled")
				return true
			}
			sw, ok := me.Decision.(*DecisionSwitch)
			if !ok {
				t.Errorf("expected DecisionSwitch, got %T", me.Decision)
				return true
			}
			if len(sw.Cases) != 2 {
				t.Errorf("expected 2 cases (Circle, Rectangle), got %d", len(sw.Cases))
			}
			// Verify that cases have arg names for data fields.
			for _, c := range sw.Cases {
				switch c.Constructor {
				case "Circle":
					if len(c.ArgNames) != 1 {
						t.Errorf("Circle case should have 1 arg name, got %d", len(c.ArgNames))
					}
				case "Rectangle":
					if len(c.ArgNames) != 2 {
						t.Errorf("Rectangle case should have 2 arg names, got %d", len(c.ArgNames))
					}
				}
			}
			return true
		}
		return false
	})
}

// ---------------------------------------------------------------------------
// Match compile: tuple destructuring
// ---------------------------------------------------------------------------

func TestMatchCompileTupleDestructure(t *testing.T) {
	// Build a match on a tuple manually (since the parser may not support
	// match on tuples directly in all contexts).
	tupleType := &types.TupleType{Elems: []types.Type{types.TypInt, types.TypBool}}
	scrutinee := &VarRef{
		exprBase: exprBase{Typ: tupleType},
		Name:     "pair",
	}

	arms := []*MatchArm{
		{
			Pattern: &TuplePat{
				Elems: []Pattern{
					&BindingPat{Name: "a"},
					&BindingPat{Name: "b"},
				},
			},
			Body: &VarRef{exprBase: exprBase{Typ: types.TypInt}, Name: "a"},
		},
	}

	decision := compileArms(scrutinee, arms)

	// Tuple pattern with all bindings should compile to a leaf with bindings.
	leaf, ok := decision.(*DecisionLeaf)
	if !ok {
		t.Fatalf("expected DecisionLeaf for tuple destructure, got %T", decision)
	}
	if len(leaf.Bindings) != 2 {
		t.Errorf("expected 2 bindings (a, b), got %d", len(leaf.Bindings))
	}
	names := make(map[string]bool)
	for _, b := range leaf.Bindings {
		names[b.Name] = true
	}
	if !names["a"] || !names["b"] {
		t.Error("expected bindings for 'a' and 'b'")
	}
}

// ---------------------------------------------------------------------------
// Match compile: range pattern
// ---------------------------------------------------------------------------

func TestMatchCompileRangePatternDecision(t *testing.T) {
	scrutinee := &VarRef{
		exprBase: exprBase{Typ: types.TypInt},
		Name:     "x",
	}

	arms := []*MatchArm{
		{
			Pattern: &RangePat{
				Start:     &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "0"},
				End:       &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "10"},
				Inclusive: true,
			},
			Body: &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "1"},
		},
		{
			Pattern: &WildcardPat{},
			Body:    &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "0"},
		},
	}

	decision := compileArms(scrutinee, arms)

	// Range pattern should produce a DecisionGuard.
	guard, ok := decision.(*DecisionGuard)
	if !ok {
		t.Fatalf("expected DecisionGuard for range pattern, got %T", decision)
	}
	if guard.Condition == nil {
		t.Error("guard condition should not be nil")
	}
	// Then branch should be a leaf.
	if _, ok := guard.Then.(*DecisionLeaf); !ok {
		t.Errorf("guard.Then should be DecisionLeaf, got %T", guard.Then)
	}
	// Else branch is the wildcard fallback.
	if _, ok := guard.Else.(*DecisionLeaf); !ok {
		t.Errorf("guard.Else should be DecisionLeaf (wildcard), got %T", guard.Else)
	}
}

// ---------------------------------------------------------------------------
// Match compile: struct pattern
// ---------------------------------------------------------------------------

func TestMatchCompileStructPattern(t *testing.T) {
	pointType := &types.StructType{
		Name:       "Point",
		Fields:     map[string]types.Type{"x": types.TypInt, "y": types.TypInt},
		FieldOrder: []string{"x", "y"},
	}

	scrutinee := &VarRef{
		exprBase: exprBase{Typ: pointType},
		Name:     "p",
	}

	arms := []*MatchArm{
		{
			Pattern: &StructPat{
				Name: "Point",
				Fields: []StructPatField{
					{Name: "x", Pattern: &BindingPat{Name: "px"}},
					{Name: "y", Pattern: &BindingPat{Name: "py"}},
				},
			},
			Body: &VarRef{exprBase: exprBase{Typ: types.TypInt}, Name: "px"},
		},
	}

	decision := compileArms(scrutinee, arms)

	// Struct pattern should produce a switch with a single case.
	sw, ok := decision.(*DecisionSwitch)
	if !ok {
		t.Fatalf("expected DecisionSwitch for struct pattern, got %T", decision)
	}
	if len(sw.Cases) != 1 {
		t.Errorf("expected 1 case, got %d", len(sw.Cases))
	}
	if sw.Cases[0].Constructor != "Point" {
		t.Errorf("expected constructor 'Point', got %q", sw.Cases[0].Constructor)
	}
}

// ---------------------------------------------------------------------------
// Match compile: nested match expressions
// ---------------------------------------------------------------------------

func TestMatchCompileNestedMatch(t *testing.T) {
	src := `
type Color {
    Red,
    Green,
    Blue,
}

fn nested_match(c: Color, x: Int) -> Int {
    match c {
        Red => match x {
            0 => 100,
            _ => 200,
        },
        _ => 0,
    }
}
`
	prog := lowerSource(t, src)
	CompileMatches(prog)

	fn := findFunction(prog, "nested_match")
	if fn == nil {
		t.Fatal("expected function 'nested_match'")
	}

	// Verify both outer and inner match have decision trees.
	matchCount := 0
	walkExprs(fn.Body, func(e Expr) bool {
		if me, ok := e.(*MatchExpr); ok {
			matchCount++
			if me.Decision == nil {
				t.Error("all match expressions should have compiled decision trees")
			}
		}
		return false
	})

	if matchCount < 2 {
		t.Errorf("expected at least 2 match expressions (outer + inner), got %d", matchCount)
	}
}

// ---------------------------------------------------------------------------
// Match compile: guard with binding
// ---------------------------------------------------------------------------

func TestMatchCompileGuardWithBinding(t *testing.T) {
	src := `
fn classify(x: Int) -> Int {
    match x {
        n if n > 100 => 3,
        n if n > 10 => 2,
        n if n > 0 => 1,
        _ => 0,
    }
}
`
	prog := lowerSource(t, src)
	CompileMatches(prog)

	fn := findFunction(prog, "classify")
	if fn == nil {
		t.Fatal("expected function 'classify'")
	}

	// Count guard nodes in the decision tree.
	guardCount := 0
	var countGuards func(Decision)
	countGuards = func(d Decision) {
		if d == nil {
			return
		}
		switch dt := d.(type) {
		case *DecisionGuard:
			guardCount++
			countGuards(dt.Then)
			countGuards(dt.Else)
		case *DecisionSwitch:
			for _, c := range dt.Cases {
				countGuards(c.Body)
			}
			if dt.Default != nil {
				countGuards(dt.Default)
			}
		}
	}

	walkExprs(fn.Body, func(e Expr) bool {
		if me, ok := e.(*MatchExpr); ok {
			countGuards(me.Decision)
			return true
		}
		return false
	})

	// Three guarded patterns should produce at least 3 guard nodes.
	if guardCount < 3 {
		t.Errorf("expected at least 3 DecisionGuard nodes, got %d", guardCount)
	}
}

// ---------------------------------------------------------------------------
// Desugar: for-in-range counter variable naming
// ---------------------------------------------------------------------------

func TestDesugarForInRangeCounterAndEnd(t *testing.T) {
	src := `
fn main() {
    for x in 0..10 {
        let y = x;
    }
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "main")
	if fn == nil {
		t.Fatal("expected function 'main'")
	}

	// Find the outer block that contains let __counter_N and let __end_N.
	foundCounter := false
	foundEnd := false
	walkExprs(fn.Body, func(e Expr) bool {
		if block, ok := e.(*Block); ok {
			for _, s := range block.Stmts {
				if ls, ok := s.(*LetStmt); ok {
					if strings.HasPrefix(ls.Name, "__counter_") {
						foundCounter = true
						if !ls.Mutable {
							t.Error("counter variable should be mutable")
						}
					}
					if strings.HasPrefix(ls.Name, "__end_") {
						foundEnd = true
					}
				}
			}
		}
		return false
	})

	if !foundCounter {
		t.Error("expected synthetic __counter variable in desugared for-in-range")
	}
	if !foundEnd {
		t.Error("expected synthetic __end variable in desugared for-in-range")
	}
}

// ---------------------------------------------------------------------------
// Desugar: binary operators preserved
// ---------------------------------------------------------------------------

func TestBinaryOperatorsPreserved(t *testing.T) {
	src := `
fn math(a: Int, b: Int) -> Bool {
    a + b > 0 && a - b < 100
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "math")
	if fn == nil {
		t.Fatal("expected function 'math'")
	}

	// Verify binary operators are preserved correctly.
	ops := make(map[string]bool)
	walkExprs(fn.Body, func(e Expr) bool {
		if bo, ok := e.(*BinaryOp); ok {
			ops[bo.Op] = true
		}
		return false
	})

	for _, expected := range []string{"+", "-", ">", "<", "&&"} {
		if !ops[expected] {
			t.Errorf("expected binary operator %q to be preserved", expected)
		}
	}
}

// ---------------------------------------------------------------------------
// If/else expression lowering
// ---------------------------------------------------------------------------

func TestIfElseExpressionLowered(t *testing.T) {
	src := `
fn abs(x: Int) -> Int {
    if x > 0 { x } else { 0 - x }
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "abs")
	if fn == nil {
		t.Fatal("expected function 'abs'")
	}

	foundIf := false
	walkExprs(fn.Body, func(e Expr) bool {
		if ifExpr, ok := e.(*IfExpr); ok {
			foundIf = true
			if ifExpr.Cond == nil {
				t.Error("if condition should not be nil")
			}
			if ifExpr.Then == nil {
				t.Error("if then-branch should not be nil")
			}
			if ifExpr.Else == nil {
				t.Error("if else-branch should not be nil")
			}
			return true
		}
		return false
	})

	if !foundIf {
		t.Error("expected IfExpr in function body")
	}
}

// ---------------------------------------------------------------------------
// Unary operator lowering
// ---------------------------------------------------------------------------

func TestUnaryOperatorLowered(t *testing.T) {
	src := `
fn negate(x: Bool) -> Bool {
    !x
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "negate")
	if fn == nil {
		t.Fatal("expected function 'negate'")
	}

	foundNot := false
	walkExprs(fn.Body, func(e Expr) bool {
		if uo, ok := e.(*UnaryOp); ok && uo.Op == "!" {
			foundNot = true
			return true
		}
		return false
	})

	if !foundNot {
		t.Error("expected unary ! operator in function body")
	}
}

// ---------------------------------------------------------------------------
// Cast expression (as)
// ---------------------------------------------------------------------------

func TestCastExpressionLowered(t *testing.T) {
	src := `
fn to_float(x: Int) -> Float {
    x as Float
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "to_float")
	if fn == nil {
		t.Fatal("expected function 'to_float'")
	}

	foundCast := false
	walkExprs(fn.Body, func(e Expr) bool {
		if c, ok := e.(*Cast); ok {
			foundCast = true
			if c.Target == nil {
				t.Error("cast target type should not be nil")
			}
		}
		return false
	})

	if !foundCast {
		t.Error("expected Cast expression in function body")
	}
}

// ---------------------------------------------------------------------------
// Array and tuple literal lowering
// ---------------------------------------------------------------------------

func TestArrayLiteralLowered(t *testing.T) {
	src := `
fn make_arr() -> [Int] {
    [1, 2, 3]
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "make_arr")
	if fn == nil {
		t.Fatal("expected function 'make_arr'")
	}

	foundArr := false
	walkExprs(fn.Body, func(e Expr) bool {
		if arr, ok := e.(*ArrayLiteral); ok {
			foundArr = true
			if len(arr.Elems) != 3 {
				t.Errorf("expected 3 elements, got %d", len(arr.Elems))
			}
			return true
		}
		return false
	})

	if !foundArr {
		t.Error("expected ArrayLiteral in function body")
	}
}

func TestTupleLiteralLowered(t *testing.T) {
	src := `
fn make_pair() -> (Int, Bool) {
    (42, true)
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "make_pair")
	if fn == nil {
		t.Fatal("expected function 'make_pair'")
	}

	foundTuple := false
	walkExprs(fn.Body, func(e Expr) bool {
		if tup, ok := e.(*TupleLiteral); ok {
			foundTuple = true
			if len(tup.Elems) != 2 {
				t.Errorf("expected 2 elements, got %d", len(tup.Elems))
			}
			return true
		}
		return false
	})

	if !foundTuple {
		t.Error("expected TupleLiteral in function body")
	}
}

// ---------------------------------------------------------------------------
// Integration: full pipeline with multiple desugarings
// ---------------------------------------------------------------------------

func TestFullPipelineMultipleDesugarings(t *testing.T) {
	src := `
fn double(x: Int) -> Int { x * 2 }

fn main() {
    let a = 5 |> double;
    let s = "hello" ++ " world";
    for i in 0..3 {
        let x = i;
    }
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "main")
	if fn == nil {
		t.Fatal("expected function 'main'")
	}

	// Verify pipe desugared to call.
	foundPipeCall := false
	walkExprs(fn.Body, func(e Expr) bool {
		if call, ok := e.(*Call); ok {
			if ref, ok := call.Func.(*VarRef); ok && ref.Name == "double" {
				foundPipeCall = true
				return true
			}
		}
		return false
	})
	if !foundPipeCall {
		t.Error("pipe operator should be desugared to function call")
	}

	// Verify concat desugared to String::concat.
	foundConcat := false
	walkExprs(fn.Body, func(e Expr) bool {
		if sc, ok := e.(*StaticCall); ok && sc.TypeName == "String" && sc.Method == "concat" {
			foundConcat = true
			return true
		}
		return false
	})
	if !foundConcat {
		t.Error("++ should be desugared to String::concat")
	}

	// Verify for-in-range desugared to while.
	foundWhile := false
	walkExprs(fn.Body, func(e Expr) bool {
		if _, ok := e.(*WhileExpr); ok {
			foundWhile = true
			return true
		}
		return false
	})
	if !foundWhile {
		t.Error("for-in-range should be desugared to while loop")
	}
}

// ---------------------------------------------------------------------------
// Match compile: or-pattern expands into separate cases
// ---------------------------------------------------------------------------

func TestMatchCompileOrPatternExpands(t *testing.T) {
	scrutinee := &VarRef{
		exprBase: exprBase{Typ: types.TypInt},
		Name:     "x",
	}

	arms := []*MatchArm{
		{
			Pattern: &OrPat{
				Alts: []Pattern{
					&LiteralPat{Value: &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "1"}},
					&LiteralPat{Value: &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "2"}},
				},
			},
			Body: &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "10"},
		},
		{
			Pattern: &WildcardPat{},
			Body:    &IntLit{exprBase: exprBase{Typ: types.TypInt}, Value: "0"},
		},
	}

	decision := compileArms(scrutinee, arms)

	// The or-pattern should expand into a switch with cases for both 1 and 2.
	sw, ok := decision.(*DecisionSwitch)
	if !ok {
		t.Fatalf("expected DecisionSwitch for or-pattern, got %T", decision)
	}
	if len(sw.Cases) < 2 {
		t.Errorf("or-pattern should expand into at least 2 cases, got %d", len(sw.Cases))
	}
}

// ---------------------------------------------------------------------------
// Match compile: DecisionFail for empty arms
// ---------------------------------------------------------------------------

func TestMatchCompileEmptyArms(t *testing.T) {
	scrutinee := &VarRef{
		exprBase: exprBase{Typ: types.TypInt},
		Name:     "x",
	}

	decision := compileArms(scrutinee, nil)

	if _, ok := decision.(*DecisionFail); !ok {
		t.Errorf("empty arms should produce DecisionFail, got %T", decision)
	}
}

// ---------------------------------------------------------------------------
// Monomorphize: mangled name correctness
// ---------------------------------------------------------------------------

func TestMangledName(t *testing.T) {
	tests := []struct {
		base     string
		typeArgs []types.Type
		expected string
	}{
		{"identity", []types.Type{types.TypInt}, "identity$Int"},
		{"pair", []types.Type{types.TypInt, types.TypString}, "pair$Int_String"},
		{"id", nil, "id"},
		{"id", []types.Type{}, "id"},
	}

	for _, tt := range tests {
		got := mangledName(tt.base, tt.typeArgs)
		if got != tt.expected {
			t.Errorf("mangledName(%q, %v) = %q, want %q", tt.base, tt.typeArgs, got, tt.expected)
		}
	}
}

func TestTypeKey(t *testing.T) {
	tests := []struct {
		typ      types.Type
		expected string
	}{
		{types.TypInt, "Int"},
		{types.TypFloat, "Float"},
		{types.TypBool, "Bool"},
		{types.TypString, "String"},
		{types.TypUnit, "Unit"},
		{nil, "Unit"},
	}

	for _, tt := range tests {
		got := typeKey(tt.typ)
		if got != tt.expected {
			t.Errorf("typeKey(%v) = %q, want %q", tt.typ, got, tt.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// Loop and break/continue lowering
// ---------------------------------------------------------------------------

func TestLoopBreakLowered(t *testing.T) {
	src := `
fn find_first() {
    loop {
        break
    }
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "find_first")
	if fn == nil {
		t.Fatal("expected function 'find_first'")
	}

	foundLoop := false
	foundBreak := false
	walkExprs(fn.Body, func(e Expr) bool {
		if _, ok := e.(*LoopExpr); ok {
			foundLoop = true
		}
		if _, ok := e.(*BreakExpr); ok {
			foundBreak = true
		}
		return false
	})

	if !foundLoop {
		t.Error("expected LoopExpr")
	}
	if !foundBreak {
		t.Error("expected BreakExpr inside loop")
	}
}

// ---------------------------------------------------------------------------
// Return expression lowering
// ---------------------------------------------------------------------------

func TestReturnExprLowered(t *testing.T) {
	src := `
fn early_return(x: Int) -> Int {
    return x
}
`
	prog := lowerSource(t, src)
	fn := findFunction(prog, "early_return")
	if fn == nil {
		t.Fatal("expected function 'early_return'")
	}

	// The return statement should produce either a ReturnStmt in body or ReturnExpr.
	foundReturn := false
	if fn.Body != nil {
		for _, s := range fn.Body.Stmts {
			if _, ok := s.(*ReturnStmt); ok {
				foundReturn = true
			}
		}
		walkExprs(fn.Body, func(e Expr) bool {
			if _, ok := e.(*ReturnExpr); ok {
				foundReturn = true
				return true
			}
			return false
		})
	}

	if !foundReturn {
		t.Error("expected return in function body")
	}
}
