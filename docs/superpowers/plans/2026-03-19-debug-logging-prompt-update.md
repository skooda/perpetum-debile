# Debug Logging & Prompt Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--debug` flag that appends claude output to `<path>/debug.log`, and update `prompt.txt` to use git commits instead of standup.log and to prefer short steps.

**Architecture:** Two independent tasks. Task 1 is a single file edit (prompt.txt). Task 2 adds a `debugLog *os.File` field to `Runner`, adds `--debug` flag to `main.go`, and replaces `c.Output()` with `c.Run()` + manual stdout tee in `runOnce`. **Prerequisite: the token counting plan (`2026-03-19-token-counting.md`) must be fully implemented before starting Task 2** — it introduces the `stdoutBuf`, `--output-format json`, and `c.Output()` that Task 2 modifies.

**Tech Stack:** Go 1.26+, `bytes`, `fmt`, `io`, `os`

---

## File Map

| File | Change |
|------|--------|
| `prompt.txt` | Replace content |
| `main.go` | Add `--debug` flag; open `debug.log` if set; pass to `Runner` |
| `runner.go` | Add `debugLog *os.File` to `Runner`; replace `c.Output()` with `c.Run()` + tee |
| `runner_test.go` | Add `TestRunnerDebugLog` |

---

## Task 1: Update prompt.txt

**Files:**
- Modify: `prompt.txt`

No tests needed — `prompt.txt` is embedded at compile time; a successful `go build` confirms it's picked up.

- [ ] **Step 1: Replace prompt.txt**

Replace the entire file content with:

```
Look at current directory, read target.md, decide what is the best thing to do next in order to move project towards target described in target.md, do it, then commit your changes with git commit -m "<one line standup about what you did>". Keep each step small and focused — it should take no more than 2-3 minutes. Prefer doing less per iteration and running more iterations.
```

- [ ] **Step 2: Build to verify embed**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go build ./...
# Expected: no errors
```

- [ ] **Step 3: Commit**

```bash
git add prompt.txt
git commit -m "feat: update prompt — git commit instead of standup.log, short steps"
```

---

## Task 2: Add --debug flag

**Files:**
- Modify: `runner.go`
- Modify: `runner_test.go`
- Modify: `main.go`

> **Prerequisite:** Confirm the token counting plan is implemented — `runner.go` must already have `--output-format json`, `stdoutBuf bytes.Buffer`, `c.Output()`, and `runOnce` returning `(bool, int64, *exec.ExitError)`. If not, implement that plan first.

- [ ] **Step 1: Write the failing test**

Add `TestRunnerDebugLog` to `runner_test.go`. Add `"strings"` to the imports block.

The test uses its own inline JSON stub (100+0+0+900 = 1000 tokens) so it does not depend on any `successJSON` constant that may or may not exist:

```go
func TestRunnerDebugLog(t *testing.T) {
	script := `printf '{"usage":{"input_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":900}}\n'; exit 0`
	projectDir, cleanup := setupTestEnv(t, script, true)
	defer cleanup()

	debugFile := filepath.Join(t.TempDir(), "debug.log")
	f, err := os.Create(debugFile)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := &Runner{path: projectDir, delay: 0, timeout: 5 * time.Second, debugLog: f}
	states := make(chan State)
	go r.Run(ctx, states)

	got := make([]State, 0)
	for s := range states {
		got = append(got, s)
		if len(got) >= 2 {
			cancel()
		}
	}
	f.Close()

	data, err := os.ReadFile(debugFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "--- ") {
		t.Errorf("debug.log missing timestamp header, got: %q", content)
	}
	if !strings.Contains(content, "usage") {
		t.Errorf("debug.log missing claude output, got: %q", content)
	}
	if got[1].RunTokens != 1000 {
		t.Errorf("RunTokens: want 1000, got %d", got[1].RunTokens)
	}
}
```

- [ ] **Step 2: Run test — expect compile error**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go test ./... 2>&1 | head -5
# Expected: compile error — unknown field debugLog in Runner
```

- [ ] **Step 3: Rewrite runner.go**

Replace the full file. Key changes from the token-counting version:
- Add `debugLog *os.File` to `Runner` struct
- Add `"bytes"`, `"fmt"`, `"io"` imports
- Replace `c.Output()` with `c.Run()` + manual stdout/stderr management

```go
// runner.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const maxConsecutiveTimeouts = 3

// Runner runs claude in a target directory sequentially in a loop.
type Runner struct {
	path     string
	delay    time.Duration
	timeout  time.Duration
	debugLog *os.File // nil = debug logging off
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

	var stdoutBuf bytes.Buffer
	if r.debugLog != nil {
		fmt.Fprintf(r.debugLog, "--- %s ---\n", time.Now().Format(time.RFC3339))
		c.Stdout = io.MultiWriter(&stdoutBuf, r.debugLog)
		c.Stderr = r.debugLog
	} else {
		c.Stdout = &stdoutBuf
	}
	err := c.Run()

	if r.debugLog != nil {
		fmt.Fprintln(r.debugLog)
	}

	if ctx.Err() != nil {
		return false, 0, nil
	}
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return true, 0, nil
	}

	tokens := parseTokens(stdoutBuf.Bytes())

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

- [ ] **Step 4: Run tests — expect all pass**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go test ./... -v
# Expected: all tests pass including TestRunnerDebugLog
```

- [ ] **Step 5: Update main.go**

Add `--debug` flag, open `debug.log` if set, pass to `Runner`. Add `"path/filepath"` to imports if not already present.

Replace the argument parsing + Runner construction section of `main.go`. The full updated `main.go`:

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
	"path/filepath"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

func main() {
	delayFlag := flag.Duration("delay", 5*time.Second, "pause between end of run and next start")
	timeoutFlag := flag.Duration("timeout", 10*time.Minute, "max duration per Claude Code run")
	debugFlag := flag.Bool("debug", false, "append claude output to <path>/debug.log")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "error: path argument is required")
		fmt.Fprintln(os.Stderr, "usage: perpetum-debile <path> [--delay 5s] [--timeout 10m] [--debug]")
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

	var debugLog *os.File
	if *debugFlag {
		debugLog, err = os.OpenFile(filepath.Join(targetPath, "debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot open debug.log: %v\n", err)
			os.Exit(1)
		}
		defer debugLog.Close()
	}

	validateIcons()

	ctx, cancel := context.WithCancel(context.Background())

	systray.Run(func() {
		systray.SetIcon(checkPNG)
		mQuit := systray.AddMenuItem("Quit", "Quit Perpetum Debile")

		runner := &Runner{path: targetPath, delay: *delayFlag, timeout: *timeoutFlag, debugLog: debugLog}
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

- [ ] **Step 6: Build and verify**

```bash
cd "/Users/pavel.skoda/Work/Perpetum Debile"
go build ./...
# Expected: no errors

go test ./...
# Expected: all tests pass

# Test --debug flag error on missing path:
./perpetum-debile /nonexistent --debug 2>&1
# Expected: error: "/nonexistent" is not a valid directory
```

- [ ] **Step 7: Commit and push**

```bash
git add runner.go runner_test.go main.go
git commit -m "feat: add --debug flag to log claude output to <path>/debug.log"
git push origin main
```

---

## Done

Build and run with debug logging:

```bash
go build -o perpetum-debile .
echo "# Build a hello world" > /tmp/test-project/target.md
./perpetum-debile /tmp/test-project --delay 30s --debug
# claude output is appended to /tmp/test-project/debug.log each run
tail -f /tmp/test-project/debug.log
```
