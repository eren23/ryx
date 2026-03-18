# Architecture Overview

Ryx is split into a conventional compiler pipeline followed by a bytecode virtual machine.

## Package Layout

```text
cmd/ryx/      CLI entry point
pkg/lexer/    Tokenization and token definitions
pkg/parser/   AST construction and formatting
pkg/resolver/ Name resolution and scope analysis
pkg/types/    Type inference, checking, and traits
pkg/hir/      High-level IR and monomorphization
pkg/mir/      Mid-level SSA-like IR
pkg/optimize/ Optimization pipeline
pkg/codegen/  Bytecode generation and encoding
pkg/vm/       Stack-based virtual machine
pkg/gc/       Incremental garbage collector
pkg/stdlib/   Built-in functions and trait methods
pkg/repl/     Interactive REPL
pkg/diagnostic/ Error reporting and source tracking
```

## Data Flow

At a high level:

1. Parse source text into an AST
2. Resolve names and scopes
3. Type-check and infer types
4. Lower to HIR and monomorphize generics
5. Build MIR
6. Run optimization passes
7. Generate bytecode
8. Execute on the VM

The CLI threads these phases together in `compilePipeline` inside `cmd/ryx/main.go`.
