package runnerpool

// Status represents the lifecycle state of a Task, following the
// naming described in the Async Queue Planning RFC.
//
// [*] StatusPending         – entry created, dependencies not yet evaluated
// [*] StatusBlocked         – waiting on at least one dependency
// [*] StatusReady           – all deps resolved, waiting for a pool slot
// [*] StatusRunning         – actively executing in a Worker
// [*] StatusSucceeded       – finished with exit code 0
// [*] StatusFailed          – finished with non‑zero exit code
// [*] StatusAncestorFailed  – skipped because an ancestor failed
// [*] StatusFailFast        – skipped due to global fail‑fast

type Status int

const (
	StatusPending Status = iota
	StatusBlocked
	StatusReady
	StatusRunning
	StatusSucceeded
	StatusFailed
	StatusAncestorFailed
	StatusFailFast
)
