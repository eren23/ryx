package types

import (
	"fmt"
	"strings"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/lexer"
	"github.com/ryx-lang/ryx/pkg/parser"
	"github.com/ryx-lang/ryx/pkg/resolver"
)

// ---------------------------------------------------------------------------
// CheckResult
// ---------------------------------------------------------------------------

// CheckResult contains the output of type checking.
type CheckResult struct {
	Diagnostics []diagnostic.Diagnostic
	// NodeTypes maps AST node spans to their inferred types.
	NodeTypes map[diagnostic.Span]Type
	// SymbolTypes maps symbol spans to their type schemes.
	SymbolTypes map[diagnostic.Span]*TypeScheme
}

// HasErrors returns true if type checking produced any errors.
func (cr *CheckResult) HasErrors() bool {
	for _, d := range cr.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Checker
// ---------------------------------------------------------------------------

// Checker performs type inference and checking on a resolved AST.
type Checker struct {
	engine   *InferenceEngine
	traits   *TraitRegistry
	resolved *resolver.ResolveResult
	registry *diagnostic.SourceRegistry

	collector *diagnostic.Collector

	// env maps variable names to their type schemes in the current scope.
	env []map[string]*TypeScheme
	// nodeTypes records inferred types for AST nodes.
	nodeTypes map[diagnostic.Span]Type
	// symbolTypes records type schemes for symbols.
	symbolTypes map[diagnostic.Span]*TypeScheme

	// currentReturnType tracks the expected return type of the current function.
	currentReturnType Type
}

// Check performs type checking on a parsed and resolved program.
func Check(program *parser.Program, resolved *resolver.ResolveResult, registry *diagnostic.SourceRegistry) *CheckResult {
	collector := diagnostic.NewCollector(registry, 50, 50)
	engine := NewInferenceEngine(collector)
	traits := NewTraitRegistry(collector)
	traits.RegisterBuiltinTraits()

	c := &Checker{
		engine:      engine,
		traits:      traits,
		resolved:    resolved,
		registry:    registry,
		collector:   collector,
		nodeTypes:   make(map[diagnostic.Span]Type),
		symbolTypes: make(map[diagnostic.Span]*TypeScheme),
	}

	// Push global scope.
	c.pushEnv()

	// Register builtin function type signatures so the type checker
	// can infer correct return types for expressions like
	// int_to_float(x) / int_to_float(y) → Float (not unresolved).
	c.registerBuiltinTypes()

	// Register local types and traits for orphan rule.
	for name := range resolved.StructDefs {
		traits.LocalTypes[name] = true
	}
	for name := range resolved.TypeDefs {
		traits.LocalTypes[name] = true
	}
	for name := range resolved.TraitDefs {
		traits.LocalTraits[name] = true
	}

	// Pass 1: Register top-level function signatures and type definitions.
	c.registerTopLevel(program)

	// Pass 2: Register trait impls.
	c.registerImpls(program)

	// Pass 3: Type-check function bodies and expressions.
	c.checkProgram(program)

	// Solve remaining constraints.
	engine.Solve()

	// [CLAUDE-FIX] Resolve all type variables in nodeTypes to concrete types
	// so that the HIR lowerer and codegen get fully resolved types.
	for span, t := range c.nodeTypes {
		c.nodeTypes[span] = engine.ResolveType(t)
	}

	// Check trait constraints.
	traits.CheckTraitConstraints(engine)

	return &CheckResult{
		Diagnostics: collector.Diagnostics(),
		NodeTypes:   c.nodeTypes,
		SymbolTypes: c.symbolTypes,
	}
}

// ---------------------------------------------------------------------------
// Environment management
// ---------------------------------------------------------------------------

func (c *Checker) pushEnv() {
	c.env = append(c.env, make(map[string]*TypeScheme))
}

func (c *Checker) popEnv() {
	c.env = c.env[:len(c.env)-1]
}

func (c *Checker) defineVar(name string, scheme *TypeScheme) {
	c.env[len(c.env)-1][name] = scheme
}

func (c *Checker) lookupVar(name string) *TypeScheme {
	for i := len(c.env) - 1; i >= 0; i-- {
		if s, ok := c.env[i][name]; ok {
			return s
		}
	}
	return nil
}

func (c *Checker) currentEnvFreeVars() map[int]bool {
	combined := make(map[string]*TypeScheme)
	for _, scope := range c.env {
		for k, v := range scope {
			combined[k] = v
		}
	}
	return EnvFreeVars(combined)
}

// ---------------------------------------------------------------------------
// Builtin function type signatures
// ---------------------------------------------------------------------------

// registerBuiltinTypes adds type signatures for all builtin functions to the
// global scope. This ensures the type checker can infer correct return types
// for builtin calls, which is critical for downstream codegen (e.g., choosing
// OpDivFloat vs OpDivInt when the result of int_to_float is used in division).
// User-defined functions with the same names will shadow these in registerTopLevel.
func (c *Checker) registerBuiltinTypes() {
	// Only register builtins with concrete, non-polymorphic signatures where
	// the return type matters for downstream type inference (e.g., arithmetic).
	// Skip polymorphic builtins (print, min, max, abs, clamp, etc.) — they
	// stay as fresh type vars so they work with any argument type.
	builtins := map[string]*FnType{
		// Type conversions — critical for arithmetic type inference
		"int_to_float":    {Params: []Type{TypInt}, Return: TypFloat},
		"float_to_int":    {Params: []Type{TypFloat}, Return: TypInt},
		"int_to_string":   {Params: []Type{TypInt}, Return: TypString},
		"float_to_string": {Params: []Type{TypFloat}, Return: TypString},
		"parse_int":       {Params: []Type{TypString}, Return: TypInt},
		"parse_float":     {Params: []Type{TypString}, Return: TypFloat},
		"string_to_int":   {Params: []Type{TypString}, Return: TypInt},
		"string_to_float": {Params: []Type{TypString}, Return: TypFloat},
		"char_to_int":     {Params: []Type{TypChar}, Return: TypInt},

		// String operations — return type matters
		"string_len": {Params: []Type{TypString}, Return: TypInt},

		// Math (Float -> Float) — return type critical for arithmetic
		"sqrt":  {Params: []Type{TypFloat}, Return: TypFloat},
		"floor": {Params: []Type{TypFloat}, Return: TypFloat},
		"ceil":  {Params: []Type{TypFloat}, Return: TypFloat},
		"round": {Params: []Type{TypFloat}, Return: TypFloat},
		"sin":   {Params: []Type{TypFloat}, Return: TypFloat},
		"cos":   {Params: []Type{TypFloat}, Return: TypFloat},
		"tan":   {Params: []Type{TypFloat}, Return: TypFloat},
		"asin":  {Params: []Type{TypFloat}, Return: TypFloat},
		"acos":  {Params: []Type{TypFloat}, Return: TypFloat},
		"atan":  {Params: []Type{TypFloat}, Return: TypFloat},
		"atan2": {Params: []Type{TypFloat, TypFloat}, Return: TypFloat},
		"log":   {Params: []Type{TypFloat}, Return: TypFloat},
		"log2":  {Params: []Type{TypFloat}, Return: TypFloat},
		"log10": {Params: []Type{TypFloat}, Return: TypFloat},
		"exp":   {Params: []Type{TypFloat}, Return: TypFloat},
		"pow":   {Params: []Type{TypFloat, TypFloat}, Return: TypFloat},
		"pi":    {Params: []Type{}, Return: TypFloat},
		"e":     {Params: []Type{}, Return: TypFloat},
		"random_float": {Params: []Type{}, Return: TypFloat},
		"random_int":   {Params: []Type{TypInt, TypInt}, Return: TypInt},

		// Time
		"time_now_ms": {Params: []Type{}, Return: TypInt},

		// Graphics — return types used in expressions
		"gfx_rgb":          {Params: []Type{TypInt, TypInt, TypInt}, Return: TypInt},
		"gfx_rgba":         {Params: []Type{TypInt, TypInt, TypInt, TypInt}, Return: TypInt},
		"gfx_load_image":   {Params: []Type{TypString}, Return: TypInt},
		"gfx_image_width":  {Params: []Type{TypInt}, Return: TypInt},
		"gfx_image_height": {Params: []Type{TypInt}, Return: TypInt},
		"gfx_width":        {Params: []Type{}, Return: TypInt},
		"gfx_height":       {Params: []Type{}, Return: TypInt},
		"gfx_fps":          {Params: []Type{}, Return: TypFloat},
		"gfx_delta_time":   {Params: []Type{}, Return: TypFloat},
		"gfx_mouse_x":      {Params: []Type{}, Return: TypInt},
		"gfx_mouse_y":      {Params: []Type{}, Return: TypInt},
	}

	for name, fnType := range builtins {
		c.defineVar(name, MonoScheme(fnType))
	}
}

// ---------------------------------------------------------------------------
// Pass 1: Register top-level signatures
// ---------------------------------------------------------------------------

func (c *Checker) registerTopLevel(program *parser.Program) {
	for _, item := range program.Items {
		switch it := item.(type) {
		case *parser.FnDef:
			fnType := c.fnDefToType(it)
			scheme := MonoScheme(fnType)
			if it.GenParams != nil && len(it.GenParams.Params) > 0 {
				// Generalize over generic params.
				fv := FreeVars(fnType)
				var vars []int
				for id := range fv {
					vars = append(vars, id)
				}
				scheme = &TypeScheme{Vars: vars, Body: fnType}
			}
			c.defineVar(it.Name, scheme)
			c.symbolTypes[it.Span()] = scheme

		case *parser.StructDef:
			c.registerStructDef(it)
		case *parser.TypeDef:
			c.registerTypeDef(it)
		case *parser.TraitDef:
			c.registerTraitDef(it)
		}
	}
}

func (c *Checker) fnDefToType(fn *parser.FnDef) *FnType {
	params := make([]Type, len(fn.Params))
	for i, p := range fn.Params {
		if p.Type != nil {
			params[i] = c.resolveTypeExpr(p.Type)
		} else {
			tv := FreshTypeVar()
			c.engine.RegisterTypeVar(tv)
			params[i] = tv
		}
	}

	var ret Type
	if fn.ReturnType != nil {
		ret = c.resolveTypeExpr(fn.ReturnType)
	} else {
		ret = TypUnit
	}

	return &FnType{Params: params, Return: ret}
}

func (c *Checker) registerStructDef(sd *parser.StructDef) {
	fields := make(map[string]Type, len(sd.Fields))
	fieldOrder := make([]string, len(sd.Fields))
	for i, f := range sd.Fields {
		fields[f.Name] = c.resolveTypeExpr(f.Type)
		fieldOrder[i] = f.Name
	}

	var genArgs []Type
	if sd.GenParams != nil {
		for _, gp := range sd.GenParams.Params {
			tv := FreshTypeVar()
			c.engine.RegisterTypeVar(tv)
			genArgs = append(genArgs, tv)
			// Register the generic param name in current env.
			c.defineVar(gp.Name, MonoScheme(tv))
		}
	}

	st := &StructType{
		Name:       sd.Name,
		GenArgs:    genArgs,
		Fields:     fields,
		FieldOrder: fieldOrder,
	}
	c.defineVar(sd.Name, MonoScheme(st))
}

func (c *Checker) registerTypeDef(td *parser.TypeDef) {
	var genArgs []Type
	genParamNames := make(map[string]*TypeVar)
	if td.GenParams != nil {
		for _, gp := range td.GenParams.Params {
			tv := FreshTypeVar()
			c.engine.RegisterTypeVar(tv)
			genArgs = append(genArgs, tv)
			genParamNames[gp.Name] = tv
		}
	}

	variants := make(map[string][]Type, len(td.Variants))
	for _, v := range td.Variants {
		fieldTypes := make([]Type, len(v.Fields))
		for i, ft := range v.Fields {
			fieldTypes[i] = c.resolveTypeExpr(ft)
		}
		variants[v.Name] = fieldTypes
	}

	et := &EnumType{
		Name:     td.Name,
		GenArgs:  genArgs,
		Variants: variants,
	}

	c.defineVar(td.Name, MonoScheme(et))

	// Register variant constructors.
	for _, v := range td.Variants {
		fieldTypes := variants[v.Name]
		if len(fieldTypes) == 0 {
			// Unit variant — just the enum type.
			c.defineVar(v.Name, MonoScheme(et))
		} else {
			// Constructor function: (fields...) -> EnumType.
			fnType := &FnType{
				Params: fieldTypes,
				Return: et,
			}
			if len(genArgs) > 0 {
				fv := FreeVars(fnType)
				var vars []int
				for id := range fv {
					vars = append(vars, id)
				}
				c.defineVar(v.Name, &TypeScheme{Vars: vars, Body: fnType})
			} else {
				c.defineVar(v.Name, MonoScheme(fnType))
			}
		}
	}
}

func (c *Checker) registerTraitDef(td *parser.TraitDef) {
	methods := make([]TraitMethod, len(td.Methods))
	for i, m := range td.Methods {
		params := make([]Type, 0, len(m.Params))
		for _, p := range m.Params {
			if p.Name == "self" {
				continue
			}
			if p.Type != nil {
				params = append(params, c.resolveTypeExpr(p.Type))
			} else {
				tv := FreshTypeVar()
				c.engine.RegisterTypeVar(tv)
				params = append(params, tv)
			}
		}
		var ret Type
		if m.ReturnType != nil {
			ret = c.resolveTypeExpr(m.ReturnType)
		} else {
			ret = TypUnit
		}
		methods[i] = TraitMethod{
			Name:       m.Name,
			ParamTypes: params,
			ReturnType: ret,
		}
	}
	c.traits.RegisterTrait(td.Name, methods)
}

// ---------------------------------------------------------------------------
// Pass 2: Register trait impls
// ---------------------------------------------------------------------------

func (c *Checker) registerImpls(program *parser.Program) {
	for _, item := range program.Items {
		impl, ok := item.(*parser.ImplBlock)
		if !ok {
			continue
		}

		targetType := c.resolveTypeExpr(impl.TargetType)

		var genParams []int
		if impl.GenParams != nil {
			for _, gp := range impl.GenParams.Params {
				tv := FreshTypeVar()
				c.engine.RegisterTypeVar(tv)
				genParams = append(genParams, tv.ID)
				c.defineVar(gp.Name, MonoScheme(tv))
			}
		}

		methods := make(map[string]*FnType, len(impl.Methods))
		for _, m := range impl.Methods {
			ft := c.fnDefToType(m)
			// Strip 'self' parameter so stored method types are consistent
			// with trait definitions (which exclude self from ParamTypes).
			if len(m.Params) > 0 && m.Params[0].Name == "self" {
				ft = &FnType{Params: ft.Params[1:], Return: ft.Return}
			}
			methods[m.Name] = ft
		}

		entry := ImplEntry{
			TraitName:  impl.TraitName,
			TargetType: targetType,
			GenParams:  genParams,
			Methods:    methods,
			Span:       impl.Span(),
		}

		c.traits.RegisterImpl(entry)

		// Also register methods directly on the type for method lookup.
		typeName := typeNameOf(targetType)
		if impl.TraitName == "" {
			// Inherent impl — register each method as a function in the env.
			for mname, ft := range methods {
				methodName := typeName + "." + mname
				c.defineVar(methodName, MonoScheme(ft))
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Pass 3: Type-check program
// ---------------------------------------------------------------------------

func (c *Checker) checkProgram(program *parser.Program) {
	for _, item := range program.Items {
		c.checkItem(item)
	}
}

func (c *Checker) checkItem(item parser.Item) {
	switch it := item.(type) {
	case *parser.FnDef:
		c.checkFnDef(it)
	case *parser.ImplBlock:
		c.checkImplBlock(it)
	}
}

func (c *Checker) checkFnDef(fn *parser.FnDef) {
	if fn.Body == nil {
		return
	}

	c.pushEnv()
	defer c.popEnv()

	fnScheme := c.lookupVar(fn.Name)
	var fnType *FnType
	if fnScheme != nil {
		if ft, ok := fnScheme.Body.(*FnType); ok {
			fnType = ft
		}
	}
	if fnType == nil {
		fnType = c.fnDefToType(fn)
	}

	// Bind parameters.
	for i, p := range fn.Params {
		c.defineVar(p.Name, MonoScheme(fnType.Params[i]))
	}

	prevReturn := c.currentReturnType
	c.currentReturnType = fnType.Return

	bodyType := c.checkBlock(fn.Body)

	// Unify body type with return type.
	if bodyType != nil {
		c.engine.Unify(fnType.Return, bodyType, fn.Body.Span())
	}

	c.currentReturnType = prevReturn
}

func (c *Checker) checkImplBlock(impl *parser.ImplBlock) {
	c.pushEnv()
	defer c.popEnv()

	targetType := c.resolveTypeExpr(impl.TargetType)
	c.defineVar("self", MonoScheme(targetType))
	c.defineVar("Self", MonoScheme(targetType))

	for _, m := range impl.Methods {
		c.checkImplMethod(m, targetType)
	}
}

func (c *Checker) checkImplMethod(fn *parser.FnDef, selfType Type) {
	if fn.Body == nil {
		return
	}

	c.pushEnv()
	defer c.popEnv()

	fnType := c.fnDefToType(fn)

	// Bind self and params.
	c.defineVar("self", MonoScheme(selfType))
	for i, p := range fn.Params {
		if p.Name == "self" {
			continue
		}
		if i < len(fnType.Params) {
			c.defineVar(p.Name, MonoScheme(fnType.Params[i]))
		}
	}

	prevReturn := c.currentReturnType
	c.currentReturnType = fnType.Return

	bodyType := c.checkBlock(fn.Body)
	if bodyType != nil {
		c.engine.Unify(fnType.Return, bodyType, fn.Body.Span())
	}

	c.currentReturnType = prevReturn
}

// ---------------------------------------------------------------------------
// Expression type checking
// ---------------------------------------------------------------------------

func (c *Checker) checkExpr(expr parser.Expr) Type {
	if expr == nil {
		return TypUnit
	}

	var t Type
	switch e := expr.(type) {
	case *parser.IntLit:
		t = TypInt
	case *parser.FloatLit:
		t = TypFloat
	case *parser.StringLit:
		t = TypString
	case *parser.CharLit:
		t = TypChar
	case *parser.BoolLit:
		t = TypBool
	case *parser.Ident:
		t = c.checkIdent(e)
	case *parser.SelfExpr:
		t = c.checkSelf(e)
	case *parser.PathExpr:
		t = c.checkPathExpr(e)
	case *parser.Block:
		t = c.checkBlock(e)
	case *parser.BinaryExpr:
		t = c.checkBinaryExpr(e)
	case *parser.UnaryExpr:
		t = c.checkUnaryExpr(e)
	case *parser.CallExpr:
		t = c.checkCallExpr(e)
	case *parser.FieldExpr:
		t = c.checkFieldExpr(e)
	case *parser.IndexExpr:
		t = c.checkIndexExpr(e)
	case *parser.IfExpr:
		t = c.checkIfExpr(e)
	case *parser.MatchExpr:
		t = c.checkMatchExpr(e)
	case *parser.ForExpr:
		t = c.checkForExpr(e)
	case *parser.WhileExpr:
		t = c.checkWhileExpr(e)
	case *parser.LoopExpr:
		t = c.checkLoopExpr(e)
	case *parser.LambdaExpr:
		t = c.checkLambdaExpr(e)
	case *parser.SpawnExpr:
		t = c.checkSpawnExpr(e)
	case *parser.ChannelExpr:
		t = c.checkChannelExpr(e)
	case *parser.ArrayLit:
		t = c.checkArrayLit(e)
	case *parser.TupleLit:
		t = c.checkTupleLit(e)
	case *parser.StructLit:
		t = c.checkStructLit(e)
	case *parser.GroupExpr:
		t = c.checkExpr(e.Inner)
	case *parser.BreakExpr:
		t = TypUnit
	case *parser.ContinueExpr:
		t = TypUnit
	case *parser.ReturnExpr:
		t = c.checkReturnExpr(e)
	case *parser.AsExpr:
		t = c.checkAsExpr(e)
	default:
		t = TypUnit
	}

	c.nodeTypes[expr.Span()] = t
	return t
}

func (c *Checker) checkIdent(id *parser.Ident) Type {
	scheme := c.lookupVar(id.Name)
	if scheme == nil {
		// Check resolved symbols from the resolver.
		if sym, ok := c.resolved.Resolutions[id.Span()]; ok {
			if sym.Kind == resolver.VariantSymbol {
				return c.lookupVariantType(sym)
			}
		}
		// Unresolved — return a fresh type var.
		tv := FreshTypeVar()
		c.engine.RegisterTypeVar(tv)
		return tv
	}

	// Instantiate polymorphic types.
	if len(scheme.Vars) > 0 {
		t, _ := c.engine.Instantiate(scheme)
		return t
	}
	return scheme.Body
}

func (c *Checker) checkSelf(e *parser.SelfExpr) Type {
	scheme := c.lookupVar("self")
	if scheme == nil {
		tv := FreshTypeVar()
		c.engine.RegisterTypeVar(tv)
		return tv
	}
	return scheme.Body
}

func (c *Checker) checkPathExpr(pe *parser.PathExpr) Type {
	if len(pe.Segments) == 0 {
		return TypUnit
	}
	// Try to look up the full path as a variant or method.
	fullName := ""
	for i, seg := range pe.Segments {
		if i > 0 {
			fullName += "::"
		}
		fullName += seg
	}

	// Check if the last segment is a variant.
	lastSeg := pe.Segments[len(pe.Segments)-1]
	scheme := c.lookupVar(lastSeg)
	if scheme != nil {
		if len(scheme.Vars) > 0 {
			t, _ := c.engine.Instantiate(scheme)
			return t
		}
		return scheme.Body
	}

	tv := FreshTypeVar()
	c.engine.RegisterTypeVar(tv)
	return tv
}

func (c *Checker) lookupVariantType(sym *resolver.Symbol) Type {
	scheme := c.lookupVar(sym.Name)
	if scheme != nil {
		if len(scheme.Vars) > 0 {
			t, _ := c.engine.Instantiate(scheme)
			return t
		}
		return scheme.Body
	}
	tv := FreshTypeVar()
	c.engine.RegisterTypeVar(tv)
	return tv
}

func (c *Checker) checkBlock(block *parser.Block) Type {
	if block == nil {
		return TypUnit
	}
	c.pushEnv()
	defer c.popEnv()

	hasReturn := false
	for _, stmt := range block.Stmts {
		c.checkStmt(stmt)
		// Track if the block contains a return statement.
		switch stmt.(type) {
		case *parser.ReturnStmt:
			hasReturn = true
		}
	}

	if block.TrailingExpr != nil {
		return c.checkExpr(block.TrailingExpr)
	}

	// If the block ends with a return statement, its "block type" is
	// effectively divergent. We use a fresh type var so that the caller
	// (e.g., checkFnDef) doesn't get a spurious Unit mismatch.
	if hasReturn && len(block.Stmts) > 0 {
		if _, ok := block.Stmts[len(block.Stmts)-1].(*parser.ReturnStmt); ok {
			tv := FreshTypeVar()
			c.engine.RegisterTypeVar(tv)
			return tv
		}
	}
	return TypUnit
}

func (c *Checker) checkStmt(stmt parser.Stmt) {
	switch s := stmt.(type) {
	case *parser.LetStmt:
		c.checkLetStmt(s)
	case *parser.ExprStmt:
		c.checkExpr(s.Expr)
	case *parser.ReturnStmt:
		c.checkReturnStmt(s)
	}
}

func (c *Checker) checkLetStmt(s *parser.LetStmt) {
	var declaredType Type
	if s.Type != nil {
		declaredType = c.resolveTypeExpr(s.Type)
	}

	var valueType Type
	if s.Value != nil {
		valueType = c.checkExpr(s.Value)
	}

	if declaredType != nil && valueType != nil {
		c.engine.Unify(declaredType, valueType, s.Span())
		// Generalize at let-binding.
		envFV := c.currentEnvFreeVars()
		scheme := c.engine.Generalize(declaredType, envFV)
		c.defineVar(s.Name, scheme)
		c.symbolTypes[s.Span()] = scheme
	} else if declaredType != nil {
		c.defineVar(s.Name, MonoScheme(declaredType))
		c.symbolTypes[s.Span()] = MonoScheme(declaredType)
	} else if valueType != nil {
		envFV := c.currentEnvFreeVars()
		scheme := c.engine.Generalize(valueType, envFV)
		c.defineVar(s.Name, scheme)
		c.symbolTypes[s.Span()] = scheme
	} else {
		tv := FreshTypeVar()
		c.engine.RegisterTypeVar(tv)
		c.defineVar(s.Name, MonoScheme(tv))
		c.symbolTypes[s.Span()] = MonoScheme(tv)
	}
}

func (c *Checker) checkReturnStmt(s *parser.ReturnStmt) {
	if s.Value != nil {
		valType := c.checkExpr(s.Value)
		if c.currentReturnType != nil {
			c.engine.Unify(c.currentReturnType, valType, s.Span())
		}
	} else {
		if c.currentReturnType != nil {
			c.engine.Unify(c.currentReturnType, TypUnit, s.Span())
		}
	}
}

func (c *Checker) checkReturnExpr(e *parser.ReturnExpr) Type {
	if e.Value != nil {
		valType := c.checkExpr(e.Value)
		if c.currentReturnType != nil {
			c.engine.Unify(c.currentReturnType, valType, e.Span())
		}
	} else {
		if c.currentReturnType != nil {
			c.engine.Unify(c.currentReturnType, TypUnit, e.Span())
		}
	}
	// Return expression itself has type Unit (it diverges).
	return TypUnit
}

// ---------------------------------------------------------------------------
// Binary expressions
// ---------------------------------------------------------------------------

func (c *Checker) checkBinaryExpr(e *parser.BinaryExpr) Type {
	leftType := c.checkExpr(e.Left)
	rightType := c.checkExpr(e.Right)

	switch e.Op {
	// Arithmetic: +, -, *, /, %
	case lexer.PLUS:
		// Could be numeric addition or string concatenation.
		c.engine.Unify(leftType, rightType, e.Span())
		return leftType
	case lexer.MINUS, lexer.STAR, lexer.SLASH, lexer.PERCENT:
		c.engine.Unify(leftType, rightType, e.Span())
		return leftType

	// Comparison: ==, !=, <, >, <=, >=
	case lexer.EQ, lexer.NEQ:
		c.engine.Unify(leftType, rightType, e.Span())
		return TypBool
	case lexer.LT, lexer.GT, lexer.LEQ, lexer.GEQ:
		c.engine.Unify(leftType, rightType, e.Span())
		return TypBool

	// Logical: &&, ||
	case lexer.AND, lexer.OR:
		c.engine.Unify(leftType, TypBool, e.Span())
		c.engine.Unify(rightType, TypBool, e.Span())
		return TypBool

	// String concatenation: ++
	case lexer.CONCAT:
		c.engine.Unify(leftType, TypString, e.Span())
		c.engine.Unify(rightType, TypString, e.Span())
		return TypString

	// Pipe: |>
	case lexer.PIPE:
		// left |> right: right must be a function taking left's type.
		resultVar := FreshTypeVar()
		c.engine.RegisterTypeVar(resultVar)
		expectedFn := &FnType{Params: []Type{leftType}, Return: resultVar}
		c.engine.Unify(rightType, expectedFn, e.Span())
		return resultVar

	// Range: .., ..=
	case lexer.RANGE, lexer.RANGE_INCLUSIVE:
		c.engine.Unify(leftType, rightType, e.Span())
		return &ArrayType{Elem: leftType}

	// Assignment: =
	case lexer.ASSIGN:
		c.engine.Unify(leftType, rightType, e.Span())
		return TypUnit

	default:
		return leftType
	}
}

// ---------------------------------------------------------------------------
// Unary expressions
// ---------------------------------------------------------------------------

func (c *Checker) checkUnaryExpr(e *parser.UnaryExpr) Type {
	operandType := c.checkExpr(e.Operand)

	switch e.Op {
	case lexer.MINUS:
		// Numeric negation.
		return operandType
	case lexer.NOT:
		// Logical not.
		c.engine.Unify(operandType, TypBool, e.Span())
		return TypBool
	default:
		return operandType
	}
}

// ---------------------------------------------------------------------------
// Call expressions
// ---------------------------------------------------------------------------

func (c *Checker) checkCallExpr(e *parser.CallExpr) Type {
	funcType := c.checkExpr(e.Func)

	argTypes := make([]Type, len(e.Args))
	for i, arg := range e.Args {
		argTypes[i] = c.checkExpr(arg)
	}

	resultVar := FreshTypeVar()
	c.engine.RegisterTypeVar(resultVar)

	expectedFn := &FnType{
		Params: argTypes,
		Return: resultVar,
	}

	c.engine.Unify(funcType, expectedFn, e.Span())

	return resultVar
}

// ---------------------------------------------------------------------------
// Field access
// ---------------------------------------------------------------------------

func (c *Checker) checkFieldExpr(e *parser.FieldExpr) Type {
	objType := c.checkExpr(e.Object)
	resolved := c.engine.ResolveType(objType)

	switch rt := resolved.(type) {
	case *StructType:
		if ft, ok := rt.Fields[e.Field]; ok {
			return ft
		}
		// Check methods.
		methodFn, _ := c.traits.LookupMethod(rt, e.Field, c.engine)
		if methodFn != nil {
			return methodFn
		}
		c.collector.Error("E120",
			fmt.Sprintf("no field `%s` on type `%s`", e.Field, rt.String()),
			e.Span())

	case *TupleType:
		// Tuple field access by index (e.g., t.0, t.1).
		// The parser stores field name as a string.
		idx := 0
		for _, ch := range e.Field {
			if ch >= '0' && ch <= '9' {
				idx = idx*10 + int(ch-'0')
			} else {
				c.collector.Error("E121",
					fmt.Sprintf("invalid tuple field `%s`", e.Field),
					e.Span())
				tv := FreshTypeVar()
				c.engine.RegisterTypeVar(tv)
				return tv
			}
		}
		if idx < len(rt.Elems) {
			return rt.Elems[idx]
		}
		c.collector.Error("E122",
			fmt.Sprintf("tuple index %d out of range (tuple has %d elements)",
				idx, len(rt.Elems)),
			e.Span())

	case *EnumType:
		// Method call on enum.
		methodFn, _ := c.traits.LookupMethod(rt, e.Field, c.engine)
		if methodFn != nil {
			return methodFn
		}

	default:
		// Try method lookup on any resolved type.
		methodFn, _ := c.traits.LookupMethod(resolved, e.Field, c.engine)
		if methodFn != nil {
			return methodFn
		}
	}

	tv := FreshTypeVar()
	c.engine.RegisterTypeVar(tv)
	return tv
}

// ---------------------------------------------------------------------------
// Index expressions
// ---------------------------------------------------------------------------

func (c *Checker) checkIndexExpr(e *parser.IndexExpr) Type {
	objType := c.checkExpr(e.Object)
	idxType := c.checkExpr(e.Index)

	// Index must be Int.
	c.engine.Unify(idxType, TypInt, e.Span())

	resolved := c.engine.ResolveType(objType)
	switch rt := resolved.(type) {
	case *ArrayType:
		return rt.Elem
	case *SliceType:
		return rt.Elem
	case *StringType:
		return TypChar
	default:
		// For unresolved types, constrain it to be an array of something.
		elemVar := FreshTypeVar()
		c.engine.RegisterTypeVar(elemVar)
		c.engine.Unify(objType, &ArrayType{Elem: elemVar}, e.Span())
		return elemVar
	}
}

// ---------------------------------------------------------------------------
// If expression
// ---------------------------------------------------------------------------

func (c *Checker) checkIfExpr(e *parser.IfExpr) Type {
	condType := c.checkExpr(e.Cond)
	c.engine.Unify(condType, TypBool, e.Span())

	thenType := c.checkBlock(e.Then)

	if e.Else != nil {
		elseType := c.checkExpr(e.Else)
		// Branch unification: both branches must have the same type.
		c.engine.Unify(thenType, elseType, e.Span())
		return thenType
	}

	// No else branch: result is Unit.
	c.engine.Unify(thenType, TypUnit, e.Span())
	return TypUnit
}

// ---------------------------------------------------------------------------
// Match expression
// ---------------------------------------------------------------------------

func (c *Checker) checkMatchExpr(e *parser.MatchExpr) Type {
	scrutType := c.checkExpr(e.Scrutinee)

	if len(e.Arms) == 0 {
		return TypUnit
	}

	resultVar := FreshTypeVar()
	c.engine.RegisterTypeVar(resultVar)

	for _, arm := range e.Arms {
		c.pushEnv()

		// Check pattern and bind pattern variables.
		patType := c.checkPattern(arm.Pattern, scrutType)
		c.engine.Unify(scrutType, patType, arm.Pattern.Span())

		// Check guard.
		if arm.Guard != nil {
			guardType := c.checkExpr(arm.Guard)
			c.engine.Unify(guardType, TypBool, arm.Guard.Span())
		}

		// Check body — all arms must produce the same type.
		bodyType := c.checkExpr(arm.Body)
		c.engine.Unify(resultVar, bodyType, arm.Body.Span())

		c.popEnv()
	}

	// Exhaustiveness check for enum types.
	c.checkExhaustiveness(e, scrutType)

	return resultVar
}

// ---------------------------------------------------------------------------
// Exhaustiveness checking
// ---------------------------------------------------------------------------

// checkExhaustiveness verifies that a match expression covers all variants
// of an enum type. Only applies when the scrutinee resolves to an EnumType.
func (c *Checker) checkExhaustiveness(e *parser.MatchExpr, scrutType Type) {
	resolved := c.engine.ResolveType(scrutType)
	et, ok := resolved.(*EnumType)
	if !ok {
		return
	}

	// Collect covered variants from patterns.
	covered := make(map[string]bool)
	hasWildcard := false

	for _, arm := range e.Arms {
		c.collectCoveredVariants(arm.Pattern, covered, &hasWildcard)
		// If a guard is present, the arm doesn't guarantee coverage.
		if arm.Guard != nil {
			// Reset coverage for guarded arms — they don't fully cover.
			// We still keep previously covered unguarded arms.
		}
	}

	if hasWildcard {
		return
	}

	// Check which variants are missing.
	var missing []string
	for name := range et.Variants {
		if !covered[name] {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		// Sort for deterministic error messages.
		sortStrings(missing)
		c.collector.Error("E150",
			fmt.Sprintf("non-exhaustive match: missing variant(s) %s of enum `%s`",
				formatVariantList(missing), et.Name),
			e.Span())
	}
}

// collectCoveredVariants walks a pattern to find which enum variant names it covers.
func (c *Checker) collectCoveredVariants(pat parser.Pattern, covered map[string]bool, hasWildcard *bool) {
	if pat == nil {
		return
	}
	switch p := pat.(type) {
	case *parser.WildcardPat:
		*hasWildcard = true
	case *parser.BindingPat:
		// A bare binding covers everything (like a wildcard).
		*hasWildcard = true
	case *parser.VariantPat:
		covered[p.Name] = true
	case *parser.OrPat:
		for _, alt := range p.Alts {
			c.collectCoveredVariants(alt, covered, hasWildcard)
		}
	}
}

// sortStrings sorts a string slice in place (simple insertion sort to avoid import).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// formatVariantList formats a list of variant names for display.
func formatVariantList(names []string) string {
	if len(names) == 1 {
		return "`" + names[0] + "`"
	}
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = "`" + n + "`"
	}
	return strings.Join(parts, ", ")
}

// checkPattern type-checks a pattern and returns the type it matches.
// Also binds pattern variables in the current environment.
func (c *Checker) checkPattern(pat parser.Pattern, expected Type) Type {
	if pat == nil {
		return expected
	}

	switch p := pat.(type) {
	case *parser.WildcardPat:
		return expected

	case *parser.BindingPat:
		c.defineVar(p.Name, MonoScheme(expected))
		return expected

	case *parser.LiteralPat:
		return c.checkExpr(p.Value)

	case *parser.TuplePat:
		elems := make([]Type, len(p.Elems))
		resolvedExpected := c.engine.ResolveType(expected)
		if tt, ok := resolvedExpected.(*TupleType); ok && len(tt.Elems) == len(p.Elems) {
			for i, ep := range p.Elems {
				elems[i] = c.checkPattern(ep, tt.Elems[i])
			}
		} else {
			for i, ep := range p.Elems {
				tv := FreshTypeVar()
				c.engine.RegisterTypeVar(tv)
				elems[i] = c.checkPattern(ep, tv)
			}
		}
		return &TupleType{Elems: elems}

	case *parser.VariantPat:
		return c.checkVariantPattern(p, expected)

	case *parser.StructPat:
		return c.checkStructPattern(p, expected)

	case *parser.OrPat:
		if len(p.Alts) == 0 {
			return expected
		}
		t := c.checkPattern(p.Alts[0], expected)
		for _, alt := range p.Alts[1:] {
			altType := c.checkPattern(alt, expected)
			c.engine.Unify(t, altType, alt.Span())
		}
		return t

	case *parser.RangePat:
		startType := c.checkExpr(p.Start)
		endType := c.checkExpr(p.End)
		c.engine.Unify(startType, endType, p.Span())
		return startType

	default:
		return expected
	}
}

func (c *Checker) checkVariantPattern(p *parser.VariantPat, expected Type) Type {
	// Look up the variant constructor.
	scheme := c.lookupVar(p.Name)
	if scheme == nil {
		return expected
	}

	instType, _ := c.engine.Instantiate(scheme)

	// If it's a function type (variant with fields), match fields.
	if ft, ok := instType.(*FnType); ok {
		if len(p.Fields) != len(ft.Params) {
			c.collector.Error("E130",
				fmt.Sprintf("variant `%s` expects %d field(s), found %d",
					p.Name, len(ft.Params), len(p.Fields)),
				p.Span())
			return expected
		}
		for i, field := range p.Fields {
			c.checkPattern(field, ft.Params[i])
		}
		c.engine.Unify(expected, ft.Return, p.Span())
		return ft.Return
	}

	// Unit variant (no fields).
	if len(p.Fields) > 0 {
		c.collector.Error("E130",
			fmt.Sprintf("variant `%s` takes no fields, found %d", p.Name, len(p.Fields)),
			p.Span())
	}
	c.engine.Unify(expected, instType, p.Span())
	return instType
}

func (c *Checker) checkStructPattern(p *parser.StructPat, expected Type) Type {
	// Look up struct definition.
	sd, ok := c.resolved.StructDefs[p.Name]
	if !ok {
		return expected
	}

	fields := make(map[string]Type, len(sd.Fields))
	fieldOrder := make([]string, len(sd.Fields))
	for i, f := range sd.Fields {
		fields[f.Name] = c.resolveTypeExpr(f.Type)
		fieldOrder[i] = f.Name
	}

	st := &StructType{Name: p.Name, Fields: fields, FieldOrder: fieldOrder}

	for _, f := range p.Fields {
		if ft, ok := fields[f.Name]; ok {
			if f.Pattern != nil {
				c.checkPattern(f.Pattern, ft)
			} else {
				// Shorthand binding.
				c.defineVar(f.Name, MonoScheme(ft))
			}
		} else {
			c.collector.Error("E131",
				fmt.Sprintf("struct `%s` has no field `%s`", p.Name, f.Name),
				f.Span())
		}
	}

	c.engine.Unify(expected, st, p.Span())
	return st
}

// ---------------------------------------------------------------------------
// Loops
// ---------------------------------------------------------------------------

func (c *Checker) checkForExpr(e *parser.ForExpr) Type {
	c.pushEnv()
	defer c.popEnv()

	iterType := c.checkExpr(e.Iter)

	// Infer element type from iter: if iter is [T] or ..range, element is T.
	elemVar := FreshTypeVar()
	c.engine.RegisterTypeVar(elemVar)

	resolved := c.engine.ResolveType(iterType)
	switch rt := resolved.(type) {
	case *ArrayType:
		c.engine.Unify(elemVar, rt.Elem, e.Span())
	case *SliceType:
		c.engine.Unify(elemVar, rt.Elem, e.Span())
	case *ChannelType:
		c.engine.Unify(elemVar, rt.Elem, e.Span())
	default:
		// For range expressions (which produce ArrayType), we rely on unification.
		c.engine.Unify(iterType, &ArrayType{Elem: elemVar}, e.Span())
	}

	c.defineVar(e.Binding, MonoScheme(elemVar))
	c.checkBlock(e.Body)

	return TypUnit
}

func (c *Checker) checkWhileExpr(e *parser.WhileExpr) Type {
	condType := c.checkExpr(e.Cond)
	c.engine.Unify(condType, TypBool, e.Span())
	c.checkBlock(e.Body)
	return TypUnit
}

func (c *Checker) checkLoopExpr(e *parser.LoopExpr) Type {
	c.checkBlock(e.Body)
	return TypUnit
}

// ---------------------------------------------------------------------------
// Lambda expression
// ---------------------------------------------------------------------------

func (c *Checker) checkLambdaExpr(e *parser.LambdaExpr) Type {
	c.pushEnv()
	defer c.popEnv()

	params := make([]Type, len(e.Params))
	for i, p := range e.Params {
		if p.Type != nil {
			params[i] = c.resolveTypeExpr(p.Type)
		} else {
			tv := FreshTypeVar()
			c.engine.RegisterTypeVar(tv)
			params[i] = tv
		}
		c.defineVar(p.Name, MonoScheme(params[i]))
	}

	var declaredReturn Type
	if e.ReturnType != nil {
		declaredReturn = c.resolveTypeExpr(e.ReturnType)
	}

	prevReturn := c.currentReturnType
	c.currentReturnType = declaredReturn

	bodyType := c.checkExpr(e.Body)

	c.currentReturnType = prevReturn

	var returnType Type
	if declaredReturn != nil {
		c.engine.Unify(declaredReturn, bodyType, e.Span())
		returnType = declaredReturn
	} else {
		returnType = bodyType
	}

	return &FnType{Params: params, Return: returnType}
}

// ---------------------------------------------------------------------------
// Spawn expression
// ---------------------------------------------------------------------------

func (c *Checker) checkSpawnExpr(e *parser.SpawnExpr) Type {
	bodyType := c.checkBlock(e.Body)
	// spawn body must return Unit.
	c.engine.Unify(bodyType, TypUnit, e.Span())
	return TypUnit
}

// ---------------------------------------------------------------------------
// Channel expression
// ---------------------------------------------------------------------------

func (c *Checker) checkChannelExpr(e *parser.ChannelExpr) Type {
	var elemType Type
	if e.ElemType != nil {
		elemType = c.resolveTypeExpr(e.ElemType)
	} else {
		tv := FreshTypeVar()
		c.engine.RegisterTypeVar(tv)
		elemType = tv
	}

	if e.Size != nil {
		sizeType := c.checkExpr(e.Size)
		c.engine.Unify(sizeType, TypInt, e.Span())
	}

	return &ChannelType{Elem: elemType}
}

// ---------------------------------------------------------------------------
// Collection literals
// ---------------------------------------------------------------------------

func (c *Checker) checkArrayLit(e *parser.ArrayLit) Type {
	if len(e.Elems) == 0 {
		tv := FreshTypeVar()
		c.engine.RegisterTypeVar(tv)
		return &ArrayType{Elem: tv}
	}

	elemType := c.checkExpr(e.Elems[0])
	for _, elem := range e.Elems[1:] {
		et := c.checkExpr(elem)
		c.engine.Unify(elemType, et, elem.Span())
	}

	return &ArrayType{Elem: elemType}
}

func (c *Checker) checkTupleLit(e *parser.TupleLit) Type {
	elems := make([]Type, len(e.Elems))
	for i, elem := range e.Elems {
		elems[i] = c.checkExpr(elem)
	}
	return &TupleType{Elems: elems}
}

func (c *Checker) checkStructLit(e *parser.StructLit) Type {
	// Look up the struct definition.
	sd, ok := c.resolved.StructDefs[e.Name]
	if !ok {
		c.collector.Error("E140",
			fmt.Sprintf("undefined struct `%s`", e.Name),
			e.Span())
		tv := FreshTypeVar()
		c.engine.RegisterTypeVar(tv)
		return tv
	}

	// If the struct is generic, create fresh type vars for the generic params
	// and use those when resolving field types.
	var genArgs []Type
	genParamMap := make(map[string]Type)
	if sd.GenParams != nil {
		for _, gp := range sd.GenParams.Params {
			tv := FreshTypeVar()
			c.engine.RegisterTypeVar(tv)
			genArgs = append(genArgs, tv)
			genParamMap[gp.Name] = tv
		}
	}

	// Resolve field types, substituting generic params.
	expectedFields := make(map[string]Type, len(sd.Fields))
	fieldOrder := make([]string, len(sd.Fields))
	for i, f := range sd.Fields {
		fieldType := c.resolveTypeExprWithGenericMap(f.Type, genParamMap)
		expectedFields[f.Name] = fieldType
		fieldOrder[i] = f.Name
	}

	fields := make(map[string]Type, len(e.Fields))
	for _, f := range e.Fields {
		valType := c.checkExpr(f.Value)
		fields[f.Name] = valType

		if expectedType, ok := expectedFields[f.Name]; ok {
			c.engine.Unify(expectedType, valType, f.Span())
		} else {
			c.collector.Error("E141",
				fmt.Sprintf("struct `%s` has no field `%s`", e.Name, f.Name),
				f.Span())
		}
	}

	// Check for missing fields.
	for name := range expectedFields {
		if _, ok := fields[name]; !ok {
			c.collector.Error("E142",
				fmt.Sprintf("missing field `%s` in struct `%s`", name, e.Name),
				e.Span())
		}
	}

	return &StructType{
		Name:       e.Name,
		GenArgs:    genArgs,
		Fields:     expectedFields,
		FieldOrder: fieldOrder,
	}
}

// resolveTypeExprWithGenericMap resolves a type expression, using the given
// map for generic parameter names instead of the environment.
func (c *Checker) resolveTypeExprWithGenericMap(te parser.TypeExpr, genMap map[string]Type) Type {
	if te == nil || len(genMap) == 0 {
		return c.resolveTypeExpr(te)
	}
	switch t := te.(type) {
	case *parser.NamedType:
		// Check if it's a generic parameter.
		if mapped, ok := genMap[t.Name]; ok {
			return mapped
		}
		return c.resolveNamedType(t)
	default:
		return c.resolveTypeExpr(te)
	}
}

// ---------------------------------------------------------------------------
// As expression (type cast)
// ---------------------------------------------------------------------------

func (c *Checker) checkAsExpr(e *parser.AsExpr) Type {
	c.checkExpr(e.Expr)
	targetType := c.resolveTypeExpr(e.Type)
	return targetType
}

// ---------------------------------------------------------------------------
// Resolve type expressions to Types
// ---------------------------------------------------------------------------

func (c *Checker) resolveTypeExpr(te parser.TypeExpr) Type {
	if te == nil {
		tv := FreshTypeVar()
		c.engine.RegisterTypeVar(tv)
		return tv
	}

	switch t := te.(type) {
	case *parser.NamedType:
		return c.resolveNamedType(t)
	case *parser.FnType:
		params := make([]Type, len(t.Params))
		for i, p := range t.Params {
			params[i] = c.resolveTypeExpr(p)
		}
		ret := c.resolveTypeExpr(t.Return)
		return &FnType{Params: params, Return: ret}
	case *parser.TupleType:
		elems := make([]Type, len(t.Elems))
		for i, e := range t.Elems {
			elems[i] = c.resolveTypeExpr(e)
		}
		return &TupleType{Elems: elems}
	case *parser.ArrayType:
		return &ArrayType{Elem: c.resolveTypeExpr(t.Elem)}
	case *parser.SliceType:
		return &SliceType{Elem: c.resolveTypeExpr(t.Elem)}
	case *parser.ReferenceType:
		// Treat &T as T for now (no borrow checker).
		return c.resolveTypeExpr(t.Elem)
	case *parser.SelfType:
		scheme := c.lookupVar("Self")
		if scheme != nil {
			return scheme.Body
		}
		tv := FreshTypeVar()
		c.engine.RegisterTypeVar(tv)
		return tv
	case *parser.ChannelType: // [CLAUDE-FIX] Resolve channel<T> type expressions
		return &ChannelType{Elem: c.resolveTypeExpr(t.Elem)}
	default:
		tv := FreshTypeVar()
		c.engine.RegisterTypeVar(tv)
		return tv
	}
}

func (c *Checker) resolveNamedType(nt *parser.NamedType) Type {
	// Check built-in types first.
	if bt := BuiltinType(nt.Name); bt != nil {
		return bt
	}

	// Check if it's a type parameter in scope.
	scheme := c.lookupVar(nt.Name)
	if scheme != nil {
		if tv, ok := scheme.Body.(*TypeVar); ok {
			return tv
		}
		// Could be a struct or enum type.
		if st, ok := scheme.Body.(*StructType); ok {
			if len(nt.GenArgs) > 0 {
				genArgs := make([]Type, len(nt.GenArgs))
				for i, ga := range nt.GenArgs {
					genArgs[i] = c.resolveTypeExpr(ga)
				}
				return &StructType{
					Name:       st.Name,
					GenArgs:    genArgs,
					Fields:     st.Fields,
					FieldOrder: st.FieldOrder,
				}
			}
			return st
		}
		if et, ok := scheme.Body.(*EnumType); ok {
			if len(nt.GenArgs) > 0 {
				genArgs := make([]Type, len(nt.GenArgs))
				for i, ga := range nt.GenArgs {
					genArgs[i] = c.resolveTypeExpr(ga)
				}
				return &EnumType{
					Name:     et.Name,
					GenArgs:  genArgs,
					Variants: et.Variants,
				}
			}
			return et
		}
	}

	// Check struct/enum definitions.
	if sd, ok := c.resolved.StructDefs[nt.Name]; ok {
		fields := make(map[string]Type, len(sd.Fields))
		fieldOrder := make([]string, len(sd.Fields))
		for i, f := range sd.Fields {
			fields[f.Name] = c.resolveTypeExpr(f.Type)
			fieldOrder[i] = f.Name
		}
		var genArgs []Type
		for _, ga := range nt.GenArgs {
			genArgs = append(genArgs, c.resolveTypeExpr(ga))
		}
		return &StructType{Name: nt.Name, GenArgs: genArgs, Fields: fields, FieldOrder: fieldOrder}
	}

	if _, ok := c.resolved.TypeDefs[nt.Name]; ok {
		var genArgs []Type
		for _, ga := range nt.GenArgs {
			genArgs = append(genArgs, c.resolveTypeExpr(ga))
		}
		// Look up enum type we registered.
		if scheme := c.lookupVar(nt.Name); scheme != nil {
			if et, ok := scheme.Body.(*EnumType); ok {
				return &EnumType{Name: et.Name, GenArgs: genArgs, Variants: et.Variants}
			}
		}
		return &EnumType{Name: nt.Name, GenArgs: genArgs}
	}

	// Unknown type — create type variable.
	tv := FreshTypeVar()
	c.engine.RegisterTypeVar(tv)
	return tv
}
