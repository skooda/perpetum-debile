# Perpetum Debile — macOS Menu Bar App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a self-contained macOS menu bar app in Go that runs a shell command in a loop and shows flame/checkmark/! icons based on state.

**Architecture:** Three goroutines — runner (sequential shell execution with timeout), animator (icon cycling via systray), and a signal handler — communicate via a single `chan State`. The runner owns the channel, closes it on permanent failure; the animator ranges over it. All icons are embedded PNGs via `go:embed`.

**Tech Stack:** Go 1.23+, `github.com/getlantern/systray`, stdlib only otherwise (`os/exec`, `context`, `time`, `flag`, `os/signal`, `image/png`, `go:embed`)

---

## File Map

| File | Responsibility |
|------|---------------|
| `go.mod` | Module declaration and dependency |
| `state.go` | `State` type and constants |
| `assets.go` | `go:embed` declarations, `flameFrames` slice |
| `runner.go` | Sequential shell execution, timeout counter, delay loop |
| `runner_test.go` | Unit tests for runner state transitions |
| `animator.go` | State → icon mapping, flame frame cycling |
| `main.go` | CLI flags, icon validation, systray init, signal handling |
| `cmd/gen-icons/main.go` | One-time script to generate placeholder PNG icons |
| `assets/` | 6 PNG files: flame1-4, check, bang (44×44px) |

---

## Task 1: Install Go and Bootstrap Project

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Install Go if not present**

```bash
brew install go
go version
# Expected: go version go1.23.x darwin/arm64 (or similar)
```

Also ensure Xcode CLI tools are installed (needed for CGo, which systray uses):
```bash
xcode-select --install 2>/dev/null || echo "already installed"
```

- [ ] **Step 2: Initialize go module**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go mod init github.com/perpetum-debile/perpetum-debile
```

- [ ] **Step 3: Add systray dependency**

```bash
go get github.com/getlantern/systray@latest
go mod tidy
```

- [ ] **Step 4: Verify go.mod looks correct**

```bash
cat go.mod
# Expected: module github.com/perpetum-debile/perpetum-debile, require github.com/getlantern/systray
```

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: initialize go module with systray dependency"
```

---

## Task 2: Generate Placeholder Icons

**Files:**
- Create: `cmd/gen-icons/main.go`
- Creates: `assets/flame1.png`, `assets/flame2.png`, `assets/flame3.png`, `assets/flame4.png`, `assets/check.png`, `assets/bang.png`

These are solid-color 44×44px PNGs for development. Replace with real icons later.

- [ ] **Step 1: Create the generator**

```go
// cmd/gen-icons/main.go
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
)

func save(path string, c color.RGBA) {
	img := image.NewRGBA(image.Rect(0, 0, 44, 44))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

func main() {
	if err := os.MkdirAll("assets", 0755); err != nil {
		log.Fatal(err)
	}
	save("assets/flame1.png", color.RGBA{255, 80, 0, 255})
	save("assets/flame2.png", color.RGBA{255, 120, 0, 255})
	save("assets/flame3.png", color.RGBA{255, 180, 0, 255})
	save("assets/flame4.png", color.RGBA{255, 220, 50, 255})
	save("assets/check.png", color.RGBA{0, 200, 80, 255})
	save("assets/bang.png", color.RGBA{220, 30, 30, 255})
	log.Println("Generated placeholder icons in assets/")
}
```

- [ ] **Step 2: Run the generator**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go run ./cmd/gen-icons/
# Expected: "Generated placeholder icons in assets/"
ls assets/
# Expected: bang.png  check.png  flame1.png  flame2.png  flame3.png  flame4.png
```

- [ ] **Step 3: Commit**

```bash
git add assets/ cmd/
git commit -m "feat: add placeholder icons and icon generator script"
```

---

## Task 3: State Type and Icon Embedding

**Files:**
- Create: `state.go`
- Create: `assets.go`

- [ ] **Step 1: Create state.go**

```go
// state.go
package main

// State represents the current execution state of the runner.
type State int

const (
	StateRunning State = iota
	StateSuccess
	StateFailed
)
```

- [ ] **Step 2: Create assets.go**

```go
// assets.go
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

var flameFrames = [][]byte{flame1PNG, flame2PNG, flame3PNG, flame4PNG}
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./...
# Expected: no errors
```

- [ ] **Step 4: Commit**

```bash
git add state.go assets.go
git commit -m "feat: add state type and embedded icon assets"
```

---

## Task 4: Runner with Tests

**Files:**
- Create: `runner.go`
- Create: `runner_test.go`

- [ ] **Step 1: Write the failing tests first**

```go
// runner_test.go
package main

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// collectStates drains the states channel until it's closed, returns all received states.
func collectStates(states <-chan State) []State {
	var result []State
	for s := range states {
		result = append(result, s)
	}
	return result
}

func TestRunnerSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := &Runner{cmd: "true", delay: 0, timeout: 5 * time.Second}
	states := make(chan State)
	go r.Run(ctx, states)

	// Collect first two states then cancel
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := &Runner{cmd: "false", delay: 0, timeout: 5 * time.Second}
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r := &Runner{cmd: "sleep 10", delay: 0, timeout: 50 * time.Millisecond}
	states := make(chan State)
	go r.Run(ctx, states)

	// Channel will be closed by runner after permanent failure
	got := collectStates(states)

	// 3 consecutive timeouts: StateRunning x3 then StateFailed, then channel closes
	want := []State{StateRunning, StateRunning, StateRunning, StateFailed}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}
```

- [ ] **Step 2: Run tests — expect compile failure (runner not defined yet)**

```bash
go test ./... 2>&1 | head -20
# Expected: undefined: Runner
```

- [ ] **Step 3: Implement runner.go**

```go
// runner.go
package main

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

const maxConsecutiveTimeouts = 3

// Runner executes a shell command sequentially in a loop.
type Runner struct {
	cmd     string
	delay   time.Duration
	timeout time.Duration
}

// Run executes the command loop, sending state transitions to states.
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

// runOnce runs the command once. Returns (timedOut, exitError).
// Returns (false, nil, nil) on success; the caller checks ctx.Err() for shutdown.
func (r *Runner) runOnce(ctx context.Context) (bool, *exec.ExitError) {
	timeoutCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "sh", "-c", r.cmd)
	err := cmd.Run()

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

- [ ] **Step 4: Run tests — expect all pass**

```bash
go test ./... -v
# Expected:
# --- PASS: TestRunnerSuccess
# --- PASS: TestRunnerFailure
# --- PASS: TestRunnerConsecutiveTimeouts
# ok  github.com/perpetum-debile/perpetum-debile
```

- [ ] **Step 5: Commit**

```bash
git add runner.go runner_test.go
git commit -m "feat: implement runner with sequential execution and timeout handling"
```

---

## Task 5: Animator

**Files:**
- Create: `animator.go`

The animator receives states and updates the systray icon. Flame cycles at 150ms; success/fail hold static icons.

- [ ] **Step 1: Create animator.go**

```go
// animator.go
package main

import (
	"context"
	"time"

	"github.com/getlantern/systray"
)

// Animator reads states and updates the systray icon accordingly.
type Animator struct {
	states <-chan State
}

// NewAnimator creates an Animator reading from states.
func NewAnimator(states <-chan State) *Animator {
	return &Animator{states: states}
}

// Run drives the icon animation until ctx is cancelled or states is closed.
func (a *Animator) Run(ctx context.Context) {
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	current := StateSuccess // initial icon before first run
	frameIdx := 0

	for {
		select {
		case <-ctx.Done():
			return
		case state, ok := <-a.states:
			if !ok {
				return
			}
			current = state
			if state == StateRunning {
				frameIdx = 0
			}
		case <-ticker.C:
			switch current {
			case StateRunning:
				systray.SetTemplateIcon(flameFrames[frameIdx], flameFrames[frameIdx])
				frameIdx = (frameIdx + 1) % len(flameFrames)
			case StateSuccess:
				systray.SetTemplateIcon(checkPNG, checkPNG)
			case StateFailed:
				systray.SetTemplateIcon(bangPNG, bangPNG)
			}
		}
	}
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./...
# Expected: no errors
```

- [ ] **Step 3: Commit**

```bash
git add animator.go
git commit -m "feat: implement animator with flame cycling and state-based icon switching"
```

---

## Task 6: Main Entrypoint, Build, and Run

**Files:**
- Create: `main.go`

- [ ] **Step 1: Create main.go**

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
	cmd := flag.String("cmd", "", "shell command to run (required)")
	delay := flag.Duration("delay", 5*time.Second, "pause between end of run and next start")
	timeout := flag.Duration("timeout", 10*time.Minute, "max duration per command run")
	flag.Parse()

	if *cmd == "" {
		fmt.Fprintln(os.Stderr, "error: --cmd is required")
		flag.Usage()
		os.Exit(1)
	}
	if *timeout <= *delay {
		fmt.Fprintln(os.Stderr, "warning: --timeout should be greater than --delay")
	}

	validateIcons()

	ctx, cancel := context.WithCancel(context.Background())

	systray.Run(func() {
		systray.SetTemplateIcon(checkPNG, checkPNG)
		mQuit := systray.AddMenuItem("Quit", "Quit Perpetum Debile")

		runner := &Runner{cmd: *cmd, delay: *delay, timeout: *timeout}
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

- [ ] **Step 2: Build the binary**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go build -o perpetum-debile .
ls -lh perpetum-debile
# Expected: binary exists, ~5-10MB
```

- [ ] **Step 3: Run a quick smoke test**

Run with a fast-succeeding command to verify the menu bar icon appears:

```bash
./perpetum-debile --cmd "echo hello" --delay 2s --timeout 30s
```

Expected:
- A colored square appears in the menu bar (checkmark green initially, then cycles orange→orange→orange during `echo`, then green again)
- Right-clicking shows "Quit" menu item
- Quit exits cleanly

- [ ] **Step 4: Test failure path**

```bash
./perpetum-debile --cmd "exit 1" --delay 2s
```

Expected: icon briefly shows flame (running), then goes red (!). After the 2s delay, the flame appears again (runner retries), then red again. The app **keeps looping** — regular non-zero exits do NOT cause permanent failure; only 3 consecutive timeouts do.

- [ ] **Step 5: Test timeout path**

```bash
./perpetum-debile --cmd "sleep 60" --delay 1s --timeout 500ms
```

Expected: flame shows while waiting, then after 3 timeouts (~1.5s + delays) icon goes red permanently.

- [ ] **Step 6: Run full test suite**

```bash
go test ./...
# Expected: PASS
```

- [ ] **Step 7: Add binary to .gitignore**

```bash
echo "perpetum-debile" >> .gitignore
git add .gitignore
```

- [ ] **Step 8: Commit**

```bash
git add main.go
git commit -m "feat: add main entrypoint — app is fully functional"
```

---

## Done

The app is functional. To use it:

```bash
./perpetum-debile --cmd "your-script.sh" --delay 30s --timeout 2m
```

**Next steps (out of scope for this plan):**
- Replace placeholder colored squares in `assets/` with real flame/check/! PNG icons
- Package as a `.app` bundle for proper macOS installation
- Add to Login Items for auto-start
