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
