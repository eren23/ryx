# Testing

The repository contains package tests plus integration and benchmark suites.

## Package-Level Tests

Run all Go tests:

```bash
go test ./...
```

Current note: in this workspace the full suite currently fails in `tests/integration` with a VM panic, so treat the full command as a known-red check until that issue is fixed.

## Targeted Test Commands

Useful focused commands:

```bash
go test ./pkg/lexer/...
go test ./pkg/parser/...
go test ./pkg/types/...
go test ./pkg/vm/...
go test ./pkg/stdlib/...
```

## Integration Coverage

The `tests/integration` suite includes coverage for:

- End-to-end program execution
- Lexer golden tests
- Parser golden tests
- Type-checking golden tests
- Optimization golden tests
- Standard library behavior
- Concurrency
- GC stress
- Diagnostic formatting
- Regression cases

## Benchmarks

Run benchmarks with:

```bash
go test -bench=. ./tests/benchmark/...
```

Benchmark files in the repo include:

- `bench_gc_test.go`
- `bench_lexer_test.go`
- `bench_programs_test.go`
- `bench_vm_test.go`

## Practical Smoke Checks

Until the integration failure is fixed, these commands are reliable quick checks:

```bash
./ryx run examples/hello.ryx
./ryx check examples/calculator.ryx
./ryx run examples/concurrent_primes.ryx
./ryx build -o /tmp/hello.ryxc examples/hello.ryx
./ryx exec /tmp/hello.ryxc
```
