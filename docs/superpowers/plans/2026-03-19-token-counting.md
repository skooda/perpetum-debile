# Token Counting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Display per-run and cumulative token usage (e.g. `847 / 12.4k`) next to the menu bar icon.

**Architecture:** Three sequential tasks — migrate `State` from `int` to a struct (Task 1), add JSON token extraction from `claude` output (Task 2), add `SetTitle` display and `formatTokens` helper to the animator (Task 3). Each task leaves the build green.

**Tech Stack:** Go 1.26+, `encoding/json`, `github.com/getlantern/systray`

---

## File Map

| File | Change |
|------|--------|
| `state.go` | Replace `State int` + iota consts with `StateKind int` + `State struct` |
| `runner.go` | Send `State` structs; add `--output-format json`; parse tokens; accumulate total |
| `runner_test.go` | Update comparisons to `.Kind`; stubs emit JSON; assert token fields |
| `animator.go` | Update comparisons to `.Kind`; add `hasReceivedState`; add `SetTitle`; add `formatTokens`/`tokenLabel` |
| `animator_test.go` | **Create** — tests for `formatTokens` |

---

## Task 1: Migrate State type and update callsites

**Files:**
- Modify: `state.go`
- Modify: `runner.go`
- Modify: `runner_test.go`
- Modify: `animator.go` — minimal compile-only fix (`.Kind` comparisons, no new features)

- [ ] **Step 1: Rewrite state.go**

Replace the entire file:

```go
package main

// StateKind is the kind of a runner state.
type StateKind int

const (
	StateRunning StateKind = iota
	StateSuccess
	StateFailed
)

// State is sent on the states channel by Runner.
type State struct {
	Kind      StateKind
	RunTokens int64 // tokens used this run (0 if unknown)
	Total     int64 // cumulative tokens since app start
}
```

- [ ] **Step 2: Update runner.go — send State structs**

`runner.go` currently sends bare `State` values (`states <- StateRunning` etc.). Now `State` is a struct so all sends must use struct literals. Tokens are all zero for now (added in Task 2).

Replace the full `runner.go` with:

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
	totalTokens := int64(0)

	for {
		select {
		case states <- State{Kind: StateRunning, Total: totalTokens}:
		case <-ctx.Done():
			return
		}

		// Check target.md exists before running
		if _, err := os.Stat(filepath.Join(r.path, "target.md")); os.IsNotExist(err) {
			select {
			case states <- State{Kind: StateFailed, Total: totalTokens}:
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
				case states <- State{Kind: StateFailed, Total: totalTokens}:
				case <-ctx.Done():
				}
				return
			}
			// non-fatal timeout: wait delay and retry (no state sent)
		} else {
			consecutiveTimeouts = 0
			var kind StateKind
			if exitErr != nil {
				kind = StateFailed
			} else {
				kind = StateSuccess
			}
			select {
			case states <- State{Kind: kind, Total: totalTokens}:
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

Note: `runOnce` keeps its current signature — token extraction comes in Task 2.

- [ ] **Step 3: Update animator.go — minimal compile fix**

`animator.go` uses `current := StateSuccess`, `state == StateRunning`, and `switch current { case StateRunning: ... }`. All of these break now that `State` is a struct and `StateRunning`/`StateSuccess`/`StateFailed` are `StateKind` values. Apply only the three mechanical changes needed to compile — no `SetTitle`, no `formatTokens` yet.

Make these three targeted edits to `animator.go`:

1. Change `current := StateSuccess` → `current := State{Kind: StateSuccess}`
2. Change `if state == StateRunning {` → `if state.Kind == StateRunning {`
3. Change `switch current {` → `switch current.Kind {`

The `case StateRunning:`, `case StateSuccess:`, `case StateFailed:` lines are fine as-is — they now compare a `StateKind` switch expression against `StateKind` constants, which is correct.

- [ ] **Step 4: Update runner_test.go — fix comparisons**

The tests currently compare `got[n] != StateRunning` etc. (direct equality against the old `State int` constants). These must change to compare `.Kind`. Replace the full file:

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
// claudeScript is the shell script body. If withTargetMD is true, creates target.md.
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
	if got[0].Kind != StateRunning {
		t.Errorf("state[0].Kind: want StateRunning, got %v", got[0].Kind)
	}
	if got[1].Kind != StateSuccess {
		t.Errorf("state[1].Kind: want StateSuccess, got %v", got[1].Kind)
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
	if got[0].Kind != StateRunning {
		t.Errorf("state[0].Kind: want StateRunning, got %v", got[0].Kind)
	}
	if got[1].Kind != StateFailed {
		t.Errorf("state[1].Kind: want StateFailed, got %v", got[1].Kind)
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

	want := []State{
		{Kind: StateRunning},
		{Kind: StateRunning},
		{Kind: StateRunning},
		{Kind: StateFailed},
	}
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
	if got[0].Kind != StateRunning {
		t.Errorf("state[0].Kind: want StateRunning, got %v", got[0].Kind)
	}
	if got[1].Kind != StateFailed {
		t.Errorf("state[1].Kind: want StateFailed, got %v", got[1].Kind)
	}
}
```

- [ ] **Step 5: Build and test**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go build ./...
# Expected: no errors

go test ./...
# Expected: all 4 tests pass
```

- [ ] **Step 6: Commit**

```bash
git add state.go runner.go runner_test.go animator.go
git commit -m "refactor: migrate State from int to struct with Kind/RunTokens/Total"
```

---

## Task 2: Add token extraction

**Files:**
- Modify: `runner.go` — add `--output-format json`, capture stdout, parse tokens, update `runOnce` signature, accumulate total
- Modify: `runner_test.go` — stubs emit JSON, assert token fields

- [ ] **Step 1: Update runner_test.go — add JSON stubs and token assertions**

Update stubs to emit JSON and add token assertions to each test. Replace the full file:

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

// successJSON emits 1000 tokens total (100+200+300+400) and exits 0.
const successJSON = `echo '{"usage":{"input_tokens":100,"cache_creation_input_tokens":200,"cache_read_input_tokens":300,"output_tokens":400}}' && exit 0`

// failureJSON emits 100 tokens total (50+50) and exits 1.
const failureJSON = `echo '{"usage":{"input_tokens":50,"output_tokens":50}}' && exit 1`

func TestRunnerSuccess(t *testing.T) {
	projectDir, cleanup := setupTestEnv(t, successJSON, true)
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
	if got[0].Kind != StateRunning {
		t.Errorf("state[0].Kind: want StateRunning, got %v", got[0].Kind)
	}
	if got[1].Kind != StateSuccess {
		t.Errorf("state[1].Kind: want StateSuccess, got %v", got[1].Kind)
	}
	if got[1].RunTokens != 1000 {
		t.Errorf("state[1].RunTokens: want 1000, got %d", got[1].RunTokens)
	}
	if got[1].Total != 1000 {
		t.Errorf("state[1].Total: want 1000, got %d", got[1].Total)
	}
}

func TestRunnerFailure(t *testing.T) {
	projectDir, cleanup := setupTestEnv(t, failureJSON, true)
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
	if got[0].Kind != StateRunning {
		t.Errorf("state[0].Kind: want StateRunning, got %v", got[0].Kind)
	}
	if got[1].Kind != StateFailed {
		t.Errorf("state[1].Kind: want StateFailed, got %v", got[1].Kind)
	}
	if got[1].RunTokens != 100 {
		t.Errorf("state[1].RunTokens: want 100, got %d", got[1].RunTokens)
	}
	if got[1].Total != 100 {
		t.Errorf("state[1].Total: want 100, got %d", got[1].Total)
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

	want := []State{
		{Kind: StateRunning},
		{Kind: StateRunning},
		{Kind: StateRunning},
		{Kind: StateFailed},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestRunnerMissingTargetMD(t *testing.T) {
	projectDir, cleanup := setupTestEnv(t, successJSON, false) // no target.md
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
	if got[0].Kind != StateRunning {
		t.Errorf("state[0].Kind: want StateRunning, got %v", got[0].Kind)
	}
	if got[1].Kind != StateFailed {
		t.Errorf("state[1].Kind: want StateFailed, got %v", got[1].Kind)
	}
	// No tokens consumed when target.md is missing
	if got[1].RunTokens != 0 {
		t.Errorf("state[1].RunTokens: want 0, got %d", got[1].RunTokens)
	}
}
```

- [ ] **Step 2: Run tests — expect failures on RunTokens/Total**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go test ./... 2>&1
# Expected: TestRunnerSuccess and TestRunnerFailure fail on RunTokens/Total assertions
# TestRunnerConsecutiveTimeouts and TestRunnerMissingTargetMD should still pass
```

- [ ] **Step 3: Rewrite runner.go with token extraction**

Replace the full file:

```go
// runner.go
package main

import (
	"context"
	"encoding/json"
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
	totalTokens := int64(0)

	for {
		select {
		case states <- State{Kind: StateRunning, Total: totalTokens}:
		case <-ctx.Done():
			return
		}

		// Check target.md exists before running
		if _, err := os.Stat(filepath.Join(r.path, "target.md")); os.IsNotExist(err) {
			select {
			case states <- State{Kind: StateFailed, Total: totalTokens}:
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

		timedOut, runTokens, exitErr := r.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}

		if timedOut {
			consecutiveTimeouts++
			if consecutiveTimeouts >= maxConsecutiveTimeouts {
				select {
				case states <- State{Kind: StateFailed, Total: totalTokens}:
				case <-ctx.Done():
				}
				return
			}
			// non-fatal timeout: wait delay and retry (no state sent)
		} else {
			consecutiveTimeouts = 0
			totalTokens += runTokens
			var kind StateKind
			if exitErr != nil {
				kind = StateFailed
			} else {
				kind = StateSuccess
			}
			select {
			case states <- State{Kind: kind, RunTokens: runTokens, Total: totalTokens}:
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

// runOnce runs claude once in r.path. Returns (timedOut, tokens, exitError).
func (r *Runner) runOnce(ctx context.Context) (bool, int64, *exec.ExitError) {
	timeoutCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	c := exec.CommandContext(timeoutCtx, "claude",
		"-p", embeddedPrompt,
		"--model", "haiku",
		"--allowedTools", "WebSearch,Edit",
		"--setting-sources", "project,local",
		"--output-format", "json",
	)
	c.Dir = r.path
	out, err := c.Output()

	if ctx.Err() != nil {
		return false, 0, nil
	}
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return true, 0, nil
	}

	tokens := parseTokens(out)

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, tokens, exitErr
	}
	return false, tokens, nil
}

// parseTokens extracts the total token count from claude --output-format json stdout.
// Sums all four fields: input, cache_creation_input, cache_read_input, output.
// Returns 0 on parse failure (best-effort; run result is unaffected).
func parseTokens(data []byte) int64 {
	var out struct {
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return 0
	}
	return out.Usage.InputTokens +
		out.Usage.CacheCreationInputTokens +
		out.Usage.CacheReadInputTokens +
		out.Usage.OutputTokens
}
```

- [ ] **Step 4: Run tests — expect all 4 pass**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go test ./... -v
# Expected: all 4 tests pass
```

- [ ] **Step 5: Commit**

```bash
git add runner.go runner_test.go
git commit -m "feat: add token extraction from claude --output-format json"
```

---

## Task 3: Animator display

**Files:**
- Modify: `animator.go`
- Create: `animator_test.go`

- [ ] **Step 1: Create animator_test.go with formatTokens tests**

```go
// animator_test.go
package main

import "testing"

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1k"},
		{1234, "1.2k"},
		{9999, "10k"},
		{12345, "12.3k"},
		{100000, "100k"},
	}
	for _, tc := range tests {
		got := formatTokens(tc.n)
		if got != tc.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run tests — expect compile error**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go test ./... 2>&1 | head -5
# Expected: compile error — undefined: formatTokens
```

- [ ] **Step 3: Rewrite animator.go**

Replace the full file:

```go
// animator.go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/getlantern/systray"
)

// Animator reads states and updates the systray icon and title accordingly.
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

	current := State{Kind: StateSuccess}
	frameIdx := 0
	hasReceivedState := false

	for {
		select {
		case <-ctx.Done():
			return
		case state, ok := <-a.states:
			if !ok {
				return
			}
			hasReceivedState = true
			current = state
			if state.Kind == StateRunning {
				frameIdx = 0
			}
		case <-ticker.C:
			switch current.Kind {
			case StateRunning:
				systray.SetIcon(flameFrames[frameIdx])
				frameIdx = (frameIdx + 1) % len(flameFrames)
			case StateSuccess:
				systray.SetIcon(checkPNG)
			case StateFailed:
				systray.SetIcon(bangPNG)
			}
			if hasReceivedState {
				systray.SetTitle(tokenLabel(current))
			}
		}
	}
}

// tokenLabel returns the menu bar text for the given state.
func tokenLabel(s State) string {
	total := formatTokens(s.Total)
	if s.Kind == StateRunning {
		return "… / " + total
	}
	return formatTokens(s.RunTokens) + " / " + total
}

// formatTokens formats a token count for display.
// Values under 1000 are shown as-is; 1000+ as "1.2k" with trailing ".0" omitted.
func formatTokens(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	s := fmt.Sprintf("%.1f", float64(n)/1000.0)
	if len(s) > 2 && s[len(s)-2:] == ".0" {
		s = s[:len(s)-2]
	}
	return s + "k"
}
```

- [ ] **Step 4: Run tests — expect all pass**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go test ./... -v
# Expected: all tests pass including TestFormatTokens
go build ./...
# Expected: no errors
```

- [ ] **Step 5: Commit and push**

```bash
git add animator.go animator_test.go
git commit -m "feat: display token usage in menu bar (per-run / cumulative)"
git push origin main
```

---

## Done

Build and run:

```bash
go build -o perpetum-debile .
echo "# Build a hello world" > /tmp/test-project/target.md
./perpetum-debile /tmp/test-project --delay 30s
```

The menu bar will show e.g. `847 / 12.4k` after each run, and `… / 12.4k` while claude is running.
