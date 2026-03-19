# Perpetum Debile — Debug Logging & Prompt Update Design

**Date:** 2026-03-19
**Status:** Approved

## Overview

Two changes:
1. **`--debug` flag** — when set, appends claude's stdout and stderr to `<path>/debug.log` each run, with a timestamp header per run.
2. **`prompt.txt` update** — replace standup.log instruction with a git commit instruction; add short-step guidance.

## prompt.txt

Replace the entire file content with:

```
Look at current directory, read target.md, decide what is the best thing to do next in order to move project towards target described in target.md, do it, then commit your changes with git commit -m "<one line standup about what you did>". Keep each step small and focused — it should take no more than 2-3 minutes. Prefer doing less per iteration and running more iterations.
```

No code changes needed — `prompt.txt` is embedded at compile time via `go:embed`. Recompile picks it up automatically.

## `--debug` Flag

### CLI

```bash
perpetum-debile <path> [--delay 5s] [--timeout 10m] [--debug]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--debug` | `false` | Append claude stdout+stderr to `<path>/debug.log` |

### Log file

- Location: `<path>/debug.log` (same directory as `target.md` and `standup.log`)
- Mode: append — survives app restarts, accumulates across all runs
- Each run is preceded by a `--- <RFC3339 timestamp> ---\n` header line
- A blank line is written after each run's output as a separator
- Created automatically if it does not exist

### Architecture

**`main.go`:** Add `--debug` boolean flag. If `--debug` is set, open `<path>/debug.log` in append+create mode (`os.O_APPEND|os.O_CREATE|os.O_WRONLY`, perm `0644`) after directory validation. Pass the `*os.File` to `Runner`. Close the file in the `systray` `onExit` callback.

**`runner.go`:** Add `debugLog *os.File` field to `Runner` struct (`nil` = debug off). In `runOnce`, when `debugLog != nil`:
- Write `--- <timestamp> ---\n` header to `debugLog`
- Capture stdout into a `bytes.Buffer` for token parsing
- Set `c.Stdout = io.MultiWriter(&stdoutBuf, debugLog)` to tee stdout to both the buffer and the log
- Set `c.Stderr = debugLog` to capture stderr to the log
- After `c.Run()`, write `\n` separator to `debugLog`

When `debugLog == nil` (debug off), `c.Stdout` is set to `&stdoutBuf` only and `c.Stderr` is left nil (discarded).

Note: `runOnce` switches from `c.Output()` to `c.Run()` with manually managed `c.Stdout` to support the conditional tee. Token parsing still reads from the same `stdoutBuf.Bytes()`.

### Token counting interaction

The `--output-format json` stdout is captured into `stdoutBuf` in all cases. Token parsing is unaffected by whether `--debug` is set.

## Files Changed

| File | Change |
|------|--------|
| `prompt.txt` | Replace content — standup.log → git commit; add short-step instruction |
| `main.go` | Add `--debug` flag; open `debug.log` if set; pass to `Runner`; close on exit |
| `runner.go` | Add `debugLog *os.File` to `Runner`; update `runOnce` to use `c.Run()` + manual stdout/stderr tee |

## Unchanged

`animator.go`, `state.go`, `assets.go`, `runner_test.go`. The debug log is not tested (it's I/O to an external file); the existing runner tests are unaffected since they pass `nil` for `debugLog`.

## Out of Scope

- Log rotation or size limits
- Configurable log file path
- Structured (JSON) log format
