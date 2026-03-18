package vm

import (
	"fmt"
	"hash"
	"hash/fnv"
	"math"
	"strings"
)

// ---------------------------------------------------------------------------
// BuiltinFunc — the signature for all standard library functions
// ---------------------------------------------------------------------------

// BuiltinFunc is the type for native functions callable from Ryx code.
type BuiltinFunc func(args []Value, heap *Heap) (Value, error)

// ---------------------------------------------------------------------------
// BuiltinRegistry — name-indexed registry of stdlib functions
// ---------------------------------------------------------------------------

// BuiltinRegistry holds all registered built-in functions.
type BuiltinRegistry struct {
	funcs map[string]BuiltinFunc
}

// NewBuiltinRegistry creates an empty registry.
func NewBuiltinRegistry() *BuiltinRegistry {
	return &BuiltinRegistry{funcs: make(map[string]BuiltinFunc)}
}

// Register adds a builtin function.
func (r *BuiltinRegistry) Register(name string, fn BuiltinFunc) {
	r.funcs[name] = fn
}

// Lookup retrieves a builtin function by name.
func (r *BuiltinRegistry) Lookup(name string) (BuiltinFunc, bool) {
	fn, ok := r.funcs[name]
	return fn, ok
}

// Names returns all registered function names.
func (r *BuiltinRegistry) Names() []string {
	names := make([]string, 0, len(r.funcs))
	for name := range r.funcs {
		names = append(names, name)
	}
	return names
}

// Call invokes a builtin by name. Returns an error if the builtin is not found.
func (r *BuiltinRegistry) Call(name string, args []Value, heap *Heap) (Value, error) {
	fn, ok := r.funcs[name]
	if !ok {
		return UnitVal(), fmt.Errorf("unknown builtin function: %s", name)
	}
	return fn(args, heap)
}

// ---------------------------------------------------------------------------
// Built-in trait system — 6 auto-implemented traits
// ---------------------------------------------------------------------------

// TraitID identifies one of the built-in traits.
type TraitID int

const (
	TraitEq      TraitID = iota // ==, !=
	TraitOrd                    // <, >, <=, >=
	TraitDisplay                // to_string
	TraitDefault                // default value
	TraitClone                  // deep clone
	TraitHash                   // hash to uint64
)

func (t TraitID) String() string {
	switch t {
	case TraitEq:
		return "Eq"
	case TraitOrd:
		return "Ord"
	case TraitDisplay:
		return "Display"
	case TraitDefault:
		return "Default"
	case TraitClone:
		return "Clone"
	case TraitHash:
		return "Hash"
	}
	return "Unknown"
}

// ---------------------------------------------------------------------------
// Eq trait — structural equality
// ---------------------------------------------------------------------------

// BuiltinEq performs structural equality comparison.
// Auto-implemented for all primitives, arrays, tuples, structs, and enums.
func BuiltinEq(args []Value, heap *Heap) (Value, error) {
	if len(args) != 2 {
		return UnitVal(), fmt.Errorf("eq: expected 2 arguments, got %d", len(args))
	}
	return BoolVal(args[0].Equal(args[1], heap)), nil
}

// BuiltinNeq returns the logical negation of Eq.
func BuiltinNeq(args []Value, heap *Heap) (Value, error) {
	if len(args) != 2 {
		return UnitVal(), fmt.Errorf("neq: expected 2 arguments, got %d", len(args))
	}
	return BoolVal(!args[0].Equal(args[1], heap)), nil
}

// ---------------------------------------------------------------------------
// Ord trait — comparison returning -1, 0, or 1
// ---------------------------------------------------------------------------

// BuiltinCompare returns -1, 0, or 1 for ordering.
// Auto-implemented for Int, Float, Char, Bool, String.
// For arrays/tuples, compares lexicographically.
func BuiltinCompare(args []Value, heap *Heap) (Value, error) {
	if len(args) != 2 {
		return UnitVal(), fmt.Errorf("compare: expected 2 arguments, got %d", len(args))
	}
	result := deepCompare(args[0], args[1], heap)
	return IntVal(int64(result)), nil
}

func deepCompare(a, b Value, heap *Heap) int {
	// Primitives.
	if a.Tag == TagInt && b.Tag == TagInt {
		ai, bi := a.AsInt(), b.AsInt()
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
		return 0
	}
	if a.Tag == TagFloat && b.Tag == TagFloat {
		af, bf := a.AsFloat(), b.AsFloat()
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	}
	if a.Tag == TagChar && b.Tag == TagChar {
		ac, bc := a.AsChar(), b.AsChar()
		if ac < bc {
			return -1
		}
		if ac > bc {
			return 1
		}
		return 0
	}
	if a.Tag == TagBool && b.Tag == TagBool {
		ab, bb := a.Data, b.Data
		if ab < bb {
			return -1
		}
		if ab > bb {
			return 1
		}
		return 0
	}

	// Heap objects.
	if a.Tag == TagObj && b.Tag == TagObj {
		oa := heap.Get(a.AsObj())
		ob := heap.Get(b.AsObj())

		switch sa := oa.Data.(type) {
		case *StringObj:
			if sb, ok := ob.Data.(*StringObj); ok {
				return strings.Compare(sa.Value, sb.Value)
			}
		case *ArrayObj:
			if sb, ok := ob.Data.(*ArrayObj); ok {
				return compareSlices(sa.Elements, sb.Elements, heap)
			}
		case *TupleObj:
			if sb, ok := ob.Data.(*TupleObj); ok {
				return compareSlices(sa.Elements, sb.Elements, heap)
			}
		case *StructObj:
			if sb, ok := ob.Data.(*StructObj); ok {
				return compareSlices(sa.Fields, sb.Fields, heap)
			}
		case *EnumObj:
			if sb, ok := ob.Data.(*EnumObj); ok {
				if sa.TypeIdx != sb.TypeIdx {
					if sa.TypeIdx < sb.TypeIdx {
						return -1
					}
					return 1
				}
				if sa.VariantIdx != sb.VariantIdx {
					if sa.VariantIdx < sb.VariantIdx {
						return -1
					}
					return 1
				}
				return compareSlices(sa.Fields, sb.Fields, heap)
			}
		}
	}

	// Fallback: compare by tag then data.
	if a.Tag != b.Tag {
		if a.Tag < b.Tag {
			return -1
		}
		return 1
	}
	if a.Data < b.Data {
		return -1
	}
	if a.Data > b.Data {
		return 1
	}
	return 0
}

func compareSlices(a, b []Value, heap *Heap) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		c := deepCompare(a[i], b[i], heap)
		if c != 0 {
			return c
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// Display trait — to_string
// ---------------------------------------------------------------------------

// BuiltinToString converts any value to its string representation.
// Auto-implemented for all types.
func BuiltinToString(args []Value, heap *Heap) (Value, error) {
	if len(args) != 1 {
		return UnitVal(), fmt.Errorf("to_string: expected 1 argument, got %d", len(args))
	}
	s := displayValue(args[0], heap)
	idx := heap.AllocString(s)
	return ObjVal(idx), nil
}

func displayValue(v Value, heap *Heap) string {
	switch v.Tag {
	case TagInt:
		return fmt.Sprintf("%d", v.AsInt())
	case TagFloat:
		return fmt.Sprintf("%g", v.AsFloat())
	case TagBool:
		if v.AsBool() {
			return "true"
		}
		return "false"
	case TagChar:
		return fmt.Sprintf("'%c'", v.AsChar())
	case TagUnit:
		return "()"
	case TagFunc:
		return fmt.Sprintf("<func %d>", v.AsFunc())
	case TagObj:
		return displayObject(v, heap)
	default:
		return fmt.Sprintf("<unknown tag=%d>", v.Tag)
	}
}

func displayObject(v Value, heap *Heap) string {
	obj := heap.Get(v.AsObj())
	switch o := obj.Data.(type) {
	case *StringObj:
		return o.Value
	case *ArrayObj:
		parts := make([]string, len(o.Elements))
		for i, e := range o.Elements {
			parts[i] = displayValue(e, heap)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *TupleObj:
		parts := make([]string, len(o.Elements))
		for i, e := range o.Elements {
			parts[i] = displayValue(e, heap)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *StructObj:
		parts := make([]string, len(o.Fields))
		for i, f := range o.Fields {
			parts[i] = displayValue(f, heap)
		}
		return fmt.Sprintf("struct{%s}", strings.Join(parts, ", "))
	case *EnumObj:
		if len(o.Fields) == 0 {
			return fmt.Sprintf("Variant(%d)", o.VariantIdx)
		}
		parts := make([]string, len(o.Fields))
		for i, f := range o.Fields {
			parts[i] = displayValue(f, heap)
		}
		return fmt.Sprintf("Variant(%d, %s)", o.VariantIdx, strings.Join(parts, ", "))
	case *ClosureObj:
		return fmt.Sprintf("<closure func=%d>", o.FuncIdx)
	case *ChannelObj:
		return fmt.Sprintf("<channel cap=%d>", o.Cap)
	default:
		return "<object>"
	}
}

// ---------------------------------------------------------------------------
// Default trait — zero/default values
// ---------------------------------------------------------------------------

// BuiltinDefault returns the default value for a type, identified by tag.
// Arguments: a single Int representing the type tag.
//
//	TagInt -> 0, TagFloat -> 0.0, TagBool -> false,
//	TagChar -> '\0', TagUnit -> ()
//
// For heap types, pass ObjString=1, ObjArray=2, etc.
func BuiltinDefault(args []Value, heap *Heap) (Value, error) {
	if len(args) != 1 {
		return UnitVal(), fmt.Errorf("default: expected 1 argument (type tag), got %d", len(args))
	}
	tag := byte(args[0].AsInt())
	switch tag {
	case TagInt:
		return IntVal(0), nil
	case TagFloat:
		return FloatVal(0), nil
	case TagBool:
		return BoolVal(false), nil
	case TagChar:
		return CharVal(0), nil
	case TagUnit:
		return UnitVal(), nil
	case ObjString:
		idx := heap.AllocString("")
		return ObjVal(idx), nil
	case ObjArray:
		idx := heap.AllocArray([]Value{})
		return ObjVal(idx), nil
	case ObjTuple:
		idx := heap.AllocTuple([]Value{})
		return ObjVal(idx), nil
	default:
		return UnitVal(), nil
	}
}

// ---------------------------------------------------------------------------
// Clone trait — deep copy
// ---------------------------------------------------------------------------

// BuiltinClone performs a deep clone of a value.
// Primitives are returned as-is. Heap objects are recursively copied.
func BuiltinClone(args []Value, heap *Heap) (Value, error) {
	if len(args) != 1 {
		return UnitVal(), fmt.Errorf("clone: expected 1 argument, got %d", len(args))
	}
	return deepClone(args[0], heap), nil
}

func deepClone(v Value, heap *Heap) Value {
	if v.Tag != TagObj {
		return v
	}
	obj := heap.Get(v.AsObj())
	switch o := obj.Data.(type) {
	case *StringObj:
		idx := heap.AllocString(o.Value)
		return ObjVal(idx)
	case *ArrayObj:
		cloned := make([]Value, len(o.Elements))
		for i, e := range o.Elements {
			cloned[i] = deepClone(e, heap)
		}
		idx := heap.AllocArray(cloned)
		return ObjVal(idx)
	case *TupleObj:
		cloned := make([]Value, len(o.Elements))
		for i, e := range o.Elements {
			cloned[i] = deepClone(e, heap)
		}
		idx := heap.AllocTuple(cloned)
		return ObjVal(idx)
	case *StructObj:
		cloned := make([]Value, len(o.Fields))
		for i, f := range o.Fields {
			cloned[i] = deepClone(f, heap)
		}
		idx := heap.AllocStruct(o.TypeIdx, cloned)
		return ObjVal(idx)
	case *EnumObj:
		cloned := make([]Value, len(o.Fields))
		for i, f := range o.Fields {
			cloned[i] = deepClone(f, heap)
		}
		idx := heap.AllocEnum(o.TypeIdx, o.VariantIdx, cloned)
		return ObjVal(idx)
	default:
		// Closures and channels are not deep-cloned (shared semantics).
		return v
	}
}

// ---------------------------------------------------------------------------
// Hash trait — FNV-1a hash to uint64
// ---------------------------------------------------------------------------

// BuiltinHash computes a hash of a value as an Int.
// Auto-implemented for primitives, strings, arrays, tuples, structs, and enums.
func BuiltinHash(args []Value, heap *Heap) (Value, error) {
	if len(args) != 1 {
		return UnitVal(), fmt.Errorf("hash: expected 1 argument, got %d", len(args))
	}
	h := fnv.New64a()
	hashValue(args[0], heap, h)
	return IntVal(int64(h.Sum64())), nil
}

func hashValue(v Value, heap *Heap, h hash.Hash64) {
	// Write the tag byte for type discrimination.
	h.Write([]byte{v.Tag})
	switch v.Tag {
	case TagInt:
		b := [8]byte{}
		n := v.AsInt()
		for i := 0; i < 8; i++ {
			b[i] = byte(n >> (i * 8))
		}
		h.Write(b[:])
	case TagFloat:
		b := [8]byte{}
		bits := math.Float64bits(v.AsFloat())
		for i := 0; i < 8; i++ {
			b[i] = byte(bits >> (i * 8))
		}
		h.Write(b[:])
	case TagBool:
		if v.AsBool() {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
	case TagChar:
		r := v.AsChar()
		b := [4]byte{byte(r), byte(r >> 8), byte(r >> 16), byte(r >> 24)}
		h.Write(b[:])
	case TagUnit:
		// Nothing extra to hash.
	case TagObj:
		obj := heap.Get(v.AsObj())
		switch o := obj.Data.(type) {
		case *StringObj:
			h.Write([]byte(o.Value))
		case *ArrayObj:
			for _, e := range o.Elements {
				hashValue(e, heap, h)
			}
		case *TupleObj:
			for _, e := range o.Elements {
				hashValue(e, heap, h)
			}
		case *StructObj:
			b := [2]byte{byte(o.TypeIdx), byte(o.TypeIdx >> 8)}
			h.Write(b[:])
			for _, f := range o.Fields {
				hashValue(f, heap, h)
			}
		case *EnumObj:
			b := [4]byte{byte(o.TypeIdx), byte(o.TypeIdx >> 8), byte(o.VariantIdx), byte(o.VariantIdx >> 8)}
			h.Write(b[:])
			for _, f := range o.Fields {
				hashValue(f, heap, h)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// RegisterBuiltinTraits registers all 6 trait methods in a BuiltinRegistry.
// ---------------------------------------------------------------------------

// RegisterBuiltinTraits adds the built-in trait methods to the registry.
func RegisterBuiltinTraits(r *BuiltinRegistry) {
	// Eq trait
	r.Register("eq", BuiltinEq)
	r.Register("neq", BuiltinNeq)

	// Ord trait
	r.Register("compare", BuiltinCompare)

	// Display trait
	r.Register("to_string", BuiltinToString)

	// Default trait
	r.Register("default", BuiltinDefault)

	// Clone trait
	r.Register("clone", BuiltinClone)

	// Hash trait
	r.Register("hash", BuiltinHash)
}
