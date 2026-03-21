# Goal: Bug Fixes, Image Support & Play Mode for Ryx Graphics

## Overview

Fix critical bugs in the graphics/closure subsystem, add PNG image loading and drawing, and build a basic play mode for the raycaster. Also create an image viewer example suitable for wall display.

**Repository**: `/Users/eren/Documents/ai/attocodepy_swarmtester_11`
**Language**: Go 1.26.1 | **External deps**: `github.com/hajimehoshi/ebiten/v2 v2.9.9`

---

## Bug #1: Mutable Closure Captures Reset After Callback Return

### Symptom

In the raycaster (`examples/raycaster.ryx`), the player position snaps back to the starting position (8.5, 8.5) every time movement keys are released. Mutable variables captured by the frame closure (`pos_x`, `pos_y`, `dir_x`, `dir_y`, `plane_x`, `plane_y`) lose their mutations between callback invocations.

### Root Cause

**File**: `pkg/mir/builder.go`, line 504-509 (`hir.Assign` handler)

When `pos_x = nx` is executed inside an `if` block within the closure body, the MIR builder's SSA construction breaks the upvalue linkage:

1. At closure entry (`buildLambdaFunction`, line 946-949), `pos_x` is registered as local L0 with `UpvalueAlias = 0`. Loads/stores of L0 correctly emit `OpLoadUpvalue`/`OpStoreUpvalue`.

2. Inside `if gfx_key_pressed(KEY_W()) { ... pos_x = nx ... }`, the `hir.Assign` handler (line 506) calls `b.fn.NewLocal(e.Name, ...)` which creates a **new** local L1 with `UpvalueAlias = -1` (the default from `NewLocal` at `mir.go:506`).

3. At the `if.merge` join point, SSA inserts a phi: `L2 = phi(then: L1, else: L0)`. L2 also gets `UpvalueAlias = -1`.

4. Codegen (`codegen.go:829-837`) checks `UpvalueAlias` to decide between `OpStoreUpvalue` and `OpStoreLocal`. Since L1 and L2 have alias `-1`, mutations go to **ephemeral stack slots** that are destroyed when the closure returns.

5. Next frame, `InvokeCallback` re-enters the closure. The upvalue cells still hold the original values.

### Fix Strategy

In the `hir.Assign` case (`builder.go:504-509`), after creating the new local, look up the current definition of the variable and propagate its `UpvalueAlias`:

```go
case *hir.Assign:
    val := b.lowerExpr(e.Value)
    local := b.fn.NewLocal(e.Name, e.Value.ExprType())
    // Propagate UpvalueAlias from the current definition so mutations
    // go to the closure cell, not ephemeral stack slots.
    if curVal := b.readVariable(e.Name, b.curBlock); curVal != nil {
        if ref, ok := curVal.(*Local); ok {
            if alias := b.fn.Locals[int(ref.ID)].UpvalueAlias; alias >= 0 {
                b.fn.Locals[int(local)].UpvalueAlias = alias
            }
        }
    }
    b.emit(&Assign{Dest: local, Src: val, Type: e.Value.ExprType()})
    b.writeVariable(e.Name, b.curBlock, local)
    return UnitConst()
```

Also in `readVariableRecursive` (line 164-214), when creating a phi dest, propagate `UpvalueAlias` if all non-trivial phi operands agree on the same alias.

### Key Code References

| What | Where | Notes |
|------|-------|-------|
| `hir.Assign` handler | `pkg/mir/builder.go:504-509` | Creates new SSA local without alias — **the bug** |
| `buildLambdaFunction` | `pkg/mir/builder.go:916-950` | Sets `UpvalueAlias` on initial capture locals |
| `NewLocal` default | `pkg/mir/mir.go:506` | `UpvalueAlias: -1` by default |
| `readVariableRecursive` | `pkg/mir/builder.go:164-214` | Creates phi dest locals — needs alias propagation |
| `emitStoreLocal` | `pkg/codegen/codegen.go:829-837` | Dispatches to `OpStoreUpvalue` if `UpvalueAlias >= 0` |
| `emitLoadLocal` | `pkg/codegen/codegen.go:818-826` | Dispatches to `OpLoadUpvalue` if `UpvalueAlias >= 0` |
| `OpMakeClosure` | `pkg/vm/vm.go:680-692` | Creates closed `UpvalueCell` — correct, not the bug |
| `UpvalueCell.Set` | `pkg/vm/value.go:306-311` | Updates closed value — correct |
| `InvokeCallback` | `pkg/vm/callback.go:12-81` | Reuses same closure object — correct |

---

## Bug #2: Movement Quality

The raycaster uses fixed `move_speed = 0.12` per frame. At variable frame rates, movement speed fluctuates. After fixing Bug #1, add delta-time-based movement via a new `gfx_delta_time()` builtin.

---

## New & Modified Files

| File | Action | Est. Lines | Description |
|------|--------|-----------|-------------|
| `pkg/mir/builder.go` | Modify | +~20 | Propagate `UpvalueAlias` in `hir.Assign` and phi construction |
| `pkg/mir/mir_test.go` | Modify | +~40 | Unit test for `UpvalueAlias` propagation through assignments and phis |
| `pkg/stdlib/graphics_bridge.go` | Modify | +~15 | Add `gfx_delta_time()` builtin |
| `pkg/stdlib/graphics_image.go` | Create | ~200 | `ImageStore`, `gfx_load_image`, `gfx_draw_image`, `gfx_draw_image_scaled`, `gfx_image_width`, `gfx_image_height` |
| `pkg/stdlib/graphics_test.go` | Modify | +~60 | Image builtin tests |
| `pkg/stdlib/core.go` | Modify | +~5 | Register image builtins |
| `pkg/codegen/codegen.go` | Modify | +~10 | Add new builtin names to `callBuiltinNames` |
| `examples/raycaster.ryx` | Modify | +~120 | Delta-time movement, items, HUD, start/game-over screens |
| `examples/image_viewer.ryx` | Create | ~60 | PNG viewer for wall display |
| `tests/testdata/programs/closure_mut_capture.ryx` | Create | ~30 | E2E test for mutable closure captures |
| `tests/testdata/programs/closure_mut_capture.expected` | Create | ~5 | Expected output |
| `tests/integration/e2e_test.go` | Modify | +~10 | Verify new examples compile |

---

## Graphics Image API

All images are managed via integer handles. The `ImageStore` is a global slice of `*ebiten.Image` protected by a mutex (same pattern as `GraphicsState`).

### New Builtins

| Builtin | Signature | Description |
|---------|-----------|-------------|
| `gfx_delta_time` | `() -> Float` | Frame delta in seconds (`1.0 / ebiten.ActualTPS()`) |
| `gfx_load_image` | `(path: String) -> Int` | Load PNG, return handle ID (or -1 on error) |
| `gfx_draw_image` | `(handle: Int, x: Int, y: Int) -> ()` | Draw image at (x,y) |
| `gfx_draw_image_scaled` | `(handle: Int, x: Int, y: Int, sx: Float, sy: Float) -> ()` | Draw scaled image |
| `gfx_image_width` | `(handle: Int) -> Int` | Get image width |
| `gfx_image_height` | `(handle: Int) -> Int` | Get image height |

### Draw Command

```go
type DrawImageCmd struct {
    Handle int
    X, Y   int
    ScaleX, ScaleY float64
}
```

`DrawImageCmd.Execute(screen)` looks up the image from `ImageStore` by handle, builds an `ebiten.DrawImageOptions` with translation (and scale if not 1.0), and calls `screen.DrawImage()`.

---

## Task Decomposition (4 Waves, 10 Tasks)

### Wave 1 — Bug Fixes

**T1: Fix mutable closure capture persistence**
- Modify `pkg/mir/builder.go`:
  - In `hir.Assign` handler (line 504-509): before creating the new local, read the current definition of the variable. If it resolves to a local with `UpvalueAlias >= 0`, propagate that alias to the newly created local.
  - In `readVariableRecursive` (line 164-214): when creating a phi dest for multiple predecessors, check if all non-trivial phi operands resolve to locals with the same `UpvalueAlias`. If so, propagate that alias to the phi dest local.
- Add test in `pkg/mir/mir_test.go`: verify `UpvalueAlias` propagation through SSA assignments inside conditionals.
- Create `tests/testdata/programs/closure_mut_capture.ryx`: a closure that captures `let mut x = 0`, increments `x` inside an `if true { x = x + 1 }`, called 3 times in a loop via a helper that simulates repeated invocation. Prints `x` each time.
- Create `tests/testdata/programs/closure_mut_capture.expected` with expected output `1\n2\n3\n`.
- Read files for context: `pkg/mir/builder.go`, `pkg/mir/mir.go` (LocalDef struct), `pkg/codegen/codegen.go` (emitStoreLocal/emitLoadLocal), `pkg/vm/value.go` (UpvalueCell), `pkg/vm/callback.go`
- Acceptance: `go test ./...` passes, mutable captures persist across closure calls.

**T2: Improve movement quality with delta-time** (depends on T1)
- Add `gfx_delta_time()` builtin to `pkg/stdlib/graphics_bridge.go`: returns `1.0 / ebiten.ActualTPS()`.
- Register in `pkg/stdlib/core.go` inside `RegisterBridgeBuiltins()`.
- Add `"gfx_delta_time"` to `callBuiltinNames` in `pkg/codegen/codegen.go`.
- Update `examples/raycaster.ryx`:
  - Replace `let move_speed = 0.12` / `let rot_speed = 0.03` with base speeds: `let base_move = 4.0` / `let base_rot = 2.0` (units per second).
  - In frame callback: `let dt = gfx_delta_time()`, then `let move_speed = base_move * dt`, `let rot_speed = base_rot * dt`.
- Read files for context: `pkg/stdlib/graphics_bridge.go`, `pkg/stdlib/core.go`, `pkg/codegen/codegen.go`, `examples/raycaster.ryx`
- Acceptance: `go build ./...` passes, raycaster movement is smooth and frame-rate independent.

### Wave 2 — Image/Sprite Support (T3 & T4 parallel, T5 depends on both)

**T3: Add `gfx_load_image` builtin (PNG loading)**
- Create `pkg/stdlib/graphics_image.go`:
  - `ImageStore` struct: `images []*ebiten.Image`, protected by `sync.Mutex`.
  - Global `var imageStore = &ImageStore{}`.
  - `GfxLoadImage(args, heap)`: takes String path, opens file, decodes PNG via `image/png`, converts via `ebiten.NewImageFromImage()`, appends to `imageStore.images`, returns `IntVal(handleID)`. Returns `IntVal(-1)` on error.
  - `GfxImageWidth(args, heap)` / `GfxImageHeight(args, heap)`: given handle, return dimensions.
  - `RegisterImageBuiltins(r *vm.BuiltinRegistry)`: registers `gfx_load_image`, `gfx_image_width`, `gfx_image_height`.
- Add `"gfx_load_image"`, `"gfx_image_width"`, `"gfx_image_height"` to `callBuiltinNames` in `pkg/codegen/codegen.go`.
- Read files for context: `pkg/stdlib/graphics_bridge.go` (pattern for GraphicsState), `pkg/stdlib/graphics_draw.go` (pattern for builtins), `pkg/stdlib/core.go` (registration pattern), `pkg/codegen/codegen.go` (callBuiltinNames)
- Acceptance: `go build ./...` passes, `gfx_load_image` loads a PNG and returns valid handle.

**T4: Add `gfx_draw_image` and `gfx_draw_image_scaled` builtins**
- Add to `pkg/stdlib/graphics_image.go`:
  - `DrawImageCmd` struct implementing `DrawCommand` interface: fields `Handle int`, `X int`, `Y int`, `ScaleX float64`, `ScaleY float64`.
  - `DrawImageCmd.Execute(screen *ebiten.Image)`: looks up image from `imageStore` by handle, creates `ebiten.DrawImageOptions` with `GeoM.Translate` and optional `GeoM.Scale`, calls `screen.DrawImage()`.
  - `GfxDrawImage(args, heap)`: `(handle: Int, x: Int, y: Int) -> ()`. Creates `DrawImageCmd{ScaleX: 1.0, ScaleY: 1.0}` and appends to draw queue.
  - `GfxDrawImageScaled(args, heap)`: `(handle: Int, x: Int, y: Int, sx: Float, sy: Float) -> ()`. Creates `DrawImageCmd` with given scale.
  - Register both in `RegisterImageBuiltins`.
- Add `"gfx_draw_image"`, `"gfx_draw_image_scaled"` to `callBuiltinNames` in `pkg/codegen/codegen.go`.
- Read files for context: `pkg/stdlib/graphics_bridge.go` (DrawCommand interface, AppendCommand pattern), `pkg/stdlib/graphics_draw.go` (existing draw command implementations like FillRectCmd)
- Acceptance: `go build ./...` passes, drawing images works.

**T5: Register image builtins and add tests** (depends on T3, T4)
- Add `RegisterImageBuiltins(r)` call in `RegisterGraphics()` in `pkg/stdlib/core.go`.
- Add tests in `pkg/stdlib/graphics_test.go`:
  - `TestGraphics_ImageStore`: create a small `image.RGBA` programmatically, convert to `ebiten.Image`, verify handle and dimensions.
  - `TestGraphics_DrawImageCmd`: verify `DrawImageCmd` implements `DrawCommand`.
- Verify: `go test ./pkg/stdlib/ -run Image` passes, `go test ./...` passes.
- Read files for context: `pkg/stdlib/core.go` (RegisterGraphics), `pkg/stdlib/graphics_test.go` (existing test patterns)
- Acceptance: all tests pass.

### Wave 3 — Play Mode (T6 first, then T7 & T8 parallel)

**T6: Add collectible/item system to raycaster** (depends on T1, T2)
- Modify `examples/raycaster.ryx`:
  - Add `let mut items = [...]` array (16x16, 1 = collectible present). Place 6-8 items in open spaces.
  - Add `let mut score = 0` and `let mut total_items = <count>` as captured mutable vars.
  - In frame callback after movement: check `items[floor(pos_y) * 16 + floor(pos_x)]`. If 1, set to 0, increment `score`.
  - On minimap: draw small colored dots (cyan) for uncollected items.
- Read files for context: `examples/raycaster.ryx` (current state)
- Acceptance: walking over items collects them, score increments, items visible on minimap.

**T7: Add score display and HUD overlay** (depends on T6)
- Modify `examples/raycaster.ryx`:
  - Draw semi-transparent HUD bar at top: `gfx_set_color(gfx_rgba(0, 0, 0, 160))`, `gfx_fill_rect(0, 0, screen_w, 30)`.
  - Display score: `gfx_text(10, 8, "Score: " ++ int_to_string(score))`.
  - Display items remaining: compute `remaining` from items array, show `"Items: " ++ int_to_string(remaining) ++ "/" ++ int_to_string(total_items)`.
  - Move FPS display into the HUD bar area.
- Read files for context: `examples/raycaster.ryx`
- Acceptance: HUD shows score, items remaining, FPS. Updates in real time.

**T8: Add start screen and game-over screen** (depends on T6)
- Modify `examples/raycaster.ryx`:
  - Add `let mut game_state = 0` (0 = start, 1 = playing, 2 = win).
  - Frame callback: branch on `game_state`.
  - State 0 (start screen): black background, centered title "RYX RAYCASTER" in white, "Press ENTER to start" below. On `gfx_key_just_pressed(KEY_ENTER())`: set `game_state = 1`.
  - State 1 (playing): existing raycaster logic. When `score == total_items`: set `game_state = 2`.
  - State 2 (win screen): "ALL ITEMS COLLECTED!" title, "Score: N" display, "Press ENTER to restart". On ENTER: reset `pos_x`/`pos_y`/`dir`/`plane`/`score`/`items` to initial values, set `game_state = 0`.
- Read files for context: `examples/raycaster.ryx`
- Acceptance: game has title screen, gameplay, and win screen with restart.

### Wave 4 — Image Viewer & Polish (T9 & T10 parallel)

**T9: Create `examples/image_viewer.ryx`** (depends on T5)
- Create `examples/image_viewer.ryx`:
  - Load a PNG: `let img = gfx_load_image("examples/assets/sample.png")`.
  - Get dimensions: `let iw = gfx_image_width(img)`, `let ih = gfx_image_height(img)`.
  - Init window: `gfx_init(800, 600, "Ryx Image Viewer")`.
  - Frame callback: clear screen black, draw image centered (`gfx_draw_image(img, 400 - iw/2, 300 - ih/2)`), ESC to quit.
  - Add zoom support: `let mut zoom = 1.0`. +/- keys adjust zoom. Use `gfx_draw_image_scaled` when zoom != 1.0.
- Create a small sample PNG at `examples/assets/sample.png` (can be programmatically generated as a 64x64 colorful pattern if no real asset available — or document that users should place their own).
- Add `KEY_PLUS` and `KEY_MINUS` key constants in `pkg/stdlib/graphics_input.go` if not already present, register in `pkg/codegen/codegen.go`.
- Read files for context: `pkg/stdlib/graphics_input.go` (existing key constants), `pkg/stdlib/graphics_image.go` (image API)
- Acceptance: `./ryx run examples/image_viewer.ryx` opens window showing the image. ESC quits.

**T10: Integration tests and polish** (depends on all)
- Add compile-only tests in `tests/integration/e2e_test.go` for `examples/raycaster.ryx` and `examples/image_viewer.ryx` (headless, no display needed).
- Create `tests/testdata/programs/closure_mut_capture.ryx` if not already created in T1: ensure it uses `array_map` or a loop to invoke the closure multiple times, printing accumulated state.
- Create corresponding `.expected` file.
- Add codegen test in `pkg/codegen/codegen_test.go`: compile a closure with mutable capture assigned inside `if`, verify `OpStoreUpvalue` is emitted (not `OpStoreLocal`).
- Run full suite: `go test ./...`.
- Read files for context: `tests/integration/e2e_test.go`, `pkg/codegen/codegen_test.go`
- Acceptance: `go test ./...` all green. No regressions.

---

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| UpvalueAlias fix could break non-closure SSA assignments | Only propagate alias when the current def already has one; non-closure locals have alias -1 and are untouched |
| Nested ifs/while inside closure may need multiple propagation hops | The `readVariable` chain recursively resolves through blocks; alias propagation at each `Assign` site ensures the chain is maintained |
| Phi nodes with mixed upvalue/non-upvalue operands | Only propagate if ALL non-trivial operands agree; mixed case can't happen with Ryx's capture semantics |
| Image loading requires filesystem access | Return -1 handle on error instead of panicking; test with programmatic images |
| Raycaster grows complex with play mode | Changes are additive: new state var + conditional branches; existing logic wrapped in `if game_state == 1` |
| `ebiten.ActualTPS()` returns 0 on first frame | Guard: `if dt <= 0.0 { dt = 1.0 / 60.0 }` |

---

## Build & Verification Commands

```bash
# After Wave 1:
go test ./pkg/mir/ -run UpvalueAlias          # Unit test for alias propagation
go test ./tests/integration/ -run E2E         # E2E including new closure test
go test ./...                                  # Full regression

# After Wave 2:
go test ./pkg/stdlib/ -run Image              # Image builtin tests
go build ./...                                 # Compiles with new builtins

# After Wave 3:
./ryx run examples/raycaster.ryx              # Play mode demo (manual)
./ryx check examples/raycaster.ryx            # Type-check only (CI)

# After Wave 4:
./ryx run examples/image_viewer.ryx           # Image viewer (manual)
go test ./...                                  # Everything green
```
