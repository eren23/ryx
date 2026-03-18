# Compiler Pipeline

The current compiler pipeline mirrors the implementation in `cmd/ryx/main.go`.

```text
Source Code (.ryx)
       |
       v
   Lexer
       |
       v
    Parser
       |
       v
   Resolver
       |
       v
 Type Checker
       |
       v
   HIR Lower
       |
       v
 Monomorphize
       |
       v
   MIR Build
       |
       v
   Optimize
       |
       v
   Codegen
       |
       v
      VM
```

## Stages

### Parse

`parser.Parse` converts source text into an AST and emits syntax errors with source locations.

### Resolve

`resolver.Resolve` performs name resolution and scope analysis.

### Type Check

`types.Check` validates types and trait usage.

### HIR and Monomorphization

`hir.Lower` desugars the checked AST into HIR, then `hir.Monomorphize` instantiates generics into concrete copies.

### Match Compilation

After monomorphization, `hir.CompileMatches` rewrites match expressions into a form suitable for later lowering and code generation.

### MIR and Optimization

`mir.Build` creates a mid-level IR. `optimize.Pipeline` then runs passes based on `opt_level`.

At `opt_level = 2`, the repo documents these optimizations:

- Constant folding
- Constant propagation
- Copy propagation
- Dead code elimination
- Block merging
- Common subexpression elimination
- Inlining
- Loop-invariant code motion
- Tail-call optimization
- Escape analysis

### Codegen

`codegen.Generate` emits the bytecode program, which can then be executed or encoded to disk as a `.ryxc` file.
