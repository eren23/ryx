package stdlib

import (
	"fmt"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Color helpers — pure functions, no Ebiten dependency
// ---------------------------------------------------------------------------

// GfxRGB packs r, g, b (0–255) into 0xRRGGBBFF.
func GfxRGB(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 3 {
		return vm.UnitVal(), fmt.Errorf("gfx_rgb: expected 3 arguments, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt || args[1].Tag != vm.TagInt || args[2].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_rgb: expected Int arguments")
	}
	r := args[0].AsInt() & 0xFF
	g := args[1].AsInt() & 0xFF
	b := args[2].AsInt() & 0xFF
	packed := (r << 24) | (g << 16) | (b << 8) | 0xFF
	return vm.IntVal(packed), nil
}

// GfxRGBA packs r, g, b, a (0–255) into 0xRRGGBBAA.
func GfxRGBA(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 4 {
		return vm.UnitVal(), fmt.Errorf("gfx_rgba: expected 4 arguments, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt || args[1].Tag != vm.TagInt || args[2].Tag != vm.TagInt || args[3].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_rgba: expected Int arguments")
	}
	r := args[0].AsInt() & 0xFF
	g := args[1].AsInt() & 0xFF
	b := args[2].AsInt() & 0xFF
	a := args[3].AsInt() & 0xFF
	packed := (r << 24) | (g << 16) | (b << 8) | a
	return vm.IntVal(packed), nil
}

// Named color constants as zero-arg builtins.

func ColorBlack(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("COLOR_BLACK: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(0x000000FF), nil
}

func ColorWhite(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("COLOR_WHITE: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(0xFFFFFFFF), nil
}

func ColorRed(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("COLOR_RED: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(0xFF0000FF), nil
}

func ColorGreen(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("COLOR_GREEN: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(0x00FF00FF), nil
}

func ColorBlue(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("COLOR_BLUE: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(0x0000FFFF), nil
}

func ColorYellow(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("COLOR_YELLOW: expected 0 args, got %d", len(args))
	}
	return vm.IntVal(0xFFFF00FF), nil
}

// RegisterColorBuiltins registers all color helper builtins.
func RegisterColorBuiltins(r *vm.BuiltinRegistry) {
	r.Register("gfx_rgb", GfxRGB)
	r.Register("gfx_rgba", GfxRGBA)
	r.Register("COLOR_BLACK", ColorBlack)
	r.Register("COLOR_WHITE", ColorWhite)
	r.Register("COLOR_RED", ColorRed)
	r.Register("COLOR_GREEN", ColorGreen)
	r.Register("COLOR_BLUE", ColorBlue)
	r.Register("COLOR_YELLOW", ColorYellow)
}
