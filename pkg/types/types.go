package types

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// Type interface
// ---------------------------------------------------------------------------

// Type represents a Ryx type in the type system.
type Type interface {
	String() string
	typeTag()
	// Equal returns true if two types are structurally equal.
	Equal(other Type) bool
}

type typeBase struct{}

func (typeBase) typeTag() {}

// ---------------------------------------------------------------------------
// Primitive types (singletons)
// ---------------------------------------------------------------------------

type IntType struct{ typeBase }
type FloatType struct{ typeBase }
type BoolType struct{ typeBase }
type CharType struct{ typeBase }
type StringType struct{ typeBase }
type UnitType struct{ typeBase }

func (*IntType) String() string    { return "Int" }
func (*FloatType) String() string  { return "Float" }
func (*BoolType) String() string   { return "Bool" }
func (*CharType) String() string   { return "Char" }
func (*StringType) String() string { return "String" }
func (*UnitType) String() string   { return "Unit" }

func (*IntType) Equal(other Type) bool    { _, ok := other.(*IntType); return ok }
func (*FloatType) Equal(other Type) bool  { _, ok := other.(*FloatType); return ok }
func (*BoolType) Equal(other Type) bool   { _, ok := other.(*BoolType); return ok }
func (*CharType) Equal(other Type) bool   { _, ok := other.(*CharType); return ok }
func (*StringType) Equal(other Type) bool { _, ok := other.(*StringType); return ok }
func (*UnitType) Equal(other Type) bool   { _, ok := other.(*UnitType); return ok }

// Singletons for primitive types.
var (
	TypInt    = &IntType{}
	TypFloat  = &FloatType{}
	TypBool   = &BoolType{}
	TypChar   = &CharType{}
	TypString = &StringType{}
	TypUnit   = &UnitType{}
)

// ---------------------------------------------------------------------------
// Composite types
// ---------------------------------------------------------------------------

// ArrayType represents a fixed-size array type [T].
type ArrayType struct {
	typeBase
	Elem Type
}

func (t *ArrayType) String() string { return "[" + t.Elem.String() + "]" }
func (t *ArrayType) Equal(other Type) bool {
	o, ok := other.(*ArrayType)
	return ok && t.Elem.Equal(o.Elem)
}

// SliceType represents a dynamically-sized slice type.
type SliceType struct {
	typeBase
	Elem Type
}

func (t *SliceType) String() string { return "[]" + t.Elem.String() }
func (t *SliceType) Equal(other Type) bool {
	o, ok := other.(*SliceType)
	return ok && t.Elem.Equal(o.Elem)
}

// TupleType represents a tuple type (T1, T2, ...).
type TupleType struct {
	typeBase
	Elems []Type
}

func (t *TupleType) String() string {
	parts := make([]string, len(t.Elems))
	for i, e := range t.Elems {
		parts[i] = e.String()
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func (t *TupleType) Equal(other Type) bool {
	o, ok := other.(*TupleType)
	if !ok || len(t.Elems) != len(o.Elems) {
		return false
	}
	for i := range t.Elems {
		if !t.Elems[i].Equal(o.Elems[i]) {
			return false
		}
	}
	return true
}

// FnType represents a function type (P1, P2, ...) -> R.
type FnType struct {
	typeBase
	Params []Type
	Return Type
}

func (t *FnType) String() string {
	parts := make([]string, len(t.Params))
	for i, p := range t.Params {
		parts[i] = p.String()
	}
	return "(" + strings.Join(parts, ", ") + ") -> " + t.Return.String()
}

func (t *FnType) Equal(other Type) bool {
	o, ok := other.(*FnType)
	if !ok || len(t.Params) != len(o.Params) {
		return false
	}
	for i := range t.Params {
		if !t.Params[i].Equal(o.Params[i]) {
			return false
		}
	}
	return t.Return.Equal(o.Return)
}

// StructType represents a struct type.
type StructType struct {
	typeBase
	Name    string
	GenArgs []Type
	Fields  map[string]Type
	// FieldOrder preserves declaration order for iteration.
	FieldOrder []string
}

func (t *StructType) String() string {
	if len(t.GenArgs) == 0 {
		return t.Name
	}
	parts := make([]string, len(t.GenArgs))
	for i, a := range t.GenArgs {
		parts[i] = a.String()
	}
	return t.Name + "<" + strings.Join(parts, ", ") + ">"
}

func (t *StructType) Equal(other Type) bool {
	o, ok := other.(*StructType)
	if !ok || t.Name != o.Name || len(t.GenArgs) != len(o.GenArgs) {
		return false
	}
	for i := range t.GenArgs {
		if !t.GenArgs[i].Equal(o.GenArgs[i]) {
			return false
		}
	}
	return true
}

// EnumType represents an algebraic data type (enum/sum type).
type EnumType struct {
	typeBase
	Name     string
	GenArgs  []Type
	Variants map[string][]Type // variant name -> field types
}

func (t *EnumType) String() string {
	if len(t.GenArgs) == 0 {
		return t.Name
	}
	parts := make([]string, len(t.GenArgs))
	for i, a := range t.GenArgs {
		parts[i] = a.String()
	}
	return t.Name + "<" + strings.Join(parts, ", ") + ">"
}

func (t *EnumType) Equal(other Type) bool {
	o, ok := other.(*EnumType)
	if !ok || t.Name != o.Name || len(t.GenArgs) != len(o.GenArgs) {
		return false
	}
	for i := range t.GenArgs {
		if !t.GenArgs[i].Equal(o.GenArgs[i]) {
			return false
		}
	}
	return true
}

// ChannelType represents a channel<T> type.
type ChannelType struct {
	typeBase
	Elem Type
}

func (t *ChannelType) String() string { return "channel<" + t.Elem.String() + ">" }
func (t *ChannelType) Equal(other Type) bool {
	o, ok := other.(*ChannelType)
	return ok && t.Elem.Equal(o.Elem)
}

// ---------------------------------------------------------------------------
// Type variables (for inference)
// ---------------------------------------------------------------------------

var nextTypeVarID atomic.Int64

// TypeVar represents a unification variable during type inference.
type TypeVar struct {
	typeBase
	ID int
}

func (t *TypeVar) String() string { return fmt.Sprintf("?T%d", t.ID) }
func (t *TypeVar) Equal(other Type) bool {
	o, ok := other.(*TypeVar)
	return ok && t.ID == o.ID
}

// FreshTypeVar allocates a new unique type variable.
func FreshTypeVar() *TypeVar {
	id := int(nextTypeVarID.Add(1))
	return &TypeVar{ID: id}
}

// ResetTypeVarCounter resets the counter (for testing determinism).
func ResetTypeVarCounter() {
	nextTypeVarID.Store(0)
}

// ---------------------------------------------------------------------------
// Type scheme (polymorphic types)
// ---------------------------------------------------------------------------

// TraitBound represents a trait constraint on a type variable.
type TraitBound struct {
	TraitName string
	TypeVar   int // the type variable ID this bound applies to
}

// TypeScheme represents a polymorphic type: forall a1, a2, ... . T
// where the type variables in Vars are universally quantified.
type TypeScheme struct {
	Vars        []int        // quantified type variable IDs
	Constraints []TraitBound // trait bounds on the quantified vars
	Body        Type         // the underlying type
}

func (ts *TypeScheme) String() string {
	if len(ts.Vars) == 0 {
		return ts.Body.String()
	}
	vars := make([]string, len(ts.Vars))
	for i, v := range ts.Vars {
		vars[i] = fmt.Sprintf("?T%d", v)
	}
	s := "forall " + strings.Join(vars, ", ")
	if len(ts.Constraints) > 0 {
		cs := make([]string, len(ts.Constraints))
		for i, c := range ts.Constraints {
			cs[i] = fmt.Sprintf("%s: %s", fmt.Sprintf("?T%d", c.TypeVar), c.TraitName)
		}
		s += " where " + strings.Join(cs, ", ")
	}
	return s + " . " + ts.Body.String()
}

// Instantiate creates a fresh copy of the type scheme's body, replacing
// each quantified variable with a fresh type variable. Returns the
// instantiated type and a mapping from old var IDs to new type vars.
func (ts *TypeScheme) Instantiate() (Type, map[int]*TypeVar) {
	subst := make(map[int]*TypeVar, len(ts.Vars))
	for _, v := range ts.Vars {
		subst[v] = FreshTypeVar()
	}
	return applySubst(ts.Body, subst), subst
}

// MonoScheme wraps a monomorphic type as a trivial type scheme.
func MonoScheme(t Type) *TypeScheme {
	return &TypeScheme{Body: t}
}

// ---------------------------------------------------------------------------
// Type substitution helper
// ---------------------------------------------------------------------------

// applySubst replaces type variables in t according to the substitution map.
func applySubst(t Type, subst map[int]*TypeVar) Type {
	switch ty := t.(type) {
	case *TypeVar:
		if replacement, ok := subst[ty.ID]; ok {
			return replacement
		}
		return ty
	case *ArrayType:
		return &ArrayType{Elem: applySubst(ty.Elem, subst)}
	case *SliceType:
		return &SliceType{Elem: applySubst(ty.Elem, subst)}
	case *TupleType:
		elems := make([]Type, len(ty.Elems))
		for i, e := range ty.Elems {
			elems[i] = applySubst(e, subst)
		}
		return &TupleType{Elems: elems}
	case *FnType:
		params := make([]Type, len(ty.Params))
		for i, p := range ty.Params {
			params[i] = applySubst(p, subst)
		}
		return &FnType{Params: params, Return: applySubst(ty.Return, subst)}
	case *StructType:
		genArgs := make([]Type, len(ty.GenArgs))
		for i, a := range ty.GenArgs {
			genArgs[i] = applySubst(a, subst)
		}
		fields := make(map[string]Type, len(ty.Fields))
		for k, v := range ty.Fields {
			fields[k] = applySubst(v, subst)
		}
		return &StructType{Name: ty.Name, GenArgs: genArgs, Fields: fields, FieldOrder: ty.FieldOrder}
	case *EnumType:
		genArgs := make([]Type, len(ty.GenArgs))
		for i, a := range ty.GenArgs {
			genArgs[i] = applySubst(a, subst)
		}
		variants := make(map[string][]Type, len(ty.Variants))
		for k, fs := range ty.Variants {
			nfs := make([]Type, len(fs))
			for i, f := range fs {
				nfs[i] = applySubst(f, subst)
			}
			variants[k] = nfs
		}
		return &EnumType{Name: ty.Name, GenArgs: genArgs, Variants: variants}
	case *ChannelType:
		return &ChannelType{Elem: applySubst(ty.Elem, subst)}
	default:
		return t // primitives
	}
}

// ---------------------------------------------------------------------------
// Helper: resolve a named type string to its Type
// ---------------------------------------------------------------------------

// BuiltinType returns the Type for a built-in type name, or nil.
func BuiltinType(name string) Type {
	switch name {
	case "Int":
		return TypInt
	case "Float":
		return TypFloat
	case "Bool":
		return TypBool
	case "Char":
		return TypChar
	case "String":
		return TypString
	case "Unit":
		return TypUnit
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// FreeVars collects all free type variable IDs in a type.
// ---------------------------------------------------------------------------

func FreeVars(t Type) map[int]bool {
	fv := make(map[int]bool)
	freeVarsHelper(t, fv)
	return fv
}

func freeVarsHelper(t Type, fv map[int]bool) {
	switch ty := t.(type) {
	case *TypeVar:
		fv[ty.ID] = true
	case *ArrayType:
		freeVarsHelper(ty.Elem, fv)
	case *SliceType:
		freeVarsHelper(ty.Elem, fv)
	case *TupleType:
		for _, e := range ty.Elems {
			freeVarsHelper(e, fv)
		}
	case *FnType:
		for _, p := range ty.Params {
			freeVarsHelper(p, fv)
		}
		freeVarsHelper(ty.Return, fv)
	case *StructType:
		for _, a := range ty.GenArgs {
			freeVarsHelper(a, fv)
		}
		for _, f := range ty.Fields {
			freeVarsHelper(f, fv)
		}
	case *EnumType:
		for _, a := range ty.GenArgs {
			freeVarsHelper(a, fv)
		}
		for _, fs := range ty.Variants {
			for _, f := range fs {
				freeVarsHelper(f, fv)
			}
		}
	case *ChannelType:
		freeVarsHelper(ty.Elem, fv)
	}
}

// Zonk replaces all type variables with their resolved types using a resolver function.
// This is used to fully resolve a type after inference is complete.
func Zonk(t Type, resolve func(*TypeVar) Type) Type {
	switch ty := t.(type) {
	case *TypeVar:
		resolved := resolve(ty)
		if resolved != ty {
			return Zonk(resolved, resolve)
		}
		return ty
	case *ArrayType:
		return &ArrayType{Elem: Zonk(ty.Elem, resolve)}
	case *SliceType:
		return &SliceType{Elem: Zonk(ty.Elem, resolve)}
	case *TupleType:
		elems := make([]Type, len(ty.Elems))
		for i, e := range ty.Elems {
			elems[i] = Zonk(e, resolve)
		}
		return &TupleType{Elems: elems}
	case *FnType:
		params := make([]Type, len(ty.Params))
		for i, p := range ty.Params {
			params[i] = Zonk(p, resolve)
		}
		return &FnType{Params: params, Return: Zonk(ty.Return, resolve)}
	case *StructType:
		genArgs := make([]Type, len(ty.GenArgs))
		for i, a := range ty.GenArgs {
			genArgs[i] = Zonk(a, resolve)
		}
		fields := make(map[string]Type, len(ty.Fields))
		for k, v := range ty.Fields {
			fields[k] = Zonk(v, resolve)
		}
		return &StructType{Name: ty.Name, GenArgs: genArgs, Fields: fields, FieldOrder: ty.FieldOrder}
	case *EnumType:
		genArgs := make([]Type, len(ty.GenArgs))
		for i, a := range ty.GenArgs {
			genArgs[i] = Zonk(a, resolve)
		}
		variants := make(map[string][]Type, len(ty.Variants))
		for k, fs := range ty.Variants {
			nfs := make([]Type, len(fs))
			for i, f := range fs {
				nfs[i] = Zonk(f, resolve)
			}
			variants[k] = nfs
		}
		return &EnumType{Name: ty.Name, GenArgs: genArgs, Variants: variants}
	case *ChannelType:
		return &ChannelType{Elem: Zonk(ty.Elem, resolve)}
	default:
		return t
	}
}
