package stdlib

import (
	"fmt"
	"image/png"
	"os"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Image store singleton
// ---------------------------------------------------------------------------

// ImageStore holds loaded images indexed by integer handles.
type ImageStore struct {
	images []*ebiten.Image
	mu     sync.Mutex
}

var imageStore = &ImageStore{}

// ---------------------------------------------------------------------------
// Image builtins
// ---------------------------------------------------------------------------

// GfxLoadImage loads a PNG file and returns an integer handle, or -1 on error.
func GfxLoadImage(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("gfx_load_image: expected 1 argument, got %d", len(args))
	}
	path, err := resolveString(args[0], heap)
	if err != nil {
		return vm.UnitVal(), fmt.Errorf("gfx_load_image: %w", err)
	}

	f, err := os.Open(path)
	if err != nil {
		return vm.IntVal(-1), nil
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return vm.IntVal(-1), nil
	}

	eImg := ebiten.NewImageFromImage(img)

	imageStore.mu.Lock()
	handle := len(imageStore.images)
	imageStore.images = append(imageStore.images, eImg)
	imageStore.mu.Unlock()

	return vm.IntVal(int64(handle)), nil
}

// GfxImageWidth returns the width of the image at the given handle.
func GfxImageWidth(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("gfx_image_width: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_image_width: expected Int argument")
	}
	handle := int(args[0].AsInt())

	imageStore.mu.Lock()
	defer imageStore.mu.Unlock()

	if handle < 0 || handle >= len(imageStore.images) {
		return vm.IntVal(0), nil
	}
	w, _ := imageStore.images[handle].Size()
	return vm.IntVal(int64(w)), nil
}

// GfxImageHeight returns the height of the image at the given handle.
func GfxImageHeight(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("gfx_image_height: expected 1 argument, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_image_height: expected Int argument")
	}
	handle := int(args[0].AsInt())

	imageStore.mu.Lock()
	defer imageStore.mu.Unlock()

	if handle < 0 || handle >= len(imageStore.images) {
		return vm.IntVal(0), nil
	}
	_, h := imageStore.images[handle].Size()
	return vm.IntVal(int64(h)), nil
}

// ---------------------------------------------------------------------------
// DrawImageCmd — draws an image from the store onto the screen
// ---------------------------------------------------------------------------

// DrawImageCmd draws an image from the image store at a given position with
// optional scaling.
type DrawImageCmd struct {
	Handle int
	X, Y   int
	ScaleX float64
	ScaleY float64
}

func (c DrawImageCmd) Execute(screen *ebiten.Image) {
	imageStore.mu.Lock()
	if c.Handle < 0 || c.Handle >= len(imageStore.images) {
		imageStore.mu.Unlock()
		return
	}
	img := imageStore.images[c.Handle]
	imageStore.mu.Unlock()

	opts := &ebiten.DrawImageOptions{}
	if c.ScaleX != 1.0 || c.ScaleY != 1.0 {
		opts.GeoM.Scale(c.ScaleX, c.ScaleY)
	}
	opts.GeoM.Translate(float64(c.X), float64(c.Y))
	screen.DrawImage(img, opts)
}

// ---------------------------------------------------------------------------
// Draw image builtins
// ---------------------------------------------------------------------------

// GfxDrawImage appends a DrawImageCmd at (x, y) with no scaling.
func GfxDrawImage(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 3 {
		return vm.UnitVal(), fmt.Errorf("gfx_draw_image: expected 3 arguments, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt || args[1].Tag != vm.TagInt || args[2].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_draw_image: expected Int arguments")
	}
	AppendDrawCommand(DrawImageCmd{
		Handle: int(args[0].AsInt()),
		X:      int(args[1].AsInt()),
		Y:      int(args[2].AsInt()),
		ScaleX: 1.0,
		ScaleY: 1.0,
	})
	return vm.UnitVal(), nil
}

// GfxDrawImageScaled appends a DrawImageCmd at (x, y) with scaling (sx, sy).
func GfxDrawImageScaled(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 5 {
		return vm.UnitVal(), fmt.Errorf("gfx_draw_image_scaled: expected 5 arguments, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt || args[1].Tag != vm.TagInt || args[2].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gfx_draw_image_scaled: expected Int for handle, x, y")
	}
	if args[3].Tag != vm.TagFloat || args[4].Tag != vm.TagFloat {
		return vm.UnitVal(), fmt.Errorf("gfx_draw_image_scaled: expected Float for sx, sy")
	}
	AppendDrawCommand(DrawImageCmd{
		Handle: int(args[0].AsInt()),
		X:      int(args[1].AsInt()),
		Y:      int(args[2].AsInt()),
		ScaleX: args[3].AsFloat(),
		ScaleY: args[4].AsFloat(),
	})
	return vm.UnitVal(), nil
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

// RegisterImageBuiltins registers the image loading, query, and drawing builtins.
func RegisterImageBuiltins(r *vm.BuiltinRegistry) {
	r.Register("gfx_load_image", GfxLoadImage)
	r.Register("gfx_image_width", GfxImageWidth)
	r.Register("gfx_image_height", GfxImageHeight)
	r.Register("gfx_draw_image", GfxDrawImage)
	r.Register("gfx_draw_image_scaled", GfxDrawImageScaled)
}
