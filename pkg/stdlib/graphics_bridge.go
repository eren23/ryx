package stdlib

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Graphics state singleton
// ---------------------------------------------------------------------------

// GraphicsState holds all mutable state for the Ebiten graphics bridge.
type GraphicsState struct {
	Width    int
	Height   int
	Title    string
	Color    int64 // packed RGBA
	Commands []DrawCommand
	// lastDraw is a snapshot of Commands taken at the end of each Update.
	// Draw may run more often than Update; we replay lastDraw every time so the
	// queue is never consumed twice and we always draw onto the real screen (the
	// vector package renders reliably there; offscreen NewImage raster was flaky).
	lastDraw []DrawCommand
	Running  bool
	FrameCB  vm.Value
	Heap     *vm.Heap

	mu sync.Mutex
}

var (
	gfxState     *GraphicsState
	gfxStateOnce sync.Once
)

// GetGraphicsState returns the singleton GraphicsState, creating it with
// defaults on first call.
func GetGraphicsState() *GraphicsState {
	gfxStateOnce.Do(func() {
		gfxState = &GraphicsState{
			Width:   640,
			Height:  480,
			Title:   "Ryx",
			Color:   0xFFFFFFFF, // white
			Running: false,
		}
	})
	return gfxState
}

// SetGraphicsState configures the graphics state fields. Pass -1 for any
// int field to leave it unchanged; pass "" for title to leave unchanged.
func SetGraphicsState(width, height int, title string, heap *vm.Heap) {
	gs := GetGraphicsState()
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if width > 0 {
		gs.Width = width
	}
	if height > 0 {
		gs.Height = height
	}
	if title != "" {
		gs.Title = title
	}
	if heap != nil {
		gs.Heap = heap
	}
}

// ---------------------------------------------------------------------------
// Draw command interface + concrete types
// ---------------------------------------------------------------------------

// DrawCommand is the interface for all queued draw operations.
type DrawCommand interface {
	Execute(screen *ebiten.Image)
}

// ClearCmd fills the entire screen with a solid color.
type ClearCmd struct{ Color color.RGBA }

func (c ClearCmd) Execute(screen *ebiten.Image) {
	screen.Fill(c.Color)
}

// PixelCmd draws a single pixel.
type PixelCmd struct {
	X, Y  int
	Color color.RGBA
}

func (c PixelCmd) Execute(screen *ebiten.Image) {
	screen.Set(c.X, c.Y, c.Color)
}

// LineCmd draws a line between two points.
type LineCmd struct {
	X1, Y1, X2, Y2 int
	Color           color.RGBA
}

func (c LineCmd) Execute(screen *ebiten.Image) {
	vector.StrokeLine(screen,
		float32(c.X1), float32(c.Y1),
		float32(c.X2), float32(c.Y2),
		1, c.Color, false)
}

// RectCmd draws a rectangle outline.
type RectCmd struct {
	X, Y, W, H int
	Color       color.RGBA
}

func (c RectCmd) Execute(screen *ebiten.Image) {
	vector.StrokeRect(screen,
		float32(c.X), float32(c.Y),
		float32(c.W), float32(c.H),
		1, c.Color, false)
}

// FillRectCmd draws a filled rectangle.
type FillRectCmd struct {
	X, Y, W, H int
	Color       color.RGBA
}

func (c FillRectCmd) Execute(screen *ebiten.Image) {
	vector.DrawFilledRect(screen,
		float32(c.X), float32(c.Y),
		float32(c.W), float32(c.H),
		c.Color, false)
}

// CircleCmd draws a circle outline.
type CircleCmd struct {
	CX, CY, R int
	Color      color.RGBA
}

func (c CircleCmd) Execute(screen *ebiten.Image) {
	vector.StrokeCircle(screen,
		float32(c.CX), float32(c.CY), float32(c.R),
		1, c.Color, false)
}

// FillCircleCmd draws a filled circle.
type FillCircleCmd struct {
	CX, CY, R int
	Color      color.RGBA
}

func (c FillCircleCmd) Execute(screen *ebiten.Image) {
	vector.DrawFilledCircle(screen,
		float32(c.CX), float32(c.CY), float32(c.R),
		c.Color, false)
}

// TextCmd draws debug text at a position.
type TextCmd struct {
	X, Y  int
	Text  string
	Color color.RGBA
}

func (c TextCmd) Execute(screen *ebiten.Image) {
	ebitenutil.DebugPrintAt(screen, c.Text, c.X, c.Y)
}

// ---------------------------------------------------------------------------
// Draw command queue helpers
// ---------------------------------------------------------------------------

// AppendDrawCommand adds a draw command to the queue (thread-safe).
func AppendDrawCommand(cmd DrawCommand) {
	gs := GetGraphicsState()
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.Commands = append(gs.Commands, cmd)
}

// snapshotCommandsForDraw copies the current queue into lastDraw and clears the
// live queue. Call once per Update after the frame callback.
func snapshotCommandsForDraw() {
	gs := GetGraphicsState()
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.lastDraw = append(gs.lastDraw[:0], gs.Commands...)
	gs.Commands = gs.Commands[:0]
}

// unpackRGBA converts a packed 0xRRGGBBAA int64 to color.RGBA.
func unpackRGBA(packed int64) color.RGBA {
	return color.RGBA{
		R: uint8((packed >> 24) & 0xFF),
		G: uint8((packed >> 16) & 0xFF),
		B: uint8((packed >> 8) & 0xFF),
		A: uint8(packed & 0xFF),
	}
}

// ---------------------------------------------------------------------------
// RyxGame — implements ebiten.Game
// ---------------------------------------------------------------------------

// RyxGame implements the ebiten.Game interface, bridging the Ryx VM's
// frame callback and draw command queue to Ebiten's game loop.
type RyxGame struct{}

// Update invokes the Ryx frame callback once per tick.
func (g *RyxGame) Update() error {
	gs := GetGraphicsState()
	gs.mu.Lock()
	running := gs.Running
	cb := gs.FrameCB
	heap := gs.Heap
	gs.mu.Unlock()

	// gfx_quit sets Running to false; return Termination to exit the loop.
	if !running {
		return ebiten.Termination
	}

	if CallbackInvoker == nil {
		return nil
	}
	// Only invoke if the callback is a valid function or closure.
	if cb.Tag == vm.TagFunc || cb.Tag == vm.TagObj {
		_, err := CallbackInvoker(cb, []vm.Value{}, heap)
		if err != nil {
			return fmt.Errorf("frame callback error: %w", err)
		}
	}
	snapshotCommandsForDraw()
	return nil
}

// Draw replays the last Update's draw list. Ebiten may call Draw more often than
// Update; replaying the same snapshot avoids an empty queue and matches screen
// rendering behavior the vector package expects.
func (g *RyxGame) Draw(screen *ebiten.Image) {
	gs := GetGraphicsState()
	gs.mu.Lock()
	cmds := gs.lastDraw
	gs.mu.Unlock()
	for _, cmd := range cmds {
		cmd.Execute(screen)
	}
}

// Layout returns the configured window dimensions.
func (g *RyxGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	gs := GetGraphicsState()
	return gs.Width, gs.Height
}

// Compile-time check that RyxGame satisfies ebiten.Game.
var _ ebiten.Game = (*RyxGame)(nil)

// ---------------------------------------------------------------------------
// Screen info builtins
// ---------------------------------------------------------------------------

// GfxWidth returns the configured window width.
func GfxWidth(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("gfx_width: expected 0 args, got %d", len(args))
	}
	gs := GetGraphicsState()
	return vm.IntVal(int64(gs.Width)), nil
}

// GfxHeight returns the configured window height.
func GfxHeight(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("gfx_height: expected 0 args, got %d", len(args))
	}
	gs := GetGraphicsState()
	return vm.IntVal(int64(gs.Height)), nil
}

// GfxFPS returns the current frames per second.
func GfxFPS(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("gfx_fps: expected 0 args, got %d", len(args))
	}
	fps := ebiten.ActualFPS()
	return vm.FloatVal(math.Float64frombits(math.Float64bits(fps))), nil
}

// GfxDeltaTime returns the time elapsed since the last frame in seconds.
func GfxDeltaTime(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("gfx_delta_time: expected 0 args, got %d", len(args))
	}
	tps := ebiten.ActualTPS()
	if tps <= 0 {
		return vm.FloatVal(1.0 / 60.0), nil
	}
	return vm.FloatVal(1.0 / tps), nil
}

// RegisterBridgeBuiltins registers the graphics bridge builtins.
func RegisterBridgeBuiltins(r *vm.BuiltinRegistry) {
	r.Register("gfx_width", GfxWidth)
	r.Register("gfx_height", GfxHeight)
	r.Register("gfx_fps", GfxFPS)
	r.Register("gfx_delta_time", GfxDeltaTime)
}
