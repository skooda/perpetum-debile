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

	current := State{Kind: StateSuccess}
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
		}
	}
}
