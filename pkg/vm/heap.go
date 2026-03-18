package vm

// Heap manages heap-allocated objects for the VM.
type Heap struct {
	Objects []HeapObject
}

// NewHeap creates an empty heap.
func NewHeap() *Heap {
	return &Heap{}
}

// Alloc adds an object to the heap and returns its index.
func (h *Heap) Alloc(obj HeapObject) uint32 {
	idx := uint32(len(h.Objects))
	h.Objects = append(h.Objects, obj)
	return idx
}

// Get returns a pointer to the heap object at the given index.
func (h *Heap) Get(idx uint32) *HeapObject {
	return &h.Objects[idx]
}

// AllocString allocates a StringObj on the heap.
func (h *Heap) AllocString(s string) uint32 {
	return h.Alloc(HeapObject{
		Header: ObjectHeader{TypeID: ObjString, Size: uint32(len(s))},
		Data:   &StringObj{Value: s},
	})
}

// AllocArray allocates an ArrayObj on the heap.
func (h *Heap) AllocArray(elems []Value) uint32 {
	return h.Alloc(HeapObject{
		Header: ObjectHeader{TypeID: ObjArray, Size: uint32(len(elems))},
		Data:   &ArrayObj{Elements: elems},
	})
}

// AllocTuple allocates a TupleObj on the heap.
func (h *Heap) AllocTuple(elems []Value) uint32 {
	return h.Alloc(HeapObject{
		Header: ObjectHeader{TypeID: ObjTuple, Size: uint32(len(elems))},
		Data:   &TupleObj{Elements: elems},
	})
}

// AllocStruct allocates a StructObj on the heap.
func (h *Heap) AllocStruct(typeIdx uint16, fields []Value) uint32 {
	return h.Alloc(HeapObject{
		Header: ObjectHeader{TypeID: ObjStruct, Size: uint32(len(fields))},
		Data:   &StructObj{TypeIdx: typeIdx, Fields: fields},
	})
}

// AllocEnum allocates an EnumObj on the heap.
func (h *Heap) AllocEnum(typeIdx, variantIdx uint16, fields []Value) uint32 {
	return h.Alloc(HeapObject{
		Header: ObjectHeader{TypeID: ObjEnum, Size: uint32(len(fields))},
		Data:   &EnumObj{TypeIdx: typeIdx, VariantIdx: variantIdx, Fields: fields},
	})
}

// AllocClosure allocates a ClosureObj on the heap.
func (h *Heap) AllocClosure(funcIdx uint16, upvalues []*UpvalueCell) uint32 {
	return h.Alloc(HeapObject{
		Header: ObjectHeader{TypeID: ObjClosure, Size: uint32(len(upvalues))},
		Data:   &ClosureObj{FuncIdx: funcIdx, Upvalues: upvalues},
	})
}

// AllocChannel allocates a ChannelObj on the heap.
func (h *Heap) AllocChannel(cap int) uint32 {
	return h.Alloc(HeapObject{
		Header: ObjectHeader{TypeID: ObjChannel},
		Data: &ChannelObj{
			Buffer: make([]Value, 0, cap),
			Cap:    cap,
		},
	})
}

// AllocMap allocates a MapObj on the heap.
func (h *Heap) AllocMap() uint32 {
	return h.Alloc(HeapObject{
		Header: ObjectHeader{TypeID: ObjMap, Size: 0},
		Data: &MapObj{
			Buckets: make(map[uint64][]MapEntry),
			Count:   0,
		},
	})
}

// Len returns the number of heap objects.
func (h *Heap) Len() int {
	return len(h.Objects)
}
