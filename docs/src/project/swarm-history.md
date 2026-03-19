# Swarm History And Versioning

This page records the recent swarm-delivered changes that landed on `main`,
which swarm branches carried them, and the operational lessons from the merge
process.

## Current Merged State

The current `main` branch includes the code-only merge commit:

- `db911b0` — `Merge swarm branch attoswarm/428e9aa6 (code-only)`

That merge intentionally brought over product code and tests while leaving
swarm runtime artifacts under `.agent/` out of the final branch history.

## Swarm Branch Timeline

Recent swarm-related commits and branches:

- `1e44569` — `attoswarm: 6/14 tasks completed`
- `6467521` — `attoswarm: 4/11 tasks completed`
- `b23c308` — `attoswarm: 7/11 tasks completed`
- `24a3ca0` — local bookkeeping update tied to swarm runtime metadata
- `393a910` on `attoswarm/38c3e3f5` — narrower phase-2 map branch
- `9206fa1` on `attoswarm/428e9aa6` — broader phase-2 completion branch
- `db911b0` on `main` — code-only merge of `attoswarm/428e9aa6`

The practical meaning:

- `attoswarm/38c3e3f5` was the first focused follow-up branch for the stdlib
  finish pass.
- `attoswarm/428e9aa6` carried the most complete follow-up implementation and
  became the source for the final code-only merge.

## What Landed

The merged swarm work adds or expands the following areas:

- Public map stdlib support in `pkg/stdlib/map_ops.go`
- Map unit coverage in `pkg/stdlib/map_ops_test.go`
- Broader stdlib regression coverage in `pkg/stdlib/stdlib_test.go`
- Runtime/compiler updates in `pkg/codegen`, `pkg/mir`, `pkg/resolver`, and
  `pkg/vm`
- GC support and coverage in `pkg/gc`
- Real map-backed integration programs under `tests/testdata/programs/`

Notable new fixtures include:

- `map_operations`
- `map_merge_groups`
- `phone_book`
- `word_counter`
- `group_by`

## Verification Baseline

After merging `attoswarm/428e9aa6` into `main`, the full repository suite
passed:

```bash
go test ./...
```

This is the current verification baseline for the merged swarm changes.

## Swarm Merge Lessons

Two operational lessons came out of this run:

1. Swarm result branches can contain real product work even when `main` does
   not yet show it. Inspect swarm branches directly before assuming work was
   lost.
2. Swarm runtime state under `.agent/` must not be treated as product changes
   during finalization. The merge path should carry code and tests forward, not
   transient swarm bookkeeping.

For this repository, the final merge was done as a code-only carry-over from
the newest completed swarm branch rather than a blind branch merge.
