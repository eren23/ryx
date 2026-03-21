# Goal: Dungeon Crawler Game Example for Ryx

## Overview

Build a top-down dungeon crawler / roguelike-lite game as a single-file Ryx program (`examples/dungeon_crawler.ryx`) that comprehensively demonstrates the language's graphics, math, array, closure, and control-flow capabilities. The game features tile-based rendering, multiple enemy types with simple AI, combat, items, particle effects, a minimap, multiple levels, and full game-state management (title, play, game-over, win screens). A compile-only integration test is added to verify the program compiles without errors.

**Repository**: `/Users/eren/Documents/ai/attocodepy_swarmtester_11`
**Language**: Go 1.26.1 | **External deps**: `github.com/hajimehoshi/ebiten/v2 v2.9.9`

### What Success Looks Like

After this project completes:
- `./ryx run examples/dungeon_crawler.ryx` opens a 640x480 window displaying a title screen
- Pressing ENTER starts the game: player navigates a tile-based dungeon, fights enemies, collects items
- Multiple levels (3 hardcoded maps) with increasing difficulty
- Game-over screen on player death, win screen on clearing all levels
- The example is approximately 1200-1600 lines of Ryx, demonstrating nearly every available builtin
- `go test ./tests/integration/ -run DungeonCrawler` passes (compile-only test)
- All existing tests continue to pass: `go test ./...`

### Language Constraints (Critical for Workers)

Ryx has **no module/import system**. The entire game MUST be a single `.ryx` file. Workers implement logical sections (delineated by comment banners) within this single file.

Ryx has **no classes or objects** in the OOP sense. State management uses:
- `let mut` variables captured by closures for mutable game state
- Flat arrays indexed by integer offsets for entity data (parallel-array pattern)
- Helper functions that take arrays + indices as arguments
- Structs for immutable data definitions (if needed, but arrays are preferred for runtime mutation)

Key Ryx syntax reminders:
- String concatenation: `++` (NOT `+`)
- Mutable variables: `let mut x = 5;`
- Array creation: `[1, 2, 3]`
- Array indexing: `arr[i]`
- Array element mutation: `arr[i] = value`
- No negative number literals: use `0 - value` or `0.0 - value`
- Closures: `|| { body }` or `|x: Int| -> Int { body }`
- No `for` loops with ranges in closures; use `while` loops: `let mut i = 0; while i < n { ...; i = i + 1 };`
- All blocks must produce a value; use `()` or a trailing expression
- Semicolons separate expressions within blocks (`;` after `if`/`while` blocks)
- Calling zero-arg builtins still requires parens: `COLOR_RED()`, `KEY_SPACE()`, `gfx_fps()`
- Boolean operators: `&&`, `||`, `!`
- No unary negation for floats in some contexts: write `0.0 - x` instead of `-x`
- Function definitions go OUTSIDE `main()`, before it
- The frame callback closure and all mutable game state go INSIDE `main()`

---

## Architecture: Entity-Component via Parallel Arrays

Since Ryx has no classes or objects for runtime mutation, entities (player, enemies, particles, items) are stored as **parallel flat arrays** where each array holds one attribute, and entities are identified by integer index.

### Enemy Storage Pattern

```ryx
// Enemy attributes — parallel arrays, max 32 enemies
let max_enemies = 32;
let mut enemy_x     = [0.0, 0.0, 0.0, ...];  // 32 floats: x position
let mut enemy_y     = [0.0, 0.0, 0.0, ...];  // 32 floats: y position
let mut enemy_type  = [0, 0, 0, ...];         // 32 ints: 0=none, 1=slime, 2=skeleton, 3=ghost, 4=boss
let mut enemy_hp    = [0, 0, 0, ...];         // 32 ints: hit points
let mut enemy_alive = [0, 0, 0, ...];         // 32 ints: 0=dead, 1=alive
let mut enemy_dir   = [0.0, 0.0, 0.0, ...];  // 32 floats: facing direction (radians)
let mut enemy_timer = [0.0, 0.0, 0.0, ...];  // 32 floats: AI cooldown timer
let mut enemy_count = 0;                       // number of active enemies
```

### Particle Storage Pattern

```ryx
// Particle attributes — parallel arrays, max 64 particles
let max_particles = 64;
let mut part_x     = [0.0, 0.0, ...];  // x position
let mut part_y     = [0.0, 0.0, ...];  // y position
let mut part_vx    = [0.0, 0.0, ...];  // x velocity
let mut part_vy    = [0.0, 0.0, ...];  // y velocity
let mut part_life  = [0.0, 0.0, ...];  // remaining lifetime (seconds)
let mut part_color = [0, 0, ...];       // packed RGBA color
let mut part_active = [0, 0, ...];     // 0=inactive, 1=active
```

### Map Storage Pattern

```ryx
// Map is a flat array of 20x15 = 300 tiles
// Tile types: 0=floor, 1=wall, 2=door, 3=exit, 4=wall_variant2, 5=wall_variant3
let map_w = 20;
let map_h = 15;
let mut tiles = [0, 0, 0, ...];  // 300 ints
```

### Item Storage Pattern

```ryx
// Items — parallel arrays, max 16 items per level
let max_items = 16;
let mut item_x     = [0, 0, ...];  // tile x
let mut item_y     = [0, 0, ...];  // tile y
let mut item_type  = [0, 0, ...];  // 0=none, 1=health_potion, 2=gem, 3=key
let mut item_active = [0, 0, ...]; // 0=collected, 1=active
let mut item_count = 0;
```

---

## Graphics API Reference (Existing Builtins Used)

All of these builtins already exist. No Go code changes are needed for the game itself.

### Drawing Primitives

| Builtin | Signature | Usage in Game |
|---------|-----------|---------------|
| `gfx_init` | `(w: Int, h: Int, title: String) -> ()` | Initialize 640x480 window |
| `gfx_run` | `(callback: fn() -> ()) -> ()` | Start game loop |
| `gfx_clear` | `(color: Int) -> ()` | Clear screen each frame |
| `gfx_set_color` | `(color: Int) -> ()` | Set current draw color |
| `gfx_fill_rect` | `(x: Int, y: Int, w: Int, h: Int) -> ()` | Draw tiles, entities, HUD bars |
| `gfx_rect` | `(x: Int, y: Int, w: Int, h: Int) -> ()` | Draw tile outlines, selection |
| `gfx_line` | `(x1: Int, y1: Int, x2: Int, y2: Int) -> ()` | Draw direction indicators, grid lines |
| `gfx_fill_circle` | `(cx: Int, cy: Int, r: Int) -> ()` | Draw particles, item glows |
| `gfx_circle` | `(cx: Int, cy: Int, r: Int) -> ()` | Draw enemy detection range (debug) |
| `gfx_text` | `(x: Int, y: Int, s: String) -> ()` | HUD text, title screen text |
| `gfx_pixel` | `(x: Int, y: Int) -> ()` | Small particle dots |
| `gfx_quit` | `() -> ()` | Exit game |

### Color Helpers

| Builtin | Signature | Usage |
|---------|-----------|-------|
| `gfx_rgb` | `(r: Int, g: Int, b: Int) -> Int` | Custom colors for tiles, enemies |
| `gfx_rgba` | `(r: Int, g: Int, b: Int, a: Int) -> Int` | Semi-transparent HUD overlays |
| `COLOR_BLACK` | `() -> Int` | Background |
| `COLOR_WHITE` | `() -> Int` | Text |
| `COLOR_RED` | `() -> Int` | Player health, damage flash |
| `COLOR_GREEN` | `() -> Int` | Health potions, floor tiles |
| `COLOR_BLUE` | `() -> Int` | Water/ice tiles |
| `COLOR_YELLOW` | `() -> Int` | Gems, score |

### Input

| Builtin | Signature | Usage |
|---------|-----------|-------|
| `gfx_key_pressed` | `(key: Int) -> Bool` | Movement (held keys) |
| `gfx_key_just_pressed` | `(key: Int) -> Bool` | Attack, menu selection (single press) |
| `gfx_mouse_x` | `() -> Int` | (reserved, not used) |
| `gfx_mouse_y` | `() -> Int` | (reserved, not used) |
| `KEY_UP` / `KEY_DOWN` / `KEY_LEFT` / `KEY_RIGHT` | `() -> Int` | Arrow movement |
| `KEY_W` / `KEY_A` / `KEY_S` / `KEY_D` | `() -> Int` | WASD movement |
| `KEY_SPACE` | `() -> Int` | Attack |
| `KEY_ESCAPE` | `() -> Int` | Quit / back to menu |
| `KEY_ENTER` | `() -> Int` | Menu confirm |

### Screen & Timing

| Builtin | Signature | Usage |
|---------|-----------|-------|
| `gfx_width` | `() -> Int` | Screen width |
| `gfx_height` | `() -> Int` | Screen height |
| `gfx_fps` | `() -> Float` | FPS display |
| `gfx_delta_time` | `() -> Float` | Frame-rate-independent movement and timers |

### Image Support

| Builtin | Signature | Usage |
|---------|-----------|-------|
| `gfx_load_image` | `(path: String) -> Int` | Load wall texture PNGs |
| `gfx_draw_image` | `(handle: Int, x: Int, y: Int) -> ()` | Draw wall textures on wall tiles |
| `gfx_draw_image_scaled` | `(handle: Int, x: Int, y: Int, sx: Float, sy: Float) -> ()` | Scale textures to tile size |
| `gfx_image_width` | `(handle: Int) -> Int` | Get texture dimensions |
| `gfx_image_height` | `(handle: Int) -> Int` | Get texture dimensions |

### Math (Existing)

| Builtin | Usage |
|---------|-------|
| `sin(x)`, `cos(x)` | Enemy patrol circles, particle spread |
| `sqrt(x)` | Distance calculations |
| `atan2(y, x)` | Angle between player and enemy |
| `abs(x)` | Absolute distance |
| `min(a, b)`, `max(a, b)` | Clamping, bounds |
| `clamp(x, lo, hi)` | Health bounds, position bounds |
| `floor(x)`, `ceil(x)` | Tile coordinate conversion |
| `random_int(lo, hi)` | Item placement, enemy behavior variance |
| `random_float()` | Particle velocity variance |
| `pi()` | Angle constants |

### Type Conversions (Existing)

| Builtin | Usage |
|---------|-------|
| `int_to_float(n)` | Convert tile coords to float for math |
| `float_to_int(f)` | Convert float positions to pixel coords |
| `int_to_string(n)` | HUD display (score, health, level) |

### Array Operations (Existing)

| Builtin | Usage |
|---------|-------|
| `array_len(arr)` | Get array length |

---

## New & Modified Files

| File | Action | Est. Lines | Description |
|------|--------|-----------|-------------|
| `examples/dungeon_crawler.ryx` | Create | ~1400 | Complete dungeon crawler game |
| `examples/assets/wall_stone.png` | Create | binary | 32x32 stone wall texture (simple gray brick pattern) |
| `examples/assets/wall_moss.png` | Create | binary | 32x32 mossy wall texture (green-gray pattern) |
| `tests/integration/e2e_test.go` | Modify | +~15 | Add compile-only test for dungeon_crawler.ryx |

---

## Game Design Specification

### Screen Layout (640 x 480)

```
+--------------------------------------------------+
|  [Minimap 80x60]    LEVEL 1    HP: #### [80/100] |  <- HUD bar (30px)
|                     Score: 42   Enemies: 3        |
+--------------------------------------------------+
|                                                    |
|     Tile grid rendered at 32x32 pixels per tile    |
|     Map is 20x15 tiles = 640x480 pixels exactly    |
|     (no scrolling needed for this tile size)        |
|                                                    |
|     Camera viewport shows the full level           |
|                                                    |
|                                                    |
+--------------------------------------------------+
```

Wait -- 20x15 tiles at 32px = 640x480, which fills the whole screen. The HUD needs to overlay on top. Let us use a slightly smaller tile grid or overlay the HUD semi-transparently.

**Final layout decision**: 20 columns x 14 rows visible tile area = 640x448 pixels for the game. The top 32 pixels (row 0 of tiles) are overlaid with a semi-transparent HUD bar. The minimap sits in the top-left corner of the HUD overlay. The tile rendering uses the full 640x480 but the HUD overlays on top.

### Tile Types

| ID | Name | Color | Walkable | Description |
|----|------|-------|----------|-------------|
| 0 | Floor | `gfx_rgb(60, 60, 70)` | Yes | Basic stone floor |
| 1 | Wall | `gfx_rgb(120, 100, 80)` | No | Stone wall (draw texture if loaded) |
| 2 | Door | `gfx_rgb(140, 90, 40)` | Yes | Opened door (visual only) |
| 3 | Exit | `gfx_rgb(0, 200, 100)` | Yes | Level exit staircase — glows |
| 4 | Wall variant | `gfx_rgb(90, 110, 80)` | No | Mossy wall (draw moss texture if loaded) |
| 5 | Water | `gfx_rgb(40, 60, 140)` | No | Decorative water tile |

### Player

- Position: float x, y (in tile coordinates, e.g., 3.5 means center of tile column 3)
- Movement: grid-aligned, smooth interpolation. When a movement key is pressed and the player is not already moving, start a smooth transition to the adjacent tile over 0.15 seconds.
- Health: 0-100, starts at 100
- Attack: press SPACE, damages all enemies within 1.5 tile radius for 20-35 damage
- Attack cooldown: 0.4 seconds
- Damage flash: screen flashes red briefly when hit
- Sprite: drawn as a colored rectangle with a directional indicator

### Enemy Types

| Type | ID | HP | Speed | Detect Range | Damage | Color | Size | Behavior |
|------|----|----|-------|-------------|--------|-------|------|----------|
| Slime | 1 | 30 | 1.5 | 4.0 | 8 | `gfx_rgb(80, 200, 80)` | 20x20 | Random patrol, chase in range |
| Skeleton | 2 | 50 | 2.0 | 6.0 | 15 | `gfx_rgb(220, 220, 200)` | 22x22 | Patrol path, chase + retreat when low HP |
| Ghost | 3 | 25 | 3.0 | 8.0 | 12 | `gfx_rgba(180, 180, 255, 180)` | 18x18 | Ignores walls, always chases |
| Boss | 4 | 150 | 1.0 | 10.0 | 25 | `gfx_rgb(200, 50, 50)` | 28x28 | Chases, spawns slimes at 50% HP |

### Item Types

| Type | ID | Effect | Color | Symbol |
|------|----|--------|-------|--------|
| Health Potion | 1 | Restore 30 HP | `gfx_rgb(255, 80, 80)` | Red square with cross |
| Gem | 2 | +50 score | `gfx_rgb(100, 200, 255)` | Cyan diamond shape |
| Key | 3 | (reserved for future door unlocking) | `gfx_rgb(255, 215, 0)` | Gold square |

### Level Definitions (3 Levels)

Each level is a flat array of 20x15 = 300 tile IDs, plus arrays specifying enemy spawn positions/types and item positions/types.

**Level 1: "The Entrance"** (Tutorial level)
- Simple layout: outer walls, a few internal walls, 2 rooms connected by a corridor
- Enemies: 3 slimes
- Items: 2 health potions, 3 gems
- Exit in the far corner

**Level 2: "The Crypt"**
- More complex layout: multiple rooms, narrow corridors, a central room with water tiles
- Enemies: 4 slimes, 2 skeletons, 1 ghost
- Items: 3 health potions, 4 gems
- Exit behind the central room

**Level 3: "The Throne Room"**
- Large open central area with pillars, side chambers
- Enemies: 2 slimes, 2 skeletons, 2 ghosts, 1 boss
- Items: 4 health potions, 5 gems
- Exit appears only after boss is defeated

### Game States

| State | ID | Description |
|-------|-----|-------------|
| Title | 0 | Show game name, "Press ENTER to start" |
| Playing | 1 | Active gameplay |
| Game Over | 2 | Player died, show score, "Press ENTER to restart" |
| Level Complete | 3 | Brief transition screen before next level |
| Victory | 4 | All 3 levels cleared, show final score |
| Paused | 5 | Press ESC during play, show "PAUSED" overlay |

### Particle Effects

- **Enemy death**: 12-16 particles in the enemy's color, spread outward, fade over 0.6 seconds
- **Item pickup**: 8 particles in the item's color, spiral upward, fade over 0.4 seconds
- **Player attack**: 6 white particles in a semicircle in the attack direction, fade over 0.3 seconds
- **Player damage**: 4 red particles from player position, fade over 0.3 seconds

### Minimap

- Drawn in the top-left corner, 80x60 pixels
- Each tile = 4x3 pixel rectangle
- Semi-transparent black background
- Walls shown as gray, floor as dark, exit as green
- Player shown as bright white dot
- Enemies shown as red dots
- Items shown as cyan dots

---

## Task Decomposition (5 Waves, 18 Tasks)

### Wave 1 — Foundation: Helper Functions & Constants (T1-T4, all parallel)

These tasks define utility functions and constants that appear BEFORE `fn main()` in the file. They have no dependencies on each other.

---

#### T1: Color Utility Functions and Game Constants

**Description**: Define all color-manipulation helper functions and game-wide constants at the top of `examples/dungeon_crawler.ryx`. These functions are used by every other section of the game for computing tile colors, shading, damage flash effects, and HUD rendering.

**File**: `examples/dungeon_crawler.ryx` (lines 1-120 approximately)

**Exact code to write**:

```ryx
// ============================================================================
// DUNGEON CRAWLER — A Ryx Roguelike Demo
// ============================================================================
//
// A top-down dungeon crawler demonstrating tile-based rendering, enemy AI,
// combat, items, particles, multiple levels, and full state management.
//
// Controls:
//   W/Up     — move up
//   S/Down   — move down
//   A/Left   — move left
//   D/Right  — move right
//   SPACE    — attack enemies in range
//   ENTER    — confirm menu selections
//   ESC      — pause / quit
//
// Requires: Ryx graphics runtime (Ebiten backend)
// ============================================================================

// ---------------------------------------------------------------------------
// Section 1: Color Utilities
// ---------------------------------------------------------------------------

// Extract red channel from packed RGBA color (0xRRGGBBAA)
fn color_r(c: Int) -> Int {
    (c / 0x1000000) % 256
}

// Extract green channel from packed RGBA color
fn color_g(c: Int) -> Int {
    (c / 0x10000) % 256
}

// Extract blue channel from packed RGBA color
fn color_b(c: Int) -> Int {
    (c / 0x100) % 256
}

// Extract alpha channel from packed RGBA color
fn color_a(c: Int) -> Int {
    c % 256
}

// Multiply RGB channels by a brightness factor [0.0 .. 1.0], preserve alpha
fn color_scale(c: Int, factor: Float) -> Int {
    let r = float_to_int(int_to_float(color_r(c)) * factor);
    let g = float_to_int(int_to_float(color_g(c)) * factor);
    let b = float_to_int(int_to_float(color_b(c)) * factor);
    let a = color_a(c);
    gfx_rgba(clamp(r, 0, 255), clamp(g, 0, 255), clamp(b, 0, 255), a)
}

// Linearly interpolate between two colors by factor t [0.0 .. 1.0]
// When t=0.0 returns c1, when t=1.0 returns c2
fn color_lerp(c1: Int, c2: Int, t: Float) -> Int {
    let r1 = int_to_float(color_r(c1));
    let g1 = int_to_float(color_g(c1));
    let b1 = int_to_float(color_b(c1));
    let r2 = int_to_float(color_r(c2));
    let g2 = int_to_float(color_g(c2));
    let b2 = int_to_float(color_b(c2));
    let r = float_to_int(r1 + (r2 - r1) * t);
    let g = float_to_int(g1 + (g2 - g1) * t);
    let b = float_to_int(b1 + (b2 - b1) * t);
    gfx_rgba(clamp(r, 0, 255), clamp(g, 0, 255), clamp(b, 0, 255), 255)
}

// Darken a color by halving RGB channels (used for alternate tile shading)
fn color_darken(c: Int) -> Int {
    gfx_rgba(color_r(c) / 2, color_g(c) / 2, color_b(c) / 2, color_a(c))
}

// Lighten a color by averaging with white
fn color_lighten(c: Int) -> Int {
    gfx_rgba(
        (color_r(c) + 255) / 2,
        (color_g(c) + 255) / 2,
        (color_b(c) + 255) / 2,
        color_a(c)
    )
}
```

Next, define game constants:

```ryx
// ---------------------------------------------------------------------------
// Section 2: Game Constants
// ---------------------------------------------------------------------------

// Screen dimensions
fn screen_w() -> Int { 640 }
fn screen_h() -> Int { 480 }

// Tile dimensions (in pixels)
fn tile_size() -> Int { 32 }

// Map dimensions (in tiles)
fn map_cols() -> Int { 20 }
fn map_rows() -> Int { 15 }
fn map_len() -> Int { 300 }  // 20 * 15

// Tile type constants
fn TILE_FLOOR() -> Int { 0 }
fn TILE_WALL() -> Int { 1 }
fn TILE_DOOR() -> Int { 2 }
fn TILE_EXIT() -> Int { 3 }
fn TILE_WALL2() -> Int { 4 }
fn TILE_WATER() -> Int { 5 }

// Entity limits
fn MAX_ENEMIES() -> Int { 32 }
fn MAX_PARTICLES() -> Int { 64 }
fn MAX_ITEMS() -> Int { 16 }

// Player constants
fn PLAYER_MAX_HP() -> Int { 100 }
fn PLAYER_MOVE_TIME() -> Float { 0.15 }
fn PLAYER_ATTACK_RANGE() -> Float { 1.5 }
fn PLAYER_ATTACK_COOLDOWN() -> Float { 0.4 }
fn PLAYER_ATTACK_MIN_DMG() -> Int { 20 }
fn PLAYER_ATTACK_MAX_DMG() -> Int { 35 }

// Enemy type constants
fn ENEMY_NONE() -> Int { 0 }
fn ENEMY_SLIME() -> Int { 1 }
fn ENEMY_SKELETON() -> Int { 2 }
fn ENEMY_GHOST() -> Int { 3 }
fn ENEMY_BOSS() -> Int { 4 }

// Item type constants
fn ITEM_NONE() -> Int { 0 }
fn ITEM_HEALTH() -> Int { 1 }
fn ITEM_GEM() -> Int { 2 }
fn ITEM_KEY() -> Int { 3 }

// Game state constants
fn STATE_TITLE() -> Int { 0 }
fn STATE_PLAYING() -> Int { 1 }
fn STATE_GAME_OVER() -> Int { 2 }
fn STATE_LEVEL_COMPLETE() -> Int { 3 }
fn STATE_VICTORY() -> Int { 4 }
fn STATE_PAUSED() -> Int { 5 }
```

**Acceptance Criteria**:
- All functions are syntactically valid Ryx and compile without errors
- `color_r`, `color_g`, `color_b`, `color_a` correctly extract packed RGBA channels
- `color_scale(gfx_rgb(200, 100, 50), 0.5)` produces approximately `gfx_rgba(100, 50, 25, 255)`
- `color_lerp(COLOR_RED(), COLOR_BLUE(), 0.5)` produces approximately `gfx_rgba(127, 0, 127, 255)`
- All constant functions return the documented values
- No mutable state is introduced (these are pure functions)
- Read `examples/raycaster.ryx` for reference on the color extraction pattern (lines 19-29)

---

#### T2: Map and Tile Helper Functions

**Description**: Define helper functions for tile-based map operations: looking up tile types, checking walkability, converting between tile coordinates and pixel coordinates, and computing distances between positions. These functions are used by movement, collision detection, enemy AI, and rendering.

**File**: `examples/dungeon_crawler.ryx` (lines ~121-230 approximately, after T1's section)

**Exact code to write**:

```ryx
// ---------------------------------------------------------------------------
// Section 3: Map & Tile Helpers
// ---------------------------------------------------------------------------

// Look up the tile type at column x, row y in a map array.
// Returns TILE_WALL (1) for out-of-bounds coordinates (safety boundary).
fn tile_at(tiles: [Int], x: Int, y: Int) -> Int {
    if x < 0 || x >= map_cols() || y < 0 || y >= map_rows() {
        TILE_WALL()
    } else {
        tiles[y * map_cols() + x]
    }
}

// Check if a tile at (x, y) can be walked on.
// Floor (0), Door (2), and Exit (3) are walkable; walls (1, 4) and water (5) are not.
fn is_walkable(tiles: [Int], x: Int, y: Int) -> Bool {
    let t = tile_at(tiles, x, y);
    t == TILE_FLOOR() || t == TILE_DOOR() || t == TILE_EXIT()
}

// Convert tile column to pixel x (left edge of tile)
fn tile_to_px(tx: Int) -> Int {
    tx * tile_size()
}

// Convert tile row to pixel y (top edge of tile)
fn tile_to_py(ty: Int) -> Int {
    ty * tile_size()
}

// Convert pixel x to tile column
fn px_to_tile(px: Int) -> Int {
    px / tile_size()
}

// Convert pixel y to tile row
fn py_to_tile(py: Int) -> Int {
    py / tile_size()
}

// Compute squared Euclidean distance between two points (avoids sqrt for comparisons)
fn dist_sq(x1: Float, y1: Float, x2: Float, y2: Float) -> Float {
    let dx = x2 - x1;
    let dy = y2 - y1;
    dx * dx + dy * dy
}

// Compute Euclidean distance between two points
fn dist(x1: Float, y1: Float, x2: Float, y2: Float) -> Float {
    sqrt(dist_sq(x1, y1, x2, y2))
}

// Compute Manhattan distance between two tile positions
fn manhattan(x1: Int, y1: Int, x2: Int, y2: Int) -> Int {
    abs(x2 - x1) + abs(y2 - y1)
}

// Get the color for rendering a tile type.
// Returns a packed RGBA color.
fn tile_color(t: Int) -> Int {
    if t == TILE_FLOOR() { gfx_rgb(60, 60, 70) }
    else if t == TILE_WALL() { gfx_rgb(120, 100, 80) }
    else if t == TILE_DOOR() { gfx_rgb(140, 90, 40) }
    else if t == TILE_EXIT() { gfx_rgb(0, 200, 100) }
    else if t == TILE_WALL2() { gfx_rgb(90, 110, 80) }
    else if t == TILE_WATER() { gfx_rgb(40, 60, 140) }
    else { gfx_rgb(60, 60, 70) }
}

// Get the checkerboard variant of the floor color.
// Alternates between two slightly different shades for visual interest.
fn floor_color(x: Int, y: Int) -> Int {
    if (x + y) % 2 == 0 {
        gfx_rgb(55, 55, 65)
    } else {
        gfx_rgb(65, 65, 75)
    }
}

// Get the color for an enemy type.
fn enemy_color(etype: Int) -> Int {
    if etype == ENEMY_SLIME() { gfx_rgb(80, 200, 80) }
    else if etype == ENEMY_SKELETON() { gfx_rgb(220, 220, 200) }
    else if etype == ENEMY_GHOST() { gfx_rgba(180, 180, 255, 180) }
    else if etype == ENEMY_BOSS() { gfx_rgb(200, 50, 50) }
    else { gfx_rgb(128, 128, 128) }
}

// Get the draw size (in pixels) for an enemy type.
fn enemy_size(etype: Int) -> Int {
    if etype == ENEMY_SLIME() { 20 }
    else if etype == ENEMY_SKELETON() { 22 }
    else if etype == ENEMY_GHOST() { 18 }
    else if etype == ENEMY_BOSS() { 28 }
    else { 16 }
}

// Get the detection range for an enemy type (in tile units).
fn enemy_detect_range(etype: Int) -> Float {
    if etype == ENEMY_SLIME() { 4.0 }
    else if etype == ENEMY_SKELETON() { 6.0 }
    else if etype == ENEMY_GHOST() { 8.0 }
    else if etype == ENEMY_BOSS() { 10.0 }
    else { 3.0 }
}

// Get the movement speed for an enemy type (tiles per second).
fn enemy_speed(etype: Int) -> Float {
    if etype == ENEMY_SLIME() { 1.5 }
    else if etype == ENEMY_SKELETON() { 2.0 }
    else if etype == ENEMY_GHOST() { 3.0 }
    else if etype == ENEMY_BOSS() { 1.0 }
    else { 1.0 }
}

// Get the max HP for an enemy type.
fn enemy_max_hp(etype: Int) -> Int {
    if etype == ENEMY_SLIME() { 30 }
    else if etype == ENEMY_SKELETON() { 50 }
    else if etype == ENEMY_GHOST() { 25 }
    else if etype == ENEMY_BOSS() { 150 }
    else { 10 }
}

// Get the contact damage for an enemy type.
fn enemy_damage(etype: Int) -> Int {
    if etype == ENEMY_SLIME() { 8 }
    else if etype == ENEMY_SKELETON() { 15 }
    else if etype == ENEMY_GHOST() { 12 }
    else if etype == ENEMY_BOSS() { 25 }
    else { 5 }
}

// Get the color for an item type.
fn item_color(itype: Int) -> Int {
    if itype == ITEM_HEALTH() { gfx_rgb(255, 80, 80) }
    else if itype == ITEM_GEM() { gfx_rgb(100, 200, 255) }
    else if itype == ITEM_KEY() { gfx_rgb(255, 215, 0) }
    else { gfx_rgb(128, 128, 128) }
}
```

**Acceptance Criteria**:
- `tile_at(some_map, -1, 5)` returns `TILE_WALL()` (1)
- `tile_at(some_map, 0, 0)` returns the first element of the map array
- `is_walkable` returns `true` for tiles 0, 2, 3 and `false` for 1, 4, 5
- `dist(0.0, 0.0, 3.0, 4.0)` returns `5.0`
- `manhattan(0, 0, 3, 4)` returns `7`
- All enemy/item attribute functions return documented values
- Functions match the existing `tile_at` pattern in `examples/raycaster.ryx` (line 57-63)
- Read `examples/raycaster.ryx` lines 35-63 for the color helper pattern

---

#### T3: Level Data Definitions

**Description**: Define three complete level layouts as functions that return flat tile arrays, plus functions that return enemy spawn data and item spawn data for each level. Each level-loading function populates the shared game arrays.

**File**: `examples/dungeon_crawler.ryx` (lines ~231-500 approximately, after T2's section)

**Exact code to write**:

```ryx
// ---------------------------------------------------------------------------
// Section 4: Level Data
// ---------------------------------------------------------------------------

// Level 1: "The Entrance" — tutorial level with simple layout
// Layout: outer walls, two rooms connected by a corridor
// Map key: 0=floor, 1=wall, 2=door, 3=exit, 4=wall_variant, 5=water
fn get_level_1_tiles() -> [Int] {
    [
        1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,
        1,0,0,0,0,1,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,0,0,0,0,1,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,0,0,0,0,1,0,0,0,0,0,1,1,1,1,0,0,0,0,1,
        1,0,0,0,0,2,0,0,0,0,0,1,0,0,0,0,0,0,0,1,
        1,0,0,0,0,1,0,0,0,0,0,1,0,0,0,0,0,0,0,1,
        1,1,1,2,1,1,0,0,0,0,0,1,0,0,0,0,0,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,2,0,0,0,0,0,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,1,0,0,0,0,0,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,1,1,1,1,0,0,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,3,1,
        1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1
    ]
}

// Level 1 enemies: 3 slimes
// Returns: [count, type1,x1,y1, type2,x2,y2, ...]
fn get_level_1_enemies() -> [Int] {
    [3,  1,3,8,  1,9,2,  1,15,10]
}

// Level 1 items: 2 health potions, 3 gems
// Returns: [count, type1,x1,y1, type2,x2,y2, ...]
fn get_level_1_items() -> [Int] {
    [5,  1,2,2,  1,14,5,  2,7,7,  2,16,3,  2,10,12]
}

// Player start position for level 1 (tile coords)
fn get_level_1_start_x() -> Int { 2 }
fn get_level_1_start_y() -> Int { 4 }


// Level 2: "The Crypt" — more complex layout with water feature
fn get_level_2_tiles() -> [Int] {
    [
        1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,
        1,0,0,0,1,0,0,0,0,0,0,0,0,0,0,1,0,0,0,1,
        1,0,0,0,1,0,0,0,0,0,0,0,0,0,0,1,0,0,0,1,
        1,0,0,0,2,0,0,4,4,4,4,4,4,0,0,2,0,0,0,1,
        1,1,1,2,1,0,0,4,5,5,5,5,4,0,0,1,2,1,1,1,
        1,0,0,0,0,0,0,4,5,5,5,5,4,0,0,0,0,0,0,1,
        1,0,0,0,0,0,0,4,5,5,5,5,4,0,0,0,0,0,0,1,
        1,0,0,0,0,0,0,4,4,2,2,4,4,0,0,0,0,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,4,4,0,0,1,1,1,0,0,0,1,1,1,0,0,4,4,0,1,
        1,0,0,0,0,1,0,0,0,3,0,0,0,1,0,0,0,0,0,1,
        1,0,0,0,0,1,0,0,0,0,0,0,0,1,0,0,0,0,0,1,
        1,0,0,0,0,1,1,1,1,1,1,1,1,1,0,0,0,0,0,1,
        1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1
    ]
}

// Level 2 enemies: 4 slimes, 2 skeletons, 1 ghost
fn get_level_2_enemies() -> [Int] {
    [7,  1,2,1,  1,17,1,  1,2,12,  1,17,12,  2,9,5,  2,10,5,  3,9,9]
}

// Level 2 items: 3 health potions, 4 gems
fn get_level_2_items() -> [Int] {
    [7,  1,1,1,  1,18,1,  1,9,8,  2,3,5,  2,16,5,  2,3,9,  2,16,9]
}

fn get_level_2_start_x() -> Int { 1 }
fn get_level_2_start_y() -> Int { 7 }


// Level 3: "The Throne Room" — boss level with pillars and side chambers
fn get_level_3_tiles() -> [Int] {
    [
        1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,
        1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,0,0,0,0,0,1,0,0,0,0,0,0,1,0,0,0,0,0,1,
        1,0,0,4,0,0,0,0,0,0,0,0,0,0,0,0,4,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,0,0,0,0,0,1,0,0,0,0,0,0,1,0,0,0,0,0,1,
        1,1,2,1,0,0,0,0,0,0,0,0,0,0,0,0,1,2,1,1,
        1,0,0,1,0,0,0,0,0,0,0,0,0,0,0,0,1,0,0,1,
        1,0,0,1,0,0,1,0,0,0,0,0,0,1,0,0,1,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,0,0,4,0,0,1,0,0,0,0,0,0,1,0,0,4,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,
        1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1
    ]
}

// Level 3 enemies: 2 slimes, 2 skeletons, 2 ghosts, 1 boss
fn get_level_3_enemies() -> [Int] {
    [7,  1,3,1,  1,16,1,  2,1,12,  2,18,12,  3,5,5,  3,14,5,  4,9,3]
}

// Level 3 items: 4 health potions, 5 gems
fn get_level_3_items() -> [Int] {
    [9,  1,1,1,  1,18,13,  1,1,8,  1,18,8,  2,5,10,  2,14,10,  2,9,12,  2,3,3,  2,16,3]
}

fn get_level_3_start_x() -> Int { 9 }
fn get_level_3_start_y() -> Int { 13 }


// ---------------------------------------------------------------------------
// Level loader dispatcher — returns tile array for given level number (1-3)
// ---------------------------------------------------------------------------

fn get_level_tiles(level: Int) -> [Int] {
    if level == 1 { get_level_1_tiles() }
    else if level == 2 { get_level_2_tiles() }
    else { get_level_3_tiles() }
}

fn get_level_enemies(level: Int) -> [Int] {
    if level == 1 { get_level_1_enemies() }
    else if level == 2 { get_level_2_enemies() }
    else { get_level_3_enemies() }
}

fn get_level_items(level: Int) -> [Int] {
    if level == 1 { get_level_1_items() }
    else if level == 2 { get_level_2_items() }
    else { get_level_3_items() }
}

fn get_level_start_x(level: Int) -> Int {
    if level == 1 { get_level_1_start_x() }
    else if level == 2 { get_level_2_start_x() }
    else { get_level_3_start_x() }
}

fn get_level_start_y(level: Int) -> Int {
    if level == 1 { get_level_1_start_y() }
    else if level == 2 { get_level_2_start_y() }
    else { get_level_3_start_y() }
}

// Total number of levels in the game
fn total_levels() -> Int { 3 }
```

**Acceptance Criteria**:
- Each level tile array has exactly 300 elements (20 * 15)
- All tile values are in the valid range 0-5
- Outer border of each level is all walls (tile type 1)
- Player start positions are on walkable tiles (type 0)
- Enemy spawn data uses the documented format `[count, type1,x1,y1, ...]`
- Enemy spawn positions are on walkable tiles
- Item spawn positions are on walkable tiles and do not overlap enemy spawns
- Exit tiles (type 3) exist in each level
- Level 3's exit position is initially blocked or will appear after boss defeat (handled by game logic, not tile data -- the exit tile exists but game logic checks if boss is alive)
- `get_level_tiles(1)` returns the Level 1 array, etc.
- Read `examples/raycaster.ryx` lines 88-105 for the flat map array pattern

---

#### T4: Particle System Spawn Helper

**Description**: Define a helper function that spawns particles into the parallel particle arrays. This function is called from the frame callback to create visual effects. Since it modifies shared mutable arrays, it will be called from within the closure in `main()`, but the spawn-pattern helper can be a pure function that computes initial velocity for a particle index.

However, since the particle arrays are mutable state inside `main()`, the actual spawning must happen inline. This task defines helper functions for computing particle initial velocities in different patterns.

**File**: `examples/dungeon_crawler.ryx` (lines ~501-570 approximately, after T3's section)

**Exact code to write**:

```ryx
// ---------------------------------------------------------------------------
// Section 5: Particle Velocity Helpers
// ---------------------------------------------------------------------------

// Compute the X velocity for a particle in a radial burst pattern.
// index: which particle in the burst (0..count-1)
// count: total particles in the burst
// speed: outward velocity magnitude
fn burst_vx(index: Int, count: Int, speed: Float) -> Float {
    let angle = 2.0 * pi() * int_to_float(index) / int_to_float(count);
    cos(angle) * speed
}

// Compute the Y velocity for a particle in a radial burst pattern.
fn burst_vy(index: Int, count: Int, speed: Float) -> Float {
    let angle = 2.0 * pi() * int_to_float(index) / int_to_float(count);
    sin(angle) * speed
}

// Compute the X velocity for an upward spiral particle.
// index: which particle (0..count-1)
// count: total particles
// spread: horizontal spread factor
fn spiral_vx(index: Int, count: Int, spread: Float) -> Float {
    let angle = 2.0 * pi() * int_to_float(index) / int_to_float(count);
    cos(angle) * spread
}

// Compute the Y velocity for an upward spiral particle (negative = upward).
fn spiral_vy(index: Int, count: Int, base_speed: Float) -> Float {
    0.0 - base_speed - int_to_float(index) * 0.1
}

// Compute the X velocity for a semicircular attack effect.
// center_angle: direction of the attack (radians)
// index: which particle (0..count-1)
// count: total particles
// speed: outward velocity
fn attack_vx(center_angle: Float, index: Int, count: Int, speed: Float) -> Float {
    let spread = pi() * 0.6;  // 108 degree arc
    let start_angle = center_angle - spread / 2.0;
    let angle = start_angle + spread * int_to_float(index) / int_to_float(max(1, count - 1));
    cos(angle) * speed
}

// Compute the Y velocity for a semicircular attack effect.
fn attack_vy(center_angle: Float, index: Int, count: Int, speed: Float) -> Float {
    let spread = pi() * 0.6;
    let start_angle = center_angle - spread / 2.0;
    let angle = start_angle + spread * int_to_float(index) / int_to_float(max(1, count - 1));
    sin(angle) * speed
}
```

**Acceptance Criteria**:
- `burst_vx(0, 8, 2.0)` returns approximately `2.0` (cos(0) * 2.0)
- `burst_vy(0, 8, 2.0)` returns approximately `0.0` (sin(0) * 2.0)
- `burst_vx(2, 8, 2.0)` returns approximately `0.0` (cos(pi/2) * 2.0)
- `spiral_vy` always returns a negative value (upward motion)
- `attack_vx` and `attack_vy` distribute particles across a semicircular arc centered on `center_angle`
- All functions are pure (no state mutation), take only primitive arguments, return Float
- No division by zero: `max(1, count - 1)` guard is present
- Read `examples/raycaster.ryx` for the math function call pattern (sin, cos, pi)

---

### Wave 2 — Core Game State & Level Loading (T5-T7, T5 first, then T6-T7 parallel)

These tasks implement the `fn main()` function's opening section: declaring all mutable game state variables and the level-loading logic.

---

#### T5: Main Function Opening — All Mutable State Declaration

**Description**: Write the opening of `fn main()` that declares ALL mutable game state variables using `let mut`. This includes player state, enemy parallel arrays, item parallel arrays, particle parallel arrays, game-state variables, and timing variables. This is the single largest variable declaration section and must be written carefully because all subsequent tasks reference these variable names.

All arrays must be initialized to the correct length using array literals. Since Ryx requires explicit array literals (no `Array.new(n)` constructor), each array must be written out with the required number of elements.

**File**: `examples/dungeon_crawler.ryx` (lines ~571-820 approximately, the beginning of `fn main()`)

**Important**: Every mutable variable name defined here is part of the game's API contract. All subsequent tasks (T6-T18) reference these names. Getting a name wrong will cause cascading compilation failures.

**Exact code to write**:

```ryx
// ===========================================================================
// MAIN — Entry point and game loop
// ===========================================================================
fn main() {

    // -----------------------------------------------------------------------
    // Section 6: Mutable Game State
    // -----------------------------------------------------------------------

    // --- Game state machine ---
    let mut game_state = 0;  // STATE_TITLE
    let mut current_level = 1;
    let mut score = 0;
    let mut total_score = 0;
    let mut level_transition_timer = 0.0;

    // --- Player state ---
    let mut player_x = 2.0;       // tile X (float for smooth movement)
    let mut player_y = 4.0;       // tile Y (float for smooth movement)
    let mut player_target_x = 2;  // target tile X (int)
    let mut player_target_y = 4;  // target tile Y (int)
    let mut player_moving = false;
    let mut player_move_progress = 0.0;  // 0.0 to 1.0 during movement
    let mut player_start_x = 2.0;  // movement start position
    let mut player_start_y = 4.0;
    let mut player_hp = 100;
    let mut player_facing = 0;    // 0=right, 1=down, 2=left, 3=up
    let mut player_attack_timer = 0.0;  // cooldown remaining
    let mut player_damage_flash = 0.0;  // flash timer for damage feedback
    let mut player_invuln_timer = 0.0;  // brief invulnerability after hit

    // --- Map tiles (20x15 = 300 elements) ---
    let mut tiles = [
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
    ];

    // --- Enemy parallel arrays (max 32) ---
    let mut enemy_x = [
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0
    ];
    let mut enemy_y = [
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0
    ];
    let mut enemy_type = [
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
    ];
    let mut enemy_hp = [
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
    ];
    let mut enemy_alive = [
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
    ];
    let mut enemy_dir = [
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0
    ];
    let mut enemy_timer = [
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0
    ];
    let mut enemy_count = 0;

    // --- Particle parallel arrays (max 64) ---
    let mut part_x = [
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0
    ];
    let mut part_y = [
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0
    ];
    let mut part_vx = [
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0
    ];
    let mut part_vy = [
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0
    ];
    let mut part_life = [
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,
        0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0
    ];
    let mut part_color = [
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
    ];
    let mut part_active = [
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
    ];

    // --- Item parallel arrays (max 16) ---
    let mut item_x = [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0];
    let mut item_y = [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0];
    let mut item_type = [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0];
    let mut item_active = [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0];
    let mut item_count = 0;

    // --- Boss state ---
    let mut boss_spawned_minions = false;
    let mut boss_alive = false;
    let mut exit_unlocked = true;  // false on level 3 until boss dies

    // --- Wall textures (loaded once, -1 means not loaded) ---
    let wall_img = gfx_load_image("examples/assets/wall_stone.png");
    let wall2_img = gfx_load_image("examples/assets/wall_moss.png");

    // --- Initialize graphics ---
    gfx_init(screen_w(), screen_h(), "Ryx Dungeon Crawler");
```

**Acceptance Criteria**:
- All array sizes match their documented constants: `tiles` = 300, enemy arrays = 32, particle arrays = 64, item arrays = 16
- All arrays use the correct element type (Float arrays use `0.0`, Int arrays use `0`)
- Variable names exactly match the names used in subsequent tasks (T6-T18)
- No syntax errors: every `let mut` statement ends with `;`
- `gfx_load_image` calls use the documented asset paths
- `gfx_init` is called with 640, 480 dimensions
- Read `examples/raycaster.ryx` lines 69-134 for the pattern of mutable state declaration inside `main()`

---

#### T6: Level Loading Logic (depends on T5)

**Description**: Implement the level-loading procedure that populates the `tiles`, enemy, and item arrays from the level data functions. This code runs once at game start and again when transitioning between levels. It is implemented as inline code inside the frame callback, triggered when `game_state` transitions to `STATE_PLAYING`.

However, for reuse, this task also writes a section of code right after the state declarations (but still inside `main()`, before the frame closure) that performs the initial level load.

**File**: `examples/dungeon_crawler.ryx` (lines ~821-920 approximately, continuing inside `fn main()`)

**Exact code to write**:

```ryx
    // -----------------------------------------------------------------------
    // Section 7: Initial Level Load
    // -----------------------------------------------------------------------
    // Load level 1 data into the mutable state arrays.
    // This same pattern is repeated in the frame callback when transitioning levels.

    // Load tile data
    let level_tiles = get_level_tiles(current_level);
    let mut ti = 0;
    while ti < map_len() {
        tiles[ti] = level_tiles[ti];
        ti = ti + 1
    };

    // Load enemy data
    let level_enemies = get_level_enemies(current_level);
    enemy_count = level_enemies[0];
    let mut ei = 0;
    while ei < enemy_count {
        let base = 1 + ei * 3;
        enemy_type[ei] = level_enemies[base];
        enemy_x[ei] = int_to_float(level_enemies[base + 1]) + 0.5;
        enemy_y[ei] = int_to_float(level_enemies[base + 2]) + 0.5;
        enemy_hp[ei] = enemy_max_hp(level_enemies[base]);
        enemy_alive[ei] = 1;
        enemy_dir[ei] = 0.0;
        enemy_timer[ei] = 0.0;
        ei = ei + 1
    };
    // Clear remaining enemy slots
    while ei < MAX_ENEMIES() {
        enemy_alive[ei] = 0;
        enemy_type[ei] = 0;
        ei = ei + 1
    };

    // Load item data
    let level_items = get_level_items(current_level);
    item_count = level_items[0];
    let mut ii = 0;
    while ii < item_count {
        let ibase = 1 + ii * 3;
        item_type[ii] = level_items[ibase];
        item_x[ii] = level_items[ibase + 1];
        item_y[ii] = level_items[ibase + 2];
        item_active[ii] = 1;
        ii = ii + 1
    };
    // Clear remaining item slots
    while ii < MAX_ITEMS() {
        item_active[ii] = 0;
        item_type[ii] = 0;
        ii = ii + 1
    };

    // Set player start position
    player_x = int_to_float(get_level_start_x(current_level)) + 0.5;
    player_y = int_to_float(get_level_start_y(current_level)) + 0.5;
    player_target_x = get_level_start_x(current_level);
    player_target_y = get_level_start_y(current_level);
    player_start_x = player_x;
    player_start_y = player_y;
    player_moving = false;
    player_move_progress = 0.0;

    // Boss state for level 3
    boss_spawned_minions = false;
    boss_alive = false;
    exit_unlocked = true;
    if current_level == 3 {
        exit_unlocked = false;
        // Check if there is a boss enemy
        let mut bi = 0;
        while bi < enemy_count {
            if enemy_type[bi] == ENEMY_BOSS() {
                boss_alive = true
            };
            bi = bi + 1
        }
    };

    // Clear all particles
    let mut pi = 0;
    while pi < MAX_PARTICLES() {
        part_active[pi] = 0;
        pi = pi + 1
    };
```

**Acceptance Criteria**:
- After execution, `tiles` array contains Level 1's tile data
- Enemy arrays are populated with 3 slimes at the documented positions, with HP set from `enemy_max_hp`
- Enemy positions have +0.5 offset (center of tile)
- Remaining enemy slots (indices 3-31) have `enemy_alive[i] = 0`
- Item arrays are populated with 5 items at documented positions
- Player position matches `get_level_1_start_x()` + 0.5
- `boss_alive` is `false` for level 1
- `exit_unlocked` is `true` for level 1
- All particles are inactive
- Read `examples/raycaster.ryx` lines 108-125 for the array-mutation pattern

---

#### T7: Frame Callback Opening — Delta Time & State Machine (depends on T5)

**Description**: Write the frame callback closure and its opening: delta-time computation, ESC handling, and the top-level game-state dispatch. This establishes the skeleton that all subsequent gameplay tasks fill in.

**File**: `examples/dungeon_crawler.ryx` (lines ~921-990 approximately, continuing inside `fn main()`)

**Exact code to write**:

```ryx
    // -----------------------------------------------------------------------
    // Section 8: Frame Callback
    // -----------------------------------------------------------------------
    let frame = || {

        // Delta time for frame-rate independence
        let raw_dt = gfx_delta_time();
        let dt = if raw_dt <= 0.0 { 1.0 / 60.0 } else { raw_dt };

        // Global ESC handling
        if gfx_key_just_pressed(KEY_ESCAPE()) {
            if game_state == STATE_PLAYING() {
                game_state = STATE_PAUSED()
            } else if game_state == STATE_PAUSED() {
                game_state = STATE_PLAYING()
            } else {
                gfx_quit()
            }
        };

        // ===================================================================
        // STATE DISPATCH
        // ===================================================================

        if game_state == STATE_TITLE() {
            // --- TITLE SCREEN (T9) ---
            // [Title screen rendering code goes here]
            ()  // placeholder

        } else if game_state == STATE_PLAYING() {
            // --- GAMEPLAY ---

            // [T10: Player input & movement]
            // [T11: Enemy AI & movement]
            // [T12: Combat — player attack]
            // [T13: Combat — enemy contact damage]
            // [T14: Item pickup]
            // [T15: Particle update]
            // [T16: Tile rendering]
            // [T16: Entity rendering]
            // [T16: Particle rendering]
            // [T17: HUD rendering]
            // [T17: Minimap rendering]
            // [Level completion check]

            ()  // placeholder

        } else if game_state == STATE_GAME_OVER() {
            // --- GAME OVER SCREEN (T9) ---
            ()  // placeholder

        } else if game_state == STATE_LEVEL_COMPLETE() {
            // --- LEVEL TRANSITION (T9) ---
            ()  // placeholder

        } else if game_state == STATE_VICTORY() {
            // --- VICTORY SCREEN (T9) ---
            ()  // placeholder

        } else if game_state == STATE_PAUSED() {
            // --- PAUSE OVERLAY (T9) ---
            ()  // placeholder
        }
    };

    // Start the game loop
    gfx_run(frame)
}
```

**Note to workers**: The placeholder `()` expressions and `// [Tx: ...]` comments indicate where subsequent tasks will insert their code. When implementing tasks T9-T17, replace the placeholder and comments in the appropriate branch with the actual implementation.

**Acceptance Criteria**:
- The frame callback closure compiles and can be passed to `gfx_run`
- Delta time defaults to 1/60 second when `gfx_delta_time()` returns 0 or negative
- ESC toggles between `STATE_PLAYING` and `STATE_PAUSED`
- ESC from any other state calls `gfx_quit()`
- State dispatch covers all 6 game states
- The overall structure (opening of `main`, state, frame closure, `gfx_run`) matches the pattern in `examples/raycaster.ryx` lines 138-489
- The closing `}` of `fn main()` is present and the file is syntactically complete

---

### Wave 3 — Gameplay Mechanics (T8-T14, dependencies noted)

These tasks implement the core gameplay: movement, AI, combat, items, and particles. They all write code inside the `game_state == STATE_PLAYING()` branch of the frame callback.

---

#### T8: Level Reload Logic Inside Frame Callback (depends on T6, T7)

**Description**: Implement the inline level-reload logic that runs inside the frame callback when transitioning from `STATE_LEVEL_COMPLETE` to `STATE_PLAYING` for the next level. This duplicates the pattern from T6 but runs inside the closure. Also implement the level-completion detection logic (checking if all enemies are dead and player is on exit tile, or if boss is dead on level 3).

This code goes inside the `STATE_LEVEL_COMPLETE` branch AND at the end of the `STATE_PLAYING` branch.

**File**: `examples/dungeon_crawler.ryx` — inside the frame callback's `STATE_LEVEL_COMPLETE` branch and end of `STATE_PLAYING` branch

**Code for STATE_LEVEL_COMPLETE branch** (replaces the placeholder):

```ryx
            // Level transition animation
            gfx_set_color(gfx_rgb(0, 0, 0));
            gfx_fill_rect(0, 0, screen_w(), screen_h());

            gfx_set_color(gfx_rgb(100, 255, 100));
            gfx_text(220, 180, "LEVEL " ++ int_to_string(current_level) ++ " COMPLETE!");

            gfx_set_color(COLOR_WHITE());
            gfx_text(250, 230, "Score: " ++ int_to_string(score));

            level_transition_timer = level_transition_timer - dt;

            if level_transition_timer <= 0.0 {
                current_level = current_level + 1;

                if current_level > total_levels() {
                    game_state = STATE_VICTORY()
                } else {
                    // Reload level data (same pattern as initial load in Section 7)
                    let new_tiles = get_level_tiles(current_level);
                    let mut lti = 0;
                    while lti < map_len() {
                        tiles[lti] = new_tiles[lti];
                        lti = lti + 1
                    };

                    let new_enemies = get_level_enemies(current_level);
                    enemy_count = new_enemies[0];
                    let mut lei = 0;
                    while lei < enemy_count {
                        let ebase = 1 + lei * 3;
                        enemy_type[lei] = new_enemies[ebase];
                        enemy_x[lei] = int_to_float(new_enemies[ebase + 1]) + 0.5;
                        enemy_y[lei] = int_to_float(new_enemies[ebase + 2]) + 0.5;
                        enemy_hp[lei] = enemy_max_hp(new_enemies[ebase]);
                        enemy_alive[lei] = 1;
                        enemy_dir[lei] = 0.0;
                        enemy_timer[lei] = 0.0;
                        lei = lei + 1
                    };
                    while lei < MAX_ENEMIES() {
                        enemy_alive[lei] = 0;
                        enemy_type[lei] = 0;
                        lei = lei + 1
                    };

                    let new_items = get_level_items(current_level);
                    item_count = new_items[0];
                    let mut lii = 0;
                    while lii < item_count {
                        let ibase = 1 + lii * 3;
                        item_type[lii] = new_items[ibase];
                        item_x[lii] = new_items[ibase + 1];
                        item_y[lii] = new_items[ibase + 2];
                        item_active[lii] = 1;
                        lii = lii + 1
                    };
                    while lii < MAX_ITEMS() {
                        item_active[lii] = 0;
                        item_type[lii] = 0;
                        lii = lii + 1
                    };

                    player_x = int_to_float(get_level_start_x(current_level)) + 0.5;
                    player_y = int_to_float(get_level_start_y(current_level)) + 0.5;
                    player_target_x = get_level_start_x(current_level);
                    player_target_y = get_level_start_y(current_level);
                    player_start_x = player_x;
                    player_start_y = player_y;
                    player_moving = false;
                    player_move_progress = 0.0;

                    boss_spawned_minions = false;
                    boss_alive = false;
                    exit_unlocked = true;
                    if current_level == 3 {
                        exit_unlocked = false;
                        let mut bci = 0;
                        while bci < enemy_count {
                            if enemy_type[bci] == ENEMY_BOSS() {
                                boss_alive = true
                            };
                            bci = bci + 1
                        }
                    };

                    let mut pci = 0;
                    while pci < MAX_PARTICLES() {
                        part_active[pci] = 0;
                        pci = pci + 1
                    };

                    game_state = STATE_PLAYING()
                }
            }
```

**Code for end of STATE_PLAYING branch** (level completion check, goes after all gameplay logic):

```ryx
            // --- Level completion check ---
            // Count alive enemies
            let mut alive_count = 0;
            let mut aci = 0;
            while aci < enemy_count {
                if enemy_alive[aci] == 1 {
                    alive_count = alive_count + 1
                };
                aci = aci + 1
            };

            // On level 3, check if boss is dead to unlock exit
            if current_level == 3 && boss_alive {
                let mut boss_dead = true;
                let mut bdi = 0;
                while bdi < enemy_count {
                    if enemy_type[bdi] == ENEMY_BOSS() && enemy_alive[bdi] == 1 {
                        boss_dead = false
                    };
                    bdi = bdi + 1
                };
                if boss_dead {
                    boss_alive = false;
                    exit_unlocked = true;
                    // Reveal exit: set tile at (9, 7) to EXIT
                    tiles[7 * map_cols() + 9] = TILE_EXIT()
                }
            };

            // Check if player is standing on exit tile and exit is unlocked
            let player_tile_x = float_to_int(floor(player_x));
            let player_tile_y = float_to_int(floor(player_y));
            if exit_unlocked && tile_at(tiles, player_tile_x, player_tile_y) == TILE_EXIT() {
                total_score = total_score + score;
                level_transition_timer = 2.0;
                game_state = STATE_LEVEL_COMPLETE()
            }
```

**Acceptance Criteria**:
- Level transition timer counts down from 2.0 seconds
- When timer expires, `current_level` increments and level data is reloaded
- If `current_level > 3`, transition to `STATE_VICTORY`
- Level 3 exit only unlocks when boss enemy dies (boss_alive becomes false)
- Exit tile appears at position (9, 7) on level 3 when boss dies
- Player standing on exit tile triggers `STATE_LEVEL_COMPLETE`
- Score is accumulated into `total_score` across levels
- All arrays are fully reset during level transition (no stale data)

---

#### T9: Menu Screens — Title, Game Over, Victory, Pause (depends on T7)

**Description**: Implement all non-gameplay screens: title screen, game over screen, victory screen, and pause overlay. Each screen clears the display, draws centered text, and waits for ENTER (or handles restart logic).

**File**: `examples/dungeon_crawler.ryx` — replaces placeholders in the frame callback's `STATE_TITLE`, `STATE_GAME_OVER`, `STATE_VICTORY`, and `STATE_PAUSED` branches

**Code for STATE_TITLE** (replaces placeholder):

```ryx
            // Title screen
            gfx_set_color(gfx_rgb(0, 0, 0));
            gfx_fill_rect(0, 0, screen_w(), screen_h());

            // Title text
            gfx_set_color(gfx_rgb(200, 170, 80));
            gfx_text(180, 150, "RYX DUNGEON CRAWLER");

            // Subtitle
            gfx_set_color(gfx_rgb(160, 160, 160));
            gfx_text(200, 200, "A Roguelike-Lite Demo");

            // Controls info
            gfx_set_color(gfx_rgb(120, 120, 120));
            gfx_text(200, 260, "WASD/Arrows - Move");
            gfx_text(200, 280, "SPACE - Attack");
            gfx_text(200, 300, "ESC - Pause/Quit");

            // Start prompt (blinking effect using frame count approximation)
            gfx_set_color(COLOR_WHITE());
            gfx_text(210, 360, "Press ENTER to start");

            if gfx_key_just_pressed(KEY_ENTER()) {
                game_state = STATE_PLAYING()
            }
```

**Code for STATE_GAME_OVER** (replaces placeholder):

```ryx
            // Game over screen
            gfx_set_color(gfx_rgb(0, 0, 0));
            gfx_fill_rect(0, 0, screen_w(), screen_h());

            gfx_set_color(gfx_rgb(200, 50, 50));
            gfx_text(240, 160, "GAME OVER");

            gfx_set_color(COLOR_WHITE());
            gfx_text(220, 220, "Level: " ++ int_to_string(current_level));
            gfx_text(220, 250, "Score: " ++ int_to_string(total_score + score));

            gfx_set_color(gfx_rgb(160, 160, 160));
            gfx_text(190, 320, "Press ENTER to restart");

            if gfx_key_just_pressed(KEY_ENTER()) {
                // Full game reset
                current_level = 1;
                score = 0;
                total_score = 0;
                player_hp = PLAYER_MAX_HP();
                player_damage_flash = 0.0;
                player_invuln_timer = 0.0;
                player_attack_timer = 0.0;
                boss_spawned_minions = false;

                // Reload level 1
                let rst_tiles = get_level_tiles(1);
                let mut rti = 0;
                while rti < map_len() {
                    tiles[rti] = rst_tiles[rti];
                    rti = rti + 1
                };

                let rst_enemies = get_level_enemies(1);
                enemy_count = rst_enemies[0];
                let mut rei = 0;
                while rei < enemy_count {
                    let rebase = 1 + rei * 3;
                    enemy_type[rei] = rst_enemies[rebase];
                    enemy_x[rei] = int_to_float(rst_enemies[rebase + 1]) + 0.5;
                    enemy_y[rei] = int_to_float(rst_enemies[rebase + 2]) + 0.5;
                    enemy_hp[rei] = enemy_max_hp(rst_enemies[rebase]);
                    enemy_alive[rei] = 1;
                    enemy_dir[rei] = 0.0;
                    enemy_timer[rei] = 0.0;
                    rei = rei + 1
                };
                while rei < MAX_ENEMIES() {
                    enemy_alive[rei] = 0;
                    enemy_type[rei] = 0;
                    rei = rei + 1
                };

                let rst_items = get_level_items(1);
                item_count = rst_items[0];
                let mut rii = 0;
                while rii < item_count {
                    let ribase = 1 + rii * 3;
                    item_type[rii] = rst_items[ribase];
                    item_x[rii] = rst_items[ribase + 1];
                    item_y[rii] = rst_items[ribase + 2];
                    item_active[rii] = 1;
                    rii = rii + 1
                };
                while rii < MAX_ITEMS() {
                    item_active[rii] = 0;
                    item_type[rii] = 0;
                    rii = rii + 1
                };

                player_x = int_to_float(get_level_start_x(1)) + 0.5;
                player_y = int_to_float(get_level_start_y(1)) + 0.5;
                player_target_x = get_level_start_x(1);
                player_target_y = get_level_start_y(1);
                player_start_x = player_x;
                player_start_y = player_y;
                player_moving = false;

                boss_alive = false;
                exit_unlocked = true;

                let mut rpi = 0;
                while rpi < MAX_PARTICLES() {
                    part_active[rpi] = 0;
                    rpi = rpi + 1
                };

                game_state = STATE_TITLE()
            }
```

**Code for STATE_VICTORY** (replaces placeholder):

```ryx
            // Victory screen
            gfx_set_color(gfx_rgb(0, 0, 0));
            gfx_fill_rect(0, 0, screen_w(), screen_h());

            gfx_set_color(gfx_rgb(255, 215, 0));
            gfx_text(180, 140, "VICTORY!");

            gfx_set_color(gfx_rgb(200, 200, 200));
            gfx_text(140, 190, "You cleared all three dungeons!");

            gfx_set_color(COLOR_WHITE());
            gfx_text(210, 240, "Final Score: " ++ int_to_string(total_score));

            gfx_set_color(gfx_rgb(160, 160, 160));
            gfx_text(190, 320, "Press ENTER to play again");

            if gfx_key_just_pressed(KEY_ENTER()) {
                current_level = 1;
                score = 0;
                total_score = 0;
                player_hp = PLAYER_MAX_HP();
                game_state = STATE_TITLE()
            }
```

**Code for STATE_PAUSED** (replaces placeholder):

```ryx
            // Pause overlay — draw semi-transparent black over the existing frame
            gfx_set_color(gfx_rgba(0, 0, 0, 160));
            gfx_fill_rect(0, 0, screen_w(), screen_h());

            gfx_set_color(COLOR_WHITE());
            gfx_text(270, 200, "PAUSED");

            gfx_set_color(gfx_rgb(160, 160, 160));
            gfx_text(210, 260, "Press ESC to resume");
            gfx_text(210, 280, "Press ENTER to quit");

            if gfx_key_just_pressed(KEY_ENTER()) {
                gfx_quit()
            }
```

**Acceptance Criteria**:
- Title screen displays game name, controls, and "Press ENTER to start"
- Pressing ENTER on title transitions to `STATE_PLAYING`
- Game over screen shows level and total score, ENTER restarts from level 1
- Victory screen shows final score, ENTER returns to title
- Pause overlay is semi-transparent, ESC resumes, ENTER quits
- Full reset on game-over restores all state to level 1 initial values
- Read `examples/raycaster.ryx` lines 145-161 for the title screen pattern
- Read `examples/raycaster.ryx` lines 451-484 for the win screen / restart pattern

---

#### T10: Player Input and Movement (depends on T5, T7)

**Description**: Implement player movement with grid-aligned smooth interpolation. When the player presses a direction key and is not currently moving, check if the destination tile is walkable, then start a smooth slide over `PLAYER_MOVE_TIME()` seconds. The player's float position interpolates linearly from the start tile center to the target tile center. Also handle the player facing direction.

**File**: `examples/dungeon_crawler.ryx` — inside the `STATE_PLAYING` branch, replacing the `// [T10]` placeholder

**Algorithm**:
1. If `player_moving` is `true`:
   - Increment `player_move_progress` by `dt / PLAYER_MOVE_TIME()`
   - If `player_move_progress >= 1.0`:
     - Set `player_x` = target x + 0.5, `player_y` = target y + 0.5
     - Set `player_moving = false`, `player_move_progress = 0.0`
   - Else: linearly interpolate `player_x` and `player_y` between start and target
2. If `player_moving` is `false`:
   - Check WASD/arrow keys (prioritize W, then S, then A, then D)
   - Compute `nx, ny` = target tile position
   - If `is_walkable(tiles, nx, ny)`: start movement
     - Set `player_start_x = player_x`, `player_start_y = player_y`
     - Set `player_target_x = nx`, `player_target_y = ny`
     - Set `player_moving = true`, `player_move_progress = 0.0`
     - Set `player_facing` based on direction (0=right, 1=down, 2=left, 3=up)

**Exact code to write**:

```ryx
            // --- Player movement ---
            if player_moving {
                player_move_progress = player_move_progress + dt / PLAYER_MOVE_TIME();
                if player_move_progress >= 1.0 {
                    player_x = int_to_float(player_target_x) + 0.5;
                    player_y = int_to_float(player_target_y) + 0.5;
                    player_moving = false;
                    player_move_progress = 0.0
                } else {
                    let tx = int_to_float(player_target_x) + 0.5;
                    let ty = int_to_float(player_target_y) + 0.5;
                    player_x = player_start_x + (tx - player_start_x) * player_move_progress;
                    player_y = player_start_y + (ty - player_start_y) * player_move_progress
                }
            } else {
                let cur_tx = float_to_int(floor(player_x));
                let cur_ty = float_to_int(floor(player_y));

                if gfx_key_pressed(KEY_W()) || gfx_key_pressed(KEY_UP()) {
                    let nx = cur_tx;
                    let ny = cur_ty - 1;
                    if is_walkable(tiles, nx, ny) {
                        player_start_x = player_x;
                        player_start_y = player_y;
                        player_target_x = nx;
                        player_target_y = ny;
                        player_moving = true;
                        player_move_progress = 0.0;
                        player_facing = 3
                    }
                } else if gfx_key_pressed(KEY_S()) || gfx_key_pressed(KEY_DOWN()) {
                    let nx = cur_tx;
                    let ny = cur_ty + 1;
                    if is_walkable(tiles, nx, ny) {
                        player_start_x = player_x;
                        player_start_y = player_y;
                        player_target_x = nx;
                        player_target_y = ny;
                        player_moving = true;
                        player_move_progress = 0.0;
                        player_facing = 1
                    }
                } else if gfx_key_pressed(KEY_A()) || gfx_key_pressed(KEY_LEFT()) {
                    let nx = cur_tx - 1;
                    let ny = cur_ty;
                    if is_walkable(tiles, nx, ny) {
                        player_start_x = player_x;
                        player_start_y = player_y;
                        player_target_x = nx;
                        player_target_y = ny;
                        player_moving = true;
                        player_move_progress = 0.0;
                        player_facing = 2
                    }
                } else if gfx_key_pressed(KEY_D()) || gfx_key_pressed(KEY_RIGHT()) {
                    let nx = cur_tx + 1;
                    let ny = cur_ty;
                    if is_walkable(tiles, nx, ny) {
                        player_start_x = player_x;
                        player_start_y = player_y;
                        player_target_x = nx;
                        player_target_y = ny;
                        player_moving = true;
                        player_move_progress = 0.0;
                        player_facing = 0
                    }
                }
            };

            // Decrement attack cooldown
            if player_attack_timer > 0.0 {
                player_attack_timer = player_attack_timer - dt
            };

            // Decrement damage flash
            if player_damage_flash > 0.0 {
                player_damage_flash = player_damage_flash - dt
            };

            // Decrement invulnerability
            if player_invuln_timer > 0.0 {
                player_invuln_timer = player_invuln_timer - dt
            };
```

**Acceptance Criteria**:
- Player moves smoothly from one tile to the next over 0.15 seconds
- Player cannot move to wall tiles (type 1, 4, 5)
- Player can walk through doors (type 2) and onto exit (type 3)
- Only one movement key is processed at a time (no diagonal movement)
- `player_facing` correctly tracks the last movement direction
- Movement starts only when the player is not already moving
- Timer decrements (attack cooldown, damage flash, invulnerability) happen every frame
- Read `examples/raycaster.ryx` lines 178-250 for the key-press movement pattern

---

#### T11: Enemy AI and Movement (depends on T5, T7)

**Description**: Implement enemy AI with two behaviors: patrol (random movement when player is far away) and chase (move toward player when in detection range). Ghosts ignore wall collision. The boss spawns minion slimes when its HP drops below 50%. Each enemy moves at its type-specific speed.

**File**: `examples/dungeon_crawler.ryx` — inside the `STATE_PLAYING` branch, after player movement code (T10)

**Algorithm for each enemy**:
1. If `enemy_alive[i] == 0`, skip
2. Compute distance from enemy position to player position using `dist()`
3. If distance < `enemy_detect_range(enemy_type[i])`:
   - **Chase mode**: compute direction vector from enemy to player
   - Normalize: `dx = (player_x - enemy_x[i]) / d`, `dy = (player_y - enemy_y[i]) / d`
   - Move: `new_x = enemy_x[i] + dx * speed * dt`, `new_y = enemy_y[i] + dy * speed * dt`
   - For non-ghost enemies: check if new position is walkable before moving (convert to tile coords and check `is_walkable`)
   - For ghosts (type 3): skip walkability check, move through walls
   - Special for skeletons (type 2): if HP < 15, reverse direction (retreat instead of chase)
4. If distance >= detect range:
   - **Patrol mode**: use `enemy_timer` as a countdown. When timer <= 0:
     - Pick a new random direction: `enemy_dir[i] = random_float() * 2.0 * pi()`
     - Reset timer to 1.0 + random_float() * 2.0 (1-3 seconds)
   - Move in current direction at 50% speed
   - Check walkability for non-ghost enemies; if blocked, pick a new direction immediately
5. Boss special: if `enemy_type[i] == ENEMY_BOSS()` and `enemy_hp[i] < 75` and `!boss_spawned_minions`:
   - Spawn 2 slimes at positions near the boss
   - Set `boss_spawned_minions = true`

**Exact code to write**:

```ryx
            // --- Enemy AI and movement ---
            let mut eai = 0;
            while eai < enemy_count {
                if enemy_alive[eai] == 1 {
                    let etype = enemy_type[eai];
                    let espeed = enemy_speed(etype) * dt;
                    let erange = enemy_detect_range(etype);
                    let ex = enemy_x[eai];
                    let ey = enemy_y[eai];
                    let d = dist(ex, ey, player_x, player_y);

                    if d < erange && d > 0.01 {
                        // Chase mode
                        let dx = (player_x - ex) / d;
                        let dy = (player_y - ey) / d;

                        // Skeleton retreat when low HP
                        let chase_dx = if etype == ENEMY_SKELETON() && enemy_hp[eai] < 15 {
                            0.0 - dx
                        } else { dx };
                        let chase_dy = if etype == ENEMY_SKELETON() && enemy_hp[eai] < 15 {
                            0.0 - dy
                        } else { dy };

                        let new_ex = ex + chase_dx * espeed;
                        let new_ey = ey + chase_dy * espeed;

                        if etype == ENEMY_GHOST() {
                            // Ghosts move through walls
                            enemy_x[eai] = new_ex;
                            enemy_y[eai] = new_ey
                        } else {
                            // Check walkability for each axis independently
                            let ntx = float_to_int(floor(new_ex));
                            let nty = float_to_int(floor(new_ey));
                            let otx = float_to_int(floor(ex));
                            let oty = float_to_int(floor(ey));

                            if is_walkable(tiles, ntx, oty) {
                                enemy_x[eai] = new_ex
                            };
                            if is_walkable(tiles, otx, nty) {
                                enemy_y[eai] = new_ey
                            }
                        }
                    } else {
                        // Patrol mode
                        enemy_timer[eai] = enemy_timer[eai] - dt;
                        if enemy_timer[eai] <= 0.0 {
                            enemy_dir[eai] = random_float() * 2.0 * pi();
                            enemy_timer[eai] = 1.0 + random_float() * 2.0
                        };

                        let patrol_speed = espeed * 0.5;
                        let pdx = cos(enemy_dir[eai]) * patrol_speed;
                        let pdy = sin(enemy_dir[eai]) * patrol_speed;
                        let pnx = ex + pdx;
                        let pny = ey + pdy;

                        if etype == ENEMY_GHOST() {
                            enemy_x[eai] = pnx;
                            enemy_y[eai] = pny
                        } else {
                            let ptx = float_to_int(floor(pnx));
                            let pty = float_to_int(floor(pny));
                            if is_walkable(tiles, ptx, float_to_int(floor(ey))) {
                                enemy_x[eai] = pnx
                            } else {
                                enemy_dir[eai] = random_float() * 2.0 * pi()
                            };
                            if is_walkable(tiles, float_to_int(floor(ex)), pty) {
                                enemy_y[eai] = pny
                            } else {
                                enemy_dir[eai] = random_float() * 2.0 * pi()
                            }
                        }
                    };

                    // Boss special: spawn minions at 50% HP
                    if etype == ENEMY_BOSS() && !boss_spawned_minions {
                        if enemy_hp[eai] < 75 {
                            boss_spawned_minions = true;
                            // Spawn 2 slimes near boss if there are open slots
                            if enemy_count < MAX_ENEMIES() - 1 {
                                let spawn_idx1 = enemy_count;
                                enemy_type[spawn_idx1] = ENEMY_SLIME();
                                enemy_x[spawn_idx1] = ex + 1.0;
                                enemy_y[spawn_idx1] = ey;
                                enemy_hp[spawn_idx1] = enemy_max_hp(ENEMY_SLIME());
                                enemy_alive[spawn_idx1] = 1;
                                enemy_dir[spawn_idx1] = 0.0;
                                enemy_timer[spawn_idx1] = 0.0;

                                let spawn_idx2 = enemy_count + 1;
                                enemy_type[spawn_idx2] = ENEMY_SLIME();
                                enemy_x[spawn_idx2] = ex - 1.0;
                                enemy_y[spawn_idx2] = ey;
                                enemy_hp[spawn_idx2] = enemy_max_hp(ENEMY_SLIME());
                                enemy_alive[spawn_idx2] = 1;
                                enemy_dir[spawn_idx2] = 0.0;
                                enemy_timer[spawn_idx2] = 0.0;

                                enemy_count = enemy_count + 2
                            }
                        }
                    }
                };
                eai = eai + 1
            };
```

**Acceptance Criteria**:
- Enemies only update when `enemy_alive[i] == 1`
- Enemies within detect range move toward player (or retreat if skeleton with low HP)
- Enemies outside detect range patrol randomly, changing direction every 1-3 seconds
- Non-ghost enemies respect wall collision (per-axis check)
- Ghost enemies move through walls
- Boss spawns 2 slimes when HP drops below 75 (half of 150)
- Spawned slimes are added to the enemy arrays at `enemy_count` and `enemy_count+1`
- `enemy_count` is incremented by 2 after spawn
- Guard against exceeding `MAX_ENEMIES()` (32)
- All movement is multiplied by `dt` for frame-rate independence
- Distance computed using the `dist()` helper from T2

---

#### T12: Player Attack — SPACE to Attack Enemies in Range (depends on T5, T7, T10)

**Description**: When the player presses SPACE and the attack cooldown has expired, deal damage to all enemies within `PLAYER_ATTACK_RANGE()` tile units. Spawn attack particles in the facing direction. When an enemy's HP drops to 0, mark it as dead and spawn death particles.

**File**: `examples/dungeon_crawler.ryx` — inside the `STATE_PLAYING` branch, after enemy AI code (T11)

**Exact code to write**:

```ryx
            // --- Player attack ---
            if gfx_key_just_pressed(KEY_SPACE()) && player_attack_timer <= 0.0 {
                player_attack_timer = PLAYER_ATTACK_COOLDOWN();

                // Spawn attack particles in facing direction
                let attack_angle = if player_facing == 0 { 0.0 }
                    else if player_facing == 1 { pi() / 2.0 }
                    else if player_facing == 2 { pi() }
                    else { 0.0 - pi() / 2.0 };

                let mut atk_pi = 0;
                while atk_pi < 6 {
                    // Find an inactive particle slot
                    let mut slot = 0;
                    let mut found_slot = false;
                    while slot < MAX_PARTICLES() && !found_slot {
                        if part_active[slot] == 0 {
                            found_slot = true
                        } else {
                            slot = slot + 1
                        }
                    };
                    if found_slot {
                        part_x[slot] = player_x;
                        part_y[slot] = player_y;
                        part_vx[slot] = attack_vx(attack_angle, atk_pi, 6, 3.0);
                        part_vy[slot] = attack_vy(attack_angle, atk_pi, 6, 3.0);
                        part_life[slot] = 0.3;
                        part_color[slot] = COLOR_WHITE();
                        part_active[slot] = 1
                    };
                    atk_pi = atk_pi + 1
                };

                // Damage enemies in range
                let mut dai = 0;
                while dai < enemy_count {
                    if enemy_alive[dai] == 1 {
                        let ed = dist(player_x, player_y, enemy_x[dai], enemy_y[dai]);
                        if ed <= PLAYER_ATTACK_RANGE() {
                            // Random damage between min and max
                            let dmg = random_int(PLAYER_ATTACK_MIN_DMG(), PLAYER_ATTACK_MAX_DMG() + 1);
                            enemy_hp[dai] = enemy_hp[dai] - dmg;

                            if enemy_hp[dai] <= 0 {
                                // Enemy killed
                                enemy_alive[dai] = 0;
                                score = score + 10 * enemy_type[dai];

                                // Spawn death particles (12 in enemy color)
                                let death_color = enemy_color(enemy_type[dai]);
                                let mut dpi = 0;
                                while dpi < 12 {
                                    let mut dslot = 0;
                                    let mut dfound = false;
                                    while dslot < MAX_PARTICLES() && !dfound {
                                        if part_active[dslot] == 0 {
                                            dfound = true
                                        } else {
                                            dslot = dslot + 1
                                        }
                                    };
                                    if dfound {
                                        part_x[dslot] = enemy_x[dai];
                                        part_y[dslot] = enemy_y[dai];
                                        part_vx[dslot] = burst_vx(dpi, 12, 2.5);
                                        part_vy[dslot] = burst_vy(dpi, 12, 2.5);
                                        part_life[dslot] = 0.6;
                                        part_color[dslot] = death_color;
                                        part_active[dslot] = 1
                                    };
                                    dpi = dpi + 1
                                }
                            }
                        }
                    };
                    dai = dai + 1
                }
            };
```

**Acceptance Criteria**:
- Attack only triggers when SPACE is just pressed (not held) AND cooldown has expired
- Attack cooldown is set to 0.4 seconds after each attack
- 6 white particles spawn in a semicircular arc in the player's facing direction
- All enemies within 1.5 tile units take 20-35 random damage
- When enemy HP <= 0, `enemy_alive[i]` is set to 0
- Score increases by `10 * enemy_type` (slime=10, skeleton=20, ghost=30, boss=40)
- 12 particles in the enemy's color spawn at the enemy's position on death
- Particle slot search skips active particles and uses first available inactive slot
- Uses `burst_vx/vy` from T4 for death particles and `attack_vx/vy` for attack particles

---

#### T13: Enemy Contact Damage (depends on T5, T7, T10)

**Description**: Enemies that touch the player deal their type-specific damage. The player has a brief invulnerability window after being hit (0.5 seconds). When player HP reaches 0, transition to game over. Spawn red damage particles when player is hit.

**File**: `examples/dungeon_crawler.ryx` — inside the `STATE_PLAYING` branch, after player attack code (T12)

**Exact code to write**:

```ryx
            // --- Enemy contact damage ---
            if player_invuln_timer <= 0.0 {
                let mut eci = 0;
                while eci < enemy_count {
                    if enemy_alive[eci] == 1 {
                        let contact_dist = dist(player_x, player_y, enemy_x[eci], enemy_y[eci]);
                        if contact_dist < 0.7 {
                            // Player takes damage
                            let edamage = enemy_damage(enemy_type[eci]);
                            player_hp = player_hp - edamage;
                            player_damage_flash = 0.3;
                            player_invuln_timer = 0.5;

                            // Spawn 4 red damage particles
                            let mut dmi = 0;
                            while dmi < 4 {
                                let mut dmslot = 0;
                                let mut dmfound = false;
                                while dmslot < MAX_PARTICLES() && !dmfound {
                                    if part_active[dmslot] == 0 {
                                        dmfound = true
                                    } else {
                                        dmslot = dmslot + 1
                                    }
                                };
                                if dmfound {
                                    part_x[dmslot] = player_x;
                                    part_y[dmslot] = player_y;
                                    part_vx[dmslot] = burst_vx(dmi, 4, 2.0);
                                    part_vy[dmslot] = burst_vy(dmi, 4, 2.0);
                                    part_life[dmslot] = 0.3;
                                    part_color[dmslot] = COLOR_RED();
                                    part_active[dmslot] = 1
                                };
                                dmi = dmi + 1
                            };

                            // Check for death
                            if player_hp <= 0 {
                                player_hp = 0;
                                game_state = STATE_GAME_OVER()
                            }
                        }
                    };
                    eci = eci + 1
                }
            };
```

**Acceptance Criteria**:
- Contact damage only occurs when `player_invuln_timer <= 0.0`
- Enemy-player contact threshold is 0.7 tile units (slightly less than 1 tile)
- Player HP decreases by the enemy's damage value
- `player_damage_flash` is set to 0.3 for visual feedback
- `player_invuln_timer` is set to 0.5 seconds to prevent rapid damage
- 4 red particles spawn at player position on hit
- When player HP reaches 0, game transitions to `STATE_GAME_OVER`
- Only the first enemy in contact range triggers the damage (invulnerability prevents stacking)

---

#### T14: Item Pickup (depends on T5, T7, T10)

**Description**: When the player walks over an active item, collect it: apply its effect (heal or score), spawn pickup particles, and deactivate the item.

**File**: `examples/dungeon_crawler.ryx` — inside the `STATE_PLAYING` branch, after contact damage code (T13)

**Exact code to write**:

```ryx
            // --- Item pickup ---
            let ptx = float_to_int(floor(player_x));
            let pty = float_to_int(floor(player_y));

            let mut ipi = 0;
            while ipi < item_count {
                if item_active[ipi] == 1 {
                    if item_x[ipi] == ptx && item_y[ipi] == pty {
                        // Collect item
                        item_active[ipi] = 0;

                        let itype = item_type[ipi];
                        if itype == ITEM_HEALTH() {
                            player_hp = min(player_hp + 30, PLAYER_MAX_HP())
                        } else if itype == ITEM_GEM() {
                            score = score + 50
                        };

                        // Spawn 8 pickup particles (spiral upward in item color)
                        let pickup_color = item_color(itype);
                        let item_center_x = int_to_float(item_x[ipi]) + 0.5;
                        let item_center_y = int_to_float(item_y[ipi]) + 0.5;

                        let mut ppi = 0;
                        while ppi < 8 {
                            let mut ppslot = 0;
                            let mut ppfound = false;
                            while ppslot < MAX_PARTICLES() && !ppfound {
                                if part_active[ppslot] == 0 {
                                    ppfound = true
                                } else {
                                    ppslot = ppslot + 1
                                }
                            };
                            if ppfound {
                                part_x[ppslot] = item_center_x;
                                part_y[ppslot] = item_center_y;
                                part_vx[ppslot] = spiral_vx(ppi, 8, 1.5);
                                part_vy[ppslot] = spiral_vy(ppi, 8, 2.0);
                                part_life[ppslot] = 0.4;
                                part_color[ppslot] = pickup_color;
                                part_active[ppslot] = 1
                            };
                            ppi = ppi + 1
                        }
                    }
                };
                ipi = ipi + 1
            };
```

**Acceptance Criteria**:
- Items are collected when the player's tile position matches the item's tile position
- Health potions restore 30 HP, capped at `PLAYER_MAX_HP()` (100)
- Gems add 50 to score
- Collected items have `item_active[i]` set to 0
- 8 particles spawn at the item's center in the item's color using `spiral_vx/vy`
- Only active items (`item_active[i] == 1`) are checked
- Uses `spiral_vx/vy` from T4 for upward spiral effect

---

### Wave 4 — Rendering & Particles (T15-T17, T15 parallel with T16, T17 depends on both)

---

#### T15: Particle System Update (depends on T5, T7)

**Description**: Update all active particles each frame: move by velocity, decrement lifetime, deactivate when lifetime expires. This runs inside the gameplay update section, after all entity logic.

**File**: `examples/dungeon_crawler.ryx` — inside the `STATE_PLAYING` branch, after item pickup code (T14)

**Exact code to write**:

```ryx
            // --- Particle update ---
            let mut pui = 0;
            while pui < MAX_PARTICLES() {
                if part_active[pui] == 1 {
                    part_life[pui] = part_life[pui] - dt;
                    if part_life[pui] <= 0.0 {
                        part_active[pui] = 0
                    } else {
                        part_x[pui] = part_x[pui] + part_vx[pui] * dt;
                        part_y[pui] = part_y[pui] + part_vy[pui] * dt;
                        // Apply light friction to slow particles down
                        part_vx[pui] = part_vx[pui] * 0.96;
                        part_vy[pui] = part_vy[pui] * 0.96
                    }
                };
                pui = pui + 1
            };
```

**Acceptance Criteria**:
- All 64 particle slots are checked each frame
- Active particles have their position updated by velocity * dt
- Lifetime decreases by dt each frame
- Particles with lifetime <= 0 are deactivated (`part_active[i] = 0`)
- Velocity decays by 4% per frame (friction factor 0.96) for natural deceleration
- Inactive particles are not updated

---

#### T16: Tile, Entity, and Particle Rendering (depends on T5, T7)

**Description**: Draw the complete game scene: tiles (with optional wall textures), items (as colored shapes), enemies (as colored rectangles with simple directional indicators), particles (as small colored rectangles fading with lifetime), and the player (as a colored rectangle with facing indicator). Draw order: tiles first, then items, then enemies, then particles, then player (player always on top).

**File**: `examples/dungeon_crawler.ryx` — inside the `STATE_PLAYING` branch, after particle update (T15)

**Exact code to write**:

```ryx
            // ===============================================================
            // RENDERING
            // ===============================================================

            // --- Draw tiles ---
            let mut ry = 0;
            while ry < map_rows() {
                let mut rx = 0;
                while rx < map_cols() {
                    let tile = tile_at(tiles, rx, ry);
                    let px = rx * tile_size();
                    let py = ry * tile_size();

                    if tile == TILE_FLOOR() || tile == TILE_DOOR() {
                        // Draw floor with checkerboard pattern
                        gfx_set_color(floor_color(rx, ry));
                        gfx_fill_rect(px, py, tile_size(), tile_size());
                        // Draw door frame if door tile
                        if tile == TILE_DOOR() {
                            gfx_set_color(gfx_rgb(140, 90, 40));
                            gfx_rect(px + 2, py + 2, tile_size() - 4, tile_size() - 4)
                        }
                    } else if tile == TILE_WALL() {
                        // Try to draw wall texture, fall back to colored rect
                        if wall_img >= 0 {
                            gfx_draw_image_scaled(wall_img, px, py,
                                int_to_float(tile_size()) / int_to_float(gfx_image_width(wall_img)),
                                int_to_float(tile_size()) / int_to_float(gfx_image_height(wall_img)))
                        } else {
                            gfx_set_color(tile_color(TILE_WALL()));
                            gfx_fill_rect(px, py, tile_size(), tile_size())
                        };
                        // Draw subtle grid line
                        gfx_set_color(gfx_rgba(0, 0, 0, 60));
                        gfx_rect(px, py, tile_size(), tile_size())
                    } else if tile == TILE_WALL2() {
                        // Mossy wall variant
                        if wall2_img >= 0 {
                            gfx_draw_image_scaled(wall2_img, px, py,
                                int_to_float(tile_size()) / int_to_float(gfx_image_width(wall2_img)),
                                int_to_float(tile_size()) / int_to_float(gfx_image_height(wall2_img)))
                        } else {
                            gfx_set_color(tile_color(TILE_WALL2()));
                            gfx_fill_rect(px, py, tile_size(), tile_size())
                        };
                        gfx_set_color(gfx_rgba(0, 0, 0, 60));
                        gfx_rect(px, py, tile_size(), tile_size())
                    } else if tile == TILE_EXIT() {
                        // Exit tile — pulsing green glow
                        gfx_set_color(tile_color(TILE_EXIT()));
                        gfx_fill_rect(px, py, tile_size(), tile_size());
                        // Draw staircase symbol (diagonal lines)
                        gfx_set_color(gfx_rgb(200, 255, 200));
                        gfx_line(px + 4, py + 28, px + 12, py + 20);
                        gfx_line(px + 12, py + 20, px + 20, py + 12);
                        gfx_line(px + 20, py + 12, px + 28, py + 4)
                    } else if tile == TILE_WATER() {
                        // Water tile
                        gfx_set_color(tile_color(TILE_WATER()));
                        gfx_fill_rect(px, py, tile_size(), tile_size());
                        // Simple wave lines
                        gfx_set_color(gfx_rgba(80, 100, 180, 200));
                        gfx_line(px + 2, py + 10, px + 30, py + 12);
                        gfx_line(px + 2, py + 20, px + 30, py + 22)
                    };
                    rx = rx + 1
                };
                ry = ry + 1
            };

            // --- Draw items ---
            let mut iri = 0;
            while iri < item_count {
                if item_active[iri] == 1 {
                    let ipx = item_x[iri] * tile_size() + tile_size() / 4;
                    let ipy = item_y[iri] * tile_size() + tile_size() / 4;
                    let isize = tile_size() / 2;

                    gfx_set_color(item_color(item_type[iri]));

                    if item_type[iri] == ITEM_HEALTH() {
                        // Health potion: red square with white cross
                        gfx_fill_rect(ipx, ipy, isize, isize);
                        gfx_set_color(COLOR_WHITE());
                        gfx_fill_rect(ipx + isize / 2 - 1, ipy + 2, 3, isize - 4);
                        gfx_fill_rect(ipx + 2, ipy + isize / 2 - 1, isize - 4, 3)
                    } else if item_type[iri] == ITEM_GEM() {
                        // Gem: cyan diamond (drawn as rotated square using lines)
                        let cx = ipx + isize / 2;
                        let cy = ipy + isize / 2;
                        let hr = isize / 2;
                        gfx_line(cx, cy - hr, cx + hr, cy);
                        gfx_line(cx + hr, cy, cx, cy + hr);
                        gfx_line(cx, cy + hr, cx - hr, cy);
                        gfx_line(cx - hr, cy, cx, cy - hr);
                        // Fill center
                        gfx_fill_rect(cx - 2, cy - 2, 5, 5)
                    } else if item_type[iri] == ITEM_KEY() {
                        // Key: gold square
                        gfx_fill_rect(ipx, ipy, isize, isize)
                    }
                };
                iri = iri + 1
            };

            // --- Draw enemies ---
            let mut eri = 0;
            while eri < enemy_count {
                if enemy_alive[eri] == 1 {
                    let etype_r = enemy_type[eri];
                    let esize_r = enemy_size(etype_r);
                    let epx = float_to_int(enemy_x[eri] * int_to_float(tile_size())) - esize_r / 2;
                    let epy = float_to_int(enemy_y[eri] * int_to_float(tile_size())) - esize_r / 2;

                    // Draw enemy body
                    gfx_set_color(enemy_color(etype_r));
                    gfx_fill_rect(epx, epy, esize_r, esize_r);

                    // Draw dark outline
                    gfx_set_color(gfx_rgba(0, 0, 0, 100));
                    gfx_rect(epx, epy, esize_r, esize_r);

                    // Draw HP bar above enemy (if damaged)
                    if enemy_hp[eri] < enemy_max_hp(etype_r) {
                        let hp_bar_w = esize_r;
                        let hp_bar_h = 3;
                        let hp_bar_x = epx;
                        let hp_bar_y = epy - 5;
                        let hp_ratio = int_to_float(enemy_hp[eri]) / int_to_float(enemy_max_hp(etype_r));
                        let hp_fill = float_to_int(int_to_float(hp_bar_w) * hp_ratio);

                        // Background (dark red)
                        gfx_set_color(gfx_rgb(100, 0, 0));
                        gfx_fill_rect(hp_bar_x, hp_bar_y, hp_bar_w, hp_bar_h);
                        // Fill (green to red gradient based on health)
                        let bar_color = color_lerp(COLOR_RED(), COLOR_GREEN(), hp_ratio);
                        gfx_set_color(bar_color);
                        gfx_fill_rect(hp_bar_x, hp_bar_y, hp_fill, hp_bar_h)
                    }
                };
                eri = eri + 1
            };

            // --- Draw particles ---
            let mut pri = 0;
            while pri < MAX_PARTICLES() {
                if part_active[pri] == 1 {
                    let ppx = float_to_int(part_x[pri] * int_to_float(tile_size()));
                    let ppy = float_to_int(part_y[pri] * int_to_float(tile_size()));

                    // Fade alpha based on remaining lifetime
                    let base_color = part_color[pri];
                    let alpha = float_to_int(clamp(part_life[pri] * 400.0, 0.0, 255.0));
                    let faded = gfx_rgba(color_r(base_color), color_g(base_color), color_b(base_color), alpha);
                    gfx_set_color(faded);

                    // Draw as small filled rect (3x3 pixels)
                    gfx_fill_rect(ppx - 1, ppy - 1, 3, 3)
                };
                pri = pri + 1
            };

            // --- Draw player ---
            let player_px = float_to_int(player_x * int_to_float(tile_size())) - 12;
            let player_py = float_to_int(player_y * int_to_float(tile_size())) - 12;
            let player_size = 24;

            // Damage flash effect: alternate between normal and red
            let player_base_color = if player_damage_flash > 0.0 {
                // Flash red/white alternation
                if float_to_int(player_damage_flash * 20.0) % 2 == 0 {
                    gfx_rgb(255, 100, 100)
                } else {
                    gfx_rgb(80, 140, 220)
                }
            } else {
                gfx_rgb(80, 140, 220)  // Normal blue color
            };

            gfx_set_color(player_base_color);
            gfx_fill_rect(player_px, player_py, player_size, player_size);

            // Draw facing indicator (small triangle/line in facing direction)
            gfx_set_color(COLOR_WHITE());
            let pcx = player_px + player_size / 2;
            let pcy = player_py + player_size / 2;
            if player_facing == 0 {
                // Right
                gfx_line(pcx + 4, pcy, pcx + 12, pcy)
            } else if player_facing == 1 {
                // Down
                gfx_line(pcx, pcy + 4, pcx, pcy + 12)
            } else if player_facing == 2 {
                // Left
                gfx_line(pcx - 4, pcy, pcx - 12, pcy)
            } else {
                // Up
                gfx_line(pcx, pcy - 4, pcx, pcy - 12)
            };

            // Draw player outline
            gfx_set_color(gfx_rgba(255, 255, 255, 100));
            gfx_rect(player_px, player_py, player_size, player_size);
```

**Acceptance Criteria**:
- All 300 tiles (20x15) are rendered each frame
- Floor tiles use checkerboard pattern via `floor_color()`
- Wall tiles use PNG texture if loaded (`wall_img >= 0`), else solid color
- Wall variant tiles use moss texture if loaded, else solid color
- Exit tile has staircase visual (diagonal lines)
- Water tiles have simple wave lines
- Items draw with type-specific visuals (cross for health, diamond for gem)
- Enemies draw as colored rectangles centered at their float position
- Enemies show HP bars when damaged (green-to-red gradient)
- Particles draw as 3x3 fading rectangles with alpha based on lifetime
- Player draws as 24x24 blue rectangle with white facing indicator line
- Player flashes red/normal when `player_damage_flash > 0`
- Draw order: tiles -> items -> enemies -> particles -> player

---

#### T17: HUD and Minimap (depends on T16)

**Description**: Draw the heads-up display overlay and minimap on top of the game scene. The HUD shows player health (as a bar), score, level number, remaining enemies, and FPS. The minimap is a small overview of the entire level.

**File**: `examples/dungeon_crawler.ryx` — inside the `STATE_PLAYING` branch, after all rendering code (T16), before the level completion check (T8)

**Exact code to write**:

```ryx
            // ===============================================================
            // HUD OVERLAY
            // ===============================================================

            // Semi-transparent HUD bar across the top
            gfx_set_color(gfx_rgba(0, 0, 0, 180));
            gfx_fill_rect(0, 0, screen_w(), 30);

            // Health bar (left side)
            let hp_bar_x_hud = 10;
            let hp_bar_y_hud = 8;
            let hp_bar_w_hud = 120;
            let hp_bar_h_hud = 14;
            let hp_pct = int_to_float(player_hp) / int_to_float(PLAYER_MAX_HP());
            let hp_fill_w = float_to_int(int_to_float(hp_bar_w_hud) * hp_pct);

            // HP bar background
            gfx_set_color(gfx_rgb(60, 0, 0));
            gfx_fill_rect(hp_bar_x_hud, hp_bar_y_hud, hp_bar_w_hud, hp_bar_h_hud);

            // HP bar fill (green when high, yellow when medium, red when low)
            let hp_color = if hp_pct > 0.6 { gfx_rgb(50, 200, 50) }
                else if hp_pct > 0.3 { gfx_rgb(200, 200, 50) }
                else { gfx_rgb(200, 50, 50) };
            gfx_set_color(hp_color);
            gfx_fill_rect(hp_bar_x_hud, hp_bar_y_hud, hp_fill_w, hp_bar_h_hud);

            // HP bar outline
            gfx_set_color(gfx_rgba(255, 255, 255, 100));
            gfx_rect(hp_bar_x_hud, hp_bar_y_hud, hp_bar_w_hud, hp_bar_h_hud);

            // HP text
            gfx_set_color(COLOR_WHITE());
            gfx_text(hp_bar_x_hud + 2, hp_bar_y_hud + 1,
                int_to_string(player_hp) ++ "/" ++ int_to_string(PLAYER_MAX_HP()));

            // Level number (center)
            gfx_text(280, 8, "Level " ++ int_to_string(current_level));

            // Score
            gfx_set_color(gfx_rgb(255, 215, 0));
            gfx_text(370, 8, "Score: " ++ int_to_string(score));

            // Enemy count
            let mut hud_alive = 0;
            let mut hai = 0;
            while hai < enemy_count {
                if enemy_alive[hai] == 1 {
                    hud_alive = hud_alive + 1
                };
                hai = hai + 1
            };
            gfx_set_color(gfx_rgb(200, 100, 100));
            gfx_text(480, 8, "Foes: " ++ int_to_string(hud_alive));

            // FPS (right side)
            gfx_set_color(gfx_rgb(120, 120, 120));
            gfx_text(580, 8, "FPS:" ++ int_to_string(float_to_int(gfx_fps())));

            // Attack cooldown indicator (small bar under HUD)
            if player_attack_timer > 0.0 {
                let cd_pct = player_attack_timer / PLAYER_ATTACK_COOLDOWN();
                let cd_w = float_to_int(60.0 * cd_pct);
                gfx_set_color(gfx_rgba(200, 200, 200, 120));
                gfx_fill_rect(140, 22, cd_w, 4)
            };

            // ===============================================================
            // MINIMAP (top-right corner)
            // ===============================================================

            let mm_x = screen_w() - 84;  // 640 - 84 = 556
            let mm_y = 34;                // just below HUD bar
            let mm_tw = 4;               // pixels per tile width
            let mm_th = 4;               // pixels per tile height

            // Minimap background
            gfx_set_color(gfx_rgba(0, 0, 0, 160));
            gfx_fill_rect(mm_x - 2, mm_y - 2, map_cols() * mm_tw + 4, map_rows() * mm_th + 4);

            // Draw tiles on minimap
            let mut mmy = 0;
            while mmy < map_rows() {
                let mut mmx = 0;
                while mmx < map_cols() {
                    let mtile = tile_at(tiles, mmx, mmy);
                    if mtile == TILE_WALL() || mtile == TILE_WALL2() {
                        gfx_set_color(gfx_rgb(140, 130, 120));
                        gfx_fill_rect(mm_x + mmx * mm_tw, mm_y + mmy * mm_th, mm_tw, mm_th)
                    } else if mtile == TILE_WATER() {
                        gfx_set_color(gfx_rgb(40, 60, 140));
                        gfx_fill_rect(mm_x + mmx * mm_tw, mm_y + mmy * mm_th, mm_tw, mm_th)
                    } else if mtile == TILE_EXIT() {
                        gfx_set_color(gfx_rgb(0, 255, 100));
                        gfx_fill_rect(mm_x + mmx * mm_tw, mm_y + mmy * mm_th, mm_tw, mm_th)
                    };
                    mmx = mmx + 1
                };
                mmy = mmy + 1
            };

            // Draw items on minimap as cyan dots
            let mut mmi = 0;
            while mmi < item_count {
                if item_active[mmi] == 1 {
                    gfx_set_color(gfx_rgb(0, 255, 255));
                    gfx_fill_rect(
                        mm_x + item_x[mmi] * mm_tw + 1,
                        mm_y + item_y[mmi] * mm_th + 1,
                        2, 2)
                };
                mmi = mmi + 1
            };

            // Draw enemies on minimap as red dots
            let mut mme = 0;
            while mme < enemy_count {
                if enemy_alive[mme] == 1 {
                    gfx_set_color(COLOR_RED());
                    let emx = float_to_int(enemy_x[mme]) * mm_tw;
                    let emy = float_to_int(enemy_y[mme]) * mm_th;
                    gfx_fill_rect(mm_x + emx, mm_y + emy, 3, 3)
                };
                mme = mme + 1
            };

            // Draw player on minimap as white dot
            gfx_set_color(COLOR_WHITE());
            let pmmx = float_to_int(player_x) * mm_tw;
            let pmmy = float_to_int(player_y) * mm_th;
            gfx_fill_rect(mm_x + pmmx, mm_y + pmmy, 3, 3);
```

**Acceptance Criteria**:
- HUD bar is 30 pixels tall, semi-transparent black, full screen width
- Health bar shows green (>60%), yellow (30-60%), or red (<30%) fill
- HP text shows current/max (e.g., "75/100")
- Level number, score, enemy count, and FPS are displayed
- Attack cooldown shows as a small diminishing bar
- Minimap is positioned at top-right, 80x60 pixels (20 tiles * 4px, 15 tiles * 4px)
- Minimap has semi-transparent black background with 2px padding
- Walls shown as gray, water as blue, exit as green on minimap
- Items shown as cyan 2x2 dots
- Enemies shown as red 3x3 dots
- Player shown as white 3x3 dot
- Read `examples/raycaster.ryx` lines 378-425 for the minimap pattern

---

### Wave 5 — Integration & Testing (T18, depends on all)

---

#### T18: Integration Test and Asset Creation (depends on all previous tasks)

**Description**: Add a compile-only integration test that verifies `examples/dungeon_crawler.ryx` compiles without errors. Create minimal placeholder wall texture PNGs. Verify the complete file compiles and runs.

**Files**:
- `tests/integration/e2e_test.go` — add compile-only test
- `examples/assets/wall_stone.png` — create 32x32 gray brick texture
- `examples/assets/wall_moss.png` — create 32x32 green-gray texture

**Code for integration test** (add to `tests/integration/e2e_test.go`):

```go
func TestDungeonCrawlerCompiles(t *testing.T) {
	src, err := os.ReadFile("../../examples/dungeon_crawler.ryx")
	if err != nil {
		t.Skipf("dungeon_crawler.ryx not found: %v", err)
	}
	_, compileErr := compileAndRun(string(src), "dungeon_crawler.ryx")
	// The program calls gfx_init/gfx_run which requires a display,
	// so we only verify it compiles (no runtime execution in CI).
	// A compile error would be reported; runtime errors from graphics
	// are expected in headless environments.
	if compileErr != nil && !strings.Contains(compileErr.Error(), "graphics") &&
		!strings.Contains(compileErr.Error(), "display") &&
		!strings.Contains(compileErr.Error(), "ebiten") {
		t.Errorf("dungeon_crawler.ryx failed to compile: %v", compileErr)
	}
}
```

**For the PNG assets**: Generate minimal 32x32 PNG images programmatically using Go's `image` package in a small standalone script, or manually create them. The images should be:
- `wall_stone.png`: 32x32 pixels, gray brick pattern (alternating #787878 and #646464 bands)
- `wall_moss.png`: 32x32 pixels, green-gray pattern (alternating #5A6E50 and #4A5E40 bands)

If the PNG assets cannot be created, the game gracefully falls back to solid-color rectangles (`gfx_load_image` returns -1 when the file is not found).

**Acceptance Criteria**:
- `go test ./tests/integration/ -run DungeonCrawler` passes
- The complete `examples/dungeon_crawler.ryx` file compiles without parse, resolve, or type errors
- `./ryx run examples/dungeon_crawler.ryx` opens a window and shows the title screen
- All existing tests continue to pass: `go test ./...`
- The game runs at 60 FPS without visible stuttering
- Read `tests/integration/e2e_test.go` for the existing test pattern

---

## Complete File Structure

The final `examples/dungeon_crawler.ryx` has this structure (approximate line ranges):

```
Lines    1-120:   Section 1-2: Color utilities + Game constants (T1)
Lines  121-230:   Section 3: Map & tile helpers (T2)
Lines  231-500:   Section 4: Level data for 3 levels (T3)
Lines  501-570:   Section 5: Particle velocity helpers (T4)
Lines  571-820:   Section 6: fn main() opening, mutable state declaration (T5)
Lines  821-920:   Section 7: Initial level load (T6)
Lines  921-990:   Section 8: Frame callback opening, state machine skeleton (T7)
Lines  991-1060:  STATE_TITLE branch (T9)
Lines 1061-1100:  STATE_PLAYING: Player input & movement (T10)
Lines 1101-1180:  STATE_PLAYING: Enemy AI & movement (T11)
Lines 1181-1260:  STATE_PLAYING: Player attack (T12)
Lines 1261-1310:  STATE_PLAYING: Enemy contact damage (T13)
Lines 1311-1370:  STATE_PLAYING: Item pickup (T14)
Lines 1371-1395:  STATE_PLAYING: Particle update (T15)
Lines 1396-1600:  STATE_PLAYING: All rendering (T16)
Lines 1601-1720:  STATE_PLAYING: HUD + Minimap (T17)
Lines 1721-1760:  STATE_PLAYING: Level completion check (T8)
Lines 1761-1830:  STATE_LEVEL_COMPLETE branch (T8)
Lines 1831-1910:  STATE_GAME_OVER branch (T9)
Lines 1911-1940:  STATE_VICTORY branch (T9)
Lines 1941-1960:  STATE_PAUSED branch (T9)
Lines 1961-1965:  Frame callback close + gfx_run + main close (T7)
```

Total: approximately 1400-1600 lines.

---

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Single-file assembly conflicts: multiple workers editing the same file | Each task targets a specific, non-overlapping section of the file. Comment banners with section numbers provide clear boundaries. Workers should NOT modify sections outside their assigned range. |
| Array size mismatches causing index-out-of-bounds | All array sizes are defined as constants (`MAX_ENEMIES()=32`, `MAX_PARTICLES()=64`, `MAX_ITEMS()=16`, `map_len()=300`). Every array loop uses these constants as bounds. |
| Mutable variable name typos across tasks | T5 defines the canonical variable names. All subsequent tasks reference the exact names from T5. A single typo will cause a compile error, caught by T18. |
| Level data errors (enemy on wall, item on water) | Each level's enemy/item spawn positions are manually verified against the tile map. Start positions are on floor tiles. |
| Performance: 64 particles + 32 enemies + 300 tiles per frame | At 60 FPS this is ~400 draw commands per frame, well within Ebiten's capability. The raycaster example draws 640 vertical lines per frame. |
| `gfx_load_image` returns -1 when assets are missing | All image-drawing code checks `if wall_img >= 0` before calling `gfx_draw_image_scaled`. The game is fully playable without textures. |
| Closure captures too many mutable variables | The raycaster example already captures 7 mutable variables and works correctly after the closure-capture bug fix. The dungeon crawler captures ~40, which should work with the same mechanism. If issues arise, reduce to fewer state variables. |
| Large array literals slow down compilation | The Ryx compiler handles the raycaster's 256-element map array. The dungeon crawler uses 300-element arrays (tiles) and 64-element arrays (particles), which should compile in comparable time. |
| Boss minion spawn exceeds `MAX_ENEMIES` | Guard clause checks `enemy_count < MAX_ENEMIES() - 1` before spawning. If no room, minions are simply not spawned. |
| Random functions may not be deterministic across runs | This is expected for a game. `random_int` and `random_float` use Go's `math/rand/v2` which is automatically seeded. |

---

## Build & Verification Commands

```bash
# After all tasks complete:

# 1. Compile check (headless, no display needed)
go build ./cmd/ryx
./ryx check examples/dungeon_crawler.ryx

# 2. Run integration tests
go test ./tests/integration/ -run DungeonCrawler -v

# 3. Full regression suite
go test ./... -short

# 4. Manual play test (requires display)
./ryx run examples/dungeon_crawler.ryx

# 5. Verify file size
wc -l examples/dungeon_crawler.ryx  # should be 1400-1600 lines
```

---

## Ryx Language Quick Reference (for workers)

```ryx
// Variable declaration
let x = 42;                    // immutable
let mut y = 0;                 // mutable

// Functions (outside main)
fn add(a: Int, b: Int) -> Int {
    a + b
}

// Closures (inside main, captures mutable vars from outer scope)
let frame = || {
    y = y + 1
};

// Loops (no for-in inside closures; use while)
let mut i = 0;
while i < 10 {
    // body
    i = i + 1
};

// Conditionals
if x > 0 {
    // body
} else if x == 0 {
    // body
} else {
    // body
};

// Arrays
let arr = [1, 2, 3];
let val = arr[0];          // indexing
arr[1] = 42;               // mutation (if arr is let mut)

// String concatenation
let s = "Hello" ++ " " ++ "World";

// Type conversions
let f = int_to_float(42);
let n = float_to_int(3.14);
let s = int_to_string(42);

// Graphics pattern
gfx_init(640, 480, "Title");
let frame = || {
    gfx_set_color(COLOR_RED());
    gfx_fill_rect(10, 10, 100, 50);
    if gfx_key_pressed(KEY_ESCAPE()) { gfx_quit() }
};
gfx_run(frame)

// Math
let angle = pi() / 4.0;
let s = sin(angle);
let c = cos(angle);
let d = sqrt(x * x + y * y);
let r = random_int(0, 100);  // [0, 100)
let rf = random_float();      // [0.0, 1.0)

// Negative floats: use 0.0 - value instead of -value
let neg = 0.0 - 3.14;

// Boolean
let b = true && false || !true;
```
