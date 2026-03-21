package stdlib

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// gfx_init(w: Int, h: Int, title: String) -> ()
// ---------------------------------------------------------------------------

// GfxInit stores window config in the GraphicsState singleton and configures
// the Ebiten window size and title.
func GfxInit(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 3 {
		return vm.UnitVal(), fmt.Errorf("gfx_init: expected 3 args, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_init: width must be Int, got tag %d", args[0].Tag)
	}
	if args[1].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_init: height must be Int, got tag %d", args[1].Tag)
	}
	title, err := resolveString(args[2], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("gfx_init: title: %w", err)
	}

	w := int(args[0].AsInt())
	h := int(args[1].AsInt())

	SetGraphicsState(w, h, title, heap)
	ebiten.SetWindowSize(w, h)
	ebiten.SetWindowTitle(title)

	return vm.UnitVal(), nil
}

// ---------------------------------------------------------------------------
// gfx_run(callback: fn() -> ()) -> ()
// ---------------------------------------------------------------------------

// GfxRun stores the frame callback in GraphicsState and starts the Ebiten
// game loop. The callback is invoked once per Update tick via CallbackInvoker.
func GfxRun(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("gfx_run: expected 1 arg, got %d", len(args))
	}

	cb := args[0]
	if cb.Tag != vm.TagFunc && cb.Tag != vm.TagObj {
		return vm.UnitVal(), fmt.Errorf("gfx_run: callback must be a function, got tag %d", cb.Tag)
	}

	gs := GetGraphicsState()
	gs.mu.Lock()
	gs.FrameCB = cb
	gs.Running = true
	if heap != nil {
		gs.Heap = heap
	}
	gs.mu.Unlock()

	if err := ebiten.RunGame(&RyxGame{}); err != nil {
		// ebiten.Termination is the normal exit signal from gfx_quit.
		if err == ebiten.Termination {
			return vm.UnitVal(), nil
		}
		return vm.UnitVal(), fmt.Errorf("gfx_run: %w", err)
	}
	return vm.UnitVal(), nil
}

// ---------------------------------------------------------------------------
// gfx_quit() -> ()
// ---------------------------------------------------------------------------

// GfxQuit sets the exit flag so that the next Update() returns
// ebiten.Termination, causing the game loop to exit gracefully.
func GfxQuit(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("gfx_quit: expected 0 args, got %d", len(args))
	}
	gs := GetGraphicsState()
	gs.mu.Lock()
	gs.Running = false
	gs.mu.Unlock()
	return vm.UnitVal(), nil
}

// ---------------------------------------------------------------------------
// gfx_set_title(title: String) -> ()
// ---------------------------------------------------------------------------

// GfxSetTitle changes the window title at runtime.
func GfxSetTitle(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("gfx_set_title: expected 1 arg, got %d", len(args))
	}
	title, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("gfx_set_title: %w", err)
	}

	gs := GetGraphicsState()
	gs.mu.Lock()
	gs.Title = title
	gs.mu.Unlock()

	ebiten.SetWindowTitle(title)
	return vm.UnitVal(), nil
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

// RegisterWindowBuiltins registers the window management builtins.
func RegisterWindowBuiltins(r *vm.BuiltinRegistry) {
	r.Register("gfx_init", GfxInit)
	r.Register("gfx_run", GfxRun)
	r.Register("gfx_quit", GfxQuit)
	r.Register("gfx_set_title", GfxSetTitle)
}
