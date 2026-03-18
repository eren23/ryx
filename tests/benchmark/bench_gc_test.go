package benchmark

import (
	"testing"

	"github.com/ryx-lang/ryx/pkg/gc"
)

// allocObjects creates n objects on the heap, each with the given size and
// optionally linking them as a chain via children references.
func allocObjects(h *gc.Heap, n int, size uint64, chain bool) []*gc.Object {
	objects := make([]*gc.Object, n)
	for i := 0; i < n; i++ {
		if chain && i > 0 {
			objects[i] = h.Alloc(gc.KindArray, size, objects[i-1])
		} else {
			objects[i] = h.Alloc(gc.KindArray, size)
		}
	}
	return objects
}

// BenchmarkGCPause measures the time for a full stop-the-world GC collection
// on a heap with a mix of live and garbage objects.
func BenchmarkGCPause(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		h := gc.NewHeap()

		// Create live objects that are retained through roots.
		liveObjects := allocObjects(h, 500, 128, true)

		// Register a root provider that keeps the chain alive.
		h.AddRootProvider(func() []*gc.Object {
			return liveObjects[len(liveObjects)-1:]
		})

		// Create garbage objects that are not referenced by any root.
		allocObjects(h, 1000, 256, false)

		b.StartTimer()
		h.FullGC()
	}
}

// BenchmarkGCThroughput measures allocation + collection throughput by
// repeatedly allocating objects and triggering full GC cycles.
func BenchmarkGCThroughput(b *testing.B) {
	h := gc.NewHeap()
	h.Threshold = 1024 // low threshold to trigger frequent GC

	var roots []*gc.Object
	h.AddRootProvider(func() []*gc.Object {
		return roots
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := h.Alloc(gc.KindArray, 64)
		// Keep the last 100 objects alive; older ones become garbage.
		roots = append(roots, obj)
		if len(roots) > 100 {
			roots = roots[len(roots)-100:]
		}

		if h.ShouldCollect() {
			h.FullGC()
		}
	}
}

// BenchmarkGCIncremental measures the time per incremental GC step on a heap
// that is in the middle of a collection cycle.
func BenchmarkGCIncremental(b *testing.B) {
	h := gc.NewHeap()
	h.Threshold = 512 // trigger collection quickly

	var roots []*gc.Object
	live := allocObjects(h, 200, 64, true)
	roots = append(roots, live[len(live)-1])
	h.AddRootProvider(func() []*gc.Object {
		return roots
	})

	// Create garbage to push past threshold.
	allocObjects(h, 500, 128, false)

	// Run one step to enter mark phase.
	h.Step()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Step()
		// If the cycle completed, allocate more garbage to start a new one.
		if h.Phase() == gc.PhaseIdle {
			b.StopTimer()
			allocObjects(h, 300, 128, false)
			h.Threshold = 512
			b.StartTimer()
		}
	}
}

// BenchmarkGCLargeHeap measures full GC performance on a larger heap with
// deeply nested object graphs.
func BenchmarkGCLargeHeap(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		h := gc.NewHeap()

		// Build a tree of objects: each node has 2 children, depth 10 = ~1023 objects.
		var buildTree func(depth int) *gc.Object
		buildTree = func(depth int) *gc.Object {
			if depth == 0 {
				return h.Alloc(gc.KindArray, 32)
			}
			left := buildTree(depth - 1)
			right := buildTree(depth - 1)
			return h.Alloc(gc.KindArray, 32, left, right)
		}
		root := buildTree(10)

		h.AddRootProvider(func() []*gc.Object {
			return []*gc.Object{root}
		})

		// Add garbage nodes not connected to the tree.
		allocObjects(h, 500, 64, false)

		b.StartTimer()
		h.FullGC()
	}
}
