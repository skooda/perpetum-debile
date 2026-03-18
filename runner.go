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
			// non-fatal timeout: just wait delay and retry (no state sent)
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
func (r *Runner) runOnce(ctx context.Context) (bool, *exec.ExitError) {
	timeoutCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	c := exec.CommandContext(timeoutCtx, "sh", "-c", r.cmd)
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
