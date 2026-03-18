package hir

import (
	"fmt"

	"github.com/ryx-lang/ryx/pkg/types"
)

// ---------------------------------------------------------------------------
// Match compilation: pattern match → decision tree
// ---------------------------------------------------------------------------

// CompileMatches walks the HIR program and compiles all MatchExpr nodes
// into decision trees. After this pass, every MatchExpr.Decision is
// populated and the Arms field may be cleared.
func CompileMatches(prog *Program) {
	for _, fn := range prog.Functions {
		if fn.Body != nil {
			compileMatchesInExpr(fn.Body)
		}
	}
}

// compileMatchesInExpr recursively walks an expression and compiles all
// MatchExpr nodes found within.
func compileMatchesInExpr(expr Expr) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *MatchExpr:
		// Compile this match expression's arms into a decision tree.
		e.Decision = compileArms(e.Scrutinee, e.Arms)
		// Walk the decision tree for nested matches.
		compileMatchesInDecision(e.Decision)
		// Walk scrutinee for nested matches.
		compileMatchesInExpr(e.Scrutinee)
	case *Block:
		for _, s := range e.Stmts {
			compileMatchesInStmt(s)
		}
		if e.TrailingExpr != nil {
			compileMatchesInExpr(e.TrailingExpr)
		}
	case *IfExpr:
		compileMatchesInExpr(e.Cond)
		compileMatchesInExpr(e.Then)
		if e.Else != nil {
			compileMatchesInExpr(e.Else)
		}
	case *WhileExpr:
		compileMatchesInExpr(e.Cond)
		compileMatchesInExpr(e.Body)
	case *LoopExpr:
		compileMatchesInExpr(e.Body)
	case *Call:
		compileMatchesInExpr(e.Func)
		for _, arg := range e.Args {
			compileMatchesInExpr(arg)
		}
	case *StaticCall:
		for _, arg := range e.Args {
			compileMatchesInExpr(arg)
		}
	case *BinaryOp:
		compileMatchesInExpr(e.Left)
		compileMatchesInExpr(e.Right)
	case *UnaryOp:
		compileMatchesInExpr(e.Operand)
	case *Lambda:
		compileMatchesInExpr(e.Body)
	case *ReturnExpr:
		if e.Value != nil {
			compileMatchesInExpr(e.Value)
		}
	case *Cast:
		compileMatchesInExpr(e.Expr)
	case *Spawn:
		compileMatchesInExpr(e.Body)
	case *ArrayLiteral:
		for _, elem := range e.Elems {
			compileMatchesInExpr(elem)
		}
	case *TupleLiteral:
		for _, elem := range e.Elems {
			compileMatchesInExpr(elem)
		}
	case *StructLiteral:
		for _, f := range e.Fields {
			compileMatchesInExpr(f.Value)
		}
	case *EnumConstruct: // [CLAUDE-FIX]
		for _, arg := range e.Args {
			compileMatchesInExpr(arg)
		}
	case *Assign: // [CLAUDE-FIX]
		compileMatchesInExpr(e.Value)
	case *FieldAssign: // [CLAUDE-FIX]
		compileMatchesInExpr(e.Object)
		compileMatchesInExpr(e.Value)
	case *IndexAssign: // [CLAUDE-FIX]
		compileMatchesInExpr(e.Object)
		compileMatchesInExpr(e.Index)
		compileMatchesInExpr(e.Value)
	case *FieldAccess:
		compileMatchesInExpr(e.Object)
	case *Index:
		compileMatchesInExpr(e.Object)
		compileMatchesInExpr(e.Idx)
	}
}

func compileMatchesInStmt(stmt Stmt) {
	switch s := stmt.(type) {
	case *LetStmt:
		if s.Value != nil {
			compileMatchesInExpr(s.Value)
		}
	case *ExprStmt:
		compileMatchesInExpr(s.Expr)
	case *ReturnStmt:
		if s.Value != nil {
			compileMatchesInExpr(s.Value)
		}
	}
}

func compileMatchesInDecision(d Decision) {
	if d == nil {
		return
	}
	switch dt := d.(type) {
	case *DecisionLeaf:
		compileMatchesInExpr(dt.Body)
	case *DecisionSwitch:
		compileMatchesInExpr(dt.Scrutinee)
		for _, c := range dt.Cases {
			compileMatchesInDecision(c.Body)
		}
		if dt.Default != nil {
			compileMatchesInDecision(dt.Default)
		}
	case *DecisionGuard:
		compileMatchesInExpr(dt.Condition)
		compileMatchesInDecision(dt.Then)
		compileMatchesInDecision(dt.Else)
	}
}

// ---------------------------------------------------------------------------
// Core compilation algorithm
// ---------------------------------------------------------------------------

// compileArms compiles a list of match arms into a decision tree.
// The algorithm groups consecutive variant/literal patterns into a single
// DecisionSwitch for efficiency (no redundant checks), and processes arms
// top-to-bottom respecting first-match semantics.
func compileArms(scrutinee Expr, arms []*MatchArm) Decision {
	if len(arms) == 0 {
		return &DecisionFail{}
	}

	// Try to group consecutive variant patterns into a single switch.
	if variantGroup, defaultArms := groupVariantArms(arms); len(variantGroup) > 0 {
		return compileVariantGroup(scrutinee, variantGroup, defaultArms)
	}

	// Try to group consecutive literal patterns into a single switch.
	if litGroup, defaultArms := groupLiteralArms(arms); len(litGroup) > 0 {
		return compileLiteralGroup(scrutinee, litGroup, defaultArms)
	}

	arm := arms[0]
	rest := arms[1:]

	return compilePattern(scrutinee, arm.Pattern, arm.Guard, arm.Body, rest)
}

// groupVariantArms collects consecutive VariantPat arms from the front.
// It stops when it encounters a duplicate constructor or a variant with
// nested non-trivial patterns (which require individual compilation).
func groupVariantArms(arms []*MatchArm) ([]*MatchArm, []*MatchArm) {
	var group []*MatchArm
	seen := make(map[string]bool)
	for i, arm := range arms {
		vp, ok := arm.Pattern.(*VariantPat)
		if !ok {
			break
		}
		if seen[vp.Name] || hasNestedPatterns(vp) {
			return group, arms[i:]
		}
		seen[vp.Name] = true
		group = append(group, arm)
	}
	if len(group) < 1 {
		return nil, arms
	}
	return group, arms[len(group):]
}

// hasNestedPatterns returns true if a VariantPat has non-trivial (nested
// variant) patterns in any of its field positions.
func hasNestedPatterns(vp *VariantPat) bool {
	for _, fp := range vp.Fields {
		switch fp.(type) {
		case *VariantPat:
			return true
		}
	}
	return false
}

// groupLiteralArms collects consecutive LiteralPat arms (without guards) from the front.
func groupLiteralArms(arms []*MatchArm) ([]*MatchArm, []*MatchArm) {
	var group []*MatchArm
	for _, arm := range arms {
		if _, ok := arm.Pattern.(*LiteralPat); ok {
			group = append(group, arm)
		} else {
			break
		}
	}
	if len(group) < 1 {
		return nil, arms
	}
	return group, arms[len(group):]
}

// compileVariantGroup creates a single DecisionSwitch with one case per variant.
func compileVariantGroup(scrutinee Expr, variantArms []*MatchArm, defaultArms []*MatchArm) Decision {
	cases := make([]*DecisionCase, 0, len(variantArms))

	for _, arm := range variantArms {
		pat := arm.Pattern.(*VariantPat)
		caseDecision := compileVariantCase(scrutinee, pat, arm.Guard, arm.Body)
		var argNames []string
		for _, fp := range pat.Fields {
			switch f := fp.(type) {
			case *BindingPat:
				argNames = append(argNames, f.Name)
			default:
				argNames = append(argNames, "_")
			}
		}
		cases = append(cases, &DecisionCase{
			Constructor: pat.Name,
			ArgNames:    argNames,
			Body:        caseDecision,
		})
	}

	var defaultDecision Decision
	if len(defaultArms) > 0 {
		defaultDecision = compileArms(scrutinee, defaultArms)
	} else {
		defaultDecision = &DecisionFail{}
	}

	return &DecisionSwitch{
		Scrutinee: scrutinee,
		Cases:     cases,
		Default:   defaultDecision,
	}
}

// compileVariantCase compiles a single variant pattern into a decision (leaf or guarded).
// Handles nested VariantPat patterns in field positions by wrapping the leaf
// in nested DecisionSwitch checks.
func compileVariantCase(scrutinee Expr, pat *VariantPat, guard Expr, body Expr) Decision {
	var bindings []Binding
	type nestedCheck struct {
		fieldAccess Expr
		pat         *VariantPat
	}
	var nested []nestedCheck

	for i, fieldPat := range pat.Fields {
		fieldAccess := &FieldAccess{
			exprBase: exprBase{
				Typ:  fieldTypeFromScrutinee(scrutinee, pat.Name, i),
				Span: scrutinee.ExprSpan(),
			},
			Object: scrutinee,
			Field:  variantFieldName(i),
		}

		switch fp := fieldPat.(type) {
		case *BindingPat:
			bindings = append(bindings, Binding{
				Name: fp.Name,
				Expr: fieldAccess,
				Type: fieldAccess.ExprType(),
			})
		case *VariantPat:
			nested = append(nested, nestedCheck{fieldAccess: fieldAccess, pat: fp})
			// Collect bindings from the nested variant's fields.
			collectNestedBindings(fieldAccess, fp, &bindings)
		}
	}

	leaf := &DecisionLeaf{Bindings: bindings, Body: body}
	var result Decision
	if guard != nil {
		result = &DecisionGuard{
			Condition: guard,
			Then:      leaf,
			Else:      &DecisionFail{},
		}
	} else {
		result = leaf
	}

	// Wrap with nested variant checks (innermost first).
	for i := len(nested) - 1; i >= 0; i-- {
		nc := nested[i]
		result = &DecisionSwitch{
			Scrutinee: nc.fieldAccess,
			Cases: []*DecisionCase{{
				Constructor: nc.pat.Name,
				Body:        result,
			}},
			Default: &DecisionFail{},
		}
	}

	return result
}

// collectNestedBindings recursively collects bindings from a nested VariantPat.
func collectNestedBindings(parentField Expr, pat *VariantPat, bindings *[]Binding) {
	for i, fp := range pat.Fields {
		fieldAccess := &FieldAccess{
			exprBase: exprBase{
				Typ:  fieldTypeFromScrutinee(parentField, pat.Name, i),
				Span: parentField.ExprSpan(),
			},
			Object: parentField,
			Field:  variantFieldName(i),
		}
		switch f := fp.(type) {
		case *BindingPat:
			*bindings = append(*bindings, Binding{
				Name: f.Name,
				Expr: fieldAccess,
				Type: fieldAccess.ExprType(),
			})
		case *VariantPat:
			collectNestedBindings(fieldAccess, f, bindings)
		}
	}
}

// compileLiteralGroup creates a single DecisionSwitch with one case per literal.
func compileLiteralGroup(scrutinee Expr, litArms []*MatchArm, defaultArms []*MatchArm) Decision {
	cases := make([]*DecisionCase, 0, len(litArms))

	for _, arm := range litArms {
		pat := arm.Pattern.(*LiteralPat)
		caseBody := withGuard(arm.Guard, arm.Body, nil, scrutinee)
		cases = append(cases, &DecisionCase{
			Constructor: FormatExpr(pat.Value),
			Body:        caseBody,
		})
	}

	var defaultDecision Decision
	if len(defaultArms) > 0 {
		defaultDecision = compileArms(scrutinee, defaultArms)
	} else {
		defaultDecision = &DecisionFail{}
	}

	return &DecisionSwitch{
		Scrutinee: scrutinee,
		Cases:     cases,
		Default:   defaultDecision,
	}
}

// compilePattern compiles a single pattern against a scrutinee, producing a
// decision tree. If the pattern doesn't match, falls through to remaining arms.
func compilePattern(
	scrutinee Expr,
	pattern Pattern,
	guard Expr,
	body Expr,
	rest []*MatchArm,
) Decision {
	switch p := pattern.(type) {
	case *WildcardPat:
		return withGuard(guard, body, rest, scrutinee)

	case *BindingPat:
		bindings := []Binding{{
			Name: p.Name,
			Expr: scrutinee,
			Type: scrutinee.ExprType(),
		}}
		leaf := &DecisionLeaf{Bindings: bindings, Body: body}
		return withGuardDecision(guard, leaf, rest, scrutinee)

	case *LiteralPat:
		litCase := &DecisionCase{
			Constructor: FormatExpr(p.Value),
			Body:        withGuard(guard, body, nil, scrutinee),
		}
		fallback := compileArms(scrutinee, rest)
		return &DecisionSwitch{
			Scrutinee: scrutinee,
			Cases:     []*DecisionCase{litCase},
			Default:   fallback,
		}

	case *VariantPat:
		caseDecision := compileVariantCase(scrutinee, p, guard, body)
		var argNames []string
		for _, fp := range p.Fields {
			switch f := fp.(type) {
			case *BindingPat:
				argNames = append(argNames, f.Name)
			default:
				argNames = append(argNames, "_")
			}
		}
		variantCase := &DecisionCase{
			Constructor: p.Name,
			ArgNames:    argNames,
			Body:        caseDecision,
		}
		fallback := compileArms(scrutinee, rest)
		return &DecisionSwitch{
			Scrutinee: scrutinee,
			Cases:     []*DecisionCase{variantCase},
			Default:   fallback,
		}

	case *TuplePat:
		return compileTuplePattern(scrutinee, p, guard, body, rest)

	case *StructPat:
		return compileStructPattern(scrutinee, p, guard, body, rest)

	case *OrPat:
		return compileOrPattern(scrutinee, p, guard, body, rest)

	case *RangePat:
		return compileRangePattern(scrutinee, p, guard, body, rest)

	default:
		return withGuard(guard, body, rest, scrutinee)
	}
}

// compileTuplePattern compiles a tuple destructuring pattern.
func compileTuplePattern(
	scrutinee Expr,
	pat *TuplePat,
	guard Expr,
	body Expr,
	rest []*MatchArm,
) Decision {
	var bindings []Binding

	for i, elemPat := range pat.Elems {
		elemAccess := &Index{
			exprBase: exprBase{
				Typ:  tupleElemType(scrutinee.ExprType(), i),
				Span: scrutinee.ExprSpan(),
			},
			Object: scrutinee,
			Idx: &IntLit{
				exprBase: exprBase{Typ: types.TypInt, Span: scrutinee.ExprSpan()},
				Value:    fmt.Sprintf("%d", i),
			},
		}

		switch ep := elemPat.(type) {
		case *BindingPat:
			bindings = append(bindings, Binding{
				Name: ep.Name,
				Expr: elemAccess,
				Type: elemAccess.ExprType(),
			})
		case *WildcardPat:
			// Skip.
		}
	}

	leaf := &DecisionLeaf{Bindings: bindings, Body: body}
	return withGuardDecision(guard, leaf, rest, scrutinee)
}

// compileStructPattern compiles a struct destructuring pattern.
func compileStructPattern(
	scrutinee Expr,
	pat *StructPat,
	guard Expr,
	body Expr,
	rest []*MatchArm,
) Decision {
	var bindings []Binding

	for _, field := range pat.Fields {
		fieldAccess := &FieldAccess{
			exprBase: exprBase{
				Typ:  structFieldType(scrutinee.ExprType(), field.Name),
				Span: scrutinee.ExprSpan(),
			},
			Object: scrutinee,
			Field:  field.Name,
		}

		switch fp := field.Pattern.(type) {
		case *BindingPat:
			bindings = append(bindings, Binding{
				Name: fp.Name,
				Expr: fieldAccess,
				Type: fieldAccess.ExprType(),
			})
		case *WildcardPat:
			// Skip.
		}
	}

	structCase := &DecisionCase{
		Constructor: pat.Name,
		Body: withGuardDecision(guard,
			&DecisionLeaf{Bindings: bindings, Body: body},
			nil, scrutinee),
	}

	fallback := compileArms(scrutinee, rest)

	return &DecisionSwitch{
		Scrutinee: scrutinee,
		Cases:     []*DecisionCase{structCase},
		Default:   fallback,
	}
}

// compileOrPattern compiles an or-pattern by trying each alternative.
func compileOrPattern(
	scrutinee Expr,
	pat *OrPat,
	guard Expr,
	body Expr,
	rest []*MatchArm,
) Decision {
	if len(pat.Alts) == 0 {
		return compileArms(scrutinee, rest)
	}

	// For each alternative, create a match arm with the same guard and body.
	altArms := make([]*MatchArm, len(pat.Alts))
	for i, alt := range pat.Alts {
		altArms[i] = &MatchArm{
			Pattern: alt,
			Guard:   guard,
			Body:    body,
		}
	}
	// Append the remaining arms after all alternatives.
	altArms = append(altArms, rest...)

	return compileArms(scrutinee, altArms)
}

// compileRangePattern compiles a range pattern into a guard-based check.
func compileRangePattern(
	scrutinee Expr,
	pat *RangePat,
	guard Expr,
	body Expr,
	rest []*MatchArm,
) Decision {
	// Generate: scrutinee >= start && scrutinee <= end (or < end).
	lowerBound := &BinaryOp{
		exprBase: exprBase{Typ: types.TypBool, Span: scrutinee.ExprSpan()},
		Op:       ">=",
		Left:     scrutinee,
		Right:    pat.Start,
	}

	upperOp := "<"
	if pat.Inclusive {
		upperOp = "<="
	}
	upperBound := &BinaryOp{
		exprBase: exprBase{Typ: types.TypBool, Span: scrutinee.ExprSpan()},
		Op:       upperOp,
		Left:     scrutinee,
		Right:    pat.End,
	}

	rangeCheck := &BinaryOp{
		exprBase: exprBase{Typ: types.TypBool, Span: scrutinee.ExprSpan()},
		Op:       "&&",
		Left:     lowerBound,
		Right:    upperBound,
	}

	// Combine with existing guard if present.
	var fullGuard Expr
	if guard != nil {
		fullGuard = &BinaryOp{
			exprBase: exprBase{Typ: types.TypBool, Span: scrutinee.ExprSpan()},
			Op:       "&&",
			Left:     rangeCheck,
			Right:    guard,
		}
	} else {
		fullGuard = rangeCheck
	}

	thenBranch := &DecisionLeaf{Body: body}
	elseBranch := compileArms(scrutinee, rest)

	return &DecisionGuard{
		Condition: fullGuard,
		Then:      thenBranch,
		Else:      elseBranch,
	}
}

// ---------------------------------------------------------------------------
// Guard handling
// ---------------------------------------------------------------------------

// withGuard wraps a match body with a guard check if present.
func withGuard(guard Expr, body Expr, rest []*MatchArm, scrutinee Expr) Decision {
	leaf := &DecisionLeaf{Body: body}
	if guard == nil {
		return leaf
	}
	elseBranch := compileArms(scrutinee, rest)
	return &DecisionGuard{
		Condition: guard,
		Then:      leaf,
		Else:      elseBranch,
	}
}

// withGuardDecision wraps a decision with a guard check if present.
func withGuardDecision(guard Expr, d Decision, rest []*MatchArm, scrutinee Expr) Decision {
	if guard == nil {
		return d
	}
	elseBranch := compileArms(scrutinee, rest)
	return &DecisionGuard{
		Condition: guard,
		Then:      d,
		Else:      elseBranch,
	}
}

// ---------------------------------------------------------------------------
// Type helpers for field access during pattern compilation
// ---------------------------------------------------------------------------

func fieldTypeFromScrutinee(scrutinee Expr, variantName string, fieldIdx int) types.Type {
	if t := scrutinee.ExprType(); t != nil {
		if et, ok := t.(*types.EnumType); ok {
			if fields, ok := et.Variants[variantName]; ok && fieldIdx < len(fields) {
				return fields[fieldIdx]
			}
		}
	}
	return types.TypUnit
}

func variantFieldName(idx int) string {
	return fmt.Sprintf("_%d", idx)
}

func tupleElemType(t types.Type, idx int) types.Type {
	if tt, ok := t.(*types.TupleType); ok && idx < len(tt.Elems) {
		return tt.Elems[idx]
	}
	return types.TypUnit
}

func structFieldType(t types.Type, fieldName string) types.Type {
	if st, ok := t.(*types.StructType); ok {
		if ft, ok := st.Fields[fieldName]; ok {
			return ft
		}
	}
	return types.TypUnit
}

