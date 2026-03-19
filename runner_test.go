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
