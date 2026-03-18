package gc

import (
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeObj is a shorthand for creating an object with a given kind and size.
func makeObj(h *Heap, kind ObjectKind, size uint64, children ...*Object) *Object {
	return h.Alloc(kind, size, children...)
}

// collectAll runs a full GC and returns the number of objects freed.
func collectAll(h *Heap) uint64 {
	before := h.Stats.TotalFreed
	h.FullGC()
	return h.Stats.TotalFreed - before
}

// ---------------------------------------------------------------------------
// 1. Unreachable objects are collected
// ---------------------------------------------------------------------------

func TestUnreachableCollected(t *testing.T) {
	h := NewHeap()

	// Allocate objects with no root references — they should all be collected.
	makeObj(h, KindString, 64)
	makeObj(h, KindArray, 128)
	makeObj(h, KindStruct, 256)

	if h.LiveObjects() != 3 {
		t.Fatalf("expected 3 live objects before GC, got %d", h.LiveObjects())
	}

	freed := collectAll(h)
	if freed != 3 {
		t.Fatalf("expected 3 freed, got %d", freed)
	}
	if h.LiveObjects() != 0 {
		t.Fatalf("expected 0 live objects after GC, got %d", h.LiveObjects())
	}
}

// ---------------------------------------------------------------------------
// 2. Reachable objects survive
// ---------------------------------------------------------------------------

func TestReachableSurvive(t *testing.T) {
	h := NewHeap()

	root := makeObj(h, KindStruct, 100)
	child := makeObj(h, KindString, 50)
	root.Children = append(root.Children, child)

	// Register root as a GC root.
	h.AddRootProvider(func() []*Object { return []*Object{root} })

	// Also allocate garbage.
	makeObj(h, KindString, 30)

	freed := collectAll(h)
	if freed != 1 {
		t.Fatalf("expected 1 freed (garbage), got %d", freed)
	}
	if !h.IsLive(root) {
		t.Fatal("root should be live")
	}
	if !h.IsLive(child) {
		t.Fatal("child should be live")
	}
}

// ---------------------------------------------------------------------------
// 3. Circular references are collected
// ---------------------------------------------------------------------------

func TestCircularReferencesCollected(t *testing.T) {
	h := NewHeap()

	a := makeObj(h, KindStruct, 64)
	b := makeObj(h, KindStruct, 64)
	c := makeObj(h, KindStruct, 64)

	// Create a cycle: a → b → c → a
	a.Children = []*Object{b}
	b.Children = []*Object{c}
	c.Children = []*Object{a}

	// No roots — the cycle is garbage.
	freed := collectAll(h)
	if freed != 3 {
		t.Fatalf("expected 3 freed (circular garbage), got %d", freed)
	}
	if h.LiveObjects() != 0 {
		t.Fatalf("expected 0 live objects, got %d", h.LiveObjects())
	}
}

// ---------------------------------------------------------------------------
// 4. Threshold adaptation
// ---------------------------------------------------------------------------

func TestThresholdAdaptation(t *testing.T) {
	h := NewHeap()

	initial := h.Threshold
	if initial != InitialThreshold {
		t.Fatalf("expected initial threshold %d, got %d", InitialThreshold, initial)
	}

	// Run a GC cycle — threshold should double.
	h.FullGC()
	if h.Threshold != initial*ThresholdGrowth {
		t.Fatalf("expected threshold %d after first GC, got %d",
			initial*ThresholdGrowth, h.Threshold)
	}

	// Run another — should double again.
	h.FullGC()
	if h.Threshold != initial*ThresholdGrowth*ThresholdGrowth {
		t.Fatalf("expected threshold %d after second GC, got %d",
			initial*ThresholdGrowth*ThresholdGrowth, h.Threshold)
	}
}

// ---------------------------------------------------------------------------
// 5. Incremental tracing is spread across multiple steps
// ---------------------------------------------------------------------------

func TestIncrementalTracingSpread(t *testing.T) {
	h := NewHeap()
	h.Threshold = 0 // Force GC to trigger immediately.

	// Create a chain of objects that is reachable.
	const chainLen = 600 // More than TraceSliceLimit (256)
	objects := make([]*Object, chainLen)
	for i := range objects {
		objects[i] = makeObj(h, KindStruct, 16)
	}
	// Link them: root → objects[0] → objects[1] → ...
	for i := 0; i < chainLen-1; i++ {
		objects[i].Children = []*Object{objects[i+1]}
	}
	root := objects[0]
	h.AddRootProvider(func() []*Object { return []*Object{root} })

	// Step through the GC incrementally.
	steps := h.RunIncrementalUntilDone(1000)

	// It should have taken more than 1 step (mark was spread out).
	if steps <= 1 {
		t.Fatalf("expected incremental GC to take multiple steps, got %d", steps)
	}

	// All chained objects should survive.
	for i, obj := range objects {
		if !h.IsLive(obj) {
			t.Fatalf("object %d in chain should be live after incremental GC", i)
		}
	}
}

// ---------------------------------------------------------------------------
// 6. Write barrier correctness
// ---------------------------------------------------------------------------

func TestWriteBarrierCorrectness(t *testing.T) {
	h := NewHeap()

	parent := makeObj(h, KindStruct, 64)
	h.AddRootProvider(func() []*Object { return []*Object{parent} })

	// Simulate being in the mark phase with parent already Black.
	h.mu.Lock()
	h.phase = PhaseMark
	parent.Color = Black
	h.mu.Unlock()

	// Allocate a new child (will be White since we're not going through
	// the normal allocation path during mark — simulate it).
	child := &Object{Kind: KindString, Color: White, Size: 32}
	h.mu.Lock()
	h.Objects = append(h.Objects, child)
	h.BytesAlloc += child.Size
	h.mu.Unlock()

	// Without write barrier: parent (Black) → child (White) violates invariant.
	// Apply write barrier.
	parent.Children = append(parent.Children, child)
	h.WriteBarrier(parent, child)

	// Parent should now be Gray.
	h.mu.Lock()
	if parent.Color != Gray {
		t.Fatalf("expected parent to be Gray after write barrier, got %v", parent.Color)
	}
	// Gray list should contain the parent.
	found := false
	for _, obj := range h.grayList {
		if obj == parent {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected parent in gray list after write barrier")
	}
	h.mu.Unlock()
}

func TestWriteBarrierNoopOutsideMark(t *testing.T) {
	h := NewHeap()

	parent := makeObj(h, KindStruct, 64)
	child := makeObj(h, KindString, 32)

	// Outside mark phase — barrier should be a no-op.
	parent.Color = Black
	child.Color = White
	h.WriteBarrier(parent, child)

	// Parent stays Black (no re-gray outside mark phase).
	if parent.Color != Black {
		t.Fatalf("expected parent to stay Black outside mark phase, got %v", parent.Color)
	}
}

func TestWriteBarrierNilSafety(t *testing.T) {
	h := NewHeap()

	obj := makeObj(h, KindStruct, 64)

	// Should not panic with nil arguments.
	h.WriteBarrier(nil, obj)
	h.WriteBarrier(obj, nil)
	h.WriteBarrier(nil, nil)
}

// ---------------------------------------------------------------------------
// 7. Stress test: 100K objects
// ---------------------------------------------------------------------------

func TestStress100KObjects(t *testing.T) {
	h := NewHeap()
	h.Threshold = 1 << 30 // Prevent automatic GC during setup.

	const total = 100_000
	const rootCount = 1000

	// Allocate 100K objects.
	allObjs := make([]*Object, total)
	for i := range allObjs {
		allObjs[i] = makeObj(h, KindString, 64)
	}

	// Keep only rootCount objects as roots (first rootCount).
	roots := allObjs[:rootCount]
	h.AddRootProvider(func() []*Object {
		result := make([]*Object, len(roots))
		copy(result, roots)
		return result
	})

	if h.LiveObjects() != total {
		t.Fatalf("expected %d live before GC, got %d", total, h.LiveObjects())
	}

	h.FullGC()

	expectedFreed := uint64(total - rootCount)
	if h.Stats.LastFreed != expectedFreed {
		t.Fatalf("expected %d freed, got %d", expectedFreed, h.Stats.LastFreed)
	}
	if h.LiveObjects() != rootCount {
		t.Fatalf("expected %d live after GC, got %d", rootCount, h.LiveObjects())
	}

	// Verify roots are all live.
	for i, root := range roots {
		if !h.IsLive(root) {
			t.Fatalf("root %d should be live", i)
		}
	}
}

// ---------------------------------------------------------------------------
// 8. Concurrent fibers + GC safety
// ---------------------------------------------------------------------------

func TestConcurrentFibersGCSafety(t *testing.T) {
	h := NewHeap()
	h.Threshold = 1 << 30 // We'll trigger GC manually.

	const fiberCount = 8
	const objectsPerFiber = 100

	var mu sync.Mutex
	fiberRoots := make([][]*Object, fiberCount)

	// Each "fiber" allocates objects and registers them as roots.
	var wg sync.WaitGroup
	for f := 0; f < fiberCount; f++ {
		f := f
		wg.Add(1)
		go func() {
			defer wg.Done()
			objs := make([]*Object, objectsPerFiber)
			for i := range objs {
				objs[i] = makeObj(h, KindStruct, 32)
			}
			mu.Lock()
			fiberRoots[f] = objs
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Register all fiber roots.
	h.AddRootProvider(func() []*Object {
		mu.Lock()
		defer mu.Unlock()
		var all []*Object
		for _, roots := range fiberRoots {
			all = append(all, roots...)
		}
		return all
	})

	// Also add garbage.
	for i := 0; i < 200; i++ {
		makeObj(h, KindString, 16)
	}

	totalBefore := h.LiveObjects()
	h.FullGC()

	expectedLive := fiberCount * objectsPerFiber
	if h.LiveObjects() != expectedLive {
		t.Fatalf("expected %d live objects after GC, got %d (had %d before)",
			expectedLive, h.LiveObjects(), totalBefore)
	}

	// Run concurrent GC + allocation.
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			makeObj(h, KindString, 16)
		}
	}()
	go func() {
		defer wg.Done()
		h.FullGC()
	}()
	wg.Wait()

	// No panics or races means we pass (run with -race flag).
}

// ---------------------------------------------------------------------------
// 9. Open/closed upvalue lifecycle
// ---------------------------------------------------------------------------

func TestUpvalueLifecycle(t *testing.T) {
	h := NewHeap()

	// Create an open upvalue cell (references a stack slot).
	upvalue := makeObj(h, KindUpvalue, 16)
	upvalue.UpvalueOpen = true

	// The closure that captures it.
	closure := makeObj(h, KindClosure, 64, upvalue)

	// Register closure as a root (simulating it being on a fiber's stack).
	h.AddRootProvider(func() []*Object { return []*Object{closure} })

	// Create garbage.
	makeObj(h, KindString, 32)

	h.FullGC()

	if !h.IsLive(closure) {
		t.Fatal("closure should be live")
	}
	if !h.IsLive(upvalue) {
		t.Fatal("open upvalue should be live (reachable from closure)")
	}

	// Close the upvalue: it now holds its own copy of the value.
	upvalue.UpvalueOpen = false
	closedValue := makeObj(h, KindString, 24)
	upvalue.Children = []*Object{closedValue}

	h.FullGC()

	if !h.IsLive(upvalue) {
		t.Fatal("closed upvalue should be live")
	}
	if !h.IsLive(closedValue) {
		t.Fatal("closed-over value should be live")
	}

	// Remove closure from roots — upvalue and its value should be collected.
	h.mu.Lock()
	h.rootProviders = nil
	h.mu.Unlock()

	before := h.Stats.TotalFreed
	h.FullGC()
	freed := h.Stats.TotalFreed - before

	if freed < 3 {
		t.Fatalf("expected at least 3 freed (closure + upvalue + closedValue), got %d", freed)
	}
}

// ---------------------------------------------------------------------------
// 10. Channel buffer values are not collected
// ---------------------------------------------------------------------------

func TestChannelBufferValuesNotCollected(t *testing.T) {
	h := NewHeap()

	// Create a channel with buffered values.
	channel := makeObj(h, KindChannel, 128)

	val1 := makeObj(h, KindString, 32)
	val2 := makeObj(h, KindStruct, 64)
	channel.Children = []*Object{val1, val2}

	// Channel is a root (e.g., held by a fiber).
	h.AddRootProvider(func() []*Object { return []*Object{channel} })

	// Add garbage.
	makeObj(h, KindString, 16)
	makeObj(h, KindString, 16)

	h.FullGC()

	if !h.IsLive(channel) {
		t.Fatal("channel should be live")
	}
	if !h.IsLive(val1) {
		t.Fatal("buffered value 1 should be live")
	}
	if !h.IsLive(val2) {
		t.Fatal("buffered value 2 should be live")
	}

	if h.Stats.LastFreed != 2 {
		t.Fatalf("expected 2 garbage objects freed, got %d", h.Stats.LastFreed)
	}
}

// ---------------------------------------------------------------------------
// 11. GC stats accuracy
// ---------------------------------------------------------------------------

func TestGCStatsAccuracy(t *testing.T) {
	h := NewHeap()
	h.Threshold = 1 << 30 // Prevent auto-GC.

	// Cycle 1: allocate and free 5 objects of known size.
	for i := 0; i < 5; i++ {
		makeObj(h, KindString, 100)
	}
	h.FullGC()

	if h.Stats.TotalCollections != 1 {
		t.Fatalf("expected 1 collection, got %d", h.Stats.TotalCollections)
	}
	if h.Stats.TotalFreed != 5 {
		t.Fatalf("expected 5 total freed, got %d", h.Stats.TotalFreed)
	}
	if h.Stats.TotalBytesFreed != 500 {
		t.Fatalf("expected 500 total bytes freed, got %d", h.Stats.TotalBytesFreed)
	}
	if h.Stats.LastFreed != 5 {
		t.Fatalf("expected 5 last freed, got %d", h.Stats.LastFreed)
	}
	if h.Stats.LastBytesFreed != 500 {
		t.Fatalf("expected 500 last bytes freed, got %d", h.Stats.LastBytesFreed)
	}
	if h.BytesAlloc != 0 {
		t.Fatalf("expected 0 bytes alloc after full GC of garbage, got %d", h.BytesAlloc)
	}

	// Cycle 2: allocate 3, keep 1 as root, free 2.
	root := makeObj(h, KindStruct, 200)
	makeObj(h, KindString, 50)
	makeObj(h, KindString, 50)
	h.AddRootProvider(func() []*Object { return []*Object{root} })

	h.FullGC()

	if h.Stats.TotalCollections != 2 {
		t.Fatalf("expected 2 collections, got %d", h.Stats.TotalCollections)
	}
	if h.Stats.TotalFreed != 7 { // 5 + 2
		t.Fatalf("expected 7 total freed, got %d", h.Stats.TotalFreed)
	}
	if h.Stats.LastFreed != 2 {
		t.Fatalf("expected 2 last freed, got %d", h.Stats.LastFreed)
	}
	if h.Stats.LastBytesFreed != 100 {
		t.Fatalf("expected 100 last bytes freed, got %d", h.Stats.LastBytesFreed)
	}
	if h.BytesAlloc != 200 {
		t.Fatalf("expected 200 bytes alloc (root only), got %d", h.BytesAlloc)
	}
}

// ---------------------------------------------------------------------------
// 12. Write barrier for specific operations
// ---------------------------------------------------------------------------

func TestBarrierStoreUpvalue(t *testing.T) {
	h := NewHeap()

	upvalue := makeObj(h, KindUpvalue, 16)
	h.mu.Lock()
	h.phase = PhaseMark
	upvalue.Color = Black
	h.mu.Unlock()

	newVal := &Object{Kind: KindString, Color: White, Size: 32}
	h.mu.Lock()
	h.Objects = append(h.Objects, newVal)
	h.mu.Unlock()

	h.BarrierStoreUpvalue(upvalue, newVal)

	h.mu.Lock()
	if upvalue.Color != Gray {
		t.Fatalf("expected upvalue to be Gray after barrier, got %v", upvalue.Color)
	}
	h.mu.Unlock()
}

func TestBarrierFieldSet(t *testing.T) {
	h := NewHeap()

	structObj := makeObj(h, KindStruct, 64)
	h.mu.Lock()
	h.phase = PhaseMark
	structObj.Color = Black
	h.mu.Unlock()

	fieldVal := &Object{Kind: KindString, Color: White, Size: 16}
	h.mu.Lock()
	h.Objects = append(h.Objects, fieldVal)
	h.mu.Unlock()

	h.BarrierFieldSet(structObj, fieldVal)

	h.mu.Lock()
	if structObj.Color != Gray {
		t.Fatalf("expected struct to be Gray after barrier, got %v", structObj.Color)
	}
	h.mu.Unlock()
}

func TestBarrierIndexSet(t *testing.T) {
	h := NewHeap()

	arrayObj := makeObj(h, KindArray, 128)
	h.mu.Lock()
	h.phase = PhaseMark
	arrayObj.Color = Black
	h.mu.Unlock()

	elemVal := &Object{Kind: KindString, Color: White, Size: 16}
	h.mu.Lock()
	h.Objects = append(h.Objects, elemVal)
	h.mu.Unlock()

	h.BarrierIndexSet(arrayObj, elemVal)

	h.mu.Lock()
	if arrayObj.Color != Gray {
		t.Fatalf("expected array to be Gray after barrier, got %v", arrayObj.Color)
	}
	h.mu.Unlock()
}

func TestBarrierChannelSend(t *testing.T) {
	h := NewHeap()

	chanObj := makeObj(h, KindChannel, 128)
	h.mu.Lock()
	h.phase = PhaseMark
	chanObj.Color = Black
	h.mu.Unlock()

	sentVal := &Object{Kind: KindStruct, Color: White, Size: 64}
	h.mu.Lock()
	h.Objects = append(h.Objects, sentVal)
	h.mu.Unlock()

	h.BarrierChannelSend(chanObj, sentVal)

	h.mu.Lock()
	if chanObj.Color != Gray {
		t.Fatalf("expected channel to be Gray after barrier, got %v", chanObj.Color)
	}
	h.mu.Unlock()
}

// ---------------------------------------------------------------------------
// 13. Incremental sweep spreads work
// ---------------------------------------------------------------------------

func TestIncrementalSweepSpreads(t *testing.T) {
	h := NewHeap()

	// Allocate more objects than SweepSliceLimit.
	const count = SweepSliceLimit + 200
	for i := 0; i < count; i++ {
		makeObj(h, KindString, 16)
	}

	// Force into mark phase, complete it immediately, then check sweep.
	h.mu.Lock()
	h.phase = PhaseMark
	// No roots → everything stays white after tracing.
	h.grayList = nil
	h.mu.Unlock()

	// First Step: tracing completes (empty gray list) → transitions to sweep.
	h.Step()
	if h.Phase() != PhaseSweep {
		t.Fatalf("expected PhaseSweep after tracing completes, got %v", h.Phase())
	}

	// Next Step: should sweep at most SweepSliceLimit objects.
	h.Step()
	// Sweep is not yet done since count > SweepSliceLimit.
	if h.Phase() != PhaseSweep {
		t.Fatalf("expected still in PhaseSweep, got %v", h.Phase())
	}

	// Continue stepping until done.
	steps := 0
	for h.Phase() == PhaseSweep && steps < 100 {
		h.Step()
		steps++
	}
	if h.Phase() != PhaseIdle {
		t.Fatalf("expected PhaseIdle after sweep completes, got %v", h.Phase())
	}
}

// ---------------------------------------------------------------------------
// 14. Objects allocated during mark phase survive
// ---------------------------------------------------------------------------

func TestAllocDuringMarkPhase(t *testing.T) {
	h := NewHeap()
	h.Threshold = 0 // Trigger GC immediately.

	root := makeObj(h, KindStruct, 32)
	h.AddRootProvider(func() []*Object { return []*Object{root} })

	// Start incremental cycle (Step moves to PhaseMark).
	h.Step()
	if h.Phase() != PhaseMark {
		t.Fatalf("expected PhaseMark, got %v", h.Phase())
	}

	// Allocate during mark — should be born Gray and survive.
	newObj := makeObj(h, KindString, 16)
	root.Children = append(root.Children, newObj)

	// Complete the cycle.
	h.RunIncrementalUntilDone(100)

	if !h.IsLive(root) {
		t.Fatal("root should be live")
	}
	if !h.IsLive(newObj) {
		t.Fatal("object allocated during mark should be live")
	}
}

// ---------------------------------------------------------------------------
// 15. Deep object graph with transitive references
// ---------------------------------------------------------------------------

func TestDeepObjectGraph(t *testing.T) {
	h := NewHeap()

	// Build a tree: root → [a, b], a → [c, d], b → [e]
	e := makeObj(h, KindString, 8)
	d := makeObj(h, KindString, 8)
	c := makeObj(h, KindString, 8)
	b := makeObj(h, KindStruct, 16, e)
	a := makeObj(h, KindStruct, 16, c, d)
	root := makeObj(h, KindStruct, 16, a, b)

	// Garbage not connected to root.
	makeObj(h, KindString, 32)
	makeObj(h, KindString, 32)

	h.AddRootProvider(func() []*Object { return []*Object{root} })

	h.FullGC()

	for _, obj := range []*Object{root, a, b, c, d, e} {
		if !h.IsLive(obj) {
			t.Fatal("all objects in the tree should be live")
		}
	}
	if h.Stats.LastFreed != 2 {
		t.Fatalf("expected 2 garbage freed, got %d", h.Stats.LastFreed)
	}
}

// ---------------------------------------------------------------------------
// 16. ManualGC (__gc()) works correctly
// ---------------------------------------------------------------------------

func TestManualGC(t *testing.T) {
	h := NewHeap()

	makeObj(h, KindString, 100)
	makeObj(h, KindString, 100)

	h.ManualGC()

	if h.Stats.TotalFreed != 2 {
		t.Fatalf("expected 2 freed by manual GC, got %d", h.Stats.TotalFreed)
	}
}

// ---------------------------------------------------------------------------
// 17. Free list reuse
// ---------------------------------------------------------------------------

func TestFreeListReuse(t *testing.T) {
	h := NewHeap()

	// Allocate and collect to populate the free list.
	makeObj(h, KindString, 32)
	makeObj(h, KindString, 32)
	h.FullGC()

	freeListLen := len(h.FreeList)
	if freeListLen != 2 {
		t.Fatalf("expected 2 free list entries, got %d", freeListLen)
	}

	// New allocations should reuse free slots.
	makeObj(h, KindString, 16)
	if len(h.FreeList) != 1 {
		t.Fatalf("expected 1 free list entry after reuse, got %d", len(h.FreeList))
	}
}

// ---------------------------------------------------------------------------
// 18. ShouldCollect respects threshold
// ---------------------------------------------------------------------------

func TestShouldCollect(t *testing.T) {
	h := NewHeap()
	h.Threshold = 100

	if h.ShouldCollect() {
		t.Fatal("should not collect with 0 bytes allocated")
	}

	makeObj(h, KindString, 50)
	if h.ShouldCollect() {
		t.Fatal("should not collect at 50/100 bytes")
	}

	makeObj(h, KindString, 60)
	if !h.ShouldCollect() {
		t.Fatal("should collect at 110/100 bytes")
	}
}

// ---------------------------------------------------------------------------
// 19. Write barrier bulk children
// ---------------------------------------------------------------------------

func TestWriteBarrierBulkChildren(t *testing.T) {
	h := NewHeap()

	parent := makeObj(h, KindArray, 128)
	h.mu.Lock()
	h.phase = PhaseMark
	parent.Color = Black
	h.mu.Unlock()

	children := make([]*Object, 5)
	for i := range children {
		children[i] = &Object{Kind: KindString, Color: White, Size: 16}
		h.mu.Lock()
		h.Objects = append(h.Objects, children[i])
		h.mu.Unlock()
	}

	h.WriteBarrierChildren(parent, children)

	h.mu.Lock()
	if parent.Color != Gray {
		t.Fatalf("expected parent Gray after bulk barrier, got %v", parent.Color)
	}
	h.mu.Unlock()
}

// ---------------------------------------------------------------------------
// 20. Emergency GC
// ---------------------------------------------------------------------------

func TestEmergencyGC(t *testing.T) {
	h := NewHeap()
	h.Threshold = 100 // Low threshold.

	// Fill the heap well past threshold.
	for i := 0; i < 50; i++ {
		makeObj(h, KindString, 32) // 50*32 = 1600 bytes, 16× threshold.
	}

	// AllocWithEmergency should trigger a full GC before allocating.
	obj := h.AllocWithEmergency(KindString, 32)
	if obj == nil {
		t.Fatal("expected non-nil allocation after emergency GC")
	}
	if h.Stats.TotalCollections < 1 {
		t.Fatal("expected at least 1 GC collection from emergency trigger")
	}
}
