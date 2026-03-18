package integration

import (
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ryx-lang/ryx/pkg/gc"
)

// ---------------------------------------------------------------------------
// GC stress tests
//
// These tests exercise the gc.Heap directly (not through the VM) to stress
// allocation, collection, write barriers, and pause times.
// ---------------------------------------------------------------------------

// TestGC1MObjects allocates 1 million small objects with no root providers,
// runs a full GC, and verifies that all unreachable objects are freed.
func TestGC1MObjects(t *testing.T) {
	h := gc.NewHeap()

	const count = 1_000_000
	for i := 0; i < count; i++ {
		h.Alloc(gc.KindString, 32)
	}

	liveBeforeGC := h.LiveObjects()
	if liveBeforeGC != count {
		t.Fatalf("expected %d live objects before GC, got %d", count, liveBeforeGC)
	}

	h.FullGC()

	liveAfterGC := h.LiveObjects()
	if liveAfterGC != 0 {
		t.Errorf("expected 0 live objects after GC (no roots), got %d", liveAfterGC)
	}

	freed := h.Stats.TotalFreed
	if freed != count {
		t.Errorf("expected %d freed objects, got %d", count, freed)
	}
}

// TestGCClosureChains creates chains of closure-like objects where each
// closure references the next, forming linked chains. It then verifies that
// GC correctly handles these reference chains.
func TestGCClosureChains(t *testing.T) {
	h := gc.NewHeap()

	const chainLength = 100
	const numChains = 50

	roots := make([]*gc.Object, 0, numChains)

	for c := 0; c < numChains; c++ {
		var prev *gc.Object
		var head *gc.Object
		for i := 0; i < chainLength; i++ {
			var obj *gc.Object
			if prev != nil {
				obj = h.Alloc(gc.KindClosure, 64, prev)
			} else {
				obj = h.Alloc(gc.KindClosure, 64)
			}
			if i == 0 {
				head = obj
			}
			prev = obj
		}
		// The last object in the chain is the root; the head is the deepest.
		_ = head
		roots = append(roots, prev)
	}

	h.AddRootProvider(func() []*gc.Object {
		return roots
	})

	totalAllocated := h.LiveObjects()
	h.FullGC()

	// All objects in the chains should survive (they are reachable from roots).
	liveAfterGC := h.LiveObjects()
	if liveAfterGC != totalAllocated {
		t.Errorf("expected %d live objects after GC (all reachable via chains), got %d",
			totalAllocated, liveAfterGC)
	}

	// Now allocate some unreachable objects.
	for i := 0; i < 1000; i++ {
		h.Alloc(gc.KindString, 16)
	}

	h.FullGC()

	// The 1000 unreachable objects should be freed.
	liveAfterSecondGC := h.LiveObjects()
	if liveAfterSecondGC != totalAllocated {
		t.Errorf("expected %d live objects after second GC, got %d",
			totalAllocated, liveAfterSecondGC)
	}
}

// TestGCConcurrentAllocation allocates objects from multiple goroutines
// concurrently while running GC cycles in the background, verifying that
// the heap's mutex properly protects concurrent access.
func TestGCConcurrentAllocation(t *testing.T) {
	h := gc.NewHeap()

	const goroutines = 8
	const allocsPerGoroutine = 10_000

	// Keep some objects alive as roots to give GC something to trace.
	var rootsMu sync.Mutex
	roots := make([]*gc.Object, 0, 100)
	h.AddRootProvider(func() []*gc.Object {
		rootsMu.Lock()
		defer rootsMu.Unlock()
		cp := make([]*gc.Object, len(roots))
		copy(cp, roots)
		return cp
	})

	var wg sync.WaitGroup

	// Allocator goroutines.
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < allocsPerGoroutine; i++ {
				obj := h.Alloc(gc.KindArray, 48)
				// Keep every 100th object as a root.
				if i%100 == 0 {
					rootsMu.Lock()
					roots = append(roots, obj)
					rootsMu.Unlock()
				}
			}
		}()
	}

	// GC goroutine running concurrently.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			h.FullGC()
		}
	}()

	wg.Wait()

	// Final GC to clean up.
	h.FullGC()

	// Roots should still be alive.
	rootsMu.Lock()
	rootCount := len(roots)
	rootsMu.Unlock()

	liveObjects := h.LiveObjects()
	if liveObjects < rootCount {
		t.Errorf("expected at least %d live objects (roots), got %d", rootCount, liveObjects)
	}

	// Verify stats show collections happened.
	if h.Stats.TotalCollections == 0 {
		t.Error("expected at least one GC collection, got 0")
	}
}

// TestGCWriteBarrier verifies that the write barrier prevents premature
// collection of objects that are attached to black parents during the mark phase.
func TestGCWriteBarrier(t *testing.T) {
	h := gc.NewHeap()

	// Create a parent object that will be a root.
	parent := h.Alloc(gc.KindStruct, 128)
	h.AddRootProvider(func() []*gc.Object { return []*gc.Object{parent} })

	// Allocate enough to exceed the threshold and trigger an incremental cycle.
	// We need to push BytesAlloc past the threshold.
	for i := 0; i < 100; i++ {
		h.Alloc(gc.KindString, 16384) // ~1.6MB total, should exceed 1MB threshold
	}

	// Start an incremental mark phase by stepping.
	h.Step()

	// Now create a new child during the mark phase.
	child := h.Alloc(gc.KindString, 64)

	// Attach child to parent using the write barrier.
	parent.Children = append(parent.Children, child)
	h.WriteBarrier(parent, child)

	// Run incremental GC until done.
	h.RunIncrementalUntilDone(10000)

	// The child should still be alive because the write barrier should have
	// prevented its premature collection.
	if !h.IsLive(child) {
		t.Error("child object was collected despite write barrier; expected it to survive")
	}

	if !h.IsLive(parent) {
		t.Error("parent object was collected; expected it to survive as a root")
	}
}

// TestGCPauseTimeP99 measures GC pause times over many full GC cycles and
// asserts that the p99 pause time is under 10 milliseconds.
func TestGCPauseTimeP99(t *testing.T) {
	h := gc.NewHeap()

	// Register a root provider that keeps some objects alive.
	roots := make([]*gc.Object, 0, 1000)
	h.AddRootProvider(func() []*gc.Object {
		return roots
	})

	const iterations = 100
	pauseTimes := make([]time.Duration, 0, iterations)

	for iter := 0; iter < iterations; iter++ {
		// Allocate a batch of objects, keeping some as roots.
		roots = roots[:0]
		for i := 0; i < 10_000; i++ {
			obj := h.Alloc(gc.KindArray, 64)
			if i%10 == 0 {
				roots = append(roots, obj)
			}
		}

		// Measure the full GC pause time.
		start := time.Now()
		h.FullGC()
		elapsed := time.Since(start)
		pauseTimes = append(pauseTimes, elapsed)
	}

	// Sort pause times and find p99.
	sort.Slice(pauseTimes, func(i, j int) bool {
		return pauseTimes[i] < pauseTimes[j]
	})

	p99Index := int(float64(len(pauseTimes)) * 0.99)
	if p99Index >= len(pauseTimes) {
		p99Index = len(pauseTimes) - 1
	}
	p99 := pauseTimes[p99Index]

	t.Logf("GC pause times: min=%v, median=%v, p99=%v, max=%v",
		pauseTimes[0],
		pauseTimes[len(pauseTimes)/2],
		p99,
		pauseTimes[len(pauseTimes)-1],
	)

	if p99 > 10*time.Millisecond {
		t.Errorf("p99 GC pause time %v exceeds 10ms threshold", p99)
	}
}
