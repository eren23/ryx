# Ryx

Ryx is a statically typed, expression-oriented programming language with algebraic data types, pattern matching, traits, closures, and lightweight concurrency via fibers and channels. The compiler and runtime are implemented in pure Go with zero external dependencies.

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

## Notes

- The current CLI expects flags before positional file arguments, for example: `./ryx build -o /tmp/hello.ryxc examples/hello.ryx`
- `go test ./...` currently fails in `tests/integration` with a VM panic; use the documented smoke commands in the docs for quick verification until that is fixed
