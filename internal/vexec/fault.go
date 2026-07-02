package vexec

import (
	"context"
	"os/exec"
)

// NoLookPathExec wraps an Exec and always fails LookPath with exec.ErrNotFound,
// simulating a system where the requested binary is not on PATH. Command
// invocations pass through to the wrapped Exec unchanged.
type NoLookPathExec struct {
	Exec
}

// LookPath always returns exec.ErrNotFound.
func (e *NoLookPathExec) LookPath(file string) (string, error) {
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

// Command delegates to the wrapped Exec so that LookPath fails but direct
// Command invocations still reach the underlying backend.
func (e *NoLookPathExec) Command(ctx context.Context, name string, args ...string) Cmd {
	return e.Exec.Command(ctx, name, args...)
}
