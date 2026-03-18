# Getting Started

This guide gets `ryx` built and running with commands verified against the current repository.

## Prerequisites

- Go 1.26.1 or later

## Build

Build the CLI from the repo root:

```bash
go build -o ryx ./cmd/ryx
```

The binary is produced at `./ryx`.

## Run Your First Program

Run the included hello-world example:

```bash
./ryx run examples/hello.ryx
```

Expected output:

```text
"Hello, ""World""!"
"Hello, ""Ryx""!"
42
true
```

## Type-Check Without Running

```bash
./ryx check examples/calculator.ryx
```

Expected output:

```text
check: no errors
```

## Build Bytecode and Execute It

Put flags before the source file:

```bash
./ryx build -o /tmp/hello.ryxc examples/hello.ryx
./ryx exec /tmp/hello.ryxc
```

This produces the same output as `run`, but in two stages.

## Start the REPL

```bash
./ryx repl
```

To exit immediately from a scriptable shell:

```bash
printf ':quit\n' | ./ryx repl
```

The REPL banner should look like:

```text
Ryx REPL (type :quit to exit, :help for commands)
ryx>
```

## Recommended First Commands

```bash
./ryx run examples/hello.ryx
./ryx run examples/calculator.ryx
./ryx run examples/concurrent_primes.ryx
./ryx disasm examples/hello.ryx
```

## Local Docs Workflow

This repo uses `mdBook` for GitHub Pages style documentation.

Serve the docs locally:

```bash
cd docs
mdbook serve
```

Build the static site:

```bash
cd docs
mdbook build
```

GitHub publishing:

- The repository workflow `.github/workflows/pages.yml` deploys `docs/book/`
- In GitHub repository settings, `Pages` must be set to `GitHub Actions`
