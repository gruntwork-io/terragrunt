package vexec

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// ErrNoSpawn is returned by execs built from [NewNoSpawnExec], wrapped in an
// error naming the command that was attempted. Match it with [errors.Is].
var ErrNoSpawn = errors.New("vexec: process execution not permitted")

// NewNoSpawnExec returns an [Exec] whose every command fails with an error
// wrapping [ErrNoSpawn]. It lets tests assert that a code path spawns no
// subprocess, the same way
// [github.com/gruntwork-io/terragrunt/internal/vhttp.NewNoNetworkClient] and
// [github.com/gruntwork-io/terragrunt/internal/vfs.NoSymlinkFS] do for their
// respective subsystems.
//
// A test whose subject is meant to run a command supplies its own handler via
// [NewMemExec] instead. Prefer this to a handler that returns an empty
// [Result]: an empty result reports success with no output, which callers that
// read a command's stdout take as an authoritative answer.
//
// LookPath still resolves, so a path lookup alone does not fail. Wrap in
// [NoLookPathExec] to close that off too.
func NewNoSpawnExec() Exec {
	return NewMemExec(func(_ context.Context, inv Invocation) Result {
		return Result{Err: fmt.Errorf("%w: %s", ErrNoSpawn, inv.Name)}
	})
}

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
