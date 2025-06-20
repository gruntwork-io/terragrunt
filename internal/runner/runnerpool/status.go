package runnerpool

// Status is the state of a Task during execution.
type Status int

const (
	statusPending Status = iota
	statusBlocked
	statusReady
	statusRunning
	statusSucceeded
	statusFailed
	statusAncestorFailed
	statusFailFast
)
