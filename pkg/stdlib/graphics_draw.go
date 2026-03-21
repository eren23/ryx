package stdlib

import (
	"fmt"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Drawing primitive builtins
// ---------------------------------------------------------------------------

// GfxClear appends a ClearCmd to fill the screen with the given color.
func GfxClear(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("gfx_clear: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_clear: expected Int argument")
	}
	AppendDrawCommand(ClearCmd{Color: unpackRGBA(args[0].AsInt())})
	return vm.UnitVal(), nil
}

// GfxSetColor sets the current drawing color in the GraphicsState.
func GfxSetColor(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("gfx_set_color: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_set_color: expected Int argument")
	}
	gs := GetGraphicsState()
	gs.mu.Lock()
	gs.Color = args[0].AsInt()
	gs.mu.Unlock()
	return vm.UnitVal(), nil
}

// GfxPixel appends a PixelCmd at (x, y) using the current color.
func GfxPixel(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("gfx_pixel: expected 2 arguments, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt || args[1].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_pixel: expected Int arguments")
	}
	gs := GetGraphicsState()
	gs.mu.Lock()
	c := unpackRGBA(gs.Color)
	gs.mu.Unlock()
	AppendDrawCommand(PixelCmd{
		X:     int(args[0].AsInt()),
		Y:     int(args[1].AsInt()),
		Color: c,
	})
	return vm.UnitVal(), nil
}

// GfxLine appends a LineCmd from (x1,y1) to (x2,y2) using the current color.
func GfxLine(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 4 {
		return vm.UnitVal(), fmt.Errorf("gfx_line: expected 4 arguments, got %d", len(args))
	}
	for i := 0; i < 4; i++ {
		if args[i].Tag != vm.TagInt {
			return vm.UnitVal(), fmt.Errorf("gfx_line: expected Int arguments")
		}
	}
	gs := GetGraphicsState()
	gs.mu.Lock()
	c := unpackRGBA(gs.Color)
	gs.mu.Unlock()
	AppendDrawCommand(LineCmd{
		X1:    int(args[0].AsInt()),
		Y1:    int(args[1].AsInt()),
		X2:    int(args[2].AsInt()),
		Y2:    int(args[3].AsInt()),
		Color: c,
	})
	return vm.UnitVal(), nil
}

// GfxRect appends a RectCmd (outline) at (x,y) with size (w,h) using the current color.
func GfxRect(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 4 {
		return vm.UnitVal(), fmt.Errorf("gfx_rect: expected 4 arguments, got %d", len(args))
	}
	for i := 0; i < 4; i++ {
		if args[i].Tag != vm.TagInt {
			return vm.UnitVal(), fmt.Errorf("gfx_rect: expected Int arguments")
		}
	}
	gs := GetGraphicsState()
	gs.mu.Lock()
	c := unpackRGBA(gs.Color)
	gs.mu.Unlock()
	AppendDrawCommand(RectCmd{
		X:     int(args[0].AsInt()),
		Y:     int(args[1].AsInt()),
		W:     int(args[2].AsInt()),
		H:     int(args[3].AsInt()),
		Color: c,
	})
	return vm.UnitVal(), nil
}

// GfxFillRect appends a FillRectCmd at (x,y) with size (w,h) using the current color.
func GfxFillRect(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 4 {
		return vm.UnitVal(), fmt.Errorf("gfx_fill_rect: expected 4 arguments, got %d", len(args))
	}
	for i := 0; i < 4; i++ {
		if args[i].Tag != vm.TagInt {
			return vm.UnitVal(), fmt.Errorf("gfx_fill_rect: expected Int arguments")
		}
	}
	gs := GetGraphicsState()
	gs.mu.Lock()
	c := unpackRGBA(gs.Color)
	gs.mu.Unlock()
	AppendDrawCommand(FillRectCmd{
		X:     int(args[0].AsInt()),
		Y:     int(args[1].AsInt()),
		W:     int(args[2].AsInt()),
		H:     int(args[3].AsInt()),
		Color: c,
	})
	return vm.UnitVal(), nil
}

// GfxCircle appends a CircleCmd (outline) centered at (cx,cy) with radius r.
func GfxCircle(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 3 {
		return vm.UnitVal(), fmt.Errorf("gfx_circle: expected 3 arguments, got %d", len(args))
	}
	for i := 0; i < 3; i++ {
		if args[i].Tag != vm.TagInt {
			return vm.UnitVal(), fmt.Errorf("gfx_circle: expected Int arguments")
		}
	}
	gs := GetGraphicsState()
	gs.mu.Lock()
	c := unpackRGBA(gs.Color)
	gs.mu.Unlock()
	AppendDrawCommand(CircleCmd{
		CX:    int(args[0].AsInt()),
		CY:    int(args[1].AsInt()),
		R:     int(args[2].AsInt()),
		Color: c,
	})
	return vm.UnitVal(), nil
}

// GfxFillCircle appends a FillCircleCmd centered at (cx,cy) with radius r.
func GfxFillCircle(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 3 {
		return vm.UnitVal(), fmt.Errorf("gfx_fill_circle: expected 3 arguments, got %d", len(args))
	}
	for i := 0; i < 3; i++ {
		if args[i].Tag != vm.TagInt {
			return vm.UnitVal(), fmt.Errorf("gfx_fill_circle: expected Int arguments")
		}
	}
	gs := GetGraphicsState()
	gs.mu.Lock()
	c := unpackRGBA(gs.Color)
	gs.mu.Unlock()
	AppendDrawCommand(FillCircleCmd{
		CX:    int(args[0].AsInt()),
		CY:    int(args[1].AsInt()),
		R:     int(args[2].AsInt()),
		Color: c,
	})
	return vm.UnitVal(), nil
}

// GfxText appends a TextCmd at (x,y) with the given string.
func GfxText(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 3 {
		return vm.UnitVal(), fmt.Errorf("gfx_text: expected 3 arguments, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt || args[1].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_text: expected Int for x, y")
	}
	s, err := resolveString(args[2], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("gfx_text: %w", err)
	}
	gs := GetGraphicsState()
	gs.mu.Lock()
	c := unpackRGBA(gs.Color)
	gs.mu.Unlock()
	AppendDrawCommand(TextCmd{
		X:     int(args[0].AsInt()),
		Y:     int(args[1].AsInt()),
		Text:  s,
		Color: c,
	})
	return vm.UnitVal(), nil
}

// RegisterDrawBuiltins registers all drawing primitive builtins.
func RegisterDrawBuiltins(r *vm.BuiltinRegistry) {
	r.Register("gfx_clear", GfxClear)
	r.Register("gfx_set_color", GfxSetColor)
	r.Register("gfx_pixel", GfxPixel)
	r.Register("gfx_line", GfxLine)
	r.Register("gfx_rect", GfxRect)
	r.Register("gfx_fill_rect", GfxFillRect)
	r.Register("gfx_circle", GfxCircle)
	r.Register("gfx_fill_circle", GfxFillCircle)
	r.Register("gfx_text", GfxText)
}
