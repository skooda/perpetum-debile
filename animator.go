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
