# Claude Code Runner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the generic `--cmd` shell runner with a Claude Code headless runner driven by an embedded prompt, with `target.md` presence check before each run.

**Architecture:** Three focused changes — add `prompt.txt` embedding to `assets.go`, rewrite `runner.go` to exec `claude` directly with the embedded prompt and tool restrictions, and update `main.go` to accept a positional `<path>` arg instead of `--cmd`. Tests use a stub `claude` binary injected via `PATH` to avoid requiring a real Claude installation.

**Tech Stack:** Go 1.26+, `os/exec`, `os`, `path/filepath`, `go:embed`

---

## File Map

| File | Change |
|------|--------|
| `prompt.txt` | **Create** — embedded prompt text |
| `assets.go` | **Modify** — add `go:embed prompt.txt` + `var embeddedPrompt string` |
| `runner.go` | **Modify** — `cmd string` → `path string`; add `target.md` check; exec `claude` directly |
| `runner_test.go` | **Modify** — replace `cmd` field with `path`; inject stub claude via PATH; add target.md tests |
| `main.go` | **Modify** — replace `--cmd` flag with positional `flag.Args()[0]`; add directory validation |

---

## Task 1: Add prompt.txt and embed it

**Files:**
- Create: `prompt.txt`
- Modify: `assets.go`

- [ ] **Step 1: Create prompt.txt**

```
Look at current directory, read target.md, decide what is the best thing to do next in order to move project towards target described in target.md, do it and add one line standup about what you did into standup.log
```

Save as `prompt.txt` at the project root (same level as `assets.go`).

- [ ] **Step 2: Add embed declaration to assets.go**

Add these two lines to `assets.go` after the existing imports:

```go
//go:embed prompt.txt
var embeddedPrompt string
```

Full `assets.go` after change:

```go
package main

import _ "embed"

//go:embed assets/flame1.png
var flame1PNG []byte

//go:embed assets/flame2.png
var flame2PNG []byte

//go:embed assets/flame3.png
var flame3PNG []byte

//go:embed assets/flame4.png
var flame4PNG []byte

//go:embed assets/check.png
var checkPNG []byte

//go:embed assets/bang.png
var bangPNG []byte

//go:embed prompt.txt
var embeddedPrompt string

var flameFrames = [][]byte{flame1PNG, flame2PNG, flame3PNG, flame4PNG}
```

- [ ] **Step 3: Verify build**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go build ./...
# Expected: no errors
```

- [ ] **Step 4: Commit**

```bash
git add prompt.txt assets.go
git commit -m "feat: add embedded prompt.txt"
```

---

## Task 2: Rewrite runner.go with TDD

**Files:**
- Modify: `runner_test.go`
- Modify: `runner.go`

The tests inject a fake `claude` binary into PATH. No real Claude installation needed.

- [ ] **Step 1: Write the new tests (replace runner_test.go entirely)**

```go
// runner_test.go
package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func collectStates(states <-chan State) []State {
	var result []State
	for s := range states {
		result = append(result, s)
	}
	return result
}

// setupTestEnv creates a stub claude binary and a project dir, injects stub into PATH.
// claudeScript is the shell script body (e.g. "exit 0" or "sleep 10").
// If withTargetMD is true, creates target.md in the project dir.
func setupTestEnv(t *testing.T, claudeScript string, withTargetMD bool) (projectDir string, cleanup func()) {
	t.Helper()

	stubDir := t.TempDir()
	script := "#!/bin/sh\n" + claudeScript + "\n"
	claudePath := filepath.Join(stubDir, "claude")
	if err := os.WriteFile(claudePath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	projectDir = t.TempDir()
	if withTargetMD {
		if err := os.WriteFile(filepath.Join(projectDir, "target.md"), []byte("# Target"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", stubDir+":"+origPath)
	return projectDir, func() { os.Setenv("PATH", origPath) }
}

func TestRunnerSuccess(t *testing.T) {
	projectDir, cleanup := setupTestEnv(t, "exit 0", true)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := &Runner{path: projectDir, delay: 0, timeout: 5 * time.Second}
	states := make(chan State)
	go r.Run(ctx, states)

	got := make([]State, 0)
	for s := range states {
		got = append(got, s)
		if len(got) >= 2 {
			cancel()
		}
	}

	if len(got) < 2 {
		t.Fatalf("expected at least 2 states, got %d: %v", len(got), got)
	}
	if got[0] != StateRunning {
		t.Errorf("state[0]: want StateRunning, got %v", got[0])
	}
	if got[1] != StateSuccess {
		t.Errorf("state[1]: want StateSuccess, got %v", got[1])
	}
}

func TestRunnerFailure(t *testing.T) {
	projectDir, cleanup := setupTestEnv(t, "exit 1", true)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := &Runner{path: projectDir, delay: 0, timeout: 5 * time.Second}
	states := make(chan State)
	go r.Run(ctx, states)

	got := make([]State, 0)
	for s := range states {
		got = append(got, s)
		if len(got) >= 2 {
			cancel()
		}
	}

	if len(got) < 2 {
		t.Fatalf("expected at least 2 states, got %d: %v", len(got), got)
	}
	if got[0] != StateRunning {
		t.Errorf("state[0]: want StateRunning, got %v", got[0])
	}
	if got[1] != StateFailed {
		t.Errorf("state[1]: want StateFailed, got %v", got[1])
	}
}

func TestRunnerConsecutiveTimeouts(t *testing.T) {
	projectDir, cleanup := setupTestEnv(t, "sleep 10", true)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r := &Runner{path: projectDir, delay: 0, timeout: 50 * time.Millisecond}
	states := make(chan State)
	go r.Run(ctx, states)

	got := collectStates(states)

	want := []State{StateRunning, StateRunning, StateRunning, StateFailed}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestRunnerMissingTargetMD(t *testing.T) {
	projectDir, cleanup := setupTestEnv(t, "exit 0", false) // no target.md
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := &Runner{path: projectDir, delay: 0, timeout: 5 * time.Second}
	states := make(chan State)
	go r.Run(ctx, states)

	got := make([]State, 0)
	for s := range states {
		got = append(got, s)
		if len(got) >= 2 {
			cancel()
		}
	}

	if len(got) < 2 {
		t.Fatalf("expected at least 2 states, got %d: %v", len(got), got)
	}
	if got[0] != StateRunning {
		t.Errorf("state[0]: want StateRunning, got %v", got[0])
	}
	if got[1] != StateFailed {
		t.Errorf("state[1]: want StateFailed, got %v", got[1])
	}
}
```

- [ ] **Step 2: Run tests — expect compile failure (Runner still has cmd field)**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go test ./... 2>&1 | head -10
# Expected: compile error — unknown field cmd in Runner
```

- [ ] **Step 3: Rewrite runner.go**

```go
// runner.go
package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const maxConsecutiveTimeouts = 3

// Runner runs claude in a target directory sequentially in a loop.
type Runner struct {
	path    string
	delay   time.Duration
	timeout time.Duration
}

// Run executes the loop, sending state transitions to states.
// Closes states when done (permanent failure or context cancelled).
func (r *Runner) Run(ctx context.Context, states chan<- State) {
	defer close(states)
	consecutiveTimeouts := 0

	for {
		select {
		case states <- StateRunning:
		case <-ctx.Done():
			return
		}

		// Check target.md exists before running
		if _, err := os.Stat(filepath.Join(r.path, "target.md")); os.IsNotExist(err) {
			select {
			case states <- StateFailed:
			case <-ctx.Done():
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(r.delay):
			}
			continue
		}

		timedOut, exitErr := r.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}

		if timedOut {
			consecutiveTimeouts++
			if consecutiveTimeouts >= maxConsecutiveTimeouts {
				select {
				case states <- StateFailed:
				case <-ctx.Done():
				}
				return
			}
			// non-fatal timeout: wait delay and retry (no state sent)
		} else {
			consecutiveTimeouts = 0
			var nextState State
			if exitErr != nil {
				nextState = StateFailed
			} else {
				nextState = StateSuccess
			}
			select {
			case states <- nextState:
			case <-ctx.Done():
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(r.delay):
		}
	}
}

// runOnce runs claude once in r.path. Returns (timedOut, exitError).
func (r *Runner) runOnce(ctx context.Context) (bool, *exec.ExitError) {
	timeoutCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	c := exec.CommandContext(timeoutCtx, "claude",
		"-p", embeddedPrompt,
		"--model", "haiku",
		"--allowedTools", "WebSearch,Edit",
		"--setting-sources", "project,local",
	)
	c.Dir = r.path
	err := c.Run()

	if ctx.Err() != nil {
		return false, nil
	}
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, exitErr
	}
	return false, nil
}
```

- [ ] **Step 4: Run tests — expect all 4 pass**

```bash
go test ./... -v
# Expected:
# --- PASS: TestRunnerSuccess
# --- PASS: TestRunnerFailure
# --- PASS: TestRunnerConsecutiveTimeouts
# --- PASS: TestRunnerMissingTargetMD
# ok  github.com/perpetum-debile/perpetum-debile
```

- [ ] **Step 5: Commit**

```bash
git add runner.go runner_test.go
git commit -m "feat: replace shell runner with Claude Code runner, add target.md check"
```

---

## Task 3: Update main.go

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Rewrite main.go**

Replace `--cmd` flag with positional path argument and add directory validation:

```go
// main.go
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

func main() {
	delayFlag := flag.Duration("delay", 5*time.Second, "pause between end of run and next start")
	timeoutFlag := flag.Duration("timeout", 10*time.Minute, "max duration per Claude Code run")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "error: path argument is required")
		fmt.Fprintln(os.Stderr, "usage: perpetum-debile <path> [--delay 5s] [--timeout 10m]")
		os.Exit(1)
	}
	targetPath := args[0]
	info, err := os.Stat(targetPath)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "error: %q is not a valid directory\n", targetPath)
		os.Exit(1)
	}

	if *timeoutFlag <= *delayFlag {
		fmt.Fprintln(os.Stderr, "warning: --timeout should be greater than --delay")
	}

	validateIcons()

	ctx, cancel := context.WithCancel(context.Background())

	systray.Run(func() {
		systray.SetIcon(checkPNG)
		mQuit := systray.AddMenuItem("Quit", "Quit Perpetum Debile")

		runner := &Runner{path: targetPath, delay: *delayFlag, timeout: *timeoutFlag}
		states := make(chan State)

		go runner.Run(ctx, states)
		go NewAnimator(states).Run(ctx)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

		go func() {
			select {
			case <-mQuit.ClickedCh:
			case <-sigCh:
			}
			cancel()
			systray.Quit()
		}()
	}, func() {
		cancel()
	})
}

// validateIcons decodes all embedded PNGs at startup, fataling on any invalid icon.
func validateIcons() {
	icons := map[string][]byte{
		"flame1": flame1PNG,
		"flame2": flame2PNG,
		"flame3": flame3PNG,
		"flame4": flame4PNG,
		"check":  checkPNG,
		"bang":   bangPNG,
	}
	for name, data := range icons {
		if _, err := png.Decode(bytes.NewReader(data)); err != nil {
			log.Fatalf("invalid icon %q: %v", name, err)
		}
	}
}
```

- [ ] **Step 2: Build and verify**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go build ./...
# Expected: no errors

go test ./...
# Expected: all 4 tests pass

# Test missing path arg:
./perpetum-debile 2>&1
# Expected: error: path argument is required

# Test invalid path:
./perpetum-debile /nonexistent 2>&1
# Expected: error: "/nonexistent" is not a valid directory
```

- [ ] **Step 3: Update .gitignore and commit**

```bash
echo "perpetum-debile" >> .gitignore  # already there, idempotent
git add main.go
git commit -m "feat: replace --cmd flag with positional path argument"
```

---

## Done

Build and run:

```bash
go build -o perpetum-debile .

# Create a test project
mkdir /tmp/test-project
echo "# Build a hello world" > /tmp/test-project/target.md

./perpetum-debile /tmp/test-project --delay 30s
```

The app will:
1. Show flame while Claude Code runs in `/tmp/test-project`
2. Show checkmark on success, `!` on failure
3. Wait 30s, repeat
4. Show `!` immediately if `target.md` is removed, recover when it reappears
