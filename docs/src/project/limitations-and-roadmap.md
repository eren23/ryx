# Limitations and Roadmap

This page reflects the current repository state, including issues observed while validating the docs examples.

## Current Limitations

- No functional module system yet; `import` and `module` are parsed but not wired up
- No incremental compilation; the full pipeline runs each time
- Generic-heavy programs can hit the monomorphization limit
- Scheduling is cooperative rather than preemptive
- No FFI support
- Full `go test ./...` is currently red because `tests/integration` hits a VM panic
- `examples/todo_list.ryx` currently fails at runtime during struct field mutation
- CLI flags currently need to appear before positional file arguments

## Near-Term Priorities

- Fix the VM panic in the integration suite
- Fix mutable struct field updates in array-backed examples like `todo_list.ryx`
- Improve CLI argument parsing so flags work both before and after the source file
- Expand the docs site as the language surface stabilizes

## Longer-Term Ideas

- File-based modules and imports
- Incremental compilation
- Debugger support
- LSP integration
- Package manager
- Async/await style sugar over fibers and channels
- Alternative backends such as WASM
