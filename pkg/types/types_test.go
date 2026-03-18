package types

import (
	"strings"
	"testing"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/parser"
	"github.com/ryx-lang/ryx/pkg/resolver"
)

// ---------------------------------------------------------------------------
// Helper: parse, resolve, and type-check source code
// ---------------------------------------------------------------------------

func checkSource(t *testing.T, src string) *CheckResult {
	t.Helper()
	ResetTypeVarCounter()

	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("test.ryx", src)

	result := parser.Parse(src, fileID)
	if result.HasErrors() {
		for _, e := range result.Errors {
			t.Logf("parse error: %s", e.Message)
		}
		t.Fatalf("source has parse errors")
	}

	resolved := resolver.Resolve(result.Program, registry)
	// Filter out resolver warnings (unused vars etc.) — we care about type errors.
	var resolverErrors []diagnostic.Diagnostic
	for _, d := range resolved.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			resolverErrors = append(resolverErrors, d)
		}
	}
	if len(resolverErrors) > 0 {
		for _, e := range resolverErrors {
			t.Logf("resolver error: [%s] %s", e.Code, e.Message)
		}
		t.Fatalf("source has resolver errors")
	}

	return Check(result.Program, resolved, registry)
}

// checkSourceAllowResolverErrors is like checkSource but does not fail on resolver errors.
// Used for tests where the resolver catches the same issue we want to test in the type checker.
func checkSourceAllowResolverErrors(t *testing.T, src string) *CheckResult {
	t.Helper()
	ResetTypeVarCounter()

	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("test.ryx", src)

	result := parser.Parse(src, fileID)
	if result.HasErrors() {
		for _, e := range result.Errors {
			t.Logf("parse error: %s", e.Message)
		}
		t.Fatalf("source has parse errors")
	}

	resolved := resolver.Resolve(result.Program, registry)
	return Check(result.Program, resolved, registry)
}

func expectNoErrors(t *testing.T, result *CheckResult) {
	t.Helper()
	errors := filterErrors(result)
	if len(errors) > 0 {
		for _, e := range errors {
			t.Errorf("unexpected error: [%s] %s", e.Code, e.Message)
		}
	}
}

func expectError(t *testing.T, result *CheckResult, code string) {
	t.Helper()
	for _, d := range result.Diagnostics {
		if d.Code == code && d.Severity == diagnostic.SeverityError {
			return
		}
	}
	t.Errorf("expected error %s, but not found. Got diagnostics:", code)
	for _, d := range result.Diagnostics {
		t.Logf("  [%s] %s (%s)", d.Code, d.Message, d.Severity)
	}
}

func expectErrorContaining(t *testing.T, result *CheckResult, substr string) {
	t.Helper()
	for _, d := range result.Diagnostics {
		if d.Severity == diagnostic.SeverityError && strings.Contains(d.Message, substr) {
			return
		}
	}
	t.Errorf("expected error containing %q, but not found. Got diagnostics:", substr)
	for _, d := range result.Diagnostics {
		t.Logf("  [%s] %s (%s)", d.Code, d.Message, d.Severity)
	}
}

func filterErrors(result *CheckResult) []diagnostic.Diagnostic {
	var errors []diagnostic.Diagnostic
	for _, d := range result.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			errors = append(errors, d)
		}
	}
	return errors
}

// ===========================================================================
// INFERENCE TESTS
// ===========================================================================

func TestInference_IntLiteral(t *testing.T) {
	result := checkSource(t, `fn main() { let x = 42; }`)
	expectNoErrors(t, result)
}

func TestInference_FloatLiteral(t *testing.T) {
	result := checkSource(t, `fn main() { let x = 3.14; }`)
	expectNoErrors(t, result)
}

func TestInference_BoolLiteral(t *testing.T) {
	result := checkSource(t, `fn main() { let x = true; }`)
	expectNoErrors(t, result)
}

func TestInference_StringLiteral(t *testing.T) {
	result := checkSource(t, `fn main() { let x = "hello"; }`)
	expectNoErrors(t, result)
}

func TestInference_CharLiteral(t *testing.T) {
	result := checkSource(t, `fn main() { let x = 'a'; }`)
	expectNoErrors(t, result)
}

func TestInference_AnnotatedLetMatch(t *testing.T) {
	result := checkSource(t, `fn main() { let x: Int = 42; }`)
	expectNoErrors(t, result)
}

func TestInference_AnnotatedLetMismatch(t *testing.T) {
	result := checkSource(t, `fn main() { let x: Int = "hello"; }`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestInference_ArithmeticOps(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let a = 1 + 2;
    let b = 3 - 1;
    let c = 2 * 3;
    let d = 10 / 2;
    let e = 10 % 3;
}
`)
	expectNoErrors(t, result)
}

func TestInference_ArithmeticTypeMismatch(t *testing.T) {
	result := checkSource(t, `fn main() { let x = 1 + "hello"; }`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestInference_ComparisonOps(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let a = 1 == 2;
    let b = 1 != 2;
    let c = 1 < 2;
    let d = 1 > 2;
    let e = 1 <= 2;
    let f = 1 >= 2;
}
`)
	expectNoErrors(t, result)
}

func TestInference_LogicalOps(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let a = true && false;
    let b = true || false;
}
`)
	expectNoErrors(t, result)
}

func TestInference_LogicalOpTypeMismatch(t *testing.T) {
	result := checkSource(t, `fn main() { let x = 1 && 2; }`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestInference_StringConcat(t *testing.T) {
	result := checkSource(t, `fn main() { let x = "hello" ++ " world"; }`)
	expectNoErrors(t, result)
}

func TestInference_StringConcatMismatch(t *testing.T) {
	result := checkSource(t, `fn main() { let x = 42 ++ "hello"; }`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestInference_FunctionReturnType(t *testing.T) {
	result := checkSource(t, `
fn add(a: Int, b: Int) -> Int {
    a + b
}
fn main() { let x = add(1, 2); }
`)
	expectNoErrors(t, result)
}

func TestInference_FunctionReturnTypeMismatch(t *testing.T) {
	result := checkSource(t, `
fn bad() -> Int {
    "hello"
}
fn main() { bad(); }
`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestInference_FunctionCallArgMismatch(t *testing.T) {
	result := checkSource(t, `
fn double(x: Int) -> Int { x * 2 }
fn main() { double("hello"); }
`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestInference_UnitReturn(t *testing.T) {
	result := checkSource(t, `
fn nothing() {
}
fn main() { nothing(); }
`)
	expectNoErrors(t, result)
}

func TestInference_NestedLet(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let x = 42;
    let y = x + 1;
}
`)
	expectNoErrors(t, result)
}

func TestInference_LetWithBlock(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let x = {
        let a = 1;
        a + 2
    };
}
`)
	expectNoErrors(t, result)
}

// ===========================================================================
// IF/ELSE BRANCH UNIFICATION
// ===========================================================================

func TestIfElse_MatchingBranches(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let x = if true { 1 } else { 2 };
}
`)
	expectNoErrors(t, result)
}

func TestIfElse_MismatchingBranches(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let x = if true { 1 } else { "hello" };
}
`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestIfElse_BoolCondRequired(t *testing.T) {
	result := checkSource(t, `fn main() { if 42 { } }`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestIfElse_NoElseIsUnit(t *testing.T) {
	result := checkSource(t, `fn main() { if true { 42; }; }`)
	expectNoErrors(t, result)
}

// ===========================================================================
// MATCH ARM UNIFICATION
// ===========================================================================

func TestMatch_BasicIntMatch(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let x = 42;
    let y = match x {
        1 => "one",
        _ => "other",
    };
}
`)
	expectNoErrors(t, result)
}

func TestMatch_ArmTypeMismatch(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let x = 42;
    let y = match x {
        1 => "one",
        _ => 42,
    };
}
`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestMatch_EnumVariants(t *testing.T) {
	result := checkSource(t, `
type Color {
    Red,
    Green,
    Blue,
}
fn main() {
    let c = Red;
    let name = match c {
        Red => "red",
        Green => "green",
        Blue => "blue",
    };
}
`)
	expectNoErrors(t, result)
}

func TestMatch_EnumWithData(t *testing.T) {
	result := checkSource(t, `
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
fn main() { let a = area(Circle(3.14)); }
`)
	expectNoErrors(t, result)
}

// ===========================================================================
// ARRAY AND TUPLE TESTS
// ===========================================================================

func TestArray_HomogeneousLiteral(t *testing.T) {
	result := checkSource(t, `fn main() { let xs = [1, 2, 3]; }`)
	expectNoErrors(t, result)
}

func TestArray_HeterogeneousLiteral(t *testing.T) {
	result := checkSource(t, `fn main() { let xs = [1, "two", 3]; }`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestArray_Indexing(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let xs = [1, 2, 3];
    let first = xs[0];
}
`)
	expectNoErrors(t, result)
}

func TestArray_IndexMustBeInt(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let xs = [1, 2, 3];
    let bad = xs["hello"];
}
`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestTuple_Literal(t *testing.T) {
	result := checkSource(t, `fn main() { let t = (1, "hello", true); }`)
	expectNoErrors(t, result)
}

// ===========================================================================
// STRUCT TESTS
// ===========================================================================

func TestStruct_LiteralCorrect(t *testing.T) {
	result := checkSource(t, `
struct Point { x: Int, y: Int }
fn main() { let p = Point { x: 1, y: 2 }; }
`)
	expectNoErrors(t, result)
}

func TestStruct_FieldTypeMismatch(t *testing.T) {
	result := checkSource(t, `
struct Point { x: Int, y: Int }
fn main() { let p = Point { x: 1, y: "hello" }; }
`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestStruct_MissingField(t *testing.T) {
	result := checkSource(t, `
struct Point { x: Int, y: Int }
fn main() { let p = Point { x: 1 }; }
`)
	expectError(t, result, "E142")
}

func TestStruct_UnknownField(t *testing.T) {
	result := checkSource(t, `
struct Point { x: Int, y: Int }
fn main() { let p = Point { x: 1, y: 2, z: 3 }; }
`)
	expectError(t, result, "E141")
}

func TestStruct_FieldAccess(t *testing.T) {
	result := checkSource(t, `
struct Point { x: Int, y: Int }
fn main() {
    let p = Point { x: 1, y: 2 };
    let a = p.x;
}
`)
	expectNoErrors(t, result)
}

// ===========================================================================
// CLOSURE/LAMBDA TESTS
// ===========================================================================

func TestLambda_Basic(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let f = |x: Int| x + 1;
    let y = f(42);
}
`)
	expectNoErrors(t, result)
}

func TestLambda_InferredParamTypes(t *testing.T) {
	result := checkSource(t, `
fn apply(f: (Int) -> Int, x: Int) -> Int { f(x) }
fn main() {
    let result = apply(|x: Int| x * 2, 21);
}
`)
	expectNoErrors(t, result)
}

// ===========================================================================
// SPAWN AND CHANNEL TESTS
// ===========================================================================

func TestSpawn_ReturnsUnit(t *testing.T) {
	result := checkSource(t, `
fn main() {
    spawn {
        let x = 42;
    };
}
`)
	expectNoErrors(t, result)
}

func TestChannel_BasicType(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let ch = channel<Int>;
}
`)
	expectNoErrors(t, result)
}

// ===========================================================================
// LOOP TESTS
// ===========================================================================

func TestWhile_BoolCondRequired(t *testing.T) {
	result := checkSource(t, `
fn main() {
    while 42 { }
}
`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestFor_Basic(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let xs = [1, 2, 3];
    for x in xs {
        let y = x + 1;
    }
}
`)
	expectNoErrors(t, result)
}

// ===========================================================================
// TRAIT TESTS
// ===========================================================================

func TestTrait_ImplAndUse(t *testing.T) {
	result := checkSource(t, `
trait Greet {
    fn greet(self) -> String;
}
struct Person { name: String }
impl Greet for Person {
    fn greet(self) -> String {
        self.name
    }
}
fn main() {
    let p = Person { name: "Alice" };
}
`)
	expectNoErrors(t, result)
}

func TestTrait_InherentImpl(t *testing.T) {
	result := checkSource(t, `
struct Counter { value: Int }
impl Counter {
    fn new() -> Counter {
        Counter { value: 0 }
    }
}
fn main() {
    let c = Counter { value: 0 };
}
`)
	expectNoErrors(t, result)
}

// ===========================================================================
// UNIFICATION ENGINE TESTS
// ===========================================================================

func TestUnionFind_Basic(t *testing.T) {
	ResetTypeVarCounter()
	uf := NewUnionFind()

	uf.MakeSet(1)
	uf.MakeSet(2)
	uf.MakeSet(3)

	uf.Union(1, 2)
	if uf.Find(1) != uf.Find(2) {
		t.Error("expected 1 and 2 in same set")
	}
	if uf.Find(1) == uf.Find(3) {
		t.Error("expected 1 and 3 in different sets")
	}
}

func TestUnionFind_TypeBinding(t *testing.T) {
	ResetTypeVarCounter()
	uf := NewUnionFind()

	uf.MakeSet(1)
	uf.BindType(1, TypInt)

	resolved := uf.ResolveVar(1)
	if !resolved.Equal(TypInt) {
		t.Errorf("expected Int, got %v", resolved)
	}
}

func TestUnionFind_TransitiveBinding(t *testing.T) {
	ResetTypeVarCounter()
	uf := NewUnionFind()

	uf.MakeSet(1)
	uf.MakeSet(2)
	uf.Union(1, 2)
	uf.BindType(1, TypString)

	resolved := uf.ResolveVar(2)
	if resolved == nil || !resolved.Equal(TypString) {
		t.Errorf("expected String via transitive binding, got %v", resolved)
	}
}

func TestUnification_OccursCheck(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	tv := FreshTypeVar()
	engine.RegisterTypeVar(tv)

	// Try to unify tv with [tv] — should produce occurs-check error.
	engine.Unify(tv, &ArrayType{Elem: tv}, diagnostic.Span{})

	if !collector.HasErrors() {
		t.Error("expected occurs check error")
	}
}

func TestUnification_StructuralMatch(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	tv1 := FreshTypeVar()
	tv2 := FreshTypeVar()
	engine.RegisterTypeVar(tv1)
	engine.RegisterTypeVar(tv2)

	fn1 := &FnType{Params: []Type{TypInt}, Return: tv1}
	fn2 := &FnType{Params: []Type{TypInt}, Return: TypBool}

	engine.Unify(fn1, fn2, diagnostic.Span{})

	resolved := engine.ResolveType(tv1)
	if !resolved.Equal(TypBool) {
		t.Errorf("expected tv1 to resolve to Bool, got %v", resolved)
	}
}

// ===========================================================================
// GENERALIZATION AND INSTANTIATION TESTS
// ===========================================================================

func TestGeneralize_NoFreeVars(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	scheme := engine.Generalize(TypInt, nil)
	if len(scheme.Vars) != 0 {
		t.Error("expected no quantified vars for monomorphic type")
	}
}

func TestGeneralize_WithFreeVar(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	tv := FreshTypeVar()
	engine.RegisterTypeVar(tv)

	fn := &FnType{Params: []Type{tv}, Return: tv}
	scheme := engine.Generalize(fn, nil)

	if len(scheme.Vars) != 1 {
		t.Errorf("expected 1 quantified var, got %d", len(scheme.Vars))
	}
}

func TestInstantiate_FreshVars(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	tv := FreshTypeVar()
	engine.RegisterTypeVar(tv)

	fn := &FnType{Params: []Type{tv}, Return: tv}
	scheme := engine.Generalize(fn, nil)

	inst1, _ := engine.Instantiate(scheme)
	inst2, _ := engine.Instantiate(scheme)

	// Two instantiations should produce different type variables.
	fn1, ok1 := inst1.(*FnType)
	fn2, ok2 := inst2.(*FnType)
	if !ok1 || !ok2 {
		t.Fatal("expected FnType instances")
	}

	tv1, ok1 := fn1.Params[0].(*TypeVar)
	tv2, ok2 := fn2.Params[0].(*TypeVar)
	if !ok1 || !ok2 {
		t.Fatal("expected TypeVar params")
	}

	if tv1.ID == tv2.ID {
		t.Error("expected different type vars from two instantiations")
	}
}

// ===========================================================================
// TRAIT REGISTRY TESTS
// ===========================================================================

func TestTraitRegistry_Coherence(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	tr := NewTraitRegistry(collector)

	tr.RegisterTrait("Show", []TraitMethod{
		{Name: "show", ReturnType: TypString},
	})

	// First impl should succeed.
	ok := tr.RegisterImpl(ImplEntry{
		TraitName:  "Show",
		TargetType: TypInt,
	})
	if !ok {
		t.Error("expected first impl to succeed")
	}

	// Duplicate impl should fail.
	ok = tr.RegisterImpl(ImplEntry{
		TraitName:  "Show",
		TargetType: TypInt,
	})
	if ok {
		t.Error("expected duplicate impl to fail")
	}
	if !collector.HasErrors() {
		t.Error("expected coherence error")
	}
}

func TestTraitRegistry_OrphanRule(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	tr := NewTraitRegistry(collector)

	// Neither trait nor type is local — should fail orphan rule.
	tr.RegisterTrait("ForeignTrait", nil)
	// Remove from local to simulate foreign.
	delete(tr.LocalTraits, "ForeignTrait")

	ok := tr.RegisterImpl(ImplEntry{
		TraitName:  "ForeignTrait",
		TargetType: TypInt, // built-in, not local
	})
	if ok {
		t.Error("expected orphan rule violation")
	}
}

func TestTraitRegistry_LookupImpl(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)
	tr := NewTraitRegistry(collector)
	tr.LocalTraits["Show"] = true

	tr.RegisterTrait("Show", []TraitMethod{
		{Name: "show", ReturnType: TypString},
	})

	tr.RegisterImpl(ImplEntry{
		TraitName:  "Show",
		TargetType: TypInt,
	})

	impl := tr.LookupImpl("Show", TypInt, engine)
	if impl == nil {
		t.Error("expected to find impl Show for Int")
	}

	impl = tr.LookupImpl("Show", TypBool, engine)
	if impl != nil {
		t.Error("expected no impl Show for Bool")
	}
}

// ===========================================================================
// TYPE EQUALITY AND STRING TESTS
// ===========================================================================

func TestType_Equality(t *testing.T) {
	tests := []struct {
		a, b Type
		want bool
	}{
		{TypInt, TypInt, true},
		{TypInt, TypFloat, false},
		{TypBool, TypBool, true},
		{&ArrayType{Elem: TypInt}, &ArrayType{Elem: TypInt}, true},
		{&ArrayType{Elem: TypInt}, &ArrayType{Elem: TypFloat}, false},
		{&TupleType{Elems: []Type{TypInt, TypBool}}, &TupleType{Elems: []Type{TypInt, TypBool}}, true},
		{&TupleType{Elems: []Type{TypInt}}, &TupleType{Elems: []Type{TypInt, TypBool}}, false},
		{&FnType{Params: []Type{TypInt}, Return: TypBool}, &FnType{Params: []Type{TypInt}, Return: TypBool}, true},
		{&FnType{Params: []Type{TypInt}, Return: TypBool}, &FnType{Params: []Type{TypFloat}, Return: TypBool}, false},
		{&ChannelType{Elem: TypInt}, &ChannelType{Elem: TypInt}, true},
		{&ChannelType{Elem: TypInt}, &ChannelType{Elem: TypString}, false},
	}

	for i, tt := range tests {
		got := tt.a.Equal(tt.b)
		if got != tt.want {
			t.Errorf("test %d: %s.Equal(%s) = %v, want %v", i, tt.a, tt.b, got, tt.want)
		}
	}
}

func TestType_String(t *testing.T) {
	tests := []struct {
		typ  Type
		want string
	}{
		{TypInt, "Int"},
		{TypFloat, "Float"},
		{TypBool, "Bool"},
		{TypChar, "Char"},
		{TypString, "String"},
		{TypUnit, "Unit"},
		{&ArrayType{Elem: TypInt}, "[Int]"},
		{&SliceType{Elem: TypString}, "[]String"},
		{&TupleType{Elems: []Type{TypInt, TypBool}}, "(Int, Bool)"},
		{&FnType{Params: []Type{TypInt}, Return: TypBool}, "(Int) -> Bool"},
		{&ChannelType{Elem: TypInt}, "channel<Int>"},
	}

	for i, tt := range tests {
		got := tt.typ.String()
		if got != tt.want {
			t.Errorf("test %d: got %q, want %q", i, got, tt.want)
		}
	}
}

// ===========================================================================
// FREE VARS TESTS
// ===========================================================================

func TestFreeVars_Primitive(t *testing.T) {
	fv := FreeVars(TypInt)
	if len(fv) != 0 {
		t.Error("expected no free vars in Int")
	}
}

func TestFreeVars_TypeVar(t *testing.T) {
	ResetTypeVarCounter()
	tv := FreshTypeVar()
	fv := FreeVars(tv)
	if len(fv) != 1 || !fv[tv.ID] {
		t.Errorf("expected free var %d, got %v", tv.ID, fv)
	}
}

func TestFreeVars_FnType(t *testing.T) {
	ResetTypeVarCounter()
	tv1 := FreshTypeVar()
	tv2 := FreshTypeVar()
	fn := &FnType{Params: []Type{tv1}, Return: tv2}

	fv := FreeVars(fn)
	if len(fv) != 2 {
		t.Errorf("expected 2 free vars, got %d", len(fv))
	}
}

// ===========================================================================
// COMPLEX TYPE CHECKING SCENARIOS
// ===========================================================================

func TestComplex_NestedFunctions(t *testing.T) {
	result := checkSource(t, `
fn add(a: Int, b: Int) -> Int { a + b }
fn double(x: Int) -> Int { x * 2 }
fn main() {
    let result = double(add(1, 2));
}
`)
	expectNoErrors(t, result)
}

func TestComplex_StructWithEnum(t *testing.T) {
	result := checkSource(t, `
type Color {
    Red,
    Green,
    Blue,
}
struct Pixel {
    x: Int,
    y: Int,
    color: Color,
}
fn main() {
    let p = Pixel { x: 0, y: 0, color: Red };
}
`)
	expectNoErrors(t, result)
}

func TestComplex_HigherOrderFunction(t *testing.T) {
	result := checkSource(t, `
fn apply(f: (Int) -> Int, x: Int) -> Int {
    f(x)
}
fn double(x: Int) -> Int { x * 2 }
fn main() {
    let result = apply(double, 21);
}
`)
	expectNoErrors(t, result)
}

func TestComplex_RecursiveFunction(t *testing.T) {
	result := checkSource(t, `
fn factorial(n: Int) -> Int {
    if n <= 1 {
        1
    } else {
        n * factorial(n - 1)
    }
}
fn main() { let x = factorial(5); }
`)
	expectNoErrors(t, result)
}

func TestComplex_PipeOperator(t *testing.T) {
	result := checkSource(t, `
fn double(x: Int) -> Int { x * 2 }
fn add_one(x: Int) -> Int { x + 1 }
fn main() {
    let result = 5 |> double |> add_one;
}
`)
	expectNoErrors(t, result)
}

func TestComplex_MutualRecursion(t *testing.T) {
	result := checkSource(t, `
fn is_even(n: Int) -> Bool {
    if n == 0 { true } else { is_odd(n - 1) }
}
fn is_odd(n: Int) -> Bool {
    if n == 0 { false } else { is_even(n - 1) }
}
fn main() {
    let a = is_even(4);
    let b = is_odd(3);
}
`)
	expectNoErrors(t, result)
}

// ===========================================================================
// AS EXPRESSION TESTS
// ===========================================================================

func TestAs_IntToFloat(t *testing.T) {
	result := checkSource(t, `fn main() { let x = 42 as Float; }`)
	expectNoErrors(t, result)
}

// ===========================================================================
// RETURN EXPRESSION TESTS
// ===========================================================================

func TestReturn_MatchesFunctionType(t *testing.T) {
	result := checkSource(t, `
fn foo() -> Int {
    return 42;
}
fn main() { foo(); }
`)
	expectNoErrors(t, result)
}

func TestReturn_TypeMismatch(t *testing.T) {
	result := checkSource(t, `
fn foo() -> Int {
    return "hello";
}
fn main() { foo(); }
`)
	expectErrorContaining(t, result, "type mismatch")
}

// ===========================================================================
// EMPTY ARRAY TESTS
// ===========================================================================

func TestEmptyArray(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let xs: [Int] = [];
}
`)
	expectNoErrors(t, result)
}

// ===========================================================================
// EXHAUSTIVENESS TESTS
// ===========================================================================

func TestExhaustiveness_AllVariantsCovered(t *testing.T) {
	result := checkSource(t, `
type Color {
    Red,
    Green,
    Blue,
}
fn main() {
    let c = Red;
    let name = match c {
        Red => "red",
        Green => "green",
        Blue => "blue",
    };
}
`)
	expectNoErrors(t, result)
}

func TestExhaustiveness_MissingVariant(t *testing.T) {
	// Resolver also catches this (E042), so use checkSourceAllowResolverErrors.
	result := checkSourceAllowResolverErrors(t, `
type Color {
    Red,
    Green,
    Blue,
}
fn main() {
    let c = Red;
    let name = match c {
        Red => "red",
        Green => "green",
    };
}
`)
	expectError(t, result, "E150")
}

func TestExhaustiveness_WildcardCoversAll(t *testing.T) {
	result := checkSource(t, `
type Color {
    Red,
    Green,
    Blue,
}
fn main() {
    let c = Red;
    let name = match c {
        Red => "red",
        _ => "other",
    };
}
`)
	expectNoErrors(t, result)
}

func TestExhaustiveness_BindingCoversAll(t *testing.T) {
	result := checkSource(t, `
type Color {
    Red,
    Green,
    Blue,
}
fn main() {
    let c = Red;
    let name = match c {
        Red => "red",
        other => "other",
    };
}
`)
	expectNoErrors(t, result)
}

func TestExhaustiveness_EnumWithData(t *testing.T) {
	result := checkSourceAllowResolverErrors(t, `
type Shape {
    Circle(Float),
    Rect(Float, Float),
}
fn main() {
    let s = Circle(1.0);
    let a = match s {
        Circle(r) => r,
    };
}
`)
	expectError(t, result, "E150")
}

func TestExhaustiveness_SingleVariantEnum(t *testing.T) {
	result := checkSource(t, `
type Wrapper {
    Value(Int),
}
fn main() {
    let w = Value(42);
    let v = match w {
        Value(x) => x,
    };
}
`)
	expectNoErrors(t, result)
}

func TestExhaustiveness_MissingMultipleVariants(t *testing.T) {
	// The resolver also catches non-exhaustive matches on enums.
	// We test the type checker's exhaustiveness check with a helper
	// that tolerates resolver errors.
	ResetTypeVarCounter()

	src := `
type Direction {
    North,
    South,
    East,
    West,
}
fn main() {
    let d = North;
    let s = match d {
        North => "n",
    };
}
`
	registry := diagnostic.NewSourceRegistry()
	fileID := registry.AddFile("test.ryx", src)

	parseResult := parser.Parse(src, fileID)
	if parseResult.HasErrors() {
		t.Fatalf("source has parse errors")
	}

	resolved := resolver.Resolve(parseResult.Program, registry)
	result := Check(parseResult.Program, resolved, registry)

	// Should have E150 from the type checker or resolver exhaustiveness error.
	found := false
	for _, d := range result.Diagnostics {
		if d.Severity == diagnostic.SeverityError && strings.Contains(d.Message, "non-exhaustive") {
			found = true
			break
		}
	}
	// Also check resolver diagnostics.
	for _, d := range resolved.Diagnostics {
		if d.Severity == diagnostic.SeverityError && strings.Contains(d.Message, "non-exhaustive") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected non-exhaustive match error from either resolver or type checker")
	}
}

// ===========================================================================
// MONOMORPHIZATION (INSTANTIATION) TESTS
// ===========================================================================

func TestMonomorphization_GenericIdentity(t *testing.T) {
	result := checkSource(t, `
fn identity<T>(x: T) -> T { x }
fn main() {
    let a = identity(42);
    let b = identity("hello");
    let c = identity(true);
}
`)
	expectNoErrors(t, result)
}

func TestMonomorphization_GenericApply(t *testing.T) {
	result := checkSource(t, `
fn apply<T, U>(f: (T) -> U, x: T) -> U { f(x) }
fn double(x: Int) -> Int { x * 2 }
fn main() {
    let result = apply(double, 21);
}
`)
	expectNoErrors(t, result)
}

func TestMonomorphization_GenericEnum(t *testing.T) {
	result := checkSource(t, `
type Option<T> {
    Some(T),
    None,
}
fn main() {
    let a = Some(42);
    let b = Some("hello");
}
`)
	expectNoErrors(t, result)
}

func TestMonomorphization_FreshVarsPerCallSite(t *testing.T) {
	// Each call to a generic function should get fresh type vars.
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	tv := FreshTypeVar()
	engine.RegisterTypeVar(tv)
	fn := &FnType{Params: []Type{tv}, Return: tv}
	scheme := engine.Generalize(fn, nil)

	// Instantiate twice — should get different vars.
	inst1, subst1 := engine.Instantiate(scheme)
	inst2, subst2 := engine.Instantiate(scheme)

	fn1 := inst1.(*FnType)
	fn2 := inst2.(*FnType)

	// Bind first to Int, second to String.
	engine.Unify(fn1.Params[0], TypInt, diagnostic.Span{})
	engine.Unify(fn2.Params[0], TypString, diagnostic.Span{})

	// Verify they resolved independently.
	for _, s := range subst1 {
		resolved := engine.ResolveType(s)
		if !resolved.Equal(TypInt) {
			t.Errorf("first instantiation param should be Int, got %s", resolved)
		}
	}
	for _, s := range subst2 {
		resolved := engine.ResolveType(s)
		if !resolved.Equal(TypString) {
			t.Errorf("second instantiation param should be String, got %s", resolved)
		}
	}
}

func TestMonomorphization_TraitConstraintsPropagated(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	tv := FreshTypeVar()
	engine.RegisterTypeVar(tv)

	fn := &FnType{Params: []Type{tv}, Return: tv}
	scheme := &TypeScheme{
		Vars:        []int{tv.ID},
		Constraints: []TraitBound{{TraitName: "Add", TypeVar: tv.ID}},
		Body:        fn,
	}

	// Instantiate — should re-emit trait constraints.
	_, _ = engine.Instantiate(scheme)

	if len(engine.TraitConstraints) != 1 {
		t.Errorf("expected 1 trait constraint after instantiation, got %d", len(engine.TraitConstraints))
	}
	if engine.TraitConstraints[0].Trait != "Add" {
		t.Errorf("expected Add trait constraint, got %s", engine.TraitConstraints[0].Trait)
	}
}

// ===========================================================================
// TRAIT CONSTRAINT CHECKING TESTS
// ===========================================================================

func TestTraitConstraint_SatisfiedByBuiltin(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)
	tr := NewTraitRegistry(collector)
	tr.RegisterBuiltinTraits()

	// Add a constraint that Int : Add (should be satisfied by builtin).
	engine.AddTraitConstraint(TypInt, "Add", diagnostic.Span{})
	tr.CheckTraitConstraints(engine)

	if collector.HasErrors() {
		t.Error("expected no errors — Int implements Add")
	}
}

func TestTraitConstraint_UnsatisfiedForBool(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)
	tr := NewTraitRegistry(collector)
	tr.RegisterBuiltinTraits()

	// Bool does NOT implement Add.
	engine.AddTraitConstraint(TypBool, "Add", diagnostic.Span{})
	tr.CheckTraitConstraints(engine)

	if !collector.HasErrors() {
		t.Error("expected error — Bool does not implement Add")
	}
}

func TestTraitConstraint_SkippedForUnresolvedTypeVar(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)
	tr := NewTraitRegistry(collector)
	tr.RegisterBuiltinTraits()

	// Constraint on unresolved type var should be skipped (polymorphic context).
	tv := FreshTypeVar()
	engine.RegisterTypeVar(tv)
	engine.AddTraitConstraint(tv, "Add", diagnostic.Span{})
	tr.CheckTraitConstraints(engine)

	if collector.HasErrors() {
		t.Error("expected no errors — unresolved type var should be skipped")
	}
}

func TestTraitConstraint_UserDefinedTrait(t *testing.T) {
	result := checkSource(t, `
trait Greet {
    fn greet(self) -> String;
}
struct Dog { name: String }
impl Greet for Dog {
    fn greet(self) -> String {
        self.name
    }
}
fn main() {
    let d = Dog { name: "Rex" };
}
`)
	expectNoErrors(t, result)
}

// ===========================================================================
// COHERENCE AND ORPHAN RULE TESTS
// ===========================================================================

func TestCoherence_DuplicateImplInSource(t *testing.T) {
	result := checkSource(t, `
trait Show {
    fn show(self) -> String;
}
struct Foo { x: Int }
impl Show for Foo {
    fn show(self) -> String { "foo" }
}
impl Show for Foo {
    fn show(self) -> String { "also foo" }
}
fn main() { }
`)
	expectError(t, result, "E110")
}

func TestCoherence_DifferentTraitsSameType(t *testing.T) {
	result := checkSource(t, `
trait A {
    fn a(self) -> Int;
}
trait B {
    fn b(self) -> Int;
}
struct Foo { x: Int }
impl A for Foo {
    fn a(self) -> Int { self.x }
}
impl B for Foo {
    fn b(self) -> Int { self.x }
}
fn main() { }
`)
	expectNoErrors(t, result)
}

func TestOrphanRule_LocalTraitForeignType(t *testing.T) {
	// Local trait + builtin type should be allowed.
	result := checkSource(t, `
trait MyTrait {
    fn my_method(self) -> Int;
}
impl MyTrait for Int {
    fn my_method(self) -> Int { 42 }
}
fn main() { }
`)
	// This should pass — the trait is local.
	expectNoErrors(t, result)
}

// ===========================================================================
// GENERIC IMPL MATCHING TESTS
// ===========================================================================

func TestGenericImplMatching_MatchesConcrete(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)
	tr := NewTraitRegistry(collector)

	tr.RegisterTrait("Show", []TraitMethod{
		{Name: "show", ReturnType: TypString},
	})

	// Register a generic impl: impl<T> Show for [T]
	tv := FreshTypeVar()
	engine.RegisterTypeVar(tv)
	tr.RegisterImpl(ImplEntry{
		TraitName:  "Show",
		TargetType: &ArrayType{Elem: tv},
		GenParams:  []int{tv.ID},
	})

	// Should match [Int].
	impl := tr.LookupImpl("Show", &ArrayType{Elem: TypInt}, engine)
	if impl == nil {
		t.Error("expected generic impl to match [Int]")
	}

	// Should match [String].
	impl = tr.LookupImpl("Show", &ArrayType{Elem: TypString}, engine)
	if impl == nil {
		t.Error("expected generic impl to match [String]")
	}
}

func TestGenericImplMatching_NoMatchWrongShape(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)
	tr := NewTraitRegistry(collector)

	tr.RegisterTrait("Show", []TraitMethod{
		{Name: "show", ReturnType: TypString},
	})

	// Register impl<T> Show for [T]
	tv := FreshTypeVar()
	engine.RegisterTypeVar(tv)
	tr.RegisterImpl(ImplEntry{
		TraitName:  "Show",
		TargetType: &ArrayType{Elem: tv},
		GenParams:  []int{tv.ID},
	})

	// Should NOT match plain Int.
	impl := tr.LookupImpl("Show", TypInt, engine)
	if impl != nil {
		t.Error("expected generic impl for [T] to NOT match Int")
	}
}

// ===========================================================================
// UNIFICATION EDGE CASES
// ===========================================================================

func TestUnification_TupleLengthMismatch(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	t1 := &TupleType{Elems: []Type{TypInt}}
	t2 := &TupleType{Elems: []Type{TypInt, TypBool}}
	engine.Unify(t1, t2, diagnostic.Span{})

	if !collector.HasErrors() {
		t.Error("expected type mismatch error for tuples of different length")
	}
}

func TestUnification_FnArityMismatch(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	f1 := &FnType{Params: []Type{TypInt}, Return: TypBool}
	f2 := &FnType{Params: []Type{TypInt, TypFloat}, Return: TypBool}
	engine.Unify(f1, f2, diagnostic.Span{})

	if !collector.HasErrors() {
		t.Error("expected arity mismatch error")
	}
}

func TestUnification_StructNameMismatch(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	s1 := &StructType{Name: "Foo"}
	s2 := &StructType{Name: "Bar"}
	engine.Unify(s1, s2, diagnostic.Span{})

	if !collector.HasErrors() {
		t.Error("expected type mismatch for different struct names")
	}
}

func TestUnification_EnumNameMismatch(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	e1 := &EnumType{Name: "Color"}
	e2 := &EnumType{Name: "Shape"}
	engine.Unify(e1, e2, diagnostic.Span{})

	if !collector.HasErrors() {
		t.Error("expected type mismatch for different enum names")
	}
}

func TestUnification_ChannelElemUnification(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	tv := FreshTypeVar()
	engine.RegisterTypeVar(tv)

	ch1 := &ChannelType{Elem: tv}
	ch2 := &ChannelType{Elem: TypInt}
	engine.Unify(ch1, ch2, diagnostic.Span{})

	resolved := engine.ResolveType(tv)
	if !resolved.Equal(TypInt) {
		t.Errorf("expected channel elem to resolve to Int, got %s", resolved)
	}
}

func TestUnification_SliceUnification(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	tv := FreshTypeVar()
	engine.RegisterTypeVar(tv)

	s1 := &SliceType{Elem: tv}
	s2 := &SliceType{Elem: TypString}
	engine.Unify(s1, s2, diagnostic.Span{})

	resolved := engine.ResolveType(tv)
	if !resolved.Equal(TypString) {
		t.Errorf("expected slice elem to resolve to String, got %s", resolved)
	}
}

func TestUnification_NestedTypeVarResolution(t *testing.T) {
	ResetTypeVarCounter()
	collector := diagnostic.NewCollector(nil, 50, 50)
	engine := NewInferenceEngine(collector)

	tv1 := FreshTypeVar()
	tv2 := FreshTypeVar()
	engine.RegisterTypeVar(tv1)
	engine.RegisterTypeVar(tv2)

	// tv1 = [tv2], tv2 = Int -> tv1 = [Int]
	engine.Unify(tv1, &ArrayType{Elem: tv2}, diagnostic.Span{})
	engine.Unify(tv2, TypInt, diagnostic.Span{})

	resolved := engine.ResolveType(tv1)
	arr, ok := resolved.(*ArrayType)
	if !ok {
		t.Fatalf("expected ArrayType, got %T", resolved)
	}
	if !arr.Elem.Equal(TypInt) {
		t.Errorf("expected [Int], got %s", resolved)
	}
}

// ===========================================================================
// TYPE SCHEME AND ZONK TESTS
// ===========================================================================

func TestTypeScheme_String(t *testing.T) {
	ResetTypeVarCounter()
	tv := FreshTypeVar()
	scheme := &TypeScheme{
		Vars:        []int{tv.ID},
		Constraints: []TraitBound{{TraitName: "Add", TypeVar: tv.ID}},
		Body:        &FnType{Params: []Type{tv}, Return: tv},
	}
	s := scheme.String()
	if !strings.Contains(s, "forall") {
		t.Errorf("expected forall in scheme string, got %q", s)
	}
	if !strings.Contains(s, "Add") {
		t.Errorf("expected Add constraint in scheme string, got %q", s)
	}
}

func TestTypeScheme_MonoScheme(t *testing.T) {
	scheme := MonoScheme(TypInt)
	if len(scheme.Vars) != 0 {
		t.Error("MonoScheme should have no quantified vars")
	}
	if !scheme.Body.Equal(TypInt) {
		t.Errorf("MonoScheme body should be Int, got %s", scheme.Body)
	}
}

func TestZonk_ResolvesTypeVars(t *testing.T) {
	ResetTypeVarCounter()
	tv := FreshTypeVar()

	fn := &FnType{Params: []Type{tv}, Return: tv}
	resolved := Zonk(fn, func(v *TypeVar) Type {
		if v.ID == tv.ID {
			return TypInt
		}
		return v
	})

	ft, ok := resolved.(*FnType)
	if !ok {
		t.Fatalf("expected FnType, got %T", resolved)
	}
	if !ft.Params[0].Equal(TypInt) || !ft.Return.Equal(TypInt) {
		t.Errorf("expected (Int) -> Int, got %s", ft)
	}
}

func TestZonk_LeavesUnresolvedVars(t *testing.T) {
	ResetTypeVarCounter()
	tv := FreshTypeVar()
	resolved := Zonk(tv, func(v *TypeVar) Type { return v })
	if _, ok := resolved.(*TypeVar); !ok {
		t.Errorf("expected unresolved TypeVar, got %T", resolved)
	}
}

// ===========================================================================
// INTEGRATION-LEVEL INFERENCE TESTS
// ===========================================================================

func TestInference_GenericIdentityUsedMultipleTimes(t *testing.T) {
	result := checkSource(t, `
fn identity<T>(x: T) -> T { x }
fn main() {
    let a = identity(42);
    let b = identity("hello");
    let c = identity(true);
}
`)
	expectNoErrors(t, result)
}

func TestInference_ClosureParamInference(t *testing.T) {
	result := checkSource(t, `
fn apply(f: (Int) -> Int, x: Int) -> Int { f(x) }
fn main() {
    let result = apply(|x: Int| x * 2, 5);
}
`)
	expectNoErrors(t, result)
}

func TestInference_NestedClosures(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let add = |a: Int| {
        |b: Int| a + b
    };
}
`)
	expectNoErrors(t, result)
}

func TestInference_ForInRange(t *testing.T) {
	result := checkSource(t, `
fn main() {
    for i in [1, 2, 3] {
        let doubled = i * 2;
    }
}
`)
	expectNoErrors(t, result)
}

func TestInference_MatchWithVariantData(t *testing.T) {
	result := checkSource(t, `
type Option {
    Some(Int),
    None,
}
fn unwrap(opt: Option) -> Int {
    match opt {
        Some(x) => x,
        None => 0,
    }
}
fn main() { let v = unwrap(Some(42)); }
`)
	expectNoErrors(t, result)
}

func TestInference_TupleLiteral(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let t = (1, "hello", true);
}
`)
	expectNoErrors(t, result)
}

func TestInference_StructPattern(t *testing.T) {
	result := checkSource(t, `
struct Point { x: Int, y: Int }
fn get_x(p: Point) -> Int {
    match p {
        Point { x, y } => x,
    }
}
fn main() {
    let p = Point { x: 1, y: 2 };
    let x = get_x(p);
}
`)
	expectNoErrors(t, result)
}

func TestInference_SpawnMustReturnUnit(t *testing.T) {
	// spawn body with a non-unit trailing expression should cause an error.
	result := checkSource(t, `
fn main() {
    spawn {
        42
    };
}
`)
	expectErrorContaining(t, result, "type mismatch")
}

func TestInference_ChannelTypeConsistency(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let ch = channel<Int>;
}
`)
	expectNoErrors(t, result)
}

func TestInference_LoopReturnsUnit(t *testing.T) {
	result := checkSource(t, `
fn main() {
    loop { break; };
}
`)
	expectNoErrors(t, result)
}

func TestInference_NestedIfElse(t *testing.T) {
	result := checkSource(t, `
fn classify(n: Int) -> String {
    if n < 0 {
        "negative"
    } else {
        if n == 0 {
            "zero"
        } else {
            "positive"
        }
    }
}
fn main() { let s = classify(5); }
`)
	expectNoErrors(t, result)
}

func TestInference_RangeExpression(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let r = 1..10;
}
`)
	expectNoErrors(t, result)
}

func TestInference_AsExpression(t *testing.T) {
	result := checkSource(t, `
fn main() {
    let x = 42 as Float;
    let y = 3.14 as Int;
}
`)
	expectNoErrors(t, result)
}

// ===========================================================================
// BUILTIN TYPE HELPER TESTS
// ===========================================================================

func TestBuiltinType_Known(t *testing.T) {
	tests := []struct {
		name string
		want Type
	}{
		{"Int", TypInt},
		{"Float", TypFloat},
		{"Bool", TypBool},
		{"Char", TypChar},
		{"String", TypString},
		{"Unit", TypUnit},
	}
	for _, tt := range tests {
		got := BuiltinType(tt.name)
		if got == nil || !got.Equal(tt.want) {
			t.Errorf("BuiltinType(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestBuiltinType_Unknown(t *testing.T) {
	if got := BuiltinType("FooBar"); got != nil {
		t.Errorf("BuiltinType(FooBar) = %v, want nil", got)
	}
}

// ===========================================================================
// APPLYSUBST TESTS
// ===========================================================================

func TestApplySubst_Channel(t *testing.T) {
	ResetTypeVarCounter()
	tv := FreshTypeVar()
	replacement := FreshTypeVar()
	subst := map[int]*TypeVar{tv.ID: replacement}

	ch := &ChannelType{Elem: tv}
	result := applySubst(ch, subst)
	cht, ok := result.(*ChannelType)
	if !ok {
		t.Fatalf("expected ChannelType, got %T", result)
	}
	if rtv, ok := cht.Elem.(*TypeVar); !ok || rtv.ID != replacement.ID {
		t.Errorf("expected channel elem to be replaced")
	}
}

func TestApplySubst_Enum(t *testing.T) {
	ResetTypeVarCounter()
	tv := FreshTypeVar()
	replacement := FreshTypeVar()
	subst := map[int]*TypeVar{tv.ID: replacement}

	et := &EnumType{
		Name:     "Option",
		GenArgs:  []Type{tv},
		Variants: map[string][]Type{"Some": {tv}, "None": {}},
	}
	result := applySubst(et, subst)
	ent, ok := result.(*EnumType)
	if !ok {
		t.Fatalf("expected EnumType, got %T", result)
	}
	if rtv, ok := ent.GenArgs[0].(*TypeVar); !ok || rtv.ID != replacement.ID {
		t.Errorf("expected GenArgs to be substituted")
	}
	if rtv, ok := ent.Variants["Some"][0].(*TypeVar); !ok || rtv.ID != replacement.ID {
		t.Errorf("expected variant field to be substituted")
	}
}

// ===========================================================================
// FREE VARS COMPOSITE TYPE TESTS
// ===========================================================================

func TestFreeVars_Channel(t *testing.T) {
	ResetTypeVarCounter()
	tv := FreshTypeVar()
	ch := &ChannelType{Elem: tv}
	fv := FreeVars(ch)
	if !fv[tv.ID] {
		t.Errorf("expected free var %d in channel type", tv.ID)
	}
}

func TestFreeVars_Enum(t *testing.T) {
	ResetTypeVarCounter()
	tv1 := FreshTypeVar()
	tv2 := FreshTypeVar()
	et := &EnumType{
		Name:     "Result",
		GenArgs:  []Type{tv1, tv2},
		Variants: map[string][]Type{"Ok": {tv1}, "Err": {tv2}},
	}
	fv := FreeVars(et)
	if len(fv) != 2 || !fv[tv1.ID] || !fv[tv2.ID] {
		t.Errorf("expected 2 free vars, got %v", fv)
	}
}

func TestFreeVars_Struct(t *testing.T) {
	ResetTypeVarCounter()
	tv := FreshTypeVar()
	st := &StructType{
		Name:    "Wrapper",
		GenArgs: []Type{tv},
		Fields:  map[string]Type{"value": tv},
	}
	fv := FreeVars(st)
	if !fv[tv.ID] {
		t.Errorf("expected free var in struct type")
	}
}

func TestFreeVars_Tuple(t *testing.T) {
	ResetTypeVarCounter()
	tv1 := FreshTypeVar()
	tv2 := FreshTypeVar()
	tt := &TupleType{Elems: []Type{tv1, TypInt, tv2}}
	fv := FreeVars(tt)
	if len(fv) != 2 || !fv[tv1.ID] || !fv[tv2.ID] {
		t.Errorf("expected 2 free vars, got %v", fv)
	}
}

func TestFreeVars_Slice(t *testing.T) {
	ResetTypeVarCounter()
	tv := FreshTypeVar()
	sl := &SliceType{Elem: tv}
	fv := FreeVars(sl)
	if !fv[tv.ID] {
		t.Errorf("expected free var in slice type")
	}
}

func TestFreeVars_Array(t *testing.T) {
	ResetTypeVarCounter()
	tv := FreshTypeVar()
	arr := &ArrayType{Elem: tv}
	fv := FreeVars(arr)
	if !fv[tv.ID] {
		t.Errorf("expected free var in array type")
	}
}
