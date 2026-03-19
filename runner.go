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
