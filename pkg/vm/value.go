package vm

import (
	"fmt"
	"math"
	"strings"
)

// ---------------------------------------------------------------------------
// Value tags
// ---------------------------------------------------------------------------

const (
	TagInt   byte = 0x01
	TagFloat byte = 0x02
	TagBool  byte = 0x03
	TagChar  byte = 0x04
	TagUnit  byte = 0x06
	TagFunc  byte = 0x0C // Data = function table index
	TagObj   byte = 0x10 // Data = heap object index
)

// Value is a tagged union: Tag discriminates the interpretation of Data.
type Value struct {
	Tag  byte
	Data uint64
}

// --- Constructors ---

func IntVal(v int64) Value      { return Value{TagInt, uint64(v)} }
func FloatVal(v float64) Value  { return Value{TagFloat, math.Float64bits(v)} }
func CharVal(v rune) Value      { return Value{TagChar, uint64(v)} }
func UnitVal() Value            { return Value{Tag: TagUnit} }
func FuncVal(idx uint32) Value  { return Value{TagFunc, uint64(idx)} }
func ObjVal(idx uint32) Value   { return Value{TagObj, uint64(idx)} }

func BoolVal(v bool) Value {
	if v {
		return Value{TagBool, 1}
	}
	return Value{TagBool, 0}
}

// --- Accessors ---

func (v Value) AsInt() int64     { return int64(v.Data) }
func (v Value) AsFloat() float64 { return math.Float64frombits(v.Data) }
func (v Value) AsBool() bool     { return v.Data != 0 }
func (v Value) AsChar() rune     { return rune(v.Data) }
func (v Value) AsFunc() uint32   { return uint32(v.Data) }
func (v Value) AsObj() uint32    { return uint32(v.Data) }

func (v Value) IsTruthy() bool {
	switch v.Tag {
	case TagBool:
		return v.Data != 0
	case TagInt:
		return v.Data != 0
	case TagUnit:
		return false
	default:
		return true
	}
}

// Equal performs structural equality, resolving heap objects via the provided heap.
func (v Value) Equal(other Value, heap *Heap) bool {
	if v.Tag != other.Tag {
		return false
	}
	if v.Tag != TagObj {
		return v.Data == other.Data
	}
	// Both heap objects — check reference equality first.
	if v.Data == other.Data {
		return true
	}
	oa := heap.Get(v.AsObj())
	ob := heap.Get(other.AsObj())
	if oa.Header.TypeID != ob.Header.TypeID {
		return false
	}
	switch sa := oa.Data.(type) {
	case *StringObj:
		return sa.Value == ob.Data.(*StringObj).Value
	case *ArrayObj:
		sb := ob.Data.(*ArrayObj)
		if len(sa.Elements) != len(sb.Elements) {
			return false
		}
		for i := range sa.Elements {
			if !sa.Elements[i].Equal(sb.Elements[i], heap) {
				return false
			}
		}
		return true
	case *TupleObj:
		sb := ob.Data.(*TupleObj)
		if len(sa.Elements) != len(sb.Elements) {
			return false
		}
		for i := range sa.Elements {
			if !sa.Elements[i].Equal(sb.Elements[i], heap) {
				return false
			}
		}
		return true
	case *EnumObj:
		sb := ob.Data.(*EnumObj)
		if sa.TypeIdx != sb.TypeIdx || sa.VariantIdx != sb.VariantIdx {
			return false
		}
		if len(sa.Fields) != len(sb.Fields) {
			return false
		}
		for i := range sa.Fields {
			if !sa.Fields[i].Equal(sb.Fields[i], heap) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (v Value) String() string {
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
		return string(v.AsChar())
	case TagUnit:
		return "()"
	case TagFunc:
		return fmt.Sprintf("<func %d>", v.AsFunc())
	case TagObj:
		return fmt.Sprintf("<obj %d>", v.AsObj())
	default:
		return fmt.Sprintf("<unknown tag=%d>", v.Tag)
	}
}

// StringValue renders a Value, resolving heap objects through the heap.
func StringValue(v Value, heap *Heap) string {
	if v.Tag != TagObj {
		return v.String()
	}
	obj := heap.Get(v.AsObj())
	switch o := obj.Data.(type) {
	case *StringObj:
		return o.Value
	case *ArrayObj:
		parts := make([]string, len(o.Elements))
		for i, e := range o.Elements {
			parts[i] = StringValue(e, heap)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *TupleObj:
		parts := make([]string, len(o.Elements))
		for i, e := range o.Elements {
			parts[i] = StringValue(e, heap)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *StructObj:
		return fmt.Sprintf("<struct type=%d>", o.TypeIdx)
	case *EnumObj:
		return fmt.Sprintf("<enum type=%d variant=%d>", o.TypeIdx, o.VariantIdx)
	case *ClosureObj:
		return fmt.Sprintf("<closure func=%d>", o.FuncIdx)
	case *ChannelObj:
		return fmt.Sprintf("<channel cap=%d>", o.Cap)
	default:
		return "<object>"
	}
}

// ---------------------------------------------------------------------------
// Heap object type IDs
// ---------------------------------------------------------------------------

const (
	ObjString  byte = 0x20
	ObjArray   byte = 0x21
	ObjTuple   byte = 0x22
	ObjStruct  byte = 0x23
	ObjEnum    byte = 0x24
	ObjClosure byte = 0x25
	ObjChannel byte = 0x26
)

// ObjectHeader is the common header for all heap-allocated objects.
type ObjectHeader struct {
	TypeID byte
	GCMark bool
	Size   uint32
}

// HeapObject wraps a header and a type-switched payload.
type HeapObject struct {
	Header ObjectHeader
	Data   any // *StringObj, *ArrayObj, etc.
}

// ---------------------------------------------------------------------------
// Payload types
// ---------------------------------------------------------------------------

// StringObj holds an immutable string on the heap.
type StringObj struct {
	Value string
}

// ArrayObj holds a mutable array of Values.
type ArrayObj struct {
	Elements []Value
}

// TupleObj holds an immutable tuple of Values.
type TupleObj struct {
	Elements []Value
}

// StructObj holds a struct instance.
type StructObj struct {
	TypeIdx uint16
	Fields  []Value
}

// EnumObj holds an enum variant instance.
type EnumObj struct {
	TypeIdx    uint16
	VariantIdx uint16
	Fields     []Value
}

// ClosureObj holds a closure: a function index plus captured upvalues.
type ClosureObj struct {
	FuncIdx  uint16
	Upvalues []*UpvalueCell
}

// UpvalueCell implements open/closed upvalue semantics.
// Open: the cell refers to a live stack slot in a Fiber.
// Closed: the cell holds the captured value directly.
type UpvalueCell struct {
	Open     bool
	Fiber    *Fiber
	StackIdx int
	Closed   Value
}

// Get returns the current value of the upvalue.
func (u *UpvalueCell) Get() Value {
	if u.Open {
		return u.Fiber.Stack[u.StackIdx]
	}
	return u.Closed
}

// Set updates the upvalue.
func (u *UpvalueCell) Set(v Value) {
	if u.Open {
		u.Fiber.Stack[u.StackIdx] = v
	} else {
		u.Closed = v
	}
}

// Close captures the current stack value and marks the cell as closed.
func (u *UpvalueCell) Close() {
	if u.Open {
		u.Closed = u.Fiber.Stack[u.StackIdx]
		u.Open = false
		u.Fiber = nil
	}
}

// ChannelObj implements a CSP-style channel stored on the heap.
type ChannelObj struct {
	Buffer   []Value
	Cap      int
	Closed   bool
	SendQ    []*Fiber // fibers blocked waiting to send
	RecvQ    []*Fiber // fibers blocked waiting to receive
	SendVals []Value  // values from blocked senders (parallel to SendQ)
}
