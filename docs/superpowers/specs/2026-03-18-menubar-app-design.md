# Perpetum Debile — macOS Menu Bar App Design

**Date:** 2026-03-18
**Status:** Approved

## Overview

A lightweight macOS menu bar app written in Go that repeatedly runs a single shell command. It displays a flame animation while the command is running, a checkmark on success, and a `!` icon on failure. All configuration is via CLI flags.

## Architecture & State Machine

The app has two goroutines communicating over a state channel:

**States:**
```
StateRunning | StateSuccess | StateFailed
```

- **Runner goroutine**: executes the shell command sequentially — run → wait for finish or timeout → pause (delay) → repeat. Sends state transitions over a channel.
- **Animator goroutine**: receives state, updates the systray icon accordingly.
- **Main goroutine**: parses CLI flags, initializes systray, wires goroutines together. Provides a single "Quit" menu item.

State is communicated via a channel (runner → animator) to keep it race-free.

## Sequential Execution Model

Commands are **not** run on a fixed interval. The cycle is:

```
run command → wait for finish (or timeout) → wait --delay → run again → ...
```

The next run never starts until the previous one completes or times out. If the command takes 2s and delay is 5s, the cycle is ~7s total.

## CLI Interface

```bash
perpetum-debile --cmd "your-shell-command" [--delay 5s] [--timeout 10m]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--cmd` | (required) | Shell command, executed via `sh -c "..."` |
| `--delay` | `5s` | Pause between end of one run and start of next |
| `--timeout` | `10m` | Max duration for a single command run |

**Validation:**
- Missing `--cmd` → print usage and exit with error
- `--timeout` should be greater than `--delay` (warn if not)

**Timeout behavior:**
- Command running longer than `--timeout` is killed and retried
- 3 consecutive timeouts → `StateFailed`, `!` icon (permanent until restart)
- Timeout retry counter resets on any successful or non-timeout failed run

## Project Structure

```
perpetum-debile/
├── main.go          # entry point, CLI flags, systray init, Quit menu item
├── runner.go        # sequential shell execution, timeout handling, delay loop
├── animator.go      # state → icon mapping, flame frame cycling
├── assets.go        # go:embed declarations
└── assets/
    ├── flame1.png   # flame animation frames (4–6 frames, 22×22px)
    ├── flame2.png
    ├── flame3.png
    ├── flame4.png
    ├── check.png    # success icon (22×22px)
    └── bang.png     # failure icon — ! (22×22px)
```

All icons are 22×22px PNG (standard macOS menu bar size), embedded at compile time via `go:embed`. The binary is fully self-contained.

## Icon Animation

| State | Icon behavior |
|-------|--------------|
| `StateRunning` | Cycle flame frames at ~150ms per frame |
| `StateSuccess` | Show checkmark, hold |
| `StateFailed` | Show `!` icon, hold |
| Waiting (delay pause) | Hold last result icon (no animation) |

## Dependencies

- [`github.com/getlantern/systray`](https://github.com/getlantern/systray) — system tray integration
- Standard library only otherwise (`os/exec`, `context`, `time`, `flag`)

## Out of Scope (for now)

- Config file support
- Built-in log output / history
- Multiple tasks
- Notifications (macOS alerts)
- Auto-start on login
