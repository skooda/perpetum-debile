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

- **Runner goroutine**: executes the shell command sequentially — run → wait for finish or timeout → pause (delay) → repeat. Sends state transitions over a channel. During the delay pause, the runner sends **no** state transition — the animator holds its last state implicitly.
- **Animator goroutine**: receives state from the channel, updates the systray icon accordingly. Holds the last received state when no new state arrives (e.g. during delay).
- **Main goroutine**: parses CLI flags, initializes systray, wires goroutines together. Provides a single "Quit" menu item. Installs `SIGTERM`/`SIGINT` signal handler.

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
- Missing `--cmd` → print usage to stderr and exit with error
- `--timeout` less than or equal to `--delay` → print warning to stderr, continue execution

**Command execution:**
The command is run as: `exec.CommandContext(ctx, "sh", "-c", cmdFlag)` where `cmdFlag` is the raw `--cmd` flag string, unmodified. This allows pipes, redirects, and shell expansions.

**Timeout behavior:**
- Command running longer than `--timeout` is killed (via context cancellation) and retried after `--delay`
- The timeout counter increments **only** on a timeout. It resets to zero on any run that completes without timing out (whether exit 0 or non-zero)
- 3 consecutive timeouts → `StateFailed`, `!` icon (permanent until restart)
- On permanent failure: runner goroutine exits its loop, sends `StateFailed` once, and closes the state channel. Animator detects channel close and stops.

## Signal Handling & Shutdown

The app installs a signal handler for `SIGTERM` and `SIGINT` (`os/signal`). On signal:

1. Cancel the runner's context (kills any running child process)
2. Wait for the child process to exit
3. Call `systray.Quit()`

This prevents orphaned child processes when the app is killed from the terminal.

## Project Structure

```
perpetum-debile/
├── go.mod           # module: github.com/perpetum-debile/perpetum-debile
├── main.go          # entry point, CLI flags, systray init, signal handling, Quit menu item
├── runner.go        # sequential shell execution, timeout handling, delay loop
├── animator.go      # state → icon mapping, flame frame cycling
├── assets.go        # go:embed declarations
└── assets/
    ├── flame1.png   # flame animation frames (4–6 frames, 44×44px @2x)
    ├── flame2.png
    ├── flame3.png
    ├── flame4.png
    ├── check.png    # success icon (44×44px @2x)
    └── bang.png     # failure icon — ! (44×44px @2x)
```

All icons are **44×44px PNG** (2× for Retina displays — macOS menu bar standard), embedded at compile time via `go:embed`. The binary is fully self-contained.

At startup, before entering the run loop, the app validates all embedded icon bytes by attempting to decode each PNG. If any icon fails to decode, the app exits with a descriptive error message.

## Icon Animation

No tooltip or title string is set in the menu bar — the icon is the only visual indicator.

| State | Icon behavior |
|-------|--------------|
| `StateRunning` | Cycle flame frames at ~150ms per frame |
| `StateSuccess` | Show checkmark, hold |
| `StateFailed` | Show `!` icon, hold |
| Delay pause | Hold last result icon (no animation, no state sent) |

## Dependencies

- [`github.com/getlantern/systray`](https://github.com/getlantern/systray) — system tray integration
- Standard library only otherwise (`os/exec`, `context`, `time`, `flag`, `os/signal`)

## Out of Scope (for now)

- Config file support
- Built-in log output / history
- Multiple tasks
- Notifications (macOS alerts)
- Auto-start on login
