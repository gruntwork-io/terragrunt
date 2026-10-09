package helpers

import (
	"context"
	"slices"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
)

// RecordedCommand is a command prepared through an [ExecRecorder].
type RecordedCommand struct {
	Name string
	Dir  string
	Args []string
}

// ExecRecorder wraps a [vexec.Exec] and records every command prepared through
// it, so a test can assert which subprocesses a run spawned.
//
// Commands it prepares do not expose the wrapped [vexec.OSCmder], so a command
// that needs a PTY fails to start.
type ExecRecorder struct {
	vexec.Exec

	commands []*RecordedCommand
	mu       sync.Mutex
}

// NewExecRecorder returns an [ExecRecorder] that runs commands through e.
func NewExecRecorder(e vexec.Exec) *ExecRecorder {
	return &ExecRecorder{Exec: e}
}

// Command records the command and prepares it through the wrapped Exec.
func (r *ExecRecorder) Command(ctx context.Context, name string, args ...string) vexec.Cmd {
	rc := &RecordedCommand{Name: name, Args: slices.Clone(args)}

	r.mu.Lock()
	r.commands = append(r.commands, rc)
	r.mu.Unlock()

	return &recordedCmd{Cmd: r.Exec.Command(ctx, name, args...), recorder: r, command: rc}
}

// Commands returns a copy of every command recorded so far, in the order they
// were prepared.
func (r *ExecRecorder) Commands() []RecordedCommand {
	r.mu.Lock()
	defer r.mu.Unlock()

	commands := make([]RecordedCommand, 0, len(r.commands))
	for _, rc := range r.commands {
		commands = append(commands, *rc)
	}

	return commands
}

// recordedCmd records the working directory its command runs in.
type recordedCmd struct {
	vexec.Cmd

	recorder *ExecRecorder
	command  *RecordedCommand
}

func (c *recordedCmd) SetDir(dir string) {
	c.recorder.mu.Lock()
	c.command.Dir = dir
	c.recorder.mu.Unlock()

	c.Cmd.SetDir(dir)
}
