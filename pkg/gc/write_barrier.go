package gc

// ---------------------------------------------------------------------------
// Write barrier — maintains the tricolor invariant
// ---------------------------------------------------------------------------
//
// The tricolor invariant requires that no Black object directly references
// a White object. When the mutator stores a reference from a parent object
// to a child object, the write barrier checks whether this would violate
// the invariant and corrects it by re-marking the parent Gray.
//
// The compiler inserts write barrier calls at these bytecode sites:
//   - OpStoreUpvalue  (STORE_UPVALUE)  — closing over or mutating an upvalue
//   - OpFieldSet      (FIELD_SET)      — setting a struct field
//   - OpIndexSet      (INDEX_SET)      — setting an array/slice element
//   - OpChannelSend   (CHANNEL_SEND)   — sending a value into a channel
//   - OpMapSet        (MAP_SET)        — inserting/updating a map entry
//
// The barrier uses the "re-mark parent" (Dijkstra-style) strategy:
//   If parent is Black and child is White → set parent to Gray and
//   push it onto the gray list so the tracer will re-scan it.

// WriteBarrier checks and enforces the tricolor invariant when a reference
// from parent to child is created or updated. Must be called while the
// heap lock is NOT held (it acquires the lock internally).
//
// This is the primary entry point called by the VM dispatch loop.
func (h *Heap) WriteBarrier(parent, child *Object) {
	if parent == nil || child == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.writeBarrierLocked(parent, child)
}

// writeBarrierLocked is the lock-held implementation of the write barrier.
func (h *Heap) writeBarrierLocked(parent, child *Object) {
	// Only active during the mark phase — outside of marking, there is no
	// invariant to maintain.
	if h.phase != PhaseMark {
		return
	}

	// Dijkstra barrier: if a Black parent gains a White child, re-gray
	// the parent so the tracer will revisit it and discover the child.
	if parent.Color == Black && child.Color == White {
		parent.Color = Gray
		h.grayList = append(h.grayList, parent)
	}
}

// ---------------------------------------------------------------------------
// Convenience write-barrier wrappers for specific bytecode operations
// ---------------------------------------------------------------------------

// BarrierStoreUpvalue is called by the VM when executing STORE_UPVALUE.
// The upvalue cell (parent) is being updated to reference a new value (child).
func (h *Heap) BarrierStoreUpvalue(upvalueCell, newValue *Object) {
	h.WriteBarrier(upvalueCell, newValue)
}

// BarrierFieldSet is called by the VM when executing FIELD_SET.
// A struct (parent) is having one of its fields set to a new value (child).
func (h *Heap) BarrierFieldSet(structObj, fieldValue *Object) {
	h.WriteBarrier(structObj, fieldValue)
}

// BarrierIndexSet is called by the VM when executing INDEX_SET.
// An array/slice (parent) is having an element set to a new value (child).
func (h *Heap) BarrierIndexSet(arrayObj, elemValue *Object) {
	h.WriteBarrier(arrayObj, elemValue)
}

// BarrierChannelSend is called by the VM when executing CHANNEL_SEND.
// A channel (parent) is receiving a new value (child) into its buffer.
func (h *Heap) BarrierChannelSend(channelObj, sentValue *Object) {
	h.WriteBarrier(channelObj, sentValue)
}

// BarrierMapSet is called by the VM when executing MAP_SET.
// A map (parent) is having an entry inserted or updated. Both the key and
// value may be heap references, so barriers are applied to each.
func (h *Heap) BarrierMapSet(mapObj, key, value *Object) {
	if mapObj == nil {
		return
	}
	if key != nil {
		h.WriteBarrier(mapObj, key)
	}
	if value != nil {
		h.WriteBarrier(mapObj, value)
	}
}

// ---------------------------------------------------------------------------
// Bulk barrier for adding multiple children at once
// ---------------------------------------------------------------------------

// WriteBarrierChildren applies the write barrier for each child being added
// to a parent object. Useful when constructing arrays or structs with
// multiple reference fields.
func (h *Heap) WriteBarrierChildren(parent *Object, children []*Object) {
	if parent == nil || len(children) == 0 {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for _, child := range children {
		if child != nil {
			h.writeBarrierLocked(parent, child)
		}
	}
}
