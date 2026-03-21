# Ryx

Ryx is a statically typed, expression-oriented programming language with algebraic data types, pattern matching, traits, closures, and lightweight concurrency via fibers and channels. The compiler and runtime are implemented in Go. Graphics programs use Ebiten v2 for windowed rendering.

Full documentation is intended to live in the GitHub Pages site:

- Docs site: `https://ryx-lang.github.io/ryx/`
- Docs sources: `docs/src/`
- Generated Pages output: `docs/book/`

## Quick Start

Requirements:

- Go 1.26.1 or later

Build the CLI:

```bash
go build -o ryx ./cmd/ryx
```

Run the hello-world example:

```bash
./ryx run examples/hello.ryx
```

Type-check another example:

```bash
./ryx check examples/calculator.ryx
```

Start the REPL:

```bash
./ryx repl
```

## Documentation

The mdBook content is split into focused chapters instead of keeping the full manual in the root README.

GitHub Pages deployment is defined in `.github/workflows/pages.yml` and publishes `docs/book/`.
In the repository settings, `Pages` should use `GitHub Actions` as the source.

Common local docs commands:

```bash
cd docs
mdbook serve
```

Or build the static site directly:

```bash
cd docs
mdbook build
```

## Repository Layout

```text
cmd/ryx/      CLI entry point
examples/     Sample Ryx programs
pkg/          Compiler, runtime, and standard library packages
tests/        Integration and benchmark suites
docs/         mdBook sources and generated GitHub Pages site
```

## Examples

Beyond the basic `hello.ryx` and `calculator.ryx`, the `examples/` directory includes:

- `raycaster.ryx` — real-time 3D raycaster with textured walls and item collection
- `dungeon_crawler.ryx` — top-down roguelike with enemy AI, combat, particles, and 3 levels
- `graphics_hello.ryx` — minimal graphics window demo
- `concurrent_primes.ryx` — concurrent prime sieve using fibers and channels

Graphics examples require a display and use the built-in `gfx_*` API backed by Ebiten.

## Notes

- The current CLI expects flags before positional file arguments, for example: `./ryx build -o /tmp/hello.ryxc examples/hello.ryx`
- The current merged swarm baseline on `main` is documented in `docs/src/project/swarm-history.md`
- `go test ./...` passes on the current `main` branch after the latest code-only swarm merge
