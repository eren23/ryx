package types

import (
	"fmt"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
)

// ---------------------------------------------------------------------------
// Constraints
// ---------------------------------------------------------------------------

// ConstraintKind classifies a type constraint.
type ConstraintKind int

const (
	EqualityConstraint ConstraintKind = iota // T1 = T2
	HasTraitConstraint                       // T : Trait
)

// Constraint represents a type constraint generated during inference.
type Constraint struct {
	Kind  ConstraintKind
	Left  Type   // for Equality: first type; for HasTrait: the type
	Right Type   // for Equality: second type; unused for HasTrait
	Trait string // for HasTrait: the trait name
	Span  diagnostic.Span
}

// ---------------------------------------------------------------------------
// Union-Find data structure for unification
// ---------------------------------------------------------------------------

// UnionFind implements a union-find structure for type variable unification
// with path compression and union by rank.
type UnionFind struct {
	parent map[int]int  // type var ID -> parent ID
	rank   map[int]int  // type var ID -> rank
	types  map[int]Type // type var ID -> resolved concrete type (if any)
}

// NewUnionFind creates a new empty union-find structure.
func NewUnionFind() *UnionFind {
	return &UnionFind{
		parent: make(map[int]int),
		rank:   make(map[int]int),
		types:  make(map[int]Type),
	}
}

// MakeSet ensures a type variable is tracked.
func (uf *UnionFind) MakeSet(id int) {
	if _, ok := uf.parent[id]; !ok {
		uf.parent[id] = id
		uf.rank[id] = 0
	}
}

// Find returns the representative of the set containing id, with path compression.
func (uf *UnionFind) Find(id int) int {
	uf.MakeSet(id)
	if uf.parent[id] != id {
		uf.parent[id] = uf.Find(uf.parent[id])
	}
	return uf.parent[id]
}

// Union merges the sets containing a and b, using union by rank.
func (uf *UnionFind) Union(a, b int) {
	ra, rb := uf.Find(a), uf.Find(b)
	if ra == rb {
		return
	}
	// Prefer to keep the root that already has a concrete type binding.
	_, raHasType := uf.types[ra]
	_, rbHasType := uf.types[rb]
	if rbHasType && !raHasType {
		ra, rb = rb, ra
	}

	if uf.rank[ra] < uf.rank[rb] {
		ra, rb = rb, ra
	}
	uf.parent[rb] = ra
	if uf.rank[ra] == uf.rank[rb] {
		uf.rank[ra]++
	}

	// Merge type bindings: if rb had a type, propagate it to ra.
	if t, ok := uf.types[rb]; ok {
		if _, ok2 := uf.types[ra]; !ok2 {
			uf.types[ra] = t
		}
	}
}

// BindType assigns a concrete type to a type variable's representative.
func (uf *UnionFind) BindType(id int, t Type) {
	rep := uf.Find(id)
	uf.types[rep] = t
}

// ResolveVar returns the concrete type bound to a type variable, or nil.
func (uf *UnionFind) ResolveVar(id int) Type {
	rep := uf.Find(id)
	return uf.types[rep]
}

// Resolve fully resolves a type through the union-find, replacing type
// variables with their bound types.
func (uf *UnionFind) Resolve(t Type) Type {
	switch ty := t.(type) {
	case *TypeVar:
		rep := uf.Find(ty.ID)
		if concrete, ok := uf.types[rep]; ok {
			return uf.Resolve(concrete)
		}
		// Return the representative TypeVar.
		return &TypeVar{ID: rep}
	case *ArrayType:
		return &ArrayType{Elem: uf.Resolve(ty.Elem)}
	case *SliceType:
		return &SliceType{Elem: uf.Resolve(ty.Elem)}
	case *TupleType:
		elems := make([]Type, len(ty.Elems))
		for i, e := range ty.Elems {
			elems[i] = uf.Resolve(e)
		}
		return &TupleType{Elems: elems}
	case *FnType:
		params := make([]Type, len(ty.Params))
		for i, p := range ty.Params {
			params[i] = uf.Resolve(p)
		}
		return &FnType{Params: params, Return: uf.Resolve(ty.Return)}
	case *StructType:
		genArgs := make([]Type, len(ty.GenArgs))
		for i, a := range ty.GenArgs {
			genArgs[i] = uf.Resolve(a)
		}
		fields := make(map[string]Type, len(ty.Fields))
		for k, v := range ty.Fields {
			fields[k] = uf.Resolve(v)
		}
		return &StructType{Name: ty.Name, GenArgs: genArgs, Fields: fields, FieldOrder: ty.FieldOrder}
	case *EnumType:
		genArgs := make([]Type, len(ty.GenArgs))
		for i, a := range ty.GenArgs {
			genArgs[i] = uf.Resolve(a)
		}
		variants := make(map[string][]Type, len(ty.Variants))
		for k, fs := range ty.Variants {
			nfs := make([]Type, len(fs))
			for i, f := range fs {
				nfs[i] = uf.Resolve(f)
			}
			variants[k] = nfs
		}
		return &EnumType{Name: ty.Name, GenArgs: genArgs, Variants: variants}
	case *ChannelType:
		return &ChannelType{Elem: uf.Resolve(ty.Elem)}
	default:
		return t
	}
}

// ---------------------------------------------------------------------------
// Unification
// ---------------------------------------------------------------------------

// InferenceEngine manages type inference state.
type InferenceEngine struct {
	UF          *UnionFind
	Constraints []Constraint
	Collector   *diagnostic.Collector
	// TraitConstraints accumulated during inference, checked later.
	TraitConstraints []Constraint
}

// NewInferenceEngine creates a new inference engine.
func NewInferenceEngine(collector *diagnostic.Collector) *InferenceEngine {
	return &InferenceEngine{
		UF:        NewUnionFind(),
		Collector: collector,
	}
}

// AddConstraint records an equality constraint.
func (ie *InferenceEngine) AddConstraint(left, right Type, span diagnostic.Span) {
	ie.Constraints = append(ie.Constraints, Constraint{
		Kind: EqualityConstraint,
		Left: left,
		Right: right,
		Span:  span,
	})
}

// AddTraitConstraint records a trait constraint.
func (ie *InferenceEngine) AddTraitConstraint(t Type, traitName string, span diagnostic.Span) {
	ie.TraitConstraints = append(ie.TraitConstraints, Constraint{
		Kind:  HasTraitConstraint,
		Left:  t,
		Trait: traitName,
		Span:  span,
	})
}

// RegisterTypeVar ensures a type variable is tracked in the union-find.
func (ie *InferenceEngine) RegisterTypeVar(tv *TypeVar) {
	ie.UF.MakeSet(tv.ID)
}

// Solve processes all accumulated equality constraints via unification.
func (ie *InferenceEngine) Solve() {
	for _, c := range ie.Constraints {
		if c.Kind == EqualityConstraint {
			ie.Unify(c.Left, c.Right, c.Span)
		}
	}
}

// Unify attempts to unify two types, updating the union-find structure.
func (ie *InferenceEngine) Unify(a, b Type, span diagnostic.Span) {
	a = ie.UF.Resolve(a)
	b = ie.UF.Resolve(b)

	// Same type — nothing to do.
	if a.Equal(b) {
		return
	}

	switch at := a.(type) {
	case *TypeVar:
		if bt, ok := b.(*TypeVar); ok {
			// Both are type vars: union them.
			ie.UF.Union(at.ID, bt.ID)
			return
		}
		// Occurs check: ensure at doesn't appear in b.
		if ie.occursIn(at.ID, b) {
			ie.Collector.Error("E100",
				fmt.Sprintf("infinite type: %s occurs in %s", at.String(), b.String()),
				span)
			return
		}
		ie.UF.BindType(at.ID, b)
		return

	case *IntType, *FloatType, *BoolType, *CharType, *StringType, *UnitType:
		if bt, ok := b.(*TypeVar); ok {
			ie.Unify(b, a, span)
			_ = bt
			return
		}
	}

	// b is a type var but a is not.
	if bt, ok := b.(*TypeVar); ok {
		ie.Unify(b, a, span)
		_ = bt
		return
	}

	// Structural unification.
	switch at := a.(type) {
	case *ArrayType:
		if bt, ok := b.(*ArrayType); ok {
			ie.Unify(at.Elem, bt.Elem, span)
			return
		}
	case *SliceType:
		if bt, ok := b.(*SliceType); ok {
			ie.Unify(at.Elem, bt.Elem, span)
			return
		}
	case *TupleType:
		if bt, ok := b.(*TupleType); ok {
			if len(at.Elems) == len(bt.Elems) {
				for i := range at.Elems {
					ie.Unify(at.Elems[i], bt.Elems[i], span)
				}
				return
			}
		}
	case *FnType:
		if bt, ok := b.(*FnType); ok {
			if len(at.Params) == len(bt.Params) {
				for i := range at.Params {
					ie.Unify(at.Params[i], bt.Params[i], span)
				}
				ie.Unify(at.Return, bt.Return, span)
				return
			}
			ie.Collector.Error("E101",
				fmt.Sprintf("function arity mismatch: expected %d parameters, found %d",
					len(at.Params), len(bt.Params)),
				span)
			return
		}
	case *StructType:
		if bt, ok := b.(*StructType); ok {
			if at.Name == bt.Name && len(at.GenArgs) == len(bt.GenArgs) {
				for i := range at.GenArgs {
					ie.Unify(at.GenArgs[i], bt.GenArgs[i], span)
				}
				return
			}
		}
	case *EnumType:
		if bt, ok := b.(*EnumType); ok {
			if at.Name == bt.Name && len(at.GenArgs) == len(bt.GenArgs) {
				for i := range at.GenArgs {
					ie.Unify(at.GenArgs[i], bt.GenArgs[i], span)
				}
				return
			}
		}
	case *ChannelType:
		if bt, ok := b.(*ChannelType); ok {
			ie.Unify(at.Elem, bt.Elem, span)
			return
		}
	}

	ie.Collector.Error("E102",
		fmt.Sprintf("type mismatch: expected `%s`, found `%s`", a.String(), b.String()),
		span)
}

// occursIn performs the occurs check: returns true if type variable id
// appears anywhere in type t.
func (ie *InferenceEngine) occursIn(id int, t Type) bool {
	t = ie.UF.Resolve(t)
	switch ty := t.(type) {
	case *TypeVar:
		return ie.UF.Find(ty.ID) == ie.UF.Find(id)
	case *ArrayType:
		return ie.occursIn(id, ty.Elem)
	case *SliceType:
		return ie.occursIn(id, ty.Elem)
	case *TupleType:
		for _, e := range ty.Elems {
			if ie.occursIn(id, e) {
				return true
			}
		}
	case *FnType:
		for _, p := range ty.Params {
			if ie.occursIn(id, p) {
				return true
			}
		}
		return ie.occursIn(id, ty.Return)
	case *StructType:
		for _, a := range ty.GenArgs {
			if ie.occursIn(id, a) {
				return true
			}
		}
	case *EnumType:
		for _, a := range ty.GenArgs {
			if ie.occursIn(id, a) {
				return true
			}
		}
	case *ChannelType:
		return ie.occursIn(id, ty.Elem)
	}
	return false
}

// ResolveType fully resolves a type through the union-find.
func (ie *InferenceEngine) ResolveType(t Type) Type {
	return ie.UF.Resolve(t)
}

// ---------------------------------------------------------------------------
// Generalization and Instantiation
// ---------------------------------------------------------------------------

// Generalize creates a type scheme by quantifying all type variables in t
// that do not appear free in the environment. envFreeVars is the set of
// type variable IDs that are free in the enclosing environment.
func (ie *InferenceEngine) Generalize(t Type, envFreeVars map[int]bool) *TypeScheme {
	resolved := ie.UF.Resolve(t)
	fv := FreeVars(resolved)

	var quantified []int
	for id := range fv {
		if !envFreeVars[id] {
			quantified = append(quantified, id)
		}
	}

	// Collect trait constraints that apply to quantified vars.
	quantifiedSet := make(map[int]bool, len(quantified))
	for _, id := range quantified {
		quantifiedSet[id] = true
	}

	var bounds []TraitBound
	for _, tc := range ie.TraitConstraints {
		resolved := ie.UF.Resolve(tc.Left)
		if tv, ok := resolved.(*TypeVar); ok {
			rep := ie.UF.Find(tv.ID)
			if quantifiedSet[rep] {
				bounds = append(bounds, TraitBound{
					TraitName: tc.Trait,
					TypeVar:   rep,
				})
			}
		}
	}

	return &TypeScheme{
		Vars:        quantified,
		Constraints: bounds,
		Body:        resolved,
	}
}

// Instantiate creates a fresh instance of a type scheme, returning the
// instantiated type and a mapping from old to new type variables.
func (ie *InferenceEngine) Instantiate(scheme *TypeScheme) (Type, map[int]*TypeVar) {
	if len(scheme.Vars) == 0 {
		return scheme.Body, nil
	}

	subst := make(map[int]*TypeVar, len(scheme.Vars))
	for _, v := range scheme.Vars {
		fresh := FreshTypeVar()
		ie.RegisterTypeVar(fresh)
		subst[v] = fresh
	}

	instantiated := applySubst(scheme.Body, subst)

	// Re-emit trait constraints for the fresh variables.
	for _, bound := range scheme.Constraints {
		if newTV, ok := subst[bound.TypeVar]; ok {
			ie.TraitConstraints = append(ie.TraitConstraints, Constraint{
				Kind:  HasTraitConstraint,
				Left:  newTV,
				Trait: bound.TraitName,
			})
		}
	}

	return instantiated, subst
}

// EnvFreeVars collects free type variables from a type environment.
func EnvFreeVars(env map[string]*TypeScheme) map[int]bool {
	fv := make(map[int]bool)
	for _, scheme := range env {
		bodyFV := FreeVars(scheme.Body)
		// Subtract quantified vars.
		quantified := make(map[int]bool, len(scheme.Vars))
		for _, v := range scheme.Vars {
			quantified[v] = true
		}
		for id := range bodyFV {
			if !quantified[id] {
				fv[id] = true
			}
		}
	}
	return fv
}
