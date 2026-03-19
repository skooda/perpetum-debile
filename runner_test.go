// runner_test.go
package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
