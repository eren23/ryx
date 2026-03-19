package gc

import (
	"sync"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// Tricolor marks
// ---------------------------------------------------------------------------

// Color represents the tricolor marking state of a heap object.
type Color byte

const (
	White Color = iota // Unmarked — candidate for collection.
	Gray               // Discovered but not yet fully traced.
	Black              // Fully traced — all references scanned.
)

// ---------------------------------------------------------------------------
// Object — a GC-managed heap value
// ---------------------------------------------------------------------------

// ObjectKind discriminates the type of a heap-allocated object.
type ObjectKind byte

const (
	KindArray   ObjectKind = iota // []Value
	KindStruct                    // named struct with fields
	KindEnum                      // tagged enum variant
	KindClosure                   // function + captured upvalues
	KindChannel                   // buffered/unbuffered channel
	KindString                    // interned or dynamic string
	KindUpvalue                   // open/closed upvalue cell
	KindFiber                     // lightweight fiber/coroutine
	KindMap                       // hash map with key-value entries
)

// Object is a single GC-managed heap allocation.
type Object struct {
	Kind  ObjectKind
	Color Color
	Mark  bool   // secondary mark used during sweep reset
	Size  uint64 // approximate byte size of this allocation

	// Reference fields — traced by the GC to find child objects.
	// Each object kind populates these differently:
	//   Array   → Children = elements that are object references
	//   Struct  → Children = field values that are object references
	//   Enum    → Children = variant payload references
	//   Closure → Children = captured upvalue cells
	//   Channel → Children = buffered values
	//   Upvalue → Children[0] = closed-over value (if closed)
	//   Fiber   → Children = stack slots + call frame refs + open upvalues
	//   Map     → Children = keys and values that are heap object references
	Children []*Object

	// Upvalue-specific: whether this upvalue cell is open (on stack) or closed.
	UpvalueOpen bool

	// Fiber-specific: reference to the fiber's own stack roots, globals, etc.
	// These are stored in Children but we keep metadata for root scanning.
	FiberID int
}

// References returns all child objects that the GC should trace.
func (o *Object) References() []*Object {
	return o.Children
}

// ---------------------------------------------------------------------------
// GCPhase — tracks which phase the incremental collector is in
// ---------------------------------------------------------------------------

// GCPhase is the current phase of the incremental collector.
type GCPhase int

const (
	PhaseIdle  GCPhase = iota // No collection in progress.
	PhaseMark                 // Root scanning + incremental tracing.
	PhaseSweep                // Incremental sweeping of white objects.
)

// ---------------------------------------------------------------------------
// GCStats — collection statistics
// ---------------------------------------------------------------------------

// GCStats tracks garbage collection metrics.
type GCStats struct {
	TotalCollections uint64 // Number of completed GC cycles.
	TotalFreed       uint64 // Cumulative objects freed.
	TotalBytesFreed  uint64 // Cumulative bytes freed.
	LastFreed        uint64 // Objects freed in the last completed cycle.
	LastBytesFreed   uint64 // Bytes freed in the last completed cycle.
}

// ---------------------------------------------------------------------------
// Incremental work limits
// ---------------------------------------------------------------------------

const (
	TraceSliceLimit = 256 // Max objects traced per incremental mark slice.
	SweepSliceLimit = 512 // Max objects swept per incremental sweep slice.
	InitialThreshold uint64 = 1 << 20 // 1 MB initial GC threshold.
	ThresholdGrowth  uint64 = 2        // Threshold doubles after each cycle.
)

// ---------------------------------------------------------------------------
// Heap — the garbage-collected heap
// ---------------------------------------------------------------------------

// Heap manages all GC-allocated objects and runs the tricolor collector.
type Heap struct {
	mu sync.Mutex

	// All live objects on the heap.
	Objects []*Object

	// Free list of recycled object slots (indices into Objects).
	FreeList []int

	// Allocation accounting.
	BytesAlloc uint64

	// Adaptive threshold: GC triggers when BytesAlloc > Threshold.
	Threshold uint64

	// Collection statistics.
	Stats GCStats

	// Root providers: functions that return root objects to scan.
	// The VM registers root providers for fiber stacks, globals, etc.
	rootProviders []func() []*Object

	// Incremental state.
	phase    GCPhase
	grayList []*Object   // Objects discovered but not yet traced.
	sweepIdx int          // Current position in Objects for sweep phase.

	// Per-cycle counters (reset each cycle).
	cycleFreed      uint64
	cycleBytesFreed uint64

	// Emergency GC flag — set when allocation fails.
	emergencyGC atomic.Bool
}

// NewHeap creates a new garbage-collected heap with default settings.
func NewHeap() *Heap {
	return &Heap{
		Objects:   make([]*Object, 0, 1024),
		FreeList:  make([]int, 0, 64),
		Threshold: InitialThreshold,
	}
}

// ---------------------------------------------------------------------------
// Root registration
// ---------------------------------------------------------------------------

// AddRootProvider registers a function that returns root objects for scanning.
// The VM calls this to register fiber stacks, global tables, etc.
func (h *Heap) AddRootProvider(provider func() []*Object) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rootProviders = append(h.rootProviders, provider)
}

// ---------------------------------------------------------------------------
// Allocation
// ---------------------------------------------------------------------------

// Alloc creates a new object on the heap. If the allocation pushes
// BytesAlloc past the threshold, it signals that a GC cycle should begin.
// Returns nil only if the heap is unable to allocate (triggers emergency GC).
func (h *Heap) Alloc(kind ObjectKind, size uint64, children ...*Object) *Object {
	h.mu.Lock()
	defer h.mu.Unlock()

	obj := &Object{
		Kind:     kind,
		Color:    White,
		Size:     size,
		Children: children,
	}

	// Reuse a free-list slot if available.
	if len(h.FreeList) > 0 {
		idx := h.FreeList[len(h.FreeList)-1]
		h.FreeList = h.FreeList[:len(h.FreeList)-1]
		h.Objects[idx] = obj
	} else {
		h.Objects = append(h.Objects, obj)
	}

	h.BytesAlloc += size

	// If we're in the middle of a mark phase, new objects must be gray
	// to prevent the mutator from hiding them behind black objects.
	if h.phase == PhaseMark {
		obj.Color = Gray
		h.grayList = append(h.grayList, obj)
	}

	return obj
}

// AllocWithEmergency is like Alloc but triggers a full emergency GC if
// the heap cannot grow (simulated by BytesAlloc exceeding 4× threshold).
func (h *Heap) AllocWithEmergency(kind ObjectKind, size uint64, children ...*Object) *Object {
	// Check if we need emergency GC before allocating.
	h.mu.Lock()
	needEmergency := h.BytesAlloc+size > h.Threshold*4
	h.mu.Unlock()

	if needEmergency {
		h.emergencyGC.Store(true)
		h.FullGC()
	}

	return h.Alloc(kind, size, children...)
}

// ---------------------------------------------------------------------------
// GC trigger check
// ---------------------------------------------------------------------------

// ShouldCollect returns true if BytesAlloc exceeds the adaptive threshold.
func (h *Heap) ShouldCollect() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.BytesAlloc > h.Threshold
}

// Phase returns the current GC phase.
func (h *Heap) Phase() GCPhase {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.phase
}

// ---------------------------------------------------------------------------
// Full (stop-the-world) GC — used for manual __gc() and emergency
// ---------------------------------------------------------------------------

// FullGC runs a complete mark-sweep cycle synchronously.
func (h *Heap) FullGC() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.fullGCLocked()
}

func (h *Heap) fullGCLocked() {
	// Reset all objects to white.
	for _, obj := range h.Objects {
		if obj != nil {
			obj.Color = White
		}
	}

	// Phase 1: Mark roots gray.
	h.grayList = h.grayList[:0]
	roots := h.collectRootsLocked()
	for _, root := range roots {
		if root != nil && root.Color == White {
			root.Color = Gray
			h.grayList = append(h.grayList, root)
		}
	}

	// Phase 2: Trace — process all gray objects (no limit).
	for len(h.grayList) > 0 {
		obj := h.grayList[len(h.grayList)-1]
		h.grayList = h.grayList[:len(h.grayList)-1]

		obj.Color = Black
		for _, child := range obj.References() {
			if child != nil && child.Color == White {
				child.Color = Gray
				h.grayList = append(h.grayList, child)
			}
		}
	}

	// Phase 3: Sweep — free all white objects.
	freed := uint64(0)
	bytesFreed := uint64(0)
	for i, obj := range h.Objects {
		if obj != nil && obj.Color == White {
			freed++
			bytesFreed += obj.Size
			h.Objects[i] = nil
			h.FreeList = append(h.FreeList, i)
		} else if obj != nil {
			obj.Color = White // Reset for next cycle.
		}
	}

	h.BytesAlloc -= bytesFreed
	h.Stats.TotalCollections++
	h.Stats.TotalFreed += freed
	h.Stats.TotalBytesFreed += bytesFreed
	h.Stats.LastFreed = freed
	h.Stats.LastBytesFreed = bytesFreed

	// Adapt threshold: grow by 2x.
	h.Threshold *= ThresholdGrowth

	h.phase = PhaseIdle
	h.emergencyGC.Store(false)
}

func (h *Heap) collectRootsLocked() []*Object {
	var roots []*Object
	for _, provider := range h.rootProviders {
		roots = append(roots, provider()...)
	}
	return roots
}

// ---------------------------------------------------------------------------
// Incremental GC — called from the VM dispatch loop
// ---------------------------------------------------------------------------

// Step performs one slice of incremental GC work. The VM calls this
// between instruction slices to spread GC work across execution.
// Returns true if a GC cycle completed during this step.
func (h *Heap) Step() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch h.phase {
	case PhaseIdle:
		// Check if we should start a new cycle.
		if h.BytesAlloc <= h.Threshold {
			return false
		}
		h.startMarkPhaseLocked()
		return false

	case PhaseMark:
		done := h.traceSliceLocked(TraceSliceLimit)
		if done {
			// All gray objects traced — transition to sweep.
			h.phase = PhaseSweep
			h.sweepIdx = 0
			h.cycleFreed = 0
			h.cycleBytesFreed = 0
		}
		return false

	case PhaseSweep:
		done := h.sweepSliceLocked(SweepSliceLimit)
		if done {
			h.finishCycleLocked()
			return true
		}
		return false
	}
	return false
}

// startMarkPhaseLocked transitions from idle to mark phase.
func (h *Heap) startMarkPhaseLocked() {
	h.phase = PhaseMark
	h.grayList = h.grayList[:0]

	// Reset all objects to white.
	for _, obj := range h.Objects {
		if obj != nil {
			obj.Color = White
		}
	}

	// Scan roots and mark them gray.
	roots := h.collectRootsLocked()
	for _, root := range roots {
		if root != nil && root.Color == White {
			root.Color = Gray
			h.grayList = append(h.grayList, root)
		}
	}
}

// traceSliceLocked processes up to `limit` gray objects.
// Returns true if the gray list is empty (tracing complete).
func (h *Heap) traceSliceLocked(limit int) bool {
	processed := 0
	for len(h.grayList) > 0 && processed < limit {
		obj := h.grayList[len(h.grayList)-1]
		h.grayList = h.grayList[:len(h.grayList)-1]

		obj.Color = Black
		for _, child := range obj.References() {
			if child != nil && child.Color == White {
				child.Color = Gray
				h.grayList = append(h.grayList, child)
			}
		}
		processed++
	}
	return len(h.grayList) == 0
}

// sweepSliceLocked frees up to `limit` white objects starting from sweepIdx.
// Returns true if the sweep is complete.
func (h *Heap) sweepSliceLocked(limit int) bool {
	processed := 0
	for h.sweepIdx < len(h.Objects) && processed < limit {
		obj := h.Objects[h.sweepIdx]
		if obj != nil {
			if obj.Color == White {
				// Unreachable — free it.
				h.cycleFreed++
				h.cycleBytesFreed += obj.Size
				h.Objects[h.sweepIdx] = nil
				h.FreeList = append(h.FreeList, h.sweepIdx)
			} else {
				// Reachable — reset to white for next cycle.
				obj.Color = White
			}
		}
		h.sweepIdx++
		processed++
	}
	return h.sweepIdx >= len(h.Objects)
}

// finishCycleLocked completes the incremental cycle and updates stats.
func (h *Heap) finishCycleLocked() {
	h.BytesAlloc -= h.cycleBytesFreed
	h.Stats.TotalCollections++
	h.Stats.TotalFreed += h.cycleFreed
	h.Stats.TotalBytesFreed += h.cycleBytesFreed
	h.Stats.LastFreed = h.cycleFreed
	h.Stats.LastBytesFreed = h.cycleBytesFreed

	// Adapt threshold.
	h.Threshold *= ThresholdGrowth

	h.phase = PhaseIdle
}

// ---------------------------------------------------------------------------
// Object liveness queries (for testing / introspection)
// ---------------------------------------------------------------------------

// LiveObjects returns the count of non-nil objects currently on the heap.
func (h *Heap) LiveObjects() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	count := 0
	for _, obj := range h.Objects {
		if obj != nil {
			count++
		}
	}
	return count
}

// IsLive returns true if the given object is still present on the heap.
func (h *Heap) IsLive(target *Object) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, obj := range h.Objects {
		if obj == target {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// ManualGC — exposed as __gc() to user code
// ---------------------------------------------------------------------------

// ManualGC runs a full collection cycle, equivalent to calling __gc() in Ryx.
func (h *Heap) ManualGC() {
	h.FullGC()
}

// ---------------------------------------------------------------------------
// RunIncrementalUntilDone — helper for tests
// ---------------------------------------------------------------------------

// RunIncrementalUntilDone runs incremental steps until a full cycle completes
// or maxSteps is reached. Returns the number of steps taken.
func (h *Heap) RunIncrementalUntilDone(maxSteps int) int {
	steps := 0
	for steps < maxSteps {
		steps++
		if h.Step() {
			return steps
		}
	}
	return steps
}
