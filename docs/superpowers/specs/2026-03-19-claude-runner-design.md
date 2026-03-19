# Perpetum Debile — Claude Code Runner Design

**Date:** 2026-03-19
**Status:** Approved

## Overview

Replace the generic shell command runner with a Claude Code runner. The app now runs `claude` in headless mode (`-p`) inside a target directory, driven by an embedded prompt. It checks for `target.md` before each run and fails gracefully if it's missing.

## CLI Interface

```bash
perpetum-debile <path> [--delay 5s] [--timeout 10m]
```

| Arg/Flag | Default | Description |
|----------|---------|-------------|
| `<path>` | (required, positional) | Directory where Claude Code runs |
| `--delay` | `5s` | Pause between end of run and next start |
| `--timeout` | `10m` | Max duration per Claude Code run |

**Validation:**
- `<path>` missing → print usage to stderr and exit with error
- `<path>` not a valid directory → exit with error
- Parsed via `flag.Args()[0]` after `flag.Parse()`

## Prompt

`prompt.txt` is embedded at compile time via `go:embed`:

```
Look at current directory, read target.md, decide what is the best thing to do next in order to move project towards target described in target.md, do it and add one line standup about what you did into standup.log
```

`prompt.txt` lives at the project root (same directory as `assets.go`). Added to `assets.go`:
```go
//go:embed prompt.txt
var embeddedPrompt string
```

## Runner Changes

`Runner.cmd string` is replaced with `Runner.path string`. Each run cycle:

1. **Send `StateRunning`** — always first, consistent with the existing loop structure.

2. **Check for `target.md`** — if `<path>/target.md` does not exist, send `StateFailed` and wait `--delay` before retrying. The loop continues — if `target.md` appears later, the app recovers automatically. Missing `target.md` does not increment or reset the consecutive timeout counter.

2. **Run Claude Code** — if `target.md` exists, execute:

```go
cmd := exec.CommandContext(ctx, "claude",
    "-p", embeddedPrompt,
    "--model", "haiku",
    "--allowedTools", "WebSearch,Edit",
    "--setting-sources", "project,local",
)
cmd.Dir = runner.path
```

No shell wrapper — direct exec of the `claude` binary.

**Tool restrictions:**
- `WebSearch` — built-in web search only
- `Edit` — file editing; scoped to the working directory by Claude Code's permission system (since `cmd.Dir` is set to `path`)

**Settings scope:**
- `--setting-sources project,local` — loads only `.claude/settings.json` and `.claude/settings.local.json` from the target project directory. Global user settings (including global MCP servers and skills) are excluded.

**Success/failure:**
- Exit code 0 → `StateSuccess`
- Non-zero exit code → `StateFailed`, loop continues after `--delay`
- Timeout → retry counter (3 consecutive timeouts → permanent `StateFailed`)
- Missing `target.md` → `StateFailed`, loop continues (not counted as timeout)

## Files Changed

| File | Change |
|------|--------|
| `prompt.txt` | New — embedded prompt text |
| `assets.go` | Add `//go:embed prompt.txt` and `var embeddedPrompt string` |
| `runner.go` | Replace `cmd string` with `path string`; add `target.md` check; build `claude` invocation |
| `main.go` | Replace `--cmd` flag with positional `flag.Args()[0]` parsing |
| `runner_test.go` | Update tests: replace `cmd` field with `path`, add `target.md` presence/absence tests. Tests inject a stub `claude` script via `PATH` manipulation (temp dir with a fake `claude` binary) to avoid requiring a real Claude installation. |

## Unchanged

Everything else stays the same: animator, state machine, icons, signal handling, timeout counter logic, delay loop.

## Out of Scope

- Configurable model (hardcoded `haiku`)
- Configurable prompt (hardcoded from `prompt.txt`)
- Multiple paths
- Output/log display in the menu
