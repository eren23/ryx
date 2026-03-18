# Ryx

**A statically typed, expression-oriented programming language built in Go.**

Ryx combines a small language surface with a full compiler and runtime stack:

- Algebraic data types and exhaustive pattern matching
- Traits, closures, arrays, tuples, and structs
- Hindley-Milner style type checking
- A bytecode VM with a garbage-collected heap
- Lightweight fibers and CSP-style channels
- Zero external Go dependencies in the compiler/runtime itself

## What This Book Covers

This book is organized for both users and implementers:

- [Getting Started](getting-started.md) shows how to build the binary and run programs quickly
- [Examples](examples/hello-world.md) walks through the shipped sample programs with runnable commands
- [Language Guide](language-guide.md) covers the syntax and core language features
- [CLI Reference](reference/cli.md) documents commands and flags from the current CLI
- [Configuration](reference/configuration.md) explains `ryx_config.toml`
- [Architecture](architecture/overview.md) describes the compiler pipeline, VM, optimizer, and GC

## At a Glance

Current repository highlights:

- CLI entry point in `cmd/ryx`
- Sample programs in `examples/`
- Compiler/runtime packages under `pkg/`
- Integration and benchmark suites under `tests/`

## Current State

The repo is already usable for local experimentation:

- `./ryx run examples/hello.ryx` works
- `./ryx check examples/calculator.ryx` works
- `./ryx build -o /tmp/hello.ryxc examples/hello.ryx` followed by `./ryx exec /tmp/hello.ryxc` works

There are also a few rough edges that this book calls out directly rather than hiding:

- CLI flags currently need to come before positional file arguments
- `go test ./...` currently fails in `tests/integration` with a VM panic
- `examples/todo_list.ryx` currently hits a runtime error when mutating a struct field in an array
