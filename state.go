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
