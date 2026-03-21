package stdlib

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Color packing: gfx_rgb / gfx_rgba
// ---------------------------------------------------------------------------

func TestGraphics_GfxRGB(t *testing.T) {
	tests := []struct {
		name    string
		r, g, b int64
		want    int64
	}{
		{"red", 255, 0, 0, 0xFF0000FF},
		{"green", 0, 255, 0, 0x00FF00FF},
		{"blue", 0, 0, 255, 0x0000FFFF},
		{"white", 255, 255, 255, 0xFFFFFFFF},
		{"black", 0, 0, 0, 0x000000FF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []vm.Value{vm.IntVal(tt.r), vm.IntVal(tt.g), vm.IntVal(tt.b)}
			got, err := GfxRGB(args, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.AsInt() != tt.want {
				t.Errorf("GfxRGB(%d,%d,%d) = 0x%08X, want 0x%08X", tt.r, tt.g, tt.b, got.AsInt(), tt.want)
			}
		})
	}
}

func TestGraphics_GfxRGBA(t *testing.T) {
	tests := []struct {
		name       string
		r, g, b, a int64
		want       int64
	}{
		{"green half alpha", 0, 255, 0, 128, 0x00FF0080},
		{"fully transparent", 255, 0, 0, 0, 0xFF000000},
		{"fully opaque white", 255, 255, 255, 255, 0xFFFFFFFF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []vm.Value{vm.IntVal(tt.r), vm.IntVal(tt.g), vm.IntVal(tt.b), vm.IntVal(tt.a)}
			got, err := GfxRGBA(args, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.AsInt() != tt.want {
				t.Errorf("GfxRGBA(%d,%d,%d,%d) = 0x%08X, want 0x%08X", tt.r, tt.g, tt.b, tt.a, got.AsInt(), tt.want)
			}
		})
	}
}

func TestGraphics_GfxRGB_Errors(t *testing.T) {
	// wrong arg count
	_, err := GfxRGB([]vm.Value{vm.IntVal(1)}, nil)
	if err == nil {
		t.Error("expected error for wrong arg count")
	}
	// wrong type
	_, err = GfxRGB([]vm.Value{vm.BoolVal(true), vm.IntVal(0), vm.IntVal(0)}, nil)
	if err == nil {
		t.Error("expected error for wrong arg type")
	}
}

func TestGraphics_GfxRGBA_Errors(t *testing.T) {
	_, err := GfxRGBA([]vm.Value{vm.IntVal(1)}, nil)
	if err == nil {
		t.Error("expected error for wrong arg count")
	}
	_, err = GfxRGBA([]vm.Value{vm.BoolVal(true), vm.IntVal(0), vm.IntVal(0), vm.IntVal(0)}, nil)
	if err == nil {
		t.Error("expected error for wrong arg type")
	}
}

// ---------------------------------------------------------------------------
// Named color constants
// ---------------------------------------------------------------------------

func TestGraphics_NamedColors(t *testing.T) {
	tests := []struct {
		name string
		fn   func([]vm.Value, *vm.Heap) (vm.Value, error)
		want int64
	}{
		{"COLOR_BLACK", ColorBlack, 0x000000FF},
		{"COLOR_WHITE", ColorWhite, 0xFFFFFFFF},
		{"COLOR_RED", ColorRed, 0xFF0000FF},
		{"COLOR_GREEN", ColorGreen, 0x00FF00FF},
		{"COLOR_BLUE", ColorBlue, 0x0000FFFF},
		{"COLOR_YELLOW", ColorYellow, 0xFFFF00FF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.AsInt() != tt.want {
				t.Errorf("%s = 0x%08X, want 0x%08X", tt.name, got.AsInt(), tt.want)
			}
		})
	}
}

func TestGraphics_NamedColors_ErrorOnArgs(t *testing.T) {
	_, err := ColorBlack([]vm.Value{vm.IntVal(1)}, nil)
	if err == nil {
		t.Error("expected error when passing args to COLOR_BLACK")
	}
}

// ---------------------------------------------------------------------------
// Color unpacking: unpackRGBA
// ---------------------------------------------------------------------------

func TestGraphics_UnpackRGBA(t *testing.T) {
	tests := []struct {
		name   string
		packed int64
		want   color.RGBA
	}{
		{"red opaque", 0xFF0000FF, color.RGBA{R: 255, G: 0, B: 0, A: 255}},
		{"green half", 0x00FF0080, color.RGBA{R: 0, G: 255, B: 0, A: 128}},
		{"blue opaque", 0x0000FFFF, color.RGBA{R: 0, G: 0, B: 255, A: 255}},
		{"white opaque", 0xFFFFFFFF, color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		{"black opaque", 0x000000FF, color.RGBA{R: 0, G: 0, B: 0, A: 255}},
		{"transparent", 0x00000000, color.RGBA{R: 0, G: 0, B: 0, A: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unpackRGBA(tt.packed)
			if got != tt.want {
				t.Errorf("unpackRGBA(0x%08X) = %+v, want %+v", tt.packed, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Draw command queue
// ---------------------------------------------------------------------------

func TestGraphics_DrawCommandQueue(t *testing.T) {
	gs := GetGraphicsState()

	// Clear any prior commands.
	gs.mu.Lock()
	gs.Commands = nil
	gs.mu.Unlock()

	// Append various command types.
	AppendDrawCommand(ClearCmd{Color: color.RGBA{R: 0, G: 0, B: 0, A: 255}})
	AppendDrawCommand(PixelCmd{X: 10, Y: 20, Color: color.RGBA{R: 255, G: 0, B: 0, A: 255}})
	AppendDrawCommand(LineCmd{X1: 0, Y1: 0, X2: 100, Y2: 100, Color: color.RGBA{R: 0, G: 255, B: 0, A: 255}})
	AppendDrawCommand(RectCmd{X: 5, Y: 5, W: 50, H: 30, Color: color.RGBA{R: 0, G: 0, B: 255, A: 255}})
	AppendDrawCommand(FillRectCmd{X: 10, Y: 10, W: 40, H: 20, Color: color.RGBA{R: 255, G: 255, B: 0, A: 255}})
	AppendDrawCommand(CircleCmd{CX: 50, CY: 50, R: 25, Color: color.RGBA{R: 255, G: 255, B: 255, A: 255}})
	AppendDrawCommand(FillCircleCmd{CX: 60, CY: 60, R: 15, Color: color.RGBA{R: 128, G: 128, B: 128, A: 255}})
	AppendDrawCommand(TextCmd{X: 0, Y: 0, Text: "hello", Color: color.RGBA{R: 255, G: 255, B: 255, A: 255}})

	gs.mu.Lock()
	n := len(gs.Commands)
	gs.mu.Unlock()

	if n != 8 {
		t.Fatalf("expected 8 commands in queue, got %d", n)
	}

	// Verify command types via type assertion.
	gs.mu.Lock()
	cmds := gs.Commands
	gs.mu.Unlock()

	if _, ok := cmds[0].(ClearCmd); !ok {
		t.Errorf("command[0] should be ClearCmd, got %T", cmds[0])
	}
	if _, ok := cmds[1].(PixelCmd); !ok {
		t.Errorf("command[1] should be PixelCmd, got %T", cmds[1])
	}
	if _, ok := cmds[2].(LineCmd); !ok {
		t.Errorf("command[2] should be LineCmd, got %T", cmds[2])
	}
	if _, ok := cmds[3].(RectCmd); !ok {
		t.Errorf("command[3] should be RectCmd, got %T", cmds[3])
	}
	if _, ok := cmds[4].(FillRectCmd); !ok {
		t.Errorf("command[4] should be FillRectCmd, got %T", cmds[4])
	}
	if _, ok := cmds[5].(CircleCmd); !ok {
		t.Errorf("command[5] should be CircleCmd, got %T", cmds[5])
	}
	if _, ok := cmds[6].(FillCircleCmd); !ok {
		t.Errorf("command[6] should be FillCircleCmd, got %T", cmds[6])
	}
	if _, ok := cmds[7].(TextCmd); !ok {
		t.Errorf("command[7] should be TextCmd, got %T", cmds[7])
	}
}

func TestGraphics_DrawCommandQueue_Clear(t *testing.T) {
	gs := GetGraphicsState()

	// Seed queue.
	gs.mu.Lock()
	gs.Commands = nil
	gs.mu.Unlock()

	AppendDrawCommand(ClearCmd{Color: color.RGBA{A: 255}})
	AppendDrawCommand(PixelCmd{X: 1, Y: 2})

	// Clear the queue (simulates what Draw() does).
	gs.mu.Lock()
	gs.Commands = nil
	gs.mu.Unlock()

	gs.mu.Lock()
	n := len(gs.Commands)
	gs.mu.Unlock()
	if n != 0 {
		t.Errorf("expected 0 commands after clear, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// Screen info: gfx_width / gfx_height
// ---------------------------------------------------------------------------

func TestGraphics_ScreenInfo(t *testing.T) {
	gs := GetGraphicsState()

	gs.mu.Lock()
	gs.Width = 800
	gs.Height = 600
	gs.mu.Unlock()

	wVal, err := GfxWidth(nil, nil)
	if err != nil {
		t.Fatalf("GfxWidth error: %v", err)
	}
	if wVal.AsInt() != 800 {
		t.Errorf("GfxWidth = %d, want 800", wVal.AsInt())
	}

	hVal, err := GfxHeight(nil, nil)
	if err != nil {
		t.Fatalf("GfxHeight error: %v", err)
	}
	if hVal.AsInt() != 600 {
		t.Errorf("GfxHeight = %d, want 600", hVal.AsInt())
	}
}

func TestGraphics_ScreenInfo_ErrorOnArgs(t *testing.T) {
	_, err := GfxWidth([]vm.Value{vm.IntVal(1)}, nil)
	if err == nil {
		t.Error("expected error for GfxWidth with args")
	}
	_, err = GfxHeight([]vm.Value{vm.IntVal(1)}, nil)
	if err == nil {
		t.Error("expected error for GfxHeight with args")
	}
}

// ---------------------------------------------------------------------------
// Round-trip: pack then unpack
// ---------------------------------------------------------------------------

func TestGraphics_PackUnpackRoundTrip(t *testing.T) {
	args := []vm.Value{vm.IntVal(42), vm.IntVal(128), vm.IntVal(200)}
	packed, err := GfxRGB(args, nil)
	if err != nil {
		t.Fatalf("GfxRGB error: %v", err)
	}
	got := unpackRGBA(packed.AsInt())
	want := color.RGBA{R: 42, G: 128, B: 200, A: 255}
	if got != want {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, want)
	}
}

// ---------------------------------------------------------------------------
// Image store: manual add + width/height query
// ---------------------------------------------------------------------------

func TestGraphics_ImageStore(t *testing.T) {
	// Create a 4x4 RGBA image with distinct pixels.
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x * 60), G: uint8(y * 60), B: 128, A: 255})
		}
	}

	eImg := ebiten.NewImageFromImage(src)

	// Manually insert into the imageStore and record the handle.
	imageStore.mu.Lock()
	handle := len(imageStore.images)
	imageStore.images = append(imageStore.images, eImg)
	imageStore.mu.Unlock()

	// Query width via GfxImageWidth.
	wVal, err := GfxImageWidth([]vm.Value{vm.IntVal(int64(handle))}, nil)
	if err != nil {
		t.Fatalf("GfxImageWidth error: %v", err)
	}
	if wVal.AsInt() != 4 {
		t.Errorf("GfxImageWidth = %d, want 4", wVal.AsInt())
	}

	// Query height via GfxImageHeight.
	hVal, err := GfxImageHeight([]vm.Value{vm.IntVal(int64(handle))}, nil)
	if err != nil {
		t.Fatalf("GfxImageHeight error: %v", err)
	}
	if hVal.AsInt() != 4 {
		t.Errorf("GfxImageHeight = %d, want 4", hVal.AsInt())
	}
}

// ---------------------------------------------------------------------------
// DrawImageCmd implements DrawCommand
// ---------------------------------------------------------------------------

// Compile-time interface check.
var _ DrawCommand = DrawImageCmd{}

func TestGraphics_DrawImageCmd(t *testing.T) {
	cmd := DrawImageCmd{
		Handle: 7,
		X:      100,
		Y:      200,
		ScaleX: 2.0,
		ScaleY: 0.5,
	}
	if cmd.Handle != 7 {
		t.Errorf("Handle = %d, want 7", cmd.Handle)
	}
	if cmd.X != 100 || cmd.Y != 200 {
		t.Errorf("X,Y = %d,%d, want 100,200", cmd.X, cmd.Y)
	}
	if cmd.ScaleX != 2.0 || cmd.ScaleY != 0.5 {
		t.Errorf("ScaleX,ScaleY = %f,%f, want 2.0,0.5", cmd.ScaleX, cmd.ScaleY)
	}
}

// ---------------------------------------------------------------------------
// Load image with invalid path returns -1
// ---------------------------------------------------------------------------

func TestGraphics_LoadImageInvalidPath(t *testing.T) {
	heap := vm.NewHeap()
	idx := heap.AllocString("/no/such/file.png")
	args := []vm.Value{vm.ObjVal(idx)}

	got, err := GfxLoadImage(args, heap)
	if err != nil {
		t.Fatalf("GfxLoadImage returned unexpected error: %v", err)
	}
	if got.AsInt() != -1 {
		t.Errorf("GfxLoadImage with invalid path = %d, want -1", got.AsInt())
	}
}
