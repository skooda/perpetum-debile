// runner_test.go
package main

import (
	"context"
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

func TestRunnerSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := &Runner{cmd: "true", delay: 0, timeout: 5 * time.Second}
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

	got := collectStates(states)

	want := []State{StateRunning, StateRunning, StateRunning, StateFailed}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}
