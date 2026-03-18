package hir

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/types"
)

// ---------------------------------------------------------------------------
// Monomorphization
// ---------------------------------------------------------------------------

// MonomorphizeResult holds the output of monomorphization.
type MonomorphizeResult struct {
	Program     *Program
	Diagnostics []diagnostic.Diagnostic
}

// Monomorphize creates one concrete copy of each generic function for every
// distinct type combination it is called with. The limit parameter caps the
// number of instantiations per generic function (64 by default). If the limit
// is exceeded, an error diagnostic is emitted and no further copies are
// generated for that function.
func Monomorphize(prog *Program, limit int) *MonomorphizeResult {
	if limit <= 0 {
		limit = 64
	}

	m := &monomorphizer{
		prog:      prog,
		limit:     limit,
		funcIndex: make(map[string]*Function),
		instances: make(map[string]map[string]*Function),
	}

	// Index functions by name.
	for _, fn := range prog.Functions {
		m.funcIndex[fn.Name] = fn
	}

	// Scan all function bodies for calls to generic functions and collect
	// the concrete type arguments used at each call site.
	for _, fn := range prog.Functions {
		if fn.Body != nil {
			m.scanExpr(fn.Body)
		}
	}

	// Generate monomorphized copies.
	m.generateInstances()

	// Rewrite call sites to use monomorphized names.
	for _, fn := range prog.Functions {
		if fn.Body != nil {
			m.rewriteExpr(fn.Body)
		}
	}
	for _, fn := range m.newFunctions {
		if fn.Body != nil {
			m.rewriteExpr(fn.Body)
		}
	}

	prog.Functions = append(prog.Functions, m.newFunctions...)

	// Monomorphize generic struct and enum types.
	m.generateStructInstances()
	m.generateEnumInstances()
	prog.Structs = append(prog.Structs, m.newStructs...)
	prog.Enums = append(prog.Enums, m.newEnums...)

	return &MonomorphizeResult{
		Program:     prog,
		Diagnostics: m.diagnostics,
	}
}

// ---------------------------------------------------------------------------
// Monomorphizer state
// ---------------------------------------------------------------------------

type monomorphizer struct {
	prog      *Program
	limit     int
	funcIndex map[string]*Function
	// instances maps generic function name → mangled name → concrete function.
	instances    map[string]map[string]*Function
	newFunctions []*Function
	diagnostics  []diagnostic.Diagnostic

	// callSites records all (function name, type arguments) pairs found.
	callSites []monoCallSite

	// Type monomorphization: generic struct/enum instantiation.
	structSites []monoTypeSite
	enumSites   []monoTypeSite
	newStructs  []*StructDef
	newEnums    []*EnumDef
}

type monoCallSite struct {
	FuncName string
	TypeArgs []types.Type
}

// monoTypeSite records a usage of a generic struct or enum with concrete type args.
type monoTypeSite struct {
	Name     string
	TypeArgs []types.Type
	FullType types.Type // *types.StructType or *types.EnumType
}

// mangledName produces a unique name for a monomorphized function.
// Example: "identity$Int" or "pair$Int_String".
func mangledName(baseName string, typeArgs []types.Type) string {
	if len(typeArgs) == 0 {
		return baseName
	}
	parts := make([]string, len(typeArgs))
	for i, t := range typeArgs {
		parts[i] = typeKey(t)
	}
	return baseName + "$" + strings.Join(parts, "_")
}

// typeKey produces a stable string key for a concrete type.
func typeKey(t types.Type) string {
	if t == nil {
		return "Unit"
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
	case *types.UnitType:
		return "Unit"
	case *types.ArrayType:
		return "Array_" + typeKey(ty.Elem)
	case *types.SliceType:
		return "Slice_" + typeKey(ty.Elem)
	case *types.TupleType:
		parts := make([]string, len(ty.Elems))
		for i, e := range ty.Elems {
			parts[i] = typeKey(e)
		}
		return "Tuple_" + strings.Join(parts, "_")
	case *types.FnType:
		parts := make([]string, len(ty.Params))
		for i, p := range ty.Params {
			parts[i] = typeKey(p)
		}
		return "Fn_" + strings.Join(parts, "_") + "_" + typeKey(ty.Return)
	case *types.StructType:
		if len(ty.GenArgs) == 0 {
			return ty.Name
		}
		parts := make([]string, len(ty.GenArgs))
		for i, a := range ty.GenArgs {
			parts[i] = typeKey(a)
		}
		return ty.Name + "_" + strings.Join(parts, "_")
	case *types.EnumType:
		if len(ty.GenArgs) == 0 {
			return ty.Name
		}
		parts := make([]string, len(ty.GenArgs))
		for i, a := range ty.GenArgs {
			parts[i] = typeKey(a)
		}
		return ty.Name + "_" + strings.Join(parts, "_")
	case *types.ChannelType:
		return "Channel_" + typeKey(ty.Elem)
	case *types.TypeVar:
		return fmt.Sprintf("T%d", ty.ID)
	default:
		return t.String()
	}
}

// ---------------------------------------------------------------------------
// Scanning: collect call sites with type arguments
// ---------------------------------------------------------------------------

// recordGenericType checks if a type is a generic struct or enum with concrete
// type arguments and records it for monomorphization.
func (m *monomorphizer) recordGenericType(t types.Type) {
	if t == nil {
		return
	}
	switch ty := t.(type) {
	case *types.StructType:
		if len(ty.GenArgs) > 0 && hasConcreteTypes(ty.GenArgs) {
			m.structSites = append(m.structSites, monoTypeSite{
				Name: ty.Name, TypeArgs: ty.GenArgs, FullType: ty,
			})
		}
	case *types.EnumType:
		if len(ty.GenArgs) > 0 && hasConcreteTypes(ty.GenArgs) {
			m.enumSites = append(m.enumSites, monoTypeSite{
				Name: ty.Name, TypeArgs: ty.GenArgs, FullType: ty,
			})
		}
	}
}

func (m *monomorphizer) scanExpr(expr Expr) {
	if expr == nil {
		return
	}
	// Record generic struct/enum types used in expressions.
	m.recordGenericType(expr.ExprType())

	switch e := expr.(type) {
	case *Call:
		// Check if calling a generic function.
		if vr, ok := e.Func.(*VarRef); ok {
			if fn, ok := m.funcIndex[vr.Name]; ok {
				typeArgs := m.inferTypeArgs(fn, e.Args)
				if len(typeArgs) > 0 && hasConcreteTypes(typeArgs) {
					m.callSites = append(m.callSites, monoCallSite{
						FuncName: vr.Name,
						TypeArgs: typeArgs,
					})
				}
			}
		}
		m.scanExpr(e.Func)
		for _, arg := range e.Args {
			m.scanExpr(arg)
		}
	case *StaticCall:
		for _, arg := range e.Args {
			m.scanExpr(arg)
		}
	case *Block:
		for _, s := range e.Stmts {
			m.scanStmt(s)
		}
		if e.TrailingExpr != nil {
			m.scanExpr(e.TrailingExpr)
		}
	case *BinaryOp:
		m.scanExpr(e.Left)
		m.scanExpr(e.Right)
	case *UnaryOp:
		m.scanExpr(e.Operand)
	case *IfExpr:
		m.scanExpr(e.Cond)
		m.scanExpr(e.Then)
		if e.Else != nil {
			m.scanExpr(e.Else)
		}
	case *WhileExpr:
		m.scanExpr(e.Cond)
		m.scanExpr(e.Body)
	case *LoopExpr:
		m.scanExpr(e.Body)
	case *FieldAccess:
		m.scanExpr(e.Object)
	case *Index:
		m.scanExpr(e.Object)
		m.scanExpr(e.Idx)
	case *ArrayLiteral:
		for _, elem := range e.Elems {
			m.scanExpr(elem)
		}
	case *TupleLiteral:
		for _, elem := range e.Elems {
			m.scanExpr(elem)
		}
	case *StructLiteral:
		for _, f := range e.Fields {
			m.scanExpr(f.Value)
		}
	case *EnumConstruct: // [CLAUDE-FIX]
		for _, arg := range e.Args {
			m.scanExpr(arg)
		}
	case *Assign: // [CLAUDE-FIX]
		m.scanExpr(e.Value)
	case *FieldAssign: // [CLAUDE-FIX]
		m.scanExpr(e.Object)
		m.scanExpr(e.Value)
	case *IndexAssign: // [CLAUDE-FIX]
		m.scanExpr(e.Object)
		m.scanExpr(e.Index)
		m.scanExpr(e.Value)
	case *MatchExpr:
		m.scanExpr(e.Scrutinee)
		for _, arm := range e.Arms {
			if arm.Guard != nil {
				m.scanExpr(arm.Guard)
			}
			m.scanExpr(arm.Body)
		}
	case *Lambda:
		m.scanExpr(e.Body)
	case *ReturnExpr:
		if e.Value != nil {
			m.scanExpr(e.Value)
		}
	case *Cast:
		m.scanExpr(e.Expr)
	case *Spawn:
		m.scanExpr(e.Body)
	}
}

func (m *monomorphizer) scanStmt(stmt Stmt) {
	switch s := stmt.(type) {
	case *LetStmt:
		m.recordGenericType(s.Type)
		if s.Value != nil {
			m.scanExpr(s.Value)
		}
	case *ExprStmt:
		m.scanExpr(s.Expr)
	case *ReturnStmt:
		if s.Value != nil {
			m.scanExpr(s.Value)
		}
	}
}

// inferTypeArgs infers type arguments for a generic function call by matching
// actual argument types against parameter types.
func (m *monomorphizer) inferTypeArgs(fn *Function, args []Expr) []types.Type {
	if len(fn.Params) == 0 || len(args) == 0 {
		return nil
	}

	// Collect all unique types used in the call that differ from params.
	var typeArgs []types.Type
	seen := make(map[string]bool)
	for i, arg := range args {
		if i >= len(fn.Params) {
			break
		}
		argType := arg.ExprType()
		paramType := fn.Params[i].Type
		if argType != nil && !typesEqual(argType, paramType) {
			key := typeKey(argType)
			if !seen[key] {
				seen[key] = true
				typeArgs = append(typeArgs, argType)
			}
		}
	}
	return typeArgs
}

// ---------------------------------------------------------------------------
// Instance generation
// ---------------------------------------------------------------------------

func (m *monomorphizer) generateInstances() {
	// Deduplicate call sites.
	type siteKey struct {
		name string
		args string
	}
	dedupd := make(map[siteKey]monoCallSite)
	for _, cs := range m.callSites {
		key := siteKey{name: cs.FuncName, args: mangledName("", cs.TypeArgs)}
		dedupd[key] = cs
	}

	// Sort for deterministic output.
	var sortedKeys []siteKey
	for k := range dedupd {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Slice(sortedKeys, func(i, j int) bool {
		if sortedKeys[i].name != sortedKeys[j].name {
			return sortedKeys[i].name < sortedKeys[j].name
		}
		return sortedKeys[i].args < sortedKeys[j].args
	})

	for _, key := range sortedKeys {
		cs := dedupd[key]
		fn, ok := m.funcIndex[cs.FuncName]
		if !ok {
			continue
		}

		mangled := mangledName(cs.FuncName, cs.TypeArgs)

		// Initialize instance map for this function.
		if m.instances[cs.FuncName] == nil {
			m.instances[cs.FuncName] = make(map[string]*Function)
		}

		// Check if already generated.
		if _, exists := m.instances[cs.FuncName][mangled]; exists {
			continue
		}

		// Check limit.
		if len(m.instances[cs.FuncName]) >= m.limit {
			m.diagnostics = append(m.diagnostics, diagnostic.Diagnostic{
				Severity: diagnostic.SeverityError,
				Code:     "E200",
				Message: fmt.Sprintf(
					"monomorphization limit exceeded: function `%s` has more than %d instantiations",
					cs.FuncName, m.limit),
				Span: fn.Span,
			})
			continue
		}

		// Create concrete copy.
		concrete := m.instantiateFunction(fn, cs.TypeArgs, mangled)
		m.instances[cs.FuncName][mangled] = concrete
		m.newFunctions = append(m.newFunctions, concrete)
	}
}

// instantiateFunction creates a concrete copy of a generic function with
// type parameters substituted.
func (m *monomorphizer) instantiateFunction(
	fn *Function,
	typeArgs []types.Type,
	name string,
) *Function {
	// Build a type substitution map: param type → concrete type.
	subst := make(map[string]types.Type)
	for i, param := range fn.Params {
		if i < len(typeArgs) {
			if tv, ok := param.Type.(*types.TypeVar); ok {
				subst[fmt.Sprintf("?T%d", tv.ID)] = typeArgs[i]
			}
		}
	}

	params := make([]Param, len(fn.Params))
	for i, p := range fn.Params {
		paramType := p.Type
		if replaced, ok := subst[typeKey(p.Type)]; ok {
			paramType = replaced
		}
		// Also try direct match with type args.
		if i < len(typeArgs) {
			if _, ok := p.Type.(*types.TypeVar); ok {
				paramType = typeArgs[i]
			}
		}
		params[i] = Param{Name: p.Name, Type: paramType}
	}

	retType := fn.ReturnType
	if replaced, ok := subst[typeKey(fn.ReturnType)]; ok {
		retType = replaced
	}

	var body *Block
	if fn.Body != nil {
		body = substituteBlockTypes(fn.Body, subst)
	}

	return &Function{
		Name:       name,
		Params:     params,
		ReturnType: retType,
		Body:       body,
		Span:       fn.Span,
	}
}

// ---------------------------------------------------------------------------
// Type substitution in HIR nodes
// ---------------------------------------------------------------------------

func substituteBlockTypes(block *Block, subst map[string]types.Type) *Block {
	if block == nil {
		return nil
	}
	stmts := make([]Stmt, len(block.Stmts))
	for i, s := range block.Stmts {
		stmts[i] = substituteStmtTypes(s, subst)
	}
	var trailing Expr
	if block.TrailingExpr != nil {
		trailing = substituteExprTypes(block.TrailingExpr, subst)
	}
	return &Block{
		exprBase:     exprBase{Typ: substType(block.Typ, subst), Span: block.Span},
		Stmts:        stmts,
		TrailingExpr: trailing,
	}
}

func substituteStmtTypes(stmt Stmt, subst map[string]types.Type) Stmt {
	switch s := stmt.(type) {
	case *LetStmt:
		var value Expr
		if s.Value != nil {
			value = substituteExprTypes(s.Value, subst)
		}
		return &LetStmt{
			Name:    s.Name,
			Type:    substType(s.Type, subst),
			Value:   value,
			Mutable: s.Mutable,
			Span:    s.Span,
		}
	case *ExprStmt:
		return &ExprStmt{
			Expr: substituteExprTypes(s.Expr, subst),
			Span: s.Span,
		}
	case *ReturnStmt:
		var value Expr
		if s.Value != nil {
			value = substituteExprTypes(s.Value, subst)
		}
		return &ReturnStmt{Value: value, Span: s.Span}
	default:
		return stmt
	}
}

func substituteExprTypes(expr Expr, subst map[string]types.Type) Expr {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *VarRef:
		return &VarRef{
			exprBase: exprBase{Typ: substType(e.Typ, subst), Span: e.Span},
			Name:     e.Name,
		}
	case *IntLit:
		return &IntLit{exprBase: exprBase{Typ: substType(e.Typ, subst), Span: e.Span}, Value: e.Value}
	case *FloatLit:
		return &FloatLit{exprBase: exprBase{Typ: substType(e.Typ, subst), Span: e.Span}, Value: e.Value}
	case *StringLit:
		return &StringLit{exprBase: exprBase{Typ: substType(e.Typ, subst), Span: e.Span}, Value: e.Value}
	case *BoolLit:
		return &BoolLit{exprBase: exprBase{Typ: substType(e.Typ, subst), Span: e.Span}, Value: e.Value}
	case *EnumConstruct: // [CLAUDE-FIX]
		args := make([]Expr, len(e.Args))
		for i, a := range e.Args {
			args[i] = substituteExprTypes(a, subst)
		}
		return &EnumConstruct{
			exprBase: exprBase{Typ: substType(e.Typ, subst), Span: e.Span},
			EnumName: e.EnumName,
			Variant:  e.Variant,
			Args:     args,
		}
	case *Call:
		args := make([]Expr, len(e.Args))
		for i, a := range e.Args {
			args[i] = substituteExprTypes(a, subst)
		}
		return &Call{
			exprBase: exprBase{Typ: substType(e.Typ, subst), Span: e.Span},
			Func:     substituteExprTypes(e.Func, subst),
			Args:     args,
		}
	case *Block:
		return substituteBlockTypes(e, subst)
	case *BinaryOp:
		return &BinaryOp{
			exprBase: exprBase{Typ: substType(e.Typ, subst), Span: e.Span},
			Op:       e.Op,
			Left:     substituteExprTypes(e.Left, subst),
			Right:    substituteExprTypes(e.Right, subst),
		}
	case *IfExpr:
		var elseExpr Expr
		if e.Else != nil {
			elseExpr = substituteExprTypes(e.Else, subst)
		}
		return &IfExpr{
			exprBase: exprBase{Typ: substType(e.Typ, subst), Span: e.Span},
			Cond:     substituteExprTypes(e.Cond, subst),
			Then:     substituteBlockTypes(e.Then, subst),
			Else:     elseExpr,
		}
	default:
		return expr
	}
}

func substType(t types.Type, subst map[string]types.Type) types.Type {
	if t == nil {
		return nil
	}
	key := typeKey(t)
	if replaced, ok := subst[key]; ok {
		return replaced
	}
	return t
}

// ---------------------------------------------------------------------------
// Rewriting: replace generic call sites with monomorphized names
// ---------------------------------------------------------------------------

func (m *monomorphizer) rewriteExpr(expr Expr) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *Call:
		if vr, ok := e.Func.(*VarRef); ok {
			if instanceMap, ok := m.instances[vr.Name]; ok {
				typeArgs := m.inferCallTypeArgs(e.Args)
				mangled := mangledName(vr.Name, typeArgs)
				if _, exists := instanceMap[mangled]; exists {
					vr.Name = mangled
				}
			}
		}
		m.rewriteExpr(e.Func)
		for _, arg := range e.Args {
			m.rewriteExpr(arg)
		}
	case *Block:
		for _, s := range e.Stmts {
			m.rewriteStmt(s)
		}
		if e.TrailingExpr != nil {
			m.rewriteExpr(e.TrailingExpr)
		}
	case *BinaryOp:
		m.rewriteExpr(e.Left)
		m.rewriteExpr(e.Right)
	case *UnaryOp:
		m.rewriteExpr(e.Operand)
	case *IfExpr:
		m.rewriteExpr(e.Cond)
		m.rewriteExpr(e.Then)
		if e.Else != nil {
			m.rewriteExpr(e.Else)
		}
	case *WhileExpr:
		m.rewriteExpr(e.Cond)
		m.rewriteExpr(e.Body)
	case *LoopExpr:
		m.rewriteExpr(e.Body)
	case *StaticCall:
		for _, arg := range e.Args {
			m.rewriteExpr(arg)
		}
	case *FieldAccess:
		m.rewriteExpr(e.Object)
	case *Index:
		m.rewriteExpr(e.Object)
		m.rewriteExpr(e.Idx)
	case *ArrayLiteral:
		for _, elem := range e.Elems {
			m.rewriteExpr(elem)
		}
	case *TupleLiteral:
		for _, elem := range e.Elems {
			m.rewriteExpr(elem)
		}
	case *StructLiteral:
		for _, f := range e.Fields {
			m.rewriteExpr(f.Value)
		}
	case *EnumConstruct: // [CLAUDE-FIX]
		for _, arg := range e.Args {
			m.rewriteExpr(arg)
		}
	case *Assign: // [CLAUDE-FIX]
		m.rewriteExpr(e.Value)
	case *FieldAssign: // [CLAUDE-FIX]
		m.rewriteExpr(e.Object)
		m.rewriteExpr(e.Value)
	case *IndexAssign: // [CLAUDE-FIX]
		m.rewriteExpr(e.Object)
		m.rewriteExpr(e.Index)
		m.rewriteExpr(e.Value)
	case *MatchExpr:
		m.rewriteExpr(e.Scrutinee)
		for _, arm := range e.Arms {
			if arm.Guard != nil {
				m.rewriteExpr(arm.Guard)
			}
			m.rewriteExpr(arm.Body)
		}
	case *Lambda:
		m.rewriteExpr(e.Body)
	case *ReturnExpr:
		if e.Value != nil {
			m.rewriteExpr(e.Value)
		}
	case *Cast:
		m.rewriteExpr(e.Expr)
	case *Spawn:
		m.rewriteExpr(e.Body)
	}
}

func (m *monomorphizer) rewriteStmt(stmt Stmt) {
	switch s := stmt.(type) {
	case *LetStmt:
		if s.Value != nil {
			m.rewriteExpr(s.Value)
		}
	case *ExprStmt:
		m.rewriteExpr(s.Expr)
	case *ReturnStmt:
		if s.Value != nil {
			m.rewriteExpr(s.Value)
		}
	}
}

func (m *monomorphizer) inferCallTypeArgs(args []Expr) []types.Type {
	var typeArgs []types.Type
	for _, arg := range args {
		t := arg.ExprType()
		if t != nil {
			typeArgs = append(typeArgs, t)
		}
	}
	return typeArgs
}

// ---------------------------------------------------------------------------
// Type monomorphization: generic struct / enum instantiation
// ---------------------------------------------------------------------------

// generateStructInstances creates concrete StructDef copies for each
// unique combination of type arguments observed at struct usage sites.
func (m *monomorphizer) generateStructInstances() {
	if len(m.structSites) == 0 {
		return
	}

	seen := make(map[string]bool)
	counts := make(map[string]int)

	for _, site := range m.structSites {
		mangled := mangledName(site.Name, site.TypeArgs)
		if seen[mangled] {
			continue
		}
		seen[mangled] = true

		counts[site.Name]++
		if counts[site.Name] > m.limit {
			m.diagnostics = append(m.diagnostics, diagnostic.Diagnostic{
				Severity: diagnostic.SeverityError,
				Code:     "E201",
				Message: fmt.Sprintf(
					"monomorphization limit exceeded: struct `%s` has more than %d instantiations",
					site.Name, m.limit),
			})
			continue
		}

		st, ok := site.FullType.(*types.StructType)
		if !ok {
			continue
		}

		// Determine field order.
		order := st.FieldOrder
		if len(order) == 0 {
			for name := range st.Fields {
				order = append(order, name)
			}
			sort.Strings(order)
		}

		fields := make([]FieldDef, 0, len(order))
		for _, name := range order {
			fields = append(fields, FieldDef{
				Name: name,
				Type: st.Fields[name],
			})
		}

		m.newStructs = append(m.newStructs, &StructDef{
			Name:   mangled,
			Fields: fields,
		})
	}
}

// generateEnumInstances creates concrete EnumDef copies for each
// unique combination of type arguments observed at enum usage sites.
func (m *monomorphizer) generateEnumInstances() {
	if len(m.enumSites) == 0 {
		return
	}

	seen := make(map[string]bool)
	counts := make(map[string]int)

	for _, site := range m.enumSites {
		mangled := mangledName(site.Name, site.TypeArgs)
		if seen[mangled] {
			continue
		}
		seen[mangled] = true

		counts[site.Name]++
		if counts[site.Name] > m.limit {
			m.diagnostics = append(m.diagnostics, diagnostic.Diagnostic{
				Severity: diagnostic.SeverityError,
				Code:     "E202",
				Message: fmt.Sprintf(
					"monomorphization limit exceeded: enum `%s` has more than %d instantiations",
					site.Name, m.limit),
			})
			continue
		}

		et, ok := site.FullType.(*types.EnumType)
		if !ok {
			continue
		}

		var variants []VariantDef
		for name, fieldTypes := range et.Variants {
			variants = append(variants, VariantDef{
				Name:   name,
				Fields: fieldTypes,
			})
		}
		// Sort for deterministic output.
		sort.Slice(variants, func(i, j int) bool {
			return variants[i].Name < variants[j].Name
		})

		m.newEnums = append(m.newEnums, &EnumDef{
			Name:     mangled,
			Variants: variants,
		})
	}
}

// StructInstanceCount returns the number of monomorphized instances for a struct.
func StructInstanceCount(prog *Program, baseName string) int {
	count := 0
	prefix := baseName + "$"
	for _, sd := range prog.Structs {
		if sd.Name == baseName || strings.HasPrefix(sd.Name, prefix) {
			count++
		}
	}
	return count
}

// EnumInstanceCount returns the number of monomorphized instances for an enum.
func EnumInstanceCount(prog *Program, baseName string) int {
	count := 0
	prefix := baseName + "$"
	for _, ed := range prog.Enums {
		if ed.Name == baseName || strings.HasPrefix(ed.Name, prefix) {
			count++
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// hasConcreteTypes returns true if any of the type args are concrete (non-TypeVar).
func hasConcreteTypes(typeArgs []types.Type) bool {
	for _, t := range typeArgs {
		if _, ok := t.(*types.TypeVar); !ok {
			return true
		}
	}
	return false
}

// typesEqual checks if two types are structurally equal.
func typesEqual(a, b types.Type) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(b)
}

// InstanceCount returns the number of monomorphized instances for a function.
// Useful for testing.
func InstanceCount(prog *Program, baseName string) int {
	count := 0
	prefix := baseName + "$"
	for _, fn := range prog.Functions {
		if fn.Name == baseName || strings.HasPrefix(fn.Name, prefix) {
			count++
		}
	}
	return count
}
