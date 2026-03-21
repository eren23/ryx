package hir

import (
	"fmt"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/lexer"
	"github.com/ryx-lang/ryx/pkg/parser"
	"github.com/ryx-lang/ryx/pkg/resolver"
	"github.com/ryx-lang/ryx/pkg/types"
)

// ---------------------------------------------------------------------------
// LowerResult
// ---------------------------------------------------------------------------

// LowerResult contains the lowered HIR program and any diagnostics.
type LowerResult struct {
	Program     *Program
	Diagnostics []diagnostic.Diagnostic
}

// HasErrors returns true if lowering produced any errors.
func (r *LowerResult) HasErrors() bool {
	for _, d := range r.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Lowerer
// ---------------------------------------------------------------------------

// lowerer holds state for the AST-to-HIR lowering pass.
type lowerer struct {
	checkResult *types.CheckResult
	resolved    *resolver.ResolveResult
	collector   *diagnostic.Collector

	// tmpCounter generates unique names for synthetic temporaries.
	tmpCounter int
}

// Lower transforms a type-checked AST into HIR, performing all desugarings:
//   - for-in-range   → while + counter
//   - for-in-channel → loop + recv
//   - method calls   → static dispatch
//   - pipe operator  → nested calls
//   - string ++      → concat call
//   - closures: captured variables identified
func Lower(
	program *parser.Program,
	checkResult *types.CheckResult,
	resolved *resolver.ResolveResult,
	registry *diagnostic.SourceRegistry,
) *LowerResult {
	collector := diagnostic.NewCollector(registry, 50, 50)
	l := &lowerer{
		checkResult: checkResult,
		resolved:    resolved,
		collector:   collector,
	}

	hirProg := l.lowerProgram(program)

	// [CLAUDE-FIX] Extract spawn bodies into top-level functions so the
	// MIR builder and codegen can reference them by function index.
	extractSpawnBodies(hirProg)

	return &LowerResult{
		Program:     hirProg,
		Diagnostics: collector.Diagnostics(),
	}
}

// freshTmp generates a unique temporary variable name.
func (l *lowerer) freshTmp(prefix string) string {
	l.tmpCounter++
	return fmt.Sprintf("__%s_%d", prefix, l.tmpCounter)
}

// nodeType looks up the resolved type of an AST node by its span.
func (l *lowerer) nodeType(span diagnostic.Span) types.Type {
	if t, ok := l.checkResult.NodeTypes[span]; ok {
		return t
	}
	return types.TypUnit
}

// ---------------------------------------------------------------------------
// Program lowering
// ---------------------------------------------------------------------------

func (l *lowerer) lowerProgram(prog *parser.Program) *Program {
	hirProg := &Program{}

	for _, item := range prog.Items {
		switch it := item.(type) {
		case *parser.FnDef:
			hirProg.Functions = append(hirProg.Functions, l.lowerFnDef(it))
		case *parser.StructDef:
			hirProg.Structs = append(hirProg.Structs, l.lowerStructDef(it))
		case *parser.TypeDef:
			hirProg.Enums = append(hirProg.Enums, l.lowerTypeDef(it))
		case *parser.ImplBlock:
			// Lower impl methods as top-level functions with Type::method naming.
			for _, method := range it.Methods {
				fn := l.lowerMethodDef(it, method)
				hirProg.Functions = append(hirProg.Functions, fn)
			}
		}
	}

	return hirProg
}

// ---------------------------------------------------------------------------
// Top-level definition lowering
// ---------------------------------------------------------------------------

func (l *lowerer) lowerFnDef(fn *parser.FnDef) *Function {
	params := make([]Param, len(fn.Params))
	for i, p := range fn.Params {
		params[i] = Param{
			Name: p.Name,
			Type: l.resolveParamType(p),
		}
	}

	var retType types.Type
	fnType := l.nodeType(fn.Span())
	if ft, ok := fnType.(*types.FnType); ok {
		retType = ft.Return
	} else {
		retType = types.TypUnit
	}

	var body *Block
	if fn.Body != nil {
		body = l.lowerBlock(fn.Body)
	}

	return &Function{
		Name:       fn.Name,
		Params:     params,
		ReturnType: retType,
		Body:       body,
		Span:       fn.Span(),
	}
}

func (l *lowerer) lowerMethodDef(impl *parser.ImplBlock, method *parser.FnDef) *Function {
	typeName := ""
	if nt, ok := impl.TargetType.(*parser.NamedType); ok {
		typeName = nt.Name
	}

	params := make([]Param, len(method.Params))
	for i, p := range method.Params {
		params[i] = Param{
			Name: p.Name,
			Type: l.resolveParamType(p),
		}
	}

	var retType types.Type
	fnType := l.nodeType(method.Span())
	if ft, ok := fnType.(*types.FnType); ok {
		retType = ft.Return
	} else {
		retType = types.TypUnit
	}

	var body *Block
	if method.Body != nil {
		body = l.lowerBlock(method.Body)
	}

	return &Function{
		Name:       typeName + "::" + method.Name,
		Params:     params,
		ReturnType: retType,
		Body:       body,
		Span:       method.Span(),
	}
}

func (l *lowerer) lowerStructDef(sd *parser.StructDef) *StructDef {
	fields := make([]FieldDef, len(sd.Fields))
	for i, f := range sd.Fields {
		fields[i] = FieldDef{
			Name:   f.Name,
			Type:   l.resolveFieldType(f),
			Public: f.Public,
		}
	}
	return &StructDef{
		Name:   sd.Name,
		Fields: fields,
		Span:   sd.Span(),
	}
}

func (l *lowerer) lowerTypeDef(td *parser.TypeDef) *EnumDef {
	variants := make([]VariantDef, len(td.Variants))
	for i, v := range td.Variants {
		fieldTypes := make([]types.Type, len(v.Fields))
		for j, f := range v.Fields {
			fieldTypes[j] = l.resolveTypeExpr(f)
		}
		variants[i] = VariantDef{
			Name:   v.Name,
			Fields: fieldTypes,
		}
	}
	return &EnumDef{
		Name:     td.Name,
		Variants: variants,
		Span:     td.Span(),
	}
}

// ---------------------------------------------------------------------------
// Type resolution helpers
// ---------------------------------------------------------------------------

func (l *lowerer) resolveParamType(p *parser.Param) types.Type {
	if t, ok := l.checkResult.NodeTypes[p.Span()]; ok {
		return t
	}
	if p.Type != nil {
		return l.resolveTypeExpr(p.Type)
	}
	return types.TypUnit
}

func (l *lowerer) resolveFieldType(f *parser.Field) types.Type {
	if t, ok := l.checkResult.NodeTypes[f.Span()]; ok {
		return t
	}
	if f.Type != nil {
		return l.resolveTypeExpr(f.Type)
	}
	return types.TypUnit
}

func (l *lowerer) resolveTypeExpr(te parser.TypeExpr) types.Type {
	if te == nil {
		return types.TypUnit
	}
	if t, ok := l.checkResult.NodeTypes[te.Span()]; ok {
		return t
	}
	// Fallback: try to resolve named types directly.
	if nt, ok := te.(*parser.NamedType); ok {
		if bt := types.BuiltinType(nt.Name); bt != nil {
			return bt
		}
		// [CLAUDE-FIX] Look up user-defined enum types from resolver.
		// Use shallow resolution (nil variant fields) to avoid infinite
		// recursion with self-referential types like Expr(Expr, Expr).
		if _, ok := l.resolved.TypeDefs[nt.Name]; ok {
			return &types.EnumType{Name: nt.Name}
		}
		// [CLAUDE-FIX] Look up user-defined struct types from resolver
		if _, ok := l.resolved.StructDefs[nt.Name]; ok {
			return &types.StructType{Name: nt.Name}
		}
	}
	// [CLAUDE-FIX] Handle channel<T> type expressions in lowerer
	if ct, ok := te.(*parser.ChannelType); ok {
		return &types.ChannelType{Elem: l.resolveTypeExpr(ct.Elem)}
	}
	return types.TypUnit
}

// ---------------------------------------------------------------------------
// Block lowering
// ---------------------------------------------------------------------------

func (l *lowerer) lowerBlock(block *parser.Block) *Block {
	if block == nil {
		return nil
	}
	stmts := make([]Stmt, 0, len(block.Stmts))
	for _, s := range block.Stmts {
		stmts = append(stmts, l.lowerStmt(s))
	}
	var trailing Expr
	if block.TrailingExpr != nil {
		trailing = l.lowerExpr(block.TrailingExpr)
	}

	typ := l.nodeType(block.Span())

	return &Block{
		exprBase:     exprBase{Typ: typ, Span: block.Span()},
		Stmts:        stmts,
		TrailingExpr: trailing,
	}
}

// ---------------------------------------------------------------------------
// Statement lowering
// ---------------------------------------------------------------------------

func (l *lowerer) lowerStmt(stmt parser.Stmt) Stmt {
	switch s := stmt.(type) {
	case *parser.LetStmt:
		var value Expr
		if s.Value != nil {
			value = l.lowerExpr(s.Value)
		}
		var typ types.Type
		if value != nil {
			typ = value.ExprType()
		} else {
			typ = l.resolveTypeExpr(s.Type)
		}
		return &LetStmt{
			Name:    s.Name,
			Type:    typ,
			Value:   value,
			Mutable: s.Mutable,
			Span:    s.Span(),
		}
	case *parser.ExprStmt:
		return &ExprStmt{
			Expr: l.lowerExpr(s.Expr),
			Span: s.Span(),
		}
	case *parser.ReturnStmt:
		var value Expr
		if s.Value != nil {
			value = l.lowerExpr(s.Value)
		}
		return &ReturnStmt{
			Value: value,
			Span:  s.Span(),
		}
	default:
		return &ExprStmt{
			Expr: &UnitLit{exprBase: exprBase{Typ: types.TypUnit, Span: stmt.Span()}},
			Span: stmt.Span(),
		}
	}
}

// ---------------------------------------------------------------------------
// Expression lowering — handles all desugarings
// ---------------------------------------------------------------------------

func (l *lowerer) lowerExpr(expr parser.Expr) Expr {
	if expr == nil {
		return &UnitLit{exprBase: exprBase{Typ: types.TypUnit}}
	}

	switch e := expr.(type) {
	case *parser.IntLit:
		return &IntLit{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Value:    e.Value,
		}
	case *parser.FloatLit:
		return &FloatLit{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Value:    e.Value,
		}
	case *parser.StringLit:
		return &StringLit{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Value:    e.Value,
		}
	case *parser.CharLit:
		return &CharLit{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Value:    e.Value,
		}
	case *parser.BoolLit:
		return &BoolLit{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Value:    e.Value,
		}
	case *parser.Ident:
		// [CLAUDE-FIX] Handle bare enum variant identifiers (unit variants without args)
		if sym, found := l.resolved.Resolutions[e.Span()]; found && sym.Kind == resolver.VariantSymbol {
			return &EnumConstruct{
				exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
				EnumName: sym.ParentType,
				Variant:  e.Name,
				Args:     nil,
			}
		}
		return &VarRef{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Name:     e.Name,
		}
	case *parser.SelfExpr:
		return &VarRef{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Name:     "self",
		}
	case *parser.PathExpr:
		return &PathRef{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Segments: e.Segments,
		}
	case *parser.GroupExpr:
		return l.lowerExpr(e.Inner)

	case *parser.Block:
		return l.lowerBlock(e)

	case *parser.BinaryExpr:
		return l.lowerBinaryExpr(e)

	case *parser.UnaryExpr:
		return l.lowerUnaryExpr(e)

	case *parser.CallExpr:
		return l.lowerCallExpr(e)

	case *parser.FieldExpr:
		return l.lowerFieldExpr(e)

	case *parser.IndexExpr:
		return &Index{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Object:   l.lowerExpr(e.Object),
			Idx:      l.lowerExpr(e.Index),
		}

	case *parser.IfExpr:
		return l.lowerIfExpr(e)

	case *parser.MatchExpr:
		return l.lowerMatchExpr(e)

	case *parser.ForExpr:
		return l.lowerForExpr(e)

	case *parser.WhileExpr:
		return &WhileExpr{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Cond:     l.lowerExpr(e.Cond),
			Body:     l.lowerBlock(e.Body),
		}

	case *parser.LoopExpr:
		return &LoopExpr{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Body:     l.lowerBlock(e.Body),
		}

	case *parser.LambdaExpr:
		return l.lowerLambdaExpr(e)

	case *parser.ArrayLit:
		elems := make([]Expr, len(e.Elems))
		for i, elem := range e.Elems {
			elems[i] = l.lowerExpr(elem)
		}
		return &ArrayLiteral{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Elems:    elems,
		}

	case *parser.TupleLit:
		elems := make([]Expr, len(e.Elems))
		for i, elem := range e.Elems {
			elems[i] = l.lowerExpr(elem)
		}
		return &TupleLiteral{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Elems:    elems,
		}

	case *parser.StructLit:
		fields := make([]FieldInit, len(e.Fields))
		for i, f := range e.Fields {
			fields[i] = FieldInit{
				Name:  f.Name,
				Value: l.lowerExpr(f.Value),
			}
		}
		return &StructLiteral{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Name:     e.Name,
			Fields:   fields,
		}

	case *parser.BreakExpr:
		return &BreakExpr{exprBase: exprBase{Typ: types.TypUnit, Span: e.Span()}}

	case *parser.ContinueExpr:
		return &ContinueExpr{exprBase: exprBase{Typ: types.TypUnit, Span: e.Span()}}

	case *parser.ReturnExpr:
		var value Expr
		if e.Value != nil {
			value = l.lowerExpr(e.Value)
		}
		return &ReturnExpr{
			exprBase: exprBase{Typ: types.TypUnit, Span: e.Span()},
			Value:    value,
		}

	case *parser.AsExpr:
		target := l.resolveTypeExpr(e.Type)
		return &Cast{
			exprBase: exprBase{Typ: target, Span: e.Span()},
			Expr:     l.lowerExpr(e.Expr),
			Target:   target,
		}

	case *parser.SpawnExpr:
		return &Spawn{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			Body:     l.lowerBlock(e.Body),
		}

	case *parser.ChannelExpr:
		elemType := l.resolveTypeExpr(e.ElemType)
		var bufSize Expr
		if e.Size != nil {
			bufSize = l.lowerExpr(e.Size)
		}
		return &ChannelCreate{
			exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
			ElemType: elemType,
			BufSize:  bufSize,
		}

	default:
		return &UnitLit{exprBase: exprBase{Typ: types.TypUnit, Span: expr.Span()}}
	}
}

// ---------------------------------------------------------------------------
// Desugaring: binary expressions
// ---------------------------------------------------------------------------

func (l *lowerer) lowerBinaryExpr(e *parser.BinaryExpr) Expr {
	span := e.Span()
	typ := l.nodeType(span)

	switch e.Op {
	case lexer.ASSIGN:
		// [CLAUDE-FIX] Desugar assignment: x = expr → Assign{Name: "x", Value: expr}
		value := l.lowerExpr(e.Right)
		if ident, ok := e.Left.(*parser.Ident); ok {
			return &Assign{
				exprBase: exprBase{Typ: types.TypUnit, Span: span},
				Name:     ident.Name,
				Value:    value,
			}
		}
		// Field assignment: obj.field = expr
		if field, ok := e.Left.(*parser.FieldExpr); ok {
			return &FieldAssign{
				exprBase: exprBase{Typ: types.TypUnit, Span: span},
				Object:   l.lowerExpr(field.Object),
				Field:    field.Field,
				Value:    value,
			}
		}
		// Index assignment: obj[idx] = expr
		if idx, ok := e.Left.(*parser.IndexExpr); ok {
			return &IndexAssign{
				exprBase: exprBase{Typ: types.TypUnit, Span: span},
				Object:   l.lowerExpr(idx.Object),
				Index:    l.lowerExpr(idx.Index),
				Value:    value,
			}
		}
		return &UnitLit{exprBase: exprBase{Typ: types.TypUnit, Span: span}}

	case lexer.PIPE:
		// Desugar: a |> f |> g → g(f(a))
		return l.desugarPipe(e)

	case lexer.CONCAT:
		// Desugar: a ++ b → String::concat(a, b)
		return &StaticCall{
			exprBase: exprBase{Typ: typ, Span: span},
			TypeName: "String",
			Method:   "concat",
			Args:     []Expr{l.lowerExpr(e.Left), l.lowerExpr(e.Right)},
		}

	default:
		return &BinaryOp{
			exprBase: exprBase{Typ: typ, Span: span},
			Op:       opString(e.Op),
			Left:     l.lowerExpr(e.Left),
			Right:    l.lowerExpr(e.Right),
		}
	}
}

// desugarPipe recursively desugars pipe expressions:
//
//	a |> f → f(a)
//	a |> f |> g → g(f(a))
func (l *lowerer) desugarPipe(e *parser.BinaryExpr) Expr {
	arg := l.lowerExpr(e.Left)
	fn := l.lowerExpr(e.Right)
	typ := l.nodeType(e.Span())

	return &Call{
		exprBase: exprBase{Typ: typ, Span: e.Span()},
		Func:     fn,
		Args:     []Expr{arg},
	}
}

func (l *lowerer) lowerUnaryExpr(e *parser.UnaryExpr) Expr {
	return &UnaryOp{
		exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
		Op:       opString(e.Op),
		Operand:  l.lowerExpr(e.Operand),
	}
}

// ---------------------------------------------------------------------------
// Desugaring: call expressions (method calls → static dispatch)
// ---------------------------------------------------------------------------

func (l *lowerer) lowerCallExpr(e *parser.CallExpr) Expr {
	typ := l.nodeType(e.Span())

	// Check if this is a method call: obj.method(args)
	if field, ok := e.Func.(*parser.FieldExpr); ok {
		return l.desugarMethodCall(field, e.Args, typ, e.Span())
	}

	// [CLAUDE-FIX] Check if call target is an enum variant constructor
	if ident, ok := e.Func.(*parser.Ident); ok {
		if sym, found := l.resolved.Resolutions[ident.Span()]; found && sym.Kind == resolver.VariantSymbol {
			args := make([]Expr, len(e.Args))
			for i, a := range e.Args {
				args[i] = l.lowerExpr(a)
			}
			return &EnumConstruct{
				exprBase: exprBase{Typ: typ, Span: e.Span()},
				EnumName: sym.ParentType,
				Variant:  ident.Name,
				Args:     args,
			}
		}
	}

	// Regular function call.
	args := make([]Expr, len(e.Args))
	for i, a := range e.Args {
		args[i] = l.lowerExpr(a)
	}
	return &Call{
		exprBase: exprBase{Typ: typ, Span: e.Span()},
		Func:     l.lowerExpr(e.Func),
		Args:     args,
	}
}

// desugarMethodCall transforms obj.method(args...) → Type::method(obj, args...).
func (l *lowerer) desugarMethodCall(
	field *parser.FieldExpr,
	astArgs []parser.Expr,
	typ types.Type,
	span diagnostic.Span,
) Expr {
	receiver := l.lowerExpr(field.Object)
	receiverType := receiver.ExprType()
	typeName := typeNameStr(receiverType)

	args := make([]Expr, 0, len(astArgs)+1)
	args = append(args, receiver)
	for _, a := range astArgs {
		args = append(args, l.lowerExpr(a))
	}

	return &StaticCall{
		exprBase: exprBase{Typ: typ, Span: span},
		TypeName: typeName,
		Method:   field.Field,
		Args:     args,
	}
}

func (l *lowerer) lowerFieldExpr(e *parser.FieldExpr) Expr {
	return &FieldAccess{
		exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
		Object:   l.lowerExpr(e.Object),
		Field:    e.Field,
	}
}

// ---------------------------------------------------------------------------
// Desugaring: for expressions
// ---------------------------------------------------------------------------

func (l *lowerer) lowerForExpr(e *parser.ForExpr) Expr {
	span := e.Span()
	typ := l.nodeType(span)

	// Detect for-in-range: for x in a..b or for x in a..=b
	if rangeExpr, ok := e.Iter.(*parser.BinaryExpr); ok &&
		(rangeExpr.Op == lexer.RANGE || rangeExpr.Op == lexer.RANGE_INCLUSIVE) {
		return l.desugarForInRange(e, rangeExpr, typ, span)
	}

	// Detect for-in-channel: for x in ch (where ch is a channel type).
	iterType := l.nodeType(e.Iter.Span())
	if _, ok := iterType.(*types.ChannelType); ok {
		return l.desugarForInChannel(e, typ, span)
	}

	// Fallback: treat as for-in-range with generic iterator.
	return l.desugarForGeneric(e, typ, span)
}

// desugarForInRange transforms:
//
//	for x in a..b { body }
//
// into:
//
//	{
//	    let __counter = a;
//	    let __end = b;
//	    while __counter < __end {
//	        let x = __counter;
//	        body;
//	        __counter = __counter + 1;
//	    }
//	}
func (l *lowerer) desugarForInRange(
	forExpr *parser.ForExpr,
	rangeExpr *parser.BinaryExpr,
	typ types.Type,
	span diagnostic.Span,
) Expr {
	counterName := l.freshTmp("counter")
	endName := l.freshTmp("end")

	startExpr := l.lowerExpr(rangeExpr.Left)
	endExpr := l.lowerExpr(rangeExpr.Right)

	counterRef := &VarRef{
		exprBase: exprBase{Typ: types.TypInt, Span: span},
		Name:     counterName,
	}
	endRef := &VarRef{
		exprBase: exprBase{Typ: types.TypInt, Span: span},
		Name:     endName,
	}

	// Comparison operator: < for exclusive range, <= for inclusive.
	cmpOp := "<"
	if rangeExpr.Op == lexer.RANGE_INCLUSIVE {
		cmpOp = "<="
	}

	cond := &BinaryOp{
		exprBase: exprBase{Typ: types.TypBool, Span: span},
		Op:       cmpOp,
		Left:     counterRef,
		Right:    endRef,
	}

	// let x = __counter;
	bindStmt := &LetStmt{
		Name:    forExpr.Binding,
		Type:    types.TypInt,
		Value:   counterRef,
		Mutable: false,
		Span:    span,
	}

	// Lower the loop body.
	innerBody := l.lowerBlock(forExpr.Body)

	// __counter = __counter + 1
	increment := &ExprStmt{
		Expr: &BinaryOp{
			exprBase: exprBase{Typ: types.TypInt, Span: span},
			Op:       "=",
			Left:     counterRef,
			Right: &BinaryOp{
				exprBase: exprBase{Typ: types.TypInt, Span: span},
				Op:       "+",
				Left:     counterRef,
				Right: &IntLit{
					exprBase: exprBase{Typ: types.TypInt, Span: span},
					Value:    "1",
				},
			},
		},
		Span: span,
	}

	// Build while body: bind + original body stmts + increment.
	whileStmts := []Stmt{bindStmt}
	if innerBody != nil {
		whileStmts = append(whileStmts, innerBody.Stmts...)
		if innerBody.TrailingExpr != nil {
			whileStmts = append(whileStmts, &ExprStmt{Expr: innerBody.TrailingExpr, Span: span})
		}
	}
	whileStmts = append(whileStmts, increment)

	whileBody := &Block{
		exprBase: exprBase{Typ: types.TypUnit, Span: span},
		Stmts:    whileStmts,
	}

	whileExpr := &WhileExpr{
		exprBase: exprBase{Typ: typ, Span: span},
		Cond:     cond,
		Body:     whileBody,
	}

	// Wrap in outer block with counter/end init.
	outerStmts := []Stmt{
		&LetStmt{Name: counterName, Type: types.TypInt, Value: startExpr, Mutable: true, Span: span},
		&LetStmt{Name: endName, Type: types.TypInt, Value: endExpr, Mutable: false, Span: span},
	}

	return &Block{
		exprBase:     exprBase{Typ: typ, Span: span},
		Stmts:        outerStmts,
		TrailingExpr: whileExpr,
	}
}

// desugarForInChannel transforms:
//
//	for x in ch { body }
//
// into:
//
//	loop {
//	    match ch.recv() {
//	        Some(x) => { body },
//	        None => break
//	    }
//	}
func (l *lowerer) desugarForInChannel(
	forExpr *parser.ForExpr,
	typ types.Type,
	span diagnostic.Span,
) Expr {
	chExpr := l.lowerExpr(forExpr.Iter)
	recvTmpName := l.freshTmp("recv")

	// ch.recv() → StaticCall
	chType := chExpr.ExprType()
	var elemType types.Type
	if ct, ok := chType.(*types.ChannelType); ok {
		elemType = ct.Elem
	} else {
		elemType = types.TypUnit
	}

	recvCall := &StaticCall{
		exprBase: exprBase{Typ: elemType, Span: span},
		TypeName: typeNameStr(chType),
		Method:   "recv",
		Args:     []Expr{chExpr},
	}

	// let __recv = ch.recv()
	recvBind := &LetStmt{
		Name:    recvTmpName,
		Type:    elemType,
		Value:   recvCall,
		Mutable: false,
		Span:    span,
	}

	// Check if recv returned Unit (channel closed signal).
	// At runtime, Value.Equal returns false for different tags, so
	// comparing an Int/Float/etc to Unit reliably detects closure.
	closedCheck := &IfExpr{
		exprBase: exprBase{Typ: types.TypUnit, Span: span},
		Cond: &BinaryOp{
			exprBase: exprBase{Typ: types.TypBool, Span: span},
			Op:       "==",
			Left:     &VarRef{exprBase: exprBase{Typ: elemType, Span: span}, Name: recvTmpName},
			Right:    &UnitLit{exprBase: exprBase{Typ: types.TypUnit, Span: span}},
		},
		Then: &Block{
			exprBase:     exprBase{Typ: types.TypUnit, Span: span},
			Stmts:        nil,
			TrailingExpr: &BreakExpr{exprBase: exprBase{Typ: types.TypUnit, Span: span}},
		},
		Else: nil,
	}

	// let x = __recv (bind the received value)
	valueBind := &LetStmt{
		Name:    forExpr.Binding,
		Type:    elemType,
		Value:   &VarRef{exprBase: exprBase{Typ: elemType, Span: span}, Name: recvTmpName},
		Mutable: false,
		Span:    span,
	}

	innerBody := l.lowerBlock(forExpr.Body)
	bodyStmts := []Stmt{recvBind}
	bodyStmts = append(bodyStmts, &ExprStmt{Expr: closedCheck, Span: span})
	bodyStmts = append(bodyStmts, valueBind)
	if innerBody != nil {
		bodyStmts = append(bodyStmts, innerBody.Stmts...)
		if innerBody.TrailingExpr != nil {
			bodyStmts = append(bodyStmts, &ExprStmt{Expr: innerBody.TrailingExpr, Span: span})
		}
	}

	loopBody := &Block{
		exprBase: exprBase{Typ: types.TypUnit, Span: span},
		Stmts:    bodyStmts,
	}

	return &LoopExpr{
		exprBase: exprBase{Typ: typ, Span: span},
		Body:     loopBody,
	}
}

// desugarForGeneric handles for-in over generic iterables (fallback).
func (l *lowerer) desugarForGeneric(
	forExpr *parser.ForExpr,
	typ types.Type,
	span diagnostic.Span,
) Expr {
	// Fallback: lower as a while loop with an iterator variable.
	iterExpr := l.lowerExpr(forExpr.Iter)
	body := l.lowerBlock(forExpr.Body)

	bindStmt := &LetStmt{
		Name:    forExpr.Binding,
		Type:    l.nodeType(forExpr.Iter.Span()),
		Value:   iterExpr,
		Mutable: false,
		Span:    span,
	}

	bodyStmts := []Stmt{bindStmt}
	if body != nil {
		bodyStmts = append(bodyStmts, body.Stmts...)
	}

	whileBody := &Block{
		exprBase:     exprBase{Typ: types.TypUnit, Span: span},
		Stmts:        bodyStmts,
		TrailingExpr: body.TrailingExpr,
	}

	return &WhileExpr{
		exprBase: exprBase{Typ: typ, Span: span},
		Cond:     &BoolLit{exprBase: exprBase{Typ: types.TypBool, Span: span}, Value: true},
		Body:     whileBody,
	}
}

// ---------------------------------------------------------------------------
// If expression lowering
// ---------------------------------------------------------------------------

func (l *lowerer) lowerIfExpr(e *parser.IfExpr) Expr {
	var elseExpr Expr
	if e.Else != nil {
		elseExpr = l.lowerExpr(e.Else)
	}
	return &IfExpr{
		exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
		Cond:     l.lowerExpr(e.Cond),
		Then:     l.lowerBlock(e.Then),
		Else:     elseExpr,
	}
}

// ---------------------------------------------------------------------------
// Match expression lowering (pre-compilation: preserves arms)
// ---------------------------------------------------------------------------

func (l *lowerer) lowerMatchExpr(e *parser.MatchExpr) Expr {
	arms := make([]*MatchArm, len(e.Arms))
	for i, arm := range e.Arms {
		var guard Expr
		if arm.Guard != nil {
			guard = l.lowerExpr(arm.Guard)
		}
		arms[i] = &MatchArm{
			Pattern: l.lowerPattern(arm.Pattern),
			Guard:   guard,
			Body:    l.lowerExpr(arm.Body),
		}
	}

	return &MatchExpr{
		exprBase:  exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
		Scrutinee: l.lowerExpr(e.Scrutinee),
		Arms:      arms,
	}
}

// ---------------------------------------------------------------------------
// Lambda lowering with closure capture analysis
// ---------------------------------------------------------------------------

func (l *lowerer) lowerLambdaExpr(e *parser.LambdaExpr) Expr {
	params := make([]Param, len(e.Params))
	paramNames := make(map[string]bool, len(e.Params))
	for i, p := range e.Params {
		params[i] = Param{
			Name: p.Name,
			Type: l.resolveParamType(p),
		}
		paramNames[p.Name] = true
	}

	body := l.lowerExpr(e.Body)

	// Identify captured variables: free variables in the body that are not
	// parameters of this lambda or global/builtin names.
	globals := l.globalNames()
	captures := identifyCaptures(body, paramNames, globals)

	return &Lambda{
		exprBase: exprBase{Typ: l.nodeType(e.Span()), Span: e.Span()},
		Params:   params,
		Body:     body,
		Captures: captures,
	}
}

// globalNames returns a set of names that should not be captured by closures
// (top-level functions and builtins resolved in the prelude/root scope).
func (l *lowerer) globalNames() map[string]bool {
	globals := make(map[string]bool)
	if l.resolved != nil && l.resolved.RootScope != nil {
		scope := l.resolved.RootScope
		// Walk up to the prelude scope (builtins).
		if scope.Parent != nil {
			for name, sym := range scope.Parent.Symbols {
				if sym.Kind == resolver.FunctionSymbol {
					globals[name] = true
				}
			}
		}
		// Root-scope function definitions (user-defined top-level functions).
		for name, sym := range scope.Symbols {
			if sym.Kind == resolver.FunctionSymbol {
				globals[name] = true
			}
		}
	}
	return globals
}

// identifyCaptures walks an HIR expression and finds all VarRef names that are
// not in the provided local scope (parameters) and not global/builtin names.
func identifyCaptures(expr Expr, locals map[string]bool, globals map[string]bool) []Capture {
	seen := make(map[string]bool)
	var captures []Capture
	walkExprForCaptures(expr, locals, globals, seen, &captures)
	return captures
}

func walkExprForCaptures(expr Expr, locals map[string]bool, globals map[string]bool, seen map[string]bool, captures *[]Capture) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *VarRef:
		if !locals[e.Name] && !globals[e.Name] && !seen[e.Name] && e.Name != "self" {
			seen[e.Name] = true
			*captures = append(*captures, Capture{Name: e.Name, Type: e.Typ})
		}
	case *Block:
		// Track let bindings as local to this block.
		innerLocals := copyStringSet(locals)
		for _, s := range e.Stmts {
			walkStmtForCaptures(s, innerLocals, globals, seen, captures)
			if ls, ok := s.(*LetStmt); ok {
				innerLocals[ls.Name] = true
			}
		}
		if e.TrailingExpr != nil {
			walkExprForCaptures(e.TrailingExpr, innerLocals, globals, seen, captures)
		}
	case *BinaryOp:
		walkExprForCaptures(e.Left, locals, globals, seen, captures)
		walkExprForCaptures(e.Right, locals, globals, seen, captures)
	case *UnaryOp:
		walkExprForCaptures(e.Operand, locals, globals, seen, captures)
	case *Call:
		walkExprForCaptures(e.Func, locals, globals, seen, captures)
		for _, arg := range e.Args {
			walkExprForCaptures(arg, locals, globals, seen, captures)
		}
	case *StaticCall:
		for _, arg := range e.Args {
			walkExprForCaptures(arg, locals, globals, seen, captures)
		}
	case *IfExpr:
		walkExprForCaptures(e.Cond, locals, globals, seen, captures)
		walkExprForCaptures(e.Then, locals, globals, seen, captures)
		if e.Else != nil {
			walkExprForCaptures(e.Else, locals, globals, seen, captures)
		}
	case *WhileExpr:
		walkExprForCaptures(e.Cond, locals, globals, seen, captures)
		walkExprForCaptures(e.Body, locals, globals, seen, captures)
	case *LoopExpr:
		walkExprForCaptures(e.Body, locals, globals, seen, captures)
	case *FieldAccess:
		walkExprForCaptures(e.Object, locals, globals, seen, captures)
	case *Index:
		walkExprForCaptures(e.Object, locals, globals, seen, captures)
		walkExprForCaptures(e.Idx, locals, globals, seen, captures)
	case *ArrayLiteral:
		for _, elem := range e.Elems {
			walkExprForCaptures(elem, locals, globals, seen, captures)
		}
	case *TupleLiteral:
		for _, elem := range e.Elems {
			walkExprForCaptures(elem, locals, globals, seen, captures)
		}
	case *StructLiteral:
		for _, f := range e.Fields {
			walkExprForCaptures(f.Value, locals, globals, seen, captures)
		}
	case *MatchExpr:
		walkExprForCaptures(e.Scrutinee, locals, globals, seen, captures)
		for _, arm := range e.Arms {
			if arm.Guard != nil {
				walkExprForCaptures(arm.Guard, locals, globals, seen, captures)
			}
			walkExprForCaptures(arm.Body, locals, globals, seen, captures)
		}
	case *Lambda:
		// Nested lambda: its own params are local.
		innerLocals := copyStringSet(locals)
		for _, p := range e.Params {
			innerLocals[p.Name] = true
		}
		walkExprForCaptures(e.Body, innerLocals, globals, seen, captures)
	case *ReturnExpr:
		if e.Value != nil {
			walkExprForCaptures(e.Value, locals, globals, seen, captures)
		}
	case *Cast:
		walkExprForCaptures(e.Expr, locals, globals, seen, captures)
	case *Spawn:
		walkExprForCaptures(e.Body, locals, globals, seen, captures)
	}
}

func walkStmtForCaptures(stmt Stmt, locals map[string]bool, globals map[string]bool, seen map[string]bool, captures *[]Capture) {
	switch s := stmt.(type) {
	case *LetStmt:
		if s.Value != nil {
			walkExprForCaptures(s.Value, locals, globals, seen, captures)
		}
	case *ExprStmt:
		walkExprForCaptures(s.Expr, locals, globals, seen, captures)
	case *ReturnStmt:
		if s.Value != nil {
			walkExprForCaptures(s.Value, locals, globals, seen, captures)
		}
	}
}

func copyStringSet(s map[string]bool) map[string]bool {
	c := make(map[string]bool, len(s))
	for k, v := range s {
		c[k] = v
	}
	return c
}

// ---------------------------------------------------------------------------
// Pattern lowering
// ---------------------------------------------------------------------------

func (l *lowerer) lowerPattern(pat parser.Pattern) Pattern {
	if pat == nil {
		return &WildcardPat{}
	}
	switch p := pat.(type) {
	case *parser.WildcardPat:
		return &WildcardPat{Span: p.Span()}
	case *parser.BindingPat:
		return &BindingPat{Name: p.Name, Span: p.Span()}
	case *parser.LiteralPat:
		return &LiteralPat{Value: l.lowerExpr(p.Value), Span: p.Span()}
	case *parser.TuplePat:
		elems := make([]Pattern, len(p.Elems))
		for i, e := range p.Elems {
			elems[i] = l.lowerPattern(e)
		}
		return &TuplePat{Elems: elems, Span: p.Span()}
	case *parser.VariantPat:
		fields := make([]Pattern, len(p.Fields))
		for i, f := range p.Fields {
			fields[i] = l.lowerPattern(f)
		}
		return &VariantPat{Name: p.Name, Fields: fields, Span: p.Span()}
	case *parser.StructPat:
		fields := make([]StructPatField, len(p.Fields))
		for i, f := range p.Fields {
			var pat Pattern
			if f.Pattern != nil {
				pat = l.lowerPattern(f.Pattern)
			} else {
				pat = &BindingPat{Name: f.Name, Span: f.Span()}
			}
			fields[i] = StructPatField{Name: f.Name, Pattern: pat}
		}
		return &StructPat{Name: p.Name, Fields: fields, Span: p.Span()}
	case *parser.OrPat:
		alts := make([]Pattern, len(p.Alts))
		for i, a := range p.Alts {
			alts[i] = l.lowerPattern(a)
		}
		return &OrPat{Alts: alts, Span: p.Span()}
	case *parser.RangePat:
		return &RangePat{
			Start:     l.lowerExpr(p.Start),
			End:       l.lowerExpr(p.End),
			Inclusive: p.Inclusive,
			Span:      p.Span(),
		}
	default:
		return &WildcardPat{Span: pat.Span()}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// typeNameStr extracts a type's name for static call dispatch.
func typeNameStr(t types.Type) string {
	if t == nil {
		return "<unknown>"
	}
	switch ty := t.(type) {
	case *types.IntType:
		return "Int"
	case *types.FloatType:
		return "Float"
	case *types.BoolType:
		return "Bool"
	case *types.CharType:
		return "Char"
	case *types.StringType:
		return "String"
	case *types.StructType:
		return ty.Name
	case *types.EnumType:
		return ty.Name
	case *types.ChannelType:
		return "channel"
	default:
		return t.String()
	}
}

// opString converts a lexer token type to an operator string.
func opString(op lexer.TokenType) string {
	switch op {
	case lexer.PLUS:
		return "+"
	case lexer.MINUS:
		return "-"
	case lexer.STAR:
		return "*"
	case lexer.SLASH:
		return "/"
	case lexer.PERCENT:
		return "%"
	case lexer.EQ:
		return "=="
	case lexer.NEQ:
		return "!="
	case lexer.LT:
		return "<"
	case lexer.GT:
		return ">"
	case lexer.LEQ:
		return "<="
	case lexer.GEQ:
		return ">="
	case lexer.AND:
		return "&&"
	case lexer.OR:
		return "||"
	case lexer.NOT:
		return "!"
	case lexer.ASSIGN:
		return "="
	case lexer.RANGE:
		return ".."
	case lexer.RANGE_INCLUSIVE:
		return "..="
	default:
		return op.String()
	}
}

// ---------------------------------------------------------------------------
// [CLAUDE-FIX] Spawn body extraction — lift spawn blocks into top-level fns
// ---------------------------------------------------------------------------

// extractSpawnBodies walks all functions and replaces inline spawn bodies
// with calls to synthetic top-level functions. Captured variables become
// parameters of the new function.
func extractSpawnBodies(prog *Program) {
	var newFns []*Function
	counter := 0
	for _, fn := range prog.Functions {
		if fn.Body == nil {
			continue
		}
		extractSpawnsInBlock(fn.Body, fn.Name, &counter, &newFns, fn)
	}
	prog.Functions = append(prog.Functions, newFns...)
}

func extractSpawnsInBlock(block *Block, fnName string, counter *int, newFns *[]*Function, enclosingFn *Function) {
	for _, stmt := range block.Stmts {
		extractSpawnsInStmt(stmt, fnName, counter, newFns, enclosingFn)
	}
	if block.TrailingExpr != nil {
		block.TrailingExpr = extractSpawnsInExpr(block.TrailingExpr, fnName, counter, newFns, enclosingFn)
	}
}

func extractSpawnsInStmt(stmt Stmt, fnName string, counter *int, newFns *[]*Function, enclosingFn *Function) {
	switch s := stmt.(type) {
	case *LetStmt:
		if s.Value != nil {
			s.Value = extractSpawnsInExpr(s.Value, fnName, counter, newFns, enclosingFn)
		}
	case *ExprStmt:
		s.Expr = extractSpawnsInExpr(s.Expr, fnName, counter, newFns, enclosingFn)
	case *ReturnStmt:
		if s.Value != nil {
			s.Value = extractSpawnsInExpr(s.Value, fnName, counter, newFns, enclosingFn)
		}
	}
}

func extractSpawnsInExpr(expr Expr, fnName string, counter *int, newFns *[]*Function, enclosingFn *Function) Expr {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *Spawn:
		// Extract the spawn body into a new function.
		spawnName := fmt.Sprintf("%s$spawn%d", fnName, *counter)
		*counter++

		// Collect free variables used in the body.
		captures := collectFreeVars(e.Body, enclosingFn)

		// Create parameters from captures.
		params := make([]Param, len(captures))
		for i, cap := range captures {
			params[i] = Param{Name: cap.Name, Type: cap.Type}
		}

		// Create the new function.
		spawnFn := &Function{
			Name:       spawnName,
			Params:     params,
			ReturnType: types.TypUnit,
			Body:       e.Body,
			Span:       e.Span,
		}
		*newFns = append(*newFns, spawnFn)

		// Build args from captures (VarRef to the enclosing scope's variables).
		args := make([]Expr, len(captures))
		for i, cap := range captures {
			args[i] = &VarRef{
				exprBase: exprBase{Typ: cap.Type, Span: e.Span},
				Name:     cap.Name,
			}
		}

		// Replace Spawn.Body with a block that calls the extracted function.
		callExpr := &Call{
			exprBase: exprBase{Typ: types.TypUnit, Span: e.Span},
			Func: &VarRef{
				exprBase: exprBase{Typ: types.TypUnit, Span: e.Span},
				Name:     spawnName,
			},
			Args: args,
		}
		return &Spawn{
			exprBase: e.exprBase,
			Body: &Block{
				exprBase:     exprBase{Typ: types.TypUnit, Span: e.Span},
				Stmts:        []Stmt{&ExprStmt{Expr: callExpr, Span: e.Span}},
				TrailingExpr: nil,
			},
		}

	case *Block:
		extractSpawnsInBlock(e, fnName, counter, newFns, enclosingFn)
	case *IfExpr:
		e.Cond = extractSpawnsInExpr(e.Cond, fnName, counter, newFns, enclosingFn)
		extractSpawnsInBlock(e.Then, fnName, counter, newFns, enclosingFn)
		if e.Else != nil {
			e.Else = extractSpawnsInExpr(e.Else, fnName, counter, newFns, enclosingFn)
		}
	case *WhileExpr:
		e.Cond = extractSpawnsInExpr(e.Cond, fnName, counter, newFns, enclosingFn)
		extractSpawnsInBlock(e.Body, fnName, counter, newFns, enclosingFn)
	case *LoopExpr:
		extractSpawnsInBlock(e.Body, fnName, counter, newFns, enclosingFn)
	case *Call:
		e.Func = extractSpawnsInExpr(e.Func, fnName, counter, newFns, enclosingFn)
		for i, a := range e.Args {
			e.Args[i] = extractSpawnsInExpr(a, fnName, counter, newFns, enclosingFn)
		}
	case *StaticCall:
		for i, a := range e.Args {
			e.Args[i] = extractSpawnsInExpr(a, fnName, counter, newFns, enclosingFn)
		}
	}
	return expr
}

// capturedVar is a variable captured by a spawn body.
type capturedVar struct {
	Name string
	Type types.Type
}

// collectFreeVars finds variables referenced in an expression that are
// local to the enclosing function (not top-level functions or builtins).
func collectFreeVars(expr Expr, enclosingFn *Function) []capturedVar {
	// Collect all VarRef names used in the body.
	refs := make(map[string]types.Type)
	collectVarRefs(expr, refs)

	// Collect names defined inside the spawn body (let bindings).
	defined := make(map[string]bool)
	collectDefinedVars(expr, defined)

	// Build set of local variable names from the enclosing function
	// (params + let bindings in the body).
	locals := make(map[string]types.Type)
	for _, p := range enclosingFn.Params {
		locals[p.Name] = p.Type
	}
	if enclosingFn.Body != nil {
		collectLetBindings(enclosingFn.Body, locals)
	}

	var captures []capturedVar
	seen := make(map[string]bool)
	for name, typ := range refs {
		if defined[name] || seen[name] {
			continue
		}
		// Only capture if it's a known local in the enclosing function.
		if _, isLocal := locals[name]; isLocal {
			seen[name] = true
			captures = append(captures, capturedVar{Name: name, Type: typ})
		}
	}
	return captures
}

func collectLetBindings(block *Block, locals map[string]types.Type) {
	for _, s := range block.Stmts {
		if ls, ok := s.(*LetStmt); ok {
			locals[ls.Name] = ls.Type
		}
	}
}

func collectDefinedVars(expr Expr, defined map[string]bool) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *Block:
		for _, s := range e.Stmts {
			if ls, ok := s.(*LetStmt); ok {
				defined[ls.Name] = true
			}
		}
	}
}

func collectVarRefs(expr Expr, refs map[string]types.Type) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *VarRef:
		refs[e.Name] = e.ExprType()
	case *Block:
		for _, s := range e.Stmts {
			switch st := s.(type) {
			case *LetStmt:
				collectVarRefs(st.Value, refs)
			case *ExprStmt:
				collectVarRefs(st.Expr, refs)
			case *ReturnStmt:
				collectVarRefs(st.Value, refs)
			}
		}
		collectVarRefs(e.TrailingExpr, refs)
	case *Call:
		collectVarRefs(e.Func, refs)
		for _, a := range e.Args {
			collectVarRefs(a, refs)
		}
	case *StaticCall:
		for _, a := range e.Args {
			collectVarRefs(a, refs)
		}
	case *IfExpr:
		collectVarRefs(e.Cond, refs)
		collectVarRefs(e.Then, refs)
		collectVarRefs(e.Else, refs)
	case *WhileExpr:
		collectVarRefs(e.Cond, refs)
		collectVarRefs(e.Body, refs)
	case *LoopExpr:
		collectVarRefs(e.Body, refs)
	case *BinaryOp:
		collectVarRefs(e.Left, refs)
		collectVarRefs(e.Right, refs)
	case *UnaryOp:
		collectVarRefs(e.Operand, refs)
	case *FieldAccess:
		collectVarRefs(e.Object, refs)
	case *Index:
		collectVarRefs(e.Object, refs)
		collectVarRefs(e.Idx, refs)
	}
}
