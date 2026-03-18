# More Examples

This repo ships a few additional programs that are useful for experimentation.

## Expression Evaluator

Run:

```bash
./ryx run examples/expression_evaluator.ryx
```

Observed output:

```text
1
```

This example focuses on explicit stack manipulation and simple operator dispatch.

## Build and Exec Flow

The hello-world example is the easiest way to test the full compile-to-bytecode path:

```bash
./ryx build -o /tmp/hello.ryxc examples/hello.ryx
./ryx exec /tmp/hello.ryxc
```

Note that flags currently need to come before the source file. The inverse ordering does not set the output path because of the current CLI flag parsing.

## Todo List

`examples/todo_list.ryx` is intended to demonstrate structs plus mutable state, but today it exposes a runtime limitation.

Command:

```bash
./ryx run examples/todo_list.ryx
```

Observed output:

```text
2
runtime error: runtime error: type mismatch: expected mutable struct, got object
  at main
```

Treat it as a debugging target rather than a happy-path tutorial until struct field mutation is fixed.
