# CLI Reference

The current CLI is implemented in `cmd/ryx/main.go`.

## Usage

```text
Usage: ryx <command> [flags] [file]
```

## Commands

| Command | Description |
|---------|-------------|
| `run` | Compile and execute a `.ryx` source file |
| `build` | Compile a `.ryx` source file to bytecode |
| `exec` | Execute a precompiled bytecode file |
| `repl` | Start the interactive REPL |
| `disasm` | Disassemble a `.ryx` or `.ryxc` file |
| `check` | Type-check without executing |
| `fmt` | Format a `.ryx` source file |

## Flags

| Flag | Description |
|------|-------------|
| `-dump-ast` | Dump the AST after parsing |
| `-dump-hir` | Dump HIR after lowering |
| `-dump-mir` | Dump MIR after optimization |
| `-dump-bytecode` | Dump disassembled bytecode |
| `-gc-stats` | Print GC statistics after execution |
| `-trace` | Trace every executed instruction |
| `-o <path>` | Output file path for `build` |

## Important Parsing Detail

Flags are parsed before positional arguments. In practice that means:

```bash
./ryx build -o /tmp/hello.ryxc examples/hello.ryx
```

works, while:

```bash
./ryx build examples/hello.ryx -o /tmp/hello.ryxc
```

does not currently route the output to `/tmp/hello.ryxc`.

## Common Examples

Run a program:

```bash
./ryx run examples/hello.ryx
```

Compile to bytecode:

```bash
./ryx build -o /tmp/hello.ryxc examples/hello.ryx
```

Execute precompiled bytecode:

```bash
./ryx exec /tmp/hello.ryxc
```

Type-check only:

```bash
./ryx check examples/calculator.ryx
```

View disassembly:

```bash
./ryx disasm examples/hello.ryx
```

Run with debug output:

```bash
./ryx run -dump-ast -dump-mir -gc-stats examples/hello.ryx
```

## REPL

Start the REPL:

```bash
./ryx repl
```

Special commands:

| Command | Description |
|---------|-------------|
| `:help` | Show help |
| `:quit` | Exit the REPL |
