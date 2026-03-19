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

`RunTokens` is 0 for states where no run completed (e.g. `StateRunning`, timeouts, missing `target.md`). `Total` is always the current running cumulative (starts at 0, never decreases).

## Token Extraction

Add `--output-format json` to the `claude` invocation. Capture stdout. Parse JSON:

```json
{"usage": {"input_tokens": 123, "output_tokens": 456}}
```

Token count for a run = `input_tokens + output_tokens`.

`runOnce` returns `(timedOut bool, tokens int64, exitErr *exec.ExitError)`. On timeout or context cancel, `tokens` is 0. On JSON parse failure, `tokens` is 0 (best-effort; run still succeeds/fails by exit code).

`Run` loop maintains `totalTokens int64`. After each completed run, `totalTokens += runTokens`. All `State` values sent carry the current `Total`.

## Animator Display

`animator.go` calls `systray.SetTitle(label)` on every ticker tick alongside `SetIcon`.

### Format

- While running: `… / <total>` (run total not yet known)
- After a run: `<run> / <total>`
- On startup (before any run): empty string (no title)

### Number formatting

| Value | Display |
|-------|---------|
| 0–999 | `"847"` |
| 1000+ | `"1.2k"` (one decimal, trailing `.0` omitted → `"1k"` not `"1.0k"`) |

Examples: `0` → `"0"`, `999` → `"999"`, `1000` → `"1k"`, `1234` → `"1.2k"`, `12345` → `"12.3k"`, `1000000` → `"1000k"` (no `m` suffix needed at this scale).

### Full label examples

- Startup, no runs yet: `""`
- Running (total so far 5600): `"… / 5.6k"`
- Success, run=847, total=12400: `"847 / 12.4k"`
- Failed (missing target.md), total=12400: `"0 / 12.4k"`

## Files Changed

| File | Change |
|------|--------|
| `state.go` | Replace `State int` + constants with struct + `StateKind` |
| `runner.go` | Add `--output-format json`; capture stdout; parse tokens; update `runOnce` signature; accumulate `totalTokens`; send `RunTokens`/`Total` in all states |
| `animator.go` | Add `systray.SetTitle` call with formatted label; add `formatTokens` helper |
| `runner_test.go` | Update all tests: stub claude scripts emit JSON; assert `RunTokens`/`Total` on states |

## Unchanged

Icons, signal handling, delay loop, timeout counter, target.md check behavior.

## Out of Scope

- Persisting cumulative total across app restarts
- Cost display (USD)
- Per-model breakdown
