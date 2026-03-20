# Goal: 2D Graphics Library + Raycaster Demo for Ryx

## Overview

Add an Ebiten-based 2D graphics library to the Ryx language and build a Wolfenstein-style raycaster as the demo program. This is the first external dependency in the project.

**Repository**: `/Users/eren/Documents/ai/attocodepy_swarmtester_11`
**Language**: Go 1.26.1 | **Total LOC**: ~46,400 across 86 Go files
**External deps (current)**: none — Ebiten v2 will be the first

---

## Architecture: Ebiten Bridge

### How it works

Ebiten drives the main game loop. The Ryx VM ticks inside Ebiten's `Update()` callback:

1. `gfx_init(w, h, title)` — stores window config in a `GraphicsState` singleton
2. `gfx_run(callback)` — calls `ebiten.RunGame()`, passing a `RyxGame` struct that implements `ebiten.Game`
3. Each frame: `RyxGame.Update()` invokes the Ryx callback via `CallbackInvoker` (same pattern as `array_map`)
4. Drawing builtins (`gfx_pixel`, `gfx_rect`, etc.) append commands to a draw queue during `Update()`
5. `RyxGame.Draw()` flushes the queue to the Ebiten screen
6. Input builtins read Ebiten key/mouse state (updated before `Update()`)

### Key code references

| What | Where | Notes |
|------|-------|-------|
| CallbackInvoker | `pkg/stdlib/array_ops.go:629-631` | Global `var CallbackInvoker func(fn vm.Value, args []vm.Value, heap *vm.Heap) (vm.Value, error)` — set by VM at startup |
| Builtin registration | `pkg/stdlib/core.go:292-425` | `RegisterAll()` calls `r.Register(name, fn)` for every stdlib function |
| Trig builtins | `pkg/stdlib/math_ops.go:122-175` | `Sin`, `Cos`, `Tan`, `Asin`, `Acos`, `Atan`, `Atan2` — **already exist** |
| Math registration | `pkg/stdlib/core.go:362-388` | All math ops registered including trig, floor, ceil, round |
| Dedicated opcodes | `pkg/codegen/codegen.go:109-121` | `builtinOpcodes` — println, print, type conversions, len, concat |
| OpCallBuiltin names | `pkg/codegen/codegen.go:123-170` | `callBuiltinNames` — everything else (math, string, array, map, I/O) |
| executeProgram bug | `cmd/ryx/main.go:610-624` | Creates `vm.NewVM(compiled)` but **never sets `machine.Builtins`** or calls `stdlib.RegisterAll()`. VM's Builtins field stays nil. |
| VM struct | `pkg/vm/vm.go` | 1,081 lines — `NewVM()` constructor, `Run()`, `Builtins` field |
| Builtin dispatch | `pkg/vm/builtins.go` | 526 lines — `OpCallBuiltin` handler |
| Example syntax | `examples/hello.ryx` | `fn`, `let`, `println()`, `++` for string concat |

### Important: Trig builtins already exist

The original plan called for adding `sin`/`cos`/`tan`/`atan2` etc. — **these already exist** in `math_ops.go` (lines 122-175) and are registered in `core.go` (lines 370-380). Do NOT re-implement them.

---

## Pre-existing Bug Fix (REQUIRED FIRST)

**File**: `cmd/ryx/main.go:610-624`

```go
// CURRENT (broken):
func executeProgram(compiled *codegen.CompiledProgram, cfg *Config) int {
    machine := vm.NewVM(compiled)
    if err := machine.Run(); err != nil { ... }
    return 0
}
```

The VM is created but `machine.Builtins` is never populated. No builtins work from CLI.

**Fix**: Call `stdlib.RegisterAll()` and wire the registry into the VM before `machine.Run()`. Look at how the REPL or test harness initializes the VM for the correct pattern.

---

## Graphics Builtin API (~30 builtins)

All colors are packed RGBA integers (`0xRRGGBBAA`). No new heap types needed.

### Window Management
| Builtin | Signature | Description |
|---------|-----------|-------------|
| `gfx_init` | `(w: Int, h: Int, title: String) -> ()` | Set window size and title |
| `gfx_run` | `(callback: fn() -> ()) -> ()` | Start game loop — calls `ebiten.RunGame()` |
| `gfx_quit` | `() -> ()` | Signal exit |
| `gfx_set_title` | `(title: String) -> ()` | Update window title at runtime |

### Drawing Primitives
| Builtin | Signature | Description |
|---------|-----------|-------------|
| `gfx_clear` | `(color: Int) -> ()` | Fill screen with color |
| `gfx_set_color` | `(color: Int) -> ()` | Set current draw color |
| `gfx_pixel` | `(x: Int, y: Int) -> ()` | Draw single pixel |
| `gfx_line` | `(x1: Int, y1: Int, x2: Int, y2: Int) -> ()` | Draw line |
| `gfx_rect` | `(x: Int, y: Int, w: Int, h: Int) -> ()` | Draw rectangle outline |
| `gfx_fill_rect` | `(x: Int, y: Int, w: Int, h: Int) -> ()` | Draw filled rectangle |
| `gfx_circle` | `(cx: Int, cy: Int, r: Int) -> ()` | Draw circle outline |
| `gfx_fill_circle` | `(cx: Int, cy: Int, r: Int) -> ()` | Draw filled circle |
| `gfx_text` | `(x: Int, y: Int, s: String) -> ()` | Draw text string |

### Color Helpers
| Builtin | Signature | Description |
|---------|-----------|-------------|
| `gfx_rgb` | `(r: Int, g: Int, b: Int) -> Int` | Pack RGB to `0xRRGGBBFF` |
| `gfx_rgba` | `(r: Int, g: Int, b: Int, a: Int) -> Int` | Pack RGBA to `0xRRGGBBAA` |
| `COLOR_BLACK` | `() -> Int` | `0x000000FF` |
| `COLOR_WHITE` | `() -> Int` | `0xFFFFFFFF` |
| `COLOR_RED` | `() -> Int` | `0xFF0000FF` |
| `COLOR_GREEN` | `() -> Int` | `0x00FF00FF` |
| `COLOR_BLUE` | `() -> Int` | `0x0000FFFF` |
| `COLOR_YELLOW` | `() -> Int` | `0xFFFF00FF` |

### Input
| Builtin | Signature | Description |
|---------|-----------|-------------|
| `gfx_key_pressed` | `(key: Int) -> Bool` | Is key currently held? |
| `gfx_key_just_pressed` | `(key: Int) -> Bool` | Was key pressed this frame? |
| `gfx_mouse_x` | `() -> Int` | Current mouse X position |
| `gfx_mouse_y` | `() -> Int` | Current mouse Y position |
| `gfx_mouse_pressed` | `(button: Int) -> Bool` | Is mouse button pressed? |

### Key Constants
| Builtin | Returns | Ebiten mapping |
|---------|---------|---------------|
| `KEY_UP` | `() -> Int` | `ebiten.KeyArrowUp` |
| `KEY_DOWN` | `() -> Int` | `ebiten.KeyArrowDown` |
| `KEY_LEFT` | `() -> Int` | `ebiten.KeyArrowLeft` |
| `KEY_RIGHT` | `() -> Int` | `ebiten.KeyArrowRight` |
| `KEY_SPACE` | `() -> Int` | `ebiten.KeySpace` |
| `KEY_ESCAPE` | `() -> Int` | `ebiten.KeyEscape` |
| `KEY_ENTER` | `() -> Int` | `ebiten.KeyEnter` |
| `KEY_W` | `() -> Int` | `ebiten.KeyW` |
| `KEY_A` | `() -> Int` | `ebiten.KeyA` |
| `KEY_S` | `() -> Int` | `ebiten.KeyS` |
| `KEY_D` | `() -> Int` | `ebiten.KeyD` |

### Screen Info
| Builtin | Signature | Description |
|---------|-----------|-------------|
| `gfx_width` | `() -> Int` | Current window width |
| `gfx_height` | `() -> Int` | Current window height |
| `gfx_fps` | `() -> Float` | Current FPS |

---

## New & Modified Files

| File | Action | Est. Lines | Description |
|------|--------|-----------|-------------|
| `pkg/stdlib/graphics_bridge.go` | Create | ~300 | `GraphicsState` singleton, `RyxGame` implementing `ebiten.Game`, draw command queue |
| `pkg/stdlib/graphics.go` | Create | ~400 | Window management + drawing primitive builtins |
| `pkg/stdlib/graphics_input.go` | Create | ~200 | Input builtins + key constants |
| `pkg/stdlib/graphics_colors.go` | Create | ~100 | `gfx_rgb`, `gfx_rgba`, named color constants |
| `pkg/stdlib/graphics_test.go` | Create | ~300 | Unit tests with mock CallbackInvoker |
| `pkg/stdlib/core.go` | Modify | +~5 | Add `RegisterGraphics()` call in `RegisterAll()` |
| `pkg/codegen/codegen.go` | Modify | +~35 | Add all graphics builtin names to `callBuiltinNames` |
| `cmd/ryx/main.go` | Modify | +~5 | Fix `executeProgram()` — wire `stdlib.RegisterAll()` |
| `examples/raycaster.ryx` | Create | ~400 | Wolfenstein-style raycaster demo |
| `examples/graphics_hello.ryx` | Create | ~50 | Minimal smoke test |
| `go.mod`, `go.sum` | Modify | — | Add `github.com/hajimehoshi/ebiten/v2` |

---

## Task Decomposition (4 Waves, 14 Tasks)

### Wave 1 — Foundation (parallel, no deps)

**T1: Add Ebiten dependency**
- `go get github.com/hajimehoshi/ebiten/v2@latest`
- Verify `go build ./...` still passes
- Acceptance: `go.mod` lists ebiten, project compiles

**T2: Graphics bridge core (`graphics_bridge.go`)**
- `GraphicsState` struct: window config, draw command queue, current color, running flag
- `RyxGame` struct implementing `ebiten.Game` interface (`Update`, `Draw`, `Layout`)
- Draw command types: `ClearCmd`, `PixelCmd`, `LineCmd`, `RectCmd`, `FillRectCmd`, `CircleCmd`, `FillCircleCmd`, `TextCmd`
- `Update()` invokes Ryx frame callback via `CallbackInvoker` (see `array_ops.go:629-631`)
- `Draw()` iterates command queue → Ebiten draw calls → clears queue
- Acceptance: compiles, `RyxGame` satisfies `ebiten.Game` interface

**T3: Color helpers (`graphics_colors.go`)**
- Pure functions, no Ebiten import needed
- `gfx_rgb(r,g,b)` → packs to `0xRRGGBBFF`
- `gfx_rgba(r,g,b,a)` → packs to `0xRRGGBBAA`
- Named constants: `COLOR_BLACK` through `COLOR_YELLOW`
- Acceptance: unit tests pass for color packing/unpacking

**T4: Fix executeProgram bug (`cmd/ryx/main.go`)**
- Line 610-624: after `vm.NewVM(compiled)`, wire in stdlib builtins
- Look at REPL/test harness for the correct initialization pattern
- Acceptance: `./ryx run examples/hello.ryx` prints output (builtins work)

### Wave 2 — Builtins (depends on T2)

**T5: Window management builtins (`graphics.go` — part 1)**
- `gfx_init(w, h, title)` — stores config in `GraphicsState`
- `gfx_run(callback)` — calls `ebiten.RunGame(&RyxGame{...})`
- `gfx_quit()` — sets exit flag
- `gfx_set_title(s)` — calls `ebiten.SetWindowTitle()`
- Acceptance: `gfx_init` + `gfx_run` opens a window

**T6: Drawing primitives (`graphics.go` — part 2)**
- `gfx_clear`, `gfx_set_color`, `gfx_pixel`, `gfx_line`, `gfx_rect`, `gfx_fill_rect`, `gfx_circle`, `gfx_fill_circle`, `gfx_text`
- Each appends a command to the draw queue
- Use `ebiten/v2/vector` for line/rect/circle rendering
- Acceptance: can draw shapes on screen

**T7: Input builtins (`graphics_input.go`)**
- `gfx_key_pressed`, `gfx_key_just_pressed` — wrap `ebiten.IsKeyPressed` / `inpututil.IsKeyJustPressed`
- `gfx_mouse_x`, `gfx_mouse_y`, `gfx_mouse_pressed`
- Key constants: `KEY_UP`, `KEY_DOWN`, `KEY_LEFT`, `KEY_RIGHT`, `KEY_SPACE`, `KEY_ESCAPE`, `KEY_ENTER`, `KEY_W/A/S/D`
- Acceptance: can detect keyboard/mouse input

**T8: Screen info builtins**
- `gfx_width()`, `gfx_height()` — return from `GraphicsState`
- `gfx_fps()` — `ebiten.ActualFPS()` as float
- Acceptance: returns correct values during game loop

### Wave 3 — Registration & Testing (depends on T5-T8)

**T9: Register all graphics builtins**
- Add all ~30 graphics builtin names to `callBuiltinNames` in `codegen.go:123-170`
- Add `RegisterGraphics()` function and call it from `RegisterAll()` in `core.go`
- Acceptance: `go build ./...` passes, graphics builtins recognized by compiler

**T10: Unit tests (`graphics_test.go`)**
- Test color packing/unpacking
- Test draw command queue (append, flush)
- Test input state reads with mocked Ebiten state where possible
- Acceptance: `go test ./pkg/stdlib/ -run Graphics` passes

**T11: Smoke test example (`examples/graphics_hello.ryx`)**
- ~50 lines: open window, draw colored shapes, respond to key presses, show FPS
- Exercises: `gfx_init`, `gfx_run`, `gfx_clear`, `gfx_fill_rect`, `gfx_circle`, `gfx_key_pressed`, `gfx_text`
- Acceptance: compiles and type-checks; window opens and renders when run

### Wave 4 — Demo & Polish (depends on T9-T11)

**T12: Raycaster demo (`examples/raycaster.ryx`)**
- See "Raycaster Spec" section below
- ~400 lines of Ryx
- Acceptance: compiles, renders 3D perspective view with WASD movement

**T13: Integration tests**
- Compile + type-check all graphics examples (can run headless)
- Verify no regressions in existing test suite
- Acceptance: `go test ./...` passes (graphics runtime tests skipped in CI)

**T14: Polish**
- FPS overlay in raycaster
- Distance fog (darken walls by distance)
- Wall color variation (map values 1-4 → different colors)
- Minimap in top-left corner
- Acceptance: raycaster looks good, runs at 60 FPS

---

## Raycaster Demo Spec (`examples/raycaster.ryx`)

### Constants & Map

```
SCREEN_W = 640, SCREEN_H = 480
MAP_W = 16, MAP_H = 16
TILE_SIZE = 32 (for minimap)

map = [                          // 16x16 grid, 0=empty, 1-4=wall colors
  1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,
  1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
  1,0,2,2,0,0,0,0,0,0,0,3,3,0,0,1,
  1,0,2,0,0,0,0,0,0,0,0,0,3,0,0,1,
  1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
  1,0,0,0,0,4,4,0,0,4,4,0,0,0,0,1,
  1,0,0,0,0,4,0,0,0,0,4,0,0,0,0,1,
  1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
  1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
  1,0,0,0,0,4,0,0,0,0,4,0,0,0,0,1,
  1,0,0,0,0,4,4,0,0,4,4,0,0,0,0,1,
  1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
  1,0,3,0,0,0,0,0,0,0,0,0,2,0,0,1,
  1,0,3,3,0,0,0,0,0,0,0,2,2,0,0,1,
  1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
  1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1
]
```

### Player State

```
pos_x, pos_y = 8.5, 8.5       // center of map
dir_x, dir_y = -1.0, 0.0      // direction vector (facing north)
plane_x, plane_y = 0.0, 0.66  // camera plane (FOV ~66°)
move_speed = 0.05
rot_speed = 0.03
```

### DDA Raycasting (per-column)

```
for x in 0..SCREEN_W:
    // 1. Calculate ray direction
    camera_x = 2.0 * x / SCREEN_W - 1.0
    ray_dir_x = dir_x + plane_x * camera_x
    ray_dir_y = dir_y + plane_y * camera_x

    // 2. Which map cell and step direction
    map_x = floor(pos_x)
    map_y = floor(pos_y)
    delta_dist_x = abs(1.0 / ray_dir_x)  // (guard div-by-zero)
    delta_dist_y = abs(1.0 / ray_dir_y)

    // 3. Initial side distances
    if ray_dir_x < 0: step_x = -1; side_dist_x = (pos_x - map_x) * delta_dist_x
    else:              step_x =  1; side_dist_x = (map_x + 1.0 - pos_x) * delta_dist_x
    // same for Y

    // 4. DDA loop — step until wall hit
    while map[map_y * MAP_W + map_x] == 0:
        if side_dist_x < side_dist_y:
            side_dist_x += delta_dist_x; map_x += step_x; side = 0
        else:
            side_dist_y += delta_dist_y; map_y += step_y; side = 1

    // 5. Perpendicular distance (no fisheye)
    if side == 0: perp_dist = (map_x - pos_x + (1 - step_x) / 2) / ray_dir_x
    else:         perp_dist = (map_y - pos_y + (1 - step_y) / 2) / ray_dir_y

    // 6. Wall strip height
    line_height = floor(SCREEN_H / perp_dist)
    draw_start = max(0, SCREEN_H / 2 - line_height / 2)
    draw_end   = min(SCREEN_H - 1, SCREEN_H / 2 + line_height / 2)

    // 7. Color by wall type + distance shading
    wall_type = map[map_y * MAP_W + map_x]
    color = wall_color(wall_type)       // 1=red, 2=green, 3=blue, 4=yellow
    if side == 1: color = darken(color) // Y-side walls darker
    shade = clamp(1.0 - perp_dist / 16.0, 0.2, 1.0)  // distance fog

    // 8. Draw vertical strip
    gfx_set_color(apply_shade(color, shade))
    gfx_line(x, draw_start, x, draw_end)
```

### Movement

```
// WASD or arrow keys
if gfx_key_pressed(KEY_W()):   // move forward
    new_x = pos_x + dir_x * move_speed
    new_y = pos_y + dir_y * move_speed
    if map[floor(new_y) * MAP_W + floor(new_x)] == 0:
        pos_x = new_x; pos_y = new_y

if gfx_key_pressed(KEY_A()):   // strafe left
    ...

// Arrow keys rotate (or LEFT/RIGHT with WASD)
if gfx_key_pressed(KEY_LEFT()):
    old_dir_x = dir_x
    dir_x = dir_x * cos(rot_speed) - dir_y * sin(rot_speed)
    dir_y = old_dir_x * sin(rot_speed) + dir_y * cos(rot_speed)
    // rotate camera plane by same angle
```

### Minimap

```
// Top-left corner, 4px per tile
for ty in 0..MAP_H:
    for tx in 0..MAP_W:
        if map[ty * MAP_W + tx] > 0:
            gfx_fill_rect(tx * 4, ty * 4, 4, 4)  // wall
// Player dot
gfx_set_color(COLOR_RED())
gfx_fill_rect(floor(pos_x * 4) - 1, floor(pos_y * 4) - 1, 3, 3)
```

### Ryx Features Exercised

- Closures (frame callback)
- Arrays (map data, color tables)
- Float math (`sqrt`, `abs`, trig via existing builtins)
- Integer math (map indexing, screen coords)
- Loops and conditionals
- `let` bindings with mutation
- String operations (FPS display)

---

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Ebiten requires main-thread on macOS | `gfx_run()` calls `ebiten.RunGame()` from main goroutine — this is Ebiten's expected usage |
| CI/headless environments | Limit CI tests to compile + type-check; skip runtime graphics tests with build tag |
| Shared file conflicts (`core.go`, `codegen.go`) | T9 is sole owner of registration, sequenced after all builtins are implemented |
| First external dependency | Ebiten v2 is mature, pure Go (no CGO by default), widely used |
| Raycaster complexity for swarm agents | DDA algorithm pseudocode is provided above; well-documented algorithm |
| `CallbackInvoker` may not support closures | Test with `array_map` + closure first; same mechanism |
| Draw queue memory | Clear queue every frame in `Draw()`; typical frame has <1000 commands |

---

## Build & Verification Commands

```bash
# After all tasks complete:
go build ./cmd/ryx                                    # compiles with Ebiten
go test ./pkg/stdlib/ -run Graphics                   # unit tests
go test ./pkg/stdlib/ -run TestColor                  # color packing tests
go test ./... -short                                  # full suite (graphics runtime skipped)
./ryx run examples/graphics_hello.ryx                 # smoke test — window opens
./ryx run examples/raycaster.ryx                      # demo — Wolfenstein view + WASD
```
