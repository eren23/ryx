package resolver

import (
	"fmt"
	"strings"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/parser"
)

// ExhaustivenessResult contains the output of an exhaustiveness check.
type ExhaustivenessResult struct {
	Exhaustive  bool
	Diagnostics []diagnostic.Diagnostic
}

// CheckExhaustiveness checks whether a match expression's arms cover all cases.
func CheckExhaustiveness(me *parser.MatchExpr, typeDefs map[string]*parser.TypeDef) *ExhaustivenessResult {
	result := &ExhaustivenessResult{Exhaustive: true}
	if len(me.Arms) == 0 {
		result.Exhaustive = false
		result.Diagnostics = append(result.Diagnostics, diagnostic.Diagnostic{
			Severity: diagnostic.SeverityError,
			Code:     "E040",
			Message:  "non-exhaustive match: no arms provided",
			Span:     me.Span(),
		})
		return result
	}

	checker := &exhaustivenessChecker{
		typeDefs: typeDefs,
		result:   result,
		matchSpan: me.Span(),
	}

	// Build pattern matrix from arms.
	var patterns []patternInfo
	for i, arm := range me.Arms {
		patterns = append(patterns, patternInfo{
			pattern:  arm.Pattern,
			hasGuard: arm.Guard != nil,
			index:    i,
		})
	}

	checker.check(patterns)
	checker.checkUnreachable(patterns)

	return result
}

type patternInfo struct {
	pattern  parser.Pattern
	hasGuard bool
	index    int
}

type exhaustivenessChecker struct {
	typeDefs  map[string]*parser.TypeDef
	result    *ExhaustivenessResult
	matchSpan diagnostic.Span
}

// check determines if the set of patterns exhaustively covers all cases.
func (c *exhaustivenessChecker) check(patterns []patternInfo) {
	if len(patterns) == 0 {
		c.result.Exhaustive = false
		return
	}

	// Check if any pattern is a catch-all (wildcard or binding without guard).
	for _, p := range patterns {
		if c.isCatchAll(p.pattern) && !p.hasGuard {
			return // exhaustive
		}
	}

	// Determine what kind of patterns we have and check accordingly.
	variantPatterns := c.collectVariantPatterns(patterns)
	if len(variantPatterns) > 0 {
		c.checkVariantExhaustiveness(patterns, variantPatterns)
		return
	}

	// Check for tuple patterns.
	tuplePatterns := c.collectTuplePatterns(patterns)
	if len(tuplePatterns) > 0 {
		c.checkTupleExhaustiveness(patterns, tuplePatterns)
		return
	}

	// Check for bool literal exhaustiveness.
	if c.checkBoolExhaustiveness(patterns) {
		return
	}

	// For literal-only patterns (int, string, etc.) without a wildcard, not exhaustive.
	hasOnlyLiterals := true
	for _, p := range patterns {
		if !c.isLiteralPattern(p.pattern) {
			hasOnlyLiterals = false
			break
		}
	}
	if hasOnlyLiterals {
		c.result.Exhaustive = false
		c.result.Diagnostics = append(c.result.Diagnostics, diagnostic.Diagnostic{
			Severity: diagnostic.SeverityError,
			Code:     "E041",
			Message:  "non-exhaustive match: literal patterns do not cover all cases, add a wildcard `_` arm",
			Span:     c.matchSpan,
		})
		return
	}

	// If we can't determine, assume non-exhaustive if no catch-all.
	c.result.Exhaustive = false
	c.result.Diagnostics = append(c.result.Diagnostics, diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError,
		Code:     "E041",
		Message:  "non-exhaustive match: not all cases are covered",
		Span:     c.matchSpan,
	})
}

// checkVariantExhaustiveness checks if all variants of an enum are covered.
func (c *exhaustivenessChecker) checkVariantExhaustiveness(allPatterns []patternInfo, variantNames map[string][]patternInfo) {
	// Find the type definition to get all variants.
	var typeDef *parser.TypeDef
	for name := range variantNames {
		typeDef = c.findTypeDefForVariant(name)
		if typeDef != nil {
			break
		}
	}
	if typeDef == nil {
		// Can't find type def — can't verify exhaustiveness.
		return
	}

	// Check which variants are covered.
	var missing []string
	for _, v := range typeDef.Variants {
		pats, covered := variantNames[v.Name]
		if !covered {
			missing = append(missing, v.Name)
			continue
		}

		// Check if the variant's sub-patterns are exhaustive.
		if len(v.Fields) > 0 {
			// All arms for this variant must collectively cover the fields.
			allGuarded := true
			for _, p := range pats {
				if !p.hasGuard {
					allGuarded = false
					break
				}
			}
			if allGuarded {
				// All arms for this variant have guards — not guaranteed exhaustive.
				missing = append(missing, v.Name+" (guarded)")
			}
		} else {
			// No fields — just check that at least one arm has no guard.
			allGuarded := true
			for _, p := range pats {
				if !p.hasGuard {
					allGuarded = false
					break
				}
			}
			if allGuarded {
				missing = append(missing, v.Name+" (guarded)")
			}
		}
	}

	if len(missing) > 0 {
		c.result.Exhaustive = false
		c.result.Diagnostics = append(c.result.Diagnostics, diagnostic.Diagnostic{
			Severity: diagnostic.SeverityError,
			Code:     "E042",
			Message:  fmt.Sprintf("non-exhaustive match: missing variant(s): %s", strings.Join(missing, ", ")),
			Span:     c.matchSpan,
		})
	}
}

// checkTupleExhaustiveness checks if tuple patterns cover all positions.
func (c *exhaustivenessChecker) checkTupleExhaustiveness(allPatterns []patternInfo, tuplePatterns []*parser.TuplePat) {
	if len(tuplePatterns) == 0 {
		return
	}

	// Check each position independently.
	arity := len(tuplePatterns[0].Elems)
	for pos := 0; pos < arity; pos++ {
		positionCovered := false
		for _, tp := range tuplePatterns {
			if pos < len(tp.Elems) {
				if c.isCatchAll(tp.Elems[pos]) {
					positionCovered = true
					break
				}
			}
		}
		if !positionCovered {
			// Check if all values at this position are covered
			// (e.g., all bool values, all variant constructors).
			var posPatterns []patternInfo
			for i, tp := range tuplePatterns {
				if pos < len(tp.Elems) {
					posPatterns = append(posPatterns, patternInfo{
						pattern:  tp.Elems[pos],
						hasGuard: allPatterns[i].hasGuard,
						index:    i,
					})
				}
			}
			// Check if position patterns are exhaustive for bools.
			if !c.arePositionPatternsExhaustive(posPatterns) {
				c.result.Exhaustive = false
				c.result.Diagnostics = append(c.result.Diagnostics, diagnostic.Diagnostic{
					Severity: diagnostic.SeverityError,
					Code:     "E043",
					Message:  fmt.Sprintf("non-exhaustive match: tuple position %d is not fully covered", pos),
					Span:     c.matchSpan,
				})
				return
			}
		}
	}
}

// arePositionPatternsExhaustive checks if patterns at a single tuple position cover all cases.
func (c *exhaustivenessChecker) arePositionPatternsExhaustive(patterns []patternInfo) bool {
	for _, p := range patterns {
		if c.isCatchAll(p.pattern) && !p.hasGuard {
			return true
		}
	}

	// Check bool exhaustiveness.
	hasTrue, hasFalse := false, false
	for _, p := range patterns {
		if lp, ok := p.pattern.(*parser.LiteralPat); ok {
			if bl, ok := lp.Value.(*parser.BoolLit); ok {
				if bl.Value {
					hasTrue = true
				} else {
					hasFalse = true
				}
			}
		}
	}
	if hasTrue && hasFalse {
		return true
	}

	// Check variant exhaustiveness.
	variantNames := c.collectVariantPatterns(patterns)
	if len(variantNames) > 0 {
		for name := range variantNames {
			td := c.findTypeDefForVariant(name)
			if td != nil {
				allCovered := true
				for _, v := range td.Variants {
					if _, ok := variantNames[v.Name]; !ok {
						allCovered = false
						break
					}
				}
				if allCovered {
					return true
				}
			}
			break
		}
	}

	return false
}

// checkBoolExhaustiveness checks if bool literal patterns cover true and false.
func (c *exhaustivenessChecker) checkBoolExhaustiveness(patterns []patternInfo) bool {
	hasTrue, hasFalse := false, false
	hasCatchAll := false

	for _, p := range patterns {
		if c.isCatchAll(p.pattern) && !p.hasGuard {
			hasCatchAll = true
			break
		}
		if lp, ok := p.pattern.(*parser.LiteralPat); ok {
			if bl, ok := lp.Value.(*parser.BoolLit); ok && !p.hasGuard {
				if bl.Value {
					hasTrue = true
				} else {
					hasFalse = true
				}
			}
		}
	}

	if hasCatchAll {
		return true
	}

	if hasTrue && hasFalse {
		return true
	}

	if hasTrue || hasFalse {
		c.result.Exhaustive = false
		missing := "true"
		if hasTrue {
			missing = "false"
		}
		c.result.Diagnostics = append(c.result.Diagnostics, diagnostic.Diagnostic{
			Severity: diagnostic.SeverityError,
			Code:     "E041",
			Message:  fmt.Sprintf("non-exhaustive match: missing case `%s`", missing),
			Span:     c.matchSpan,
		})
		return true // we handled it (even though it's non-exhaustive)
	}

	return false // not a bool match
}

// checkUnreachable checks for arms that can never match because a previous
// arm already covers them.
func (c *exhaustivenessChecker) checkUnreachable(patterns []patternInfo) {
	for i, p := range patterns {
		if i == 0 {
			continue
		}

		// Check if any previous pattern without a guard is a catch-all.
		for j := 0; j < i; j++ {
			prev := patterns[j]
			if prev.hasGuard {
				continue
			}
			if c.isCatchAll(prev.pattern) {
				c.result.Diagnostics = append(c.result.Diagnostics, diagnostic.Diagnostic{
					Severity: diagnostic.SeverityWarning,
					Code:     "W020",
					Message:  "unreachable match arm",
					Span:     p.pattern.Span(),
				})
				break
			}
			// Check if a previous variant pattern covers this one.
			if c.patternSubsumes(prev.pattern, p.pattern) {
				c.result.Diagnostics = append(c.result.Diagnostics, diagnostic.Diagnostic{
					Severity: diagnostic.SeverityWarning,
					Code:     "W020",
					Message:  "unreachable match arm",
					Span:     p.pattern.Span(),
				})
				break
			}
		}
	}
}

// patternSubsumes returns true if pattern a covers all cases of pattern b.
func (c *exhaustivenessChecker) patternSubsumes(a, b parser.Pattern) bool {
	if c.isCatchAll(a) {
		return true
	}

	switch pa := a.(type) {
	case *parser.VariantPat:
		pb, ok := b.(*parser.VariantPat)
		if !ok || pa.Name != pb.Name {
			return false
		}
		// Check sub-patterns.
		if len(pa.Fields) != len(pb.Fields) {
			return false
		}
		for i := range pa.Fields {
			if !c.patternSubsumes(pa.Fields[i], pb.Fields[i]) {
				return false
			}
		}
		return true
	case *parser.TuplePat:
		pb, ok := b.(*parser.TuplePat)
		if !ok || len(pa.Elems) != len(pb.Elems) {
			return false
		}
		for i := range pa.Elems {
			if !c.patternSubsumes(pa.Elems[i], pb.Elems[i]) {
				return false
			}
		}
		return true
	case *parser.LiteralPat:
		pb, ok := b.(*parser.LiteralPat)
		if !ok {
			return false
		}
		return c.literalsEqual(pa.Value, pb.Value)
	case *parser.OrPat:
		// Or pattern subsumes b if any alternative does.
		for _, alt := range pa.Alts {
			if c.patternSubsumes(alt, b) {
				return true
			}
		}
		return false
	}

	return false
}

// literalsEqual compares two literal expressions for equality.
func (c *exhaustivenessChecker) literalsEqual(a, b parser.Expr) bool {
	switch la := a.(type) {
	case *parser.IntLit:
		lb, ok := b.(*parser.IntLit)
		return ok && la.Value == lb.Value
	case *parser.FloatLit:
		lb, ok := b.(*parser.FloatLit)
		return ok && la.Value == lb.Value
	case *parser.StringLit:
		lb, ok := b.(*parser.StringLit)
		return ok && la.Value == lb.Value
	case *parser.CharLit:
		lb, ok := b.(*parser.CharLit)
		return ok && la.Value == lb.Value
	case *parser.BoolLit:
		lb, ok := b.(*parser.BoolLit)
		return ok && la.Value == lb.Value
	}
	return false
}

// ---------------------------------------------------------------------------
// Pattern classification helpers
// ---------------------------------------------------------------------------

// isCatchAll returns true for wildcard, binding, and struct patterns.
// Struct patterns are catch-alls because structs have only one form.
func (c *exhaustivenessChecker) isCatchAll(pat parser.Pattern) bool {
	switch pat.(type) {
	case *parser.WildcardPat:
		return true
	case *parser.BindingPat:
		return true
	case *parser.StructPat:
		return true
	}
	return false
}

func (c *exhaustivenessChecker) isLiteralPattern(pat parser.Pattern) bool {
	_, ok := pat.(*parser.LiteralPat)
	return ok
}

// collectVariantPatterns extracts variant pattern names from the pattern list.
func (c *exhaustivenessChecker) collectVariantPatterns(patterns []patternInfo) map[string][]patternInfo {
	result := make(map[string][]patternInfo)
	for _, p := range patterns {
		c.addVariantPatterns(p, p.pattern, result)
	}
	return result
}

func (c *exhaustivenessChecker) addVariantPatterns(pi patternInfo, pat parser.Pattern, result map[string][]patternInfo) {
	switch p := pat.(type) {
	case *parser.VariantPat:
		result[p.Name] = append(result[p.Name], pi)
	case *parser.OrPat:
		for _, alt := range p.Alts {
			c.addVariantPatterns(pi, alt, result)
		}
	}
}

// collectTuplePatterns extracts tuple patterns from the pattern list.
func (c *exhaustivenessChecker) collectTuplePatterns(patterns []patternInfo) []*parser.TuplePat {
	var result []*parser.TuplePat
	for _, p := range patterns {
		if tp, ok := p.pattern.(*parser.TuplePat); ok {
			result = append(result, tp)
		}
	}
	return result
}

// findTypeDefForVariant finds the TypeDef that contains a variant with the given name.
func (c *exhaustivenessChecker) findTypeDefForVariant(variantName string) *parser.TypeDef {
	for _, td := range c.typeDefs {
		for _, v := range td.Variants {
			if v.Name == variantName {
				return td
			}
		}
	}
	return nil
}
