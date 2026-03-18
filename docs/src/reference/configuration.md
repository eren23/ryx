# Configuration

Ryx reads `ryx_config.toml` from the current directory. CLI flags override values from the config file.

## Current Config File

```toml
[compiler]
max_errors = 20
max_warnings = 100
monomorphize_limit = 64
opt_level = 2
inline_threshold = 8
dump_ast = false
dump_hir = false
dump_mir = false
dump_bytecode = false

[vm]
stack_size = 65536
max_call_depth = 1024
fiber_timeslice = 4096
max_fibers = 10000

[gc]
initial_threshold_bytes = 1048576
growth_factor = 2.0
incremental_trace_batch = 256
incremental_sweep_batch = 512
enable_incremental = true

[debug]
source_maps = true
stack_traces = true
gc_stats = false
instruction_trace = false
```

## Compiler Settings

| Key | Meaning |
|-----|---------|
| `max_errors` | Maximum parse/type errors to accumulate |
| `max_warnings` | Maximum warnings to keep |
| `monomorphize_limit` | Upper bound on generic instantiations |
| `opt_level` | `0`, `1`, or `2` optimization pipeline |
| `inline_threshold` | Maximum MIR statement count for inlining |
| `dump_ast` / `dump_hir` / `dump_mir` / `dump_bytecode` | Enable debug dumps by default |

## VM Settings

| Key | Meaning |
|-----|---------|
| `stack_size` | Operand stack capacity |
| `max_call_depth` | Maximum call frames per fiber |
| `fiber_timeslice` | Instructions per scheduler slice |
| `max_fibers` | Hard limit on concurrent fibers |

## GC Settings

| Key | Meaning |
|-----|---------|
| `initial_threshold_bytes` | Heap size that triggers the first collection |
| `growth_factor` | Heap growth multiplier after a collection |
| `incremental_trace_batch` | Objects traced per incremental step |
| `incremental_sweep_batch` | Objects swept per incremental step |
| `enable_incremental` | Toggle incremental versus stop-the-world behavior |

## Debug Settings

| Key | Meaning |
|-----|---------|
| `source_maps` | Preserve source mapping metadata |
| `stack_traces` | Include stack traces in runtime errors |
| `gc_stats` | Print GC stats after execution |
| `instruction_trace` | Trace every instruction; very slow |

## Practical Notes

- CLI flags override config values after the config file is loaded
- `opt_level = 2` is the current default
- `instruction_trace = true` is useful for debugging but will generate large output quickly
