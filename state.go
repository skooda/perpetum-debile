package main

type State int

const (
	StateRunning State = iota
	StateSuccess
	StateFailed
)
