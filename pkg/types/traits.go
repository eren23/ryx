package types

import (
	"fmt"
	"strings"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
)

// ---------------------------------------------------------------------------
// Trait definition (type-level representation)
// ---------------------------------------------------------------------------

// TraitMethod describes a method signature in a trait definition.
type TraitMethod struct {
	Name       string
	ParamTypes []Type // does not include self
	ReturnType Type
}

// TraitInfo describes a trait definition in the type system.
type TraitInfo struct {
	Name       string
	GenParams  []int // type param IDs for generic traits
	Methods    []TraitMethod
	MethodMap  map[string]*TraitMethod
}

// ---------------------------------------------------------------------------
// Impl entries
// ---------------------------------------------------------------------------

// ImplEntry represents a concrete trait implementation: impl Trait for Type.
type ImplEntry struct {
	TraitName  string
	TargetType Type   // the concrete (possibly generic) target type
	GenParams  []int  // type variable IDs for generic impls
	Methods    map[string]*FnType
	Span       diagnostic.Span
}

// ImplKey identifies a specific impl for coherence checking.
type ImplKey struct {
	TraitName  string
	TypeName   string // concrete type name (e.g., "Int", "MyStruct")
}

// ---------------------------------------------------------------------------
// Trait Registry
// ---------------------------------------------------------------------------

// TraitRegistry manages trait definitions and implementations, enforcing
// coherence (at most one impl per trait+type pair) and the orphan rule.
type TraitRegistry struct {
	Traits map[string]*TraitInfo
	Impls  []ImplEntry
	// implIndex maps (trait, type_name) to impl for coherence checking.
	implIndex map[ImplKey]*ImplEntry
	Collector *diagnostic.Collector

	// LocalTraits tracks trait names defined in the current crate/module.
	LocalTraits map[string]bool
	// LocalTypes tracks type names defined in the current crate/module.
	LocalTypes map[string]bool
}

// NewTraitRegistry creates a new trait registry.
func NewTraitRegistry(collector *diagnostic.Collector) *TraitRegistry {
	return &TraitRegistry{
		Traits:      make(map[string]*TraitInfo),
		implIndex:   make(map[ImplKey]*ImplEntry),
		Collector:   collector,
		LocalTraits: make(map[string]bool),
		LocalTypes:  make(map[string]bool),
	}
}

// RegisterTrait adds a trait definition to the registry.
func (tr *TraitRegistry) RegisterTrait(name string, methods []TraitMethod) {
	methodMap := make(map[string]*TraitMethod, len(methods))
	for i := range methods {
		methodMap[methods[i].Name] = &methods[i]
	}
	tr.Traits[name] = &TraitInfo{
		Name:      name,
		Methods:   methods,
		MethodMap: methodMap,
	}
	tr.LocalTraits[name] = true
}

// RegisterImpl registers a trait implementation, checking coherence and orphan rules.
func (tr *TraitRegistry) RegisterImpl(entry ImplEntry) bool {
	typeName := typeNameOf(entry.TargetType)

	// Check coherence: at most one impl per (trait, type) pair.
	key := ImplKey{TraitName: entry.TraitName, TypeName: typeName}
	if existing, ok := tr.implIndex[key]; ok {
		tr.Collector.Error("E110",
			fmt.Sprintf("conflicting implementations of trait `%s` for type `%s`",
				entry.TraitName, typeName),
			entry.Span)
		tr.Collector.Add(diagnostic.Diagnostic{
			Severity: diagnostic.SeverityError,
			Code:     "E110",
			Message:  "first implementation here",
			Span:     existing.Span,
		})
		return false
	}

	// Check orphan rule: at least one of trait or type must be local.
	if !tr.LocalTraits[entry.TraitName] && !tr.LocalTypes[typeName] {
		tr.Collector.Error("E111",
			fmt.Sprintf("orphan impl: neither trait `%s` nor type `%s` is defined in this module",
				entry.TraitName, typeName),
			entry.Span)
		return false
	}

	tr.implIndex[key] = &entry
	tr.Impls = append(tr.Impls, entry)
	return true
}

// LookupImpl finds a concrete impl for the given trait and target type.
// For generic impls, it attempts to match by unifying the target type
// with the impl's generic pattern.
func (tr *TraitRegistry) LookupImpl(traitName string, targetType Type, ie *InferenceEngine) *ImplEntry {
	typeName := typeNameOf(targetType)

	// Direct lookup first.
	key := ImplKey{TraitName: traitName, TypeName: typeName}
	if entry, ok := tr.implIndex[key]; ok {
		return entry
	}

	// Try generic impl matching: find any impl for this trait and
	// attempt unification with the target type.
	for i := range tr.Impls {
		impl := &tr.Impls[i]
		if impl.TraitName != traitName {
			continue
		}
		if len(impl.GenParams) == 0 {
			continue
		}

		// Create fresh type variables for the generic params.
		matched := tr.matchGenericImpl(impl, targetType, ie)
		if matched {
			return impl
		}
	}

	return nil
}

// matchGenericImpl attempts to unify a generic impl's target type with a
// concrete type to determine if the impl matches.
func (tr *TraitRegistry) matchGenericImpl(impl *ImplEntry, targetType Type, ie *InferenceEngine) bool {
	// Create a temporary inference engine to try matching without polluting state.
	tempCollector := diagnostic.NewCollector(nil, 100, 100)
	tempIE := NewInferenceEngine(tempCollector)

	// Create fresh vars for the generic params.
	subst := make(map[int]*TypeVar, len(impl.GenParams))
	for _, id := range impl.GenParams {
		fresh := FreshTypeVar()
		tempIE.RegisterTypeVar(fresh)
		subst[id] = fresh
	}

	// Apply substitution to get the impl's target with fresh vars.
	implTarget := applySubst(impl.TargetType, subst)

	// Try to unify.
	tempIE.Unify(implTarget, targetType, diagnostic.Span{})

	// If no errors, the impl matches.
	return !tempCollector.HasErrors()
}

// HasImpl checks whether a trait is implemented for a given type.
func (tr *TraitRegistry) HasImpl(traitName string, targetType Type, ie *InferenceEngine) bool {
	return tr.LookupImpl(traitName, targetType, ie) != nil
}

// CheckTraitConstraints verifies all accumulated trait constraints against
// the registry. Called after solving equality constraints.
func (tr *TraitRegistry) CheckTraitConstraints(ie *InferenceEngine) {
	for _, c := range ie.TraitConstraints {
		resolved := ie.ResolveType(c.Left)

		// If the type is still a type variable, skip (polymorphic context).
		if _, ok := resolved.(*TypeVar); ok {
			continue
		}

		if !tr.HasImpl(c.Trait, resolved, ie) {
			ie.Collector.Error("E112",
				fmt.Sprintf("trait `%s` is not implemented for type `%s`",
					c.Trait, resolved.String()),
				c.Span)
		}
	}
}

// LookupMethod finds a method on a type, checking both inherent impls and
// trait impls.
func (tr *TraitRegistry) LookupMethod(targetType Type, methodName string, ie *InferenceEngine) (*FnType, string) {
	typeName := typeNameOf(targetType)

	// Check inherent impls first (trait name = "").
	key := ImplKey{TraitName: "", TypeName: typeName}
	if entry, ok := tr.implIndex[key]; ok {
		if ft, ok := entry.Methods[methodName]; ok {
			return ft, ""
		}
	}

	// Check trait impls.
	for i := range tr.Impls {
		impl := &tr.Impls[i]
		if impl.TraitName == "" {
			continue
		}
		implTypeName := typeNameOf(impl.TargetType)
		if implTypeName != typeName {
			continue
		}
		if ft, ok := impl.Methods[methodName]; ok {
			return ft, impl.TraitName
		}
	}

	return nil, ""
}

// ---------------------------------------------------------------------------
// Helper: extract type name for impl indexing
// ---------------------------------------------------------------------------

func typeNameOf(t Type) string {
	switch ty := t.(type) {
	case *IntType:
		return "Int"
	case *FloatType:
		return "Float"
	case *BoolType:
		return "Bool"
	case *CharType:
		return "Char"
	case *StringType:
		return "String"
	case *UnitType:
		return "Unit"
	case *StructType:
		return ty.Name
	case *EnumType:
		return ty.Name
	case *ArrayType:
		return "[" + typeNameOf(ty.Elem) + "]"
	case *SliceType:
		return "[]" + typeNameOf(ty.Elem)
	case *TupleType:
		return "(tuple)"
	case *FnType:
		return "(fn)"
	case *ChannelType:
		return "channel"
	case *TypeVar:
		return fmt.Sprintf("?T%d", ty.ID)
	default:
		return "<unknown>"
	}
}

// ---------------------------------------------------------------------------
// Built-in trait registrations
// ---------------------------------------------------------------------------

// RegisterBuiltinTraits registers standard library traits like Display, Debug,
// Add, Sub, etc. with default impls for primitive types.
func (tr *TraitRegistry) RegisterBuiltinTraits() {
	// Numeric traits.
	numericTraits := []string{"Add", "Sub", "Mul", "Div", "Rem"}
	for _, name := range numericTraits {
		tr.RegisterTrait(name, []TraitMethod{
			{Name: strings.ToLower(name[:1]) + name[1:], ParamTypes: []Type{}, ReturnType: nil},
		})
	}

	// Comparison traits.
	tr.RegisterTrait("Eq", []TraitMethod{
		{Name: "eq", ParamTypes: []Type{}, ReturnType: TypBool},
	})
	tr.RegisterTrait("Ord", []TraitMethod{
		{Name: "cmp", ParamTypes: []Type{}, ReturnType: TypInt},
	})

	// Display/Debug.
	tr.RegisterTrait("Display", []TraitMethod{
		{Name: "to_string", ParamTypes: []Type{}, ReturnType: TypString},
	})
	tr.RegisterTrait("Debug", []TraitMethod{
		{Name: "debug_string", ParamTypes: []Type{}, ReturnType: TypString},
	})

	// Register impls for primitive types.
	numericTypes := []Type{TypInt, TypFloat}
	for _, t := range numericTypes {
		for _, trait := range numericTraits {
			tr.implIndex[ImplKey{TraitName: trait, TypeName: typeNameOf(t)}] = &ImplEntry{
				TraitName:  trait,
				TargetType: t,
			}
		}
		tr.implIndex[ImplKey{TraitName: "Eq", TypeName: typeNameOf(t)}] = &ImplEntry{
			TraitName: "Eq", TargetType: t,
		}
		tr.implIndex[ImplKey{TraitName: "Ord", TypeName: typeNameOf(t)}] = &ImplEntry{
			TraitName: "Ord", TargetType: t,
		}
	}

	// Eq for Bool, Char, String.
	for _, t := range []Type{TypBool, TypChar, TypString} {
		tr.implIndex[ImplKey{TraitName: "Eq", TypeName: typeNameOf(t)}] = &ImplEntry{
			TraitName: "Eq", TargetType: t,
		}
	}

	// Ord for Char, String.
	for _, t := range []Type{TypChar, TypString} {
		tr.implIndex[ImplKey{TraitName: "Ord", TypeName: typeNameOf(t)}] = &ImplEntry{
			TraitName: "Ord", TargetType: t,
		}
	}

	// Display for all primitives.
	for _, t := range []Type{TypInt, TypFloat, TypBool, TypChar, TypString} {
		tr.implIndex[ImplKey{TraitName: "Display", TypeName: typeNameOf(t)}] = &ImplEntry{
			TraitName: "Display", TargetType: t,
		}
		tr.implIndex[ImplKey{TraitName: "Debug", TypeName: typeNameOf(t)}] = &ImplEntry{
			TraitName: "Debug", TargetType: t,
		}
	}

	// String concatenation via Concat trait.
	tr.RegisterTrait("Concat", []TraitMethod{
		{Name: "concat", ParamTypes: []Type{}, ReturnType: TypString},
	})
	tr.implIndex[ImplKey{TraitName: "Concat", TypeName: "String"}] = &ImplEntry{
		TraitName: "Concat", TargetType: TypString,
	}

	// Mark builtin traits/types as non-local for orphan rule purposes.
	// They're "builtin" but we treat them as local for simplicity.
}
