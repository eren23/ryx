# Runtime

Ryx executes bytecode on a stack-based VM with a heap managed by an incremental garbage collector.

## VM

The runtime lives primarily under `pkg/vm/`.

Core responsibilities include:

- Managing call frames and the operand stack
- Executing bytecode instructions
- Representing arrays, structs, enums, closures, strings, channels, and fibers
- Scheduling lightweight fibers cooperatively

## Fibers and Channels

Concurrency is based on:

- Fibers: lightweight user-space execution contexts
- Channels: CSP-style communication primitives

The `spawn` language form lowers into runtime support that schedules additional work on the VM's cooperative scheduler.

## Garbage Collector

The GC implementation lives under `pkg/gc/` and uses incremental tricolor mark-and-sweep collection.

### Object Kinds

The README describes the GC as managing:

- Arrays
- Structs
- Enums
- Closures
- Channels
- Strings
- Upvalues
- Fibers

### GC Phases

1. Idle
2. Mark
3. Sweep

### Tuning Knobs

The most important runtime knobs come from `[gc]` in `ryx_config.toml`:

- `initial_threshold_bytes`
- `growth_factor`
- `incremental_trace_batch`
- `incremental_sweep_batch`
- `enable_incremental`

For quick visibility into collection behavior:

```bash
./ryx run -gc-stats examples/hello.ryx
```
