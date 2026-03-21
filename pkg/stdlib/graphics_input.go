package stdlib

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Input functions
// ---------------------------------------------------------------------------

func GfxKeyPressed(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("gfx_key_pressed: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_key_pressed: expected Int argument")
	}
	pressed := ebiten.IsKeyPressed(ebiten.Key(args[0].AsInt()))
	return vm.BoolVal(pressed), nil
}

func GfxKeyJustPressed(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("gfx_key_just_pressed: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_key_just_pressed: expected Int argument")
	}
	pressed := inpututil.IsKeyJustPressed(ebiten.Key(args[0].AsInt()))
	return vm.BoolVal(pressed), nil
}

func GfxMouseX(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("gfx_mouse_x: expected 0 arguments, got %d", len(args))
	}
	x, _ := ebiten.CursorPosition()
	return vm.IntVal(int64(x)), nil
}

func GfxMouseY(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("gfx_mouse_y: expected 0 arguments, got %d", len(args))
	}
	_, y := ebiten.CursorPosition()
	return vm.IntVal(int64(y)), nil
}

func GfxMousePressed(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("gfx_mouse_pressed: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_mouse_pressed: expected Int argument")
	}
	pressed := ebiten.IsMouseButtonPressed(ebiten.MouseButton(args[0].AsInt()))
	return vm.BoolVal(pressed), nil
}

// ---------------------------------------------------------------------------
// Key constants (zero-arg builtins returning Int)
// ---------------------------------------------------------------------------

func KeyUp(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_UP: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeyArrowUp)), nil
}

func KeyDown(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_DOWN: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeyArrowDown)), nil
}

func KeyLeft(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_LEFT: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeyArrowLeft)), nil
}

func KeyRight(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_RIGHT: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeyArrowRight)), nil
}

func KeySpace(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_SPACE: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeySpace)), nil
}

func KeyEscape(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_ESCAPE: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeyEscape)), nil
}

func KeyEnter(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_ENTER: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeyEnter)), nil
}

func KeyW(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_W: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeyW)), nil
}

func KeyA(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_A: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeyA)), nil
}

func KeyS(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_S: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeyS)), nil
}

func KeyD(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_D: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeyD)), nil
}

func KeyEqual(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_EQUAL: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeyEqual)), nil
}

func KeyMinus(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("KEY_MINUS: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(int64(ebiten.KeyMinus)), nil
}

// RegisterInputBuiltins registers all input builtins and key constants.
func RegisterInputBuiltins(r *vm.BuiltinRegistry) {
	// Input functions
	r.Register("gfx_key_pressed", GfxKeyPressed)
	r.Register("gfx_key_just_pressed", GfxKeyJustPressed)
	r.Register("gfx_mouse_x", GfxMouseX)
	r.Register("gfx_mouse_y", GfxMouseY)
	r.Register("gfx_mouse_pressed", GfxMousePressed)

	// Key constants
	r.Register("KEY_UP", KeyUp)
	r.Register("KEY_DOWN", KeyDown)
	r.Register("KEY_LEFT", KeyLeft)
	r.Register("KEY_RIGHT", KeyRight)
	r.Register("KEY_SPACE", KeySpace)
	r.Register("KEY_ESCAPE", KeyEscape)
	r.Register("KEY_ENTER", KeyEnter)
	r.Register("KEY_W", KeyW)
	r.Register("KEY_A", KeyA)
	r.Register("KEY_S", KeyS)
	r.Register("KEY_D", KeyD)
	r.Register("KEY_EQUAL", KeyEqual)
	r.Register("KEY_MINUS", KeyMinus)
}
