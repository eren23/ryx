package stdlib

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/ryx-lang/ryx/pkg/vm"
)

var (
	rngMu sync.Mutex
	rng   *rand.Rand
)

func init() {
	rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

// TODO: Move RandomInt and RandomFloat in math_ops.go to this shared RNG so all
// stdlib randomness uses the same mutex-protected state.

// TimeNowMs returns the current Unix timestamp in milliseconds.
func TimeNowMs(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("time_now_ms: expected 0 arguments, got %d", len(args))
	}
	return vm.IntVal(time.Now().UnixMilli()), nil
}

// SleepMs sleeps for the given number of milliseconds.
func SleepMs(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("sleep_ms: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("sleep_ms: expected Int, got tag %d", args[0].Tag)
	}
	ms := args[0].AsInt()
	if ms < 0 {
		return vm.UnitVal(), fmt.Errorf("sleep_ms: milliseconds must be non-negative, got %d", ms)
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return vm.UnitVal(), nil
}

// RandomSeed resets the shared RNG to the given seed.
func RandomSeed(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("random_seed: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("random_seed: expected Int, got tag %d", args[0].Tag)
	}
	seed := args[0].AsInt()
	rngMu.Lock()
	rng = rand.New(rand.NewSource(seed))
	rngMu.Unlock()
	return vm.UnitVal(), nil
}

// RandomShuffle returns a new array with elements shuffled using Fisher-Yates.
func RandomShuffle(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("random_shuffle: expected 1 argument, got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("random_shuffle: %w", err)
	}
	shuffled := make([]vm.Value, len(a.Elements))
	copy(shuffled, a.Elements)

	rngMu.Lock()
	for i := len(shuffled) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	rngMu.Unlock()

	idx := heap.AllocArray(shuffled)
	return vm.ObjVal(idx), nil
}

// RandomChoice returns a random element from a non-empty array.
func RandomChoice(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("random_choice: expected 1 argument, got %d", len(args))
	}
	a, err := resolveArray(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("random_choice: %w", err)
	}
	if len(a.Elements) == 0 {
		return vm.UnitVal(), fmt.Errorf("random_choice: expected non-empty array")
	}

	rngMu.Lock()
	idx := rng.Intn(len(a.Elements))
	rngMu.Unlock()

	return a.Elements[idx], nil
}
