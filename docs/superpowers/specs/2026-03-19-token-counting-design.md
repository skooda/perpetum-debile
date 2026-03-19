# Perpetum Debile — Token Counting Design

**Date:** 2026-03-19
**Status:** Approved

## Overview

Display token usage next to the menu bar icon as `<per-run> / <cumulative>` (e.g. `847 / 12.4k`). Per-run resets each completed run; cumulative accumulates since app start.

## CLI Interface

No new flags. Token display is always on.

## Data Layer

Replace `State int` in `state.go` with a struct:

```go
type StateKind int

const (
    StateRunning StateKind = iota
    StateSuccess
    StateFailed
)

type State struct {
    Kind      StateKind
    RunTokens int64 // tokens used this run (0 if unknown: running, timeout, missing target.md)
    Total     int64 // cumulative tokens since app start
}
```

`RunTokens` is 0 for states where no run completed (`StateRunning`, timeouts, missing `target.md`). `Total` is always the current running cumulative (starts at 0, never decreases).

## Token Extraction

Add `--output-format json` to the `claude` invocation. Capture stdout. The Claude Code CLI outputs JSON with this structure:

```json
{
  "usage": {
    "input_tokens": 4,
    "cache_creation_input_tokens": 20136,
    "cache_read_input_tokens": 13389,
    "output_tokens": 350
  }
}
```

Token count for a run = `input_tokens + cache_creation_input_tokens + cache_read_input_tokens + output_tokens` (all four fields, since `input_tokens` alone is only the uncached fraction and would severely undercount real usage).

`runOnce` returns `(timedOut bool, tokens int64, exitErr *exec.ExitError)`. On timeout or context cancel, `tokens` is 0. On JSON parse failure, `tokens` is 0 (best-effort; run still succeeds/fails by exit code).

`Run` loop maintains `totalTokens int64`. After each completed run, `totalTokens += runTokens`.

### State values sent

Every `State` sent on the channel must have `Total` set to the current `totalTokens` at the time of sending:

- Start of each loop iteration: `State{Kind: StateRunning, RunTokens: 0, Total: totalTokens}`
- Missing `target.md`: `State{Kind: StateFailed, RunTokens: 0, Total: totalTokens}`
- Successful run: `State{Kind: StateSuccess, RunTokens: tokens, Total: totalTokens}` (after `totalTokens += tokens`)
- Failed run: `State{Kind: StateFailed, RunTokens: tokens, Total: totalTokens}` (after `totalTokens += tokens`)
- Permanent timeout failure: `State{Kind: StateFailed, RunTokens: 0, Total: totalTokens}`

## Animator Display

`animator.go` calls `systray.SetTitle(label)` on every ticker tick alongside `SetIcon`.

### State comparisons

All existing `state == StateRunning`, `switch current { case StateRunning: ... }` comparisons change to use `.Kind`:

```go
current = state
if state.Kind == StateRunning { ... }
switch current.Kind {
case StateRunning: ...
case StateSuccess: ...
case StateFailed:  ...
}
```

Initial value changes from `current := StateSuccess` (which no longer works as `StateSuccess` is now a `StateKind`, not a `State`) to:

```go
current := State{Kind: StateSuccess}
```

The `Animator` also adds a `hasReceivedState bool` field (initialized `false`). It is set to `true` the first time a state arrives on the channel. `SetTitle` is only called when `hasReceivedState` is true — before any state arrives, the title stays empty.

### Format

- Startup (before first state received, `hasReceivedState == false`): `""` — `SetTitle` not called
- While running (`hasReceivedState == true`, `Kind == StateRunning`): `"… / <total>"`
- After a completed run (`hasReceivedState == true`, `Kind != StateRunning`): `"<run> / <total>"`

### Number formatting (`formatTokens` helper in `animator.go`)

| Value | Display |
|-------|---------|
| 0–999 | `"847"` |
| 1000+ | `"1.2k"` (one decimal, trailing `.0` omitted → `"1k"` not `"1.0k"`) |

Examples: `0` → `"0"`, `999` → `"999"`, `1000` → `"1k"`, `1234` → `"1.2k"`, `12345` → `"12.3k"`.

`formatTokens` is a pure function. Tests for it live in `animator_test.go`.

### Full label examples

- Startup, no runs yet: `""`
- Running (total so far 5600): `"… / 5.6k"`
- Success, run=847, total=12400: `"847 / 12.4k"`
- Failed (missing target.md), total=12400: `"0 / 12.4k"`

## Files Changed

| File | Change |
|------|--------|
| `state.go` | Replace `State int` + constants with `State struct` + `StateKind` |
| `runner.go` | Add `--output-format json`; capture stdout; parse all 4 token fields; update `runOnce` signature to return `(bool, int64, *exec.ExitError)`; accumulate `totalTokens`; send full `State` structs |
| `animator.go` | Update `State` comparisons to `.Kind`; update initial `current` value; add `SetTitle` call; add `formatTokens` helper |
| `animator_test.go` | **Create** — tests for `formatTokens` |
| `runner_test.go` | Update all tests: stub claude scripts emit valid JSON stdout; assert `RunTokens`/`Total` fields on received states |

## Unchanged

`main.go` (channel type `chan State` is unaffected by the struct change). Icons, signal handling, delay loop, timeout counter, target.md check behavior.

## Out of Scope

- Persisting cumulative total across app restarts
- Cost display (USD)
- Per-model breakdown
