package run

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/tflint"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestHookErrorMessage_WithStderr(t *testing.T) {
	t.Parallel()

	var output util.CmdOutput
	output.Stderr.WriteString("resource missing required tags")

	err := util.ProcessExecutionError{
		Err:        stubExitErr{code: 2},
		Command:    "tflint",
		Args:       []string{"--config", ".tflint.hcl"},
		WorkingDir: "/tmp",
		Output:     output,
	}

	msg := hookErrorMessage("my-lint", errors.New(err))
	assert.Contains(t, msg, `Hook "my-lint"`)
	assert.Contains(t, msg, "tflint --config .tflint.hcl")
	assert.Contains(t, msg, "non-zero exit code 2")
	assert.Contains(t, msg, "resource missing required tags")
}

func TestHookErrorMessage_StdoutFallback(t *testing.T) {
	t.Parallel()

	var output util.CmdOutput
	output.Stdout.WriteString("warning: deprecated feature")

	err := util.ProcessExecutionError{
		Err:        stubExitErr{code: 1},
		Command:    "custom-lint",
		Args:       []string{"--fix"},
		WorkingDir: "/tmp",
		Output:     output,
	}

	msg := hookErrorMessage("lint-hook", errors.New(err))
	assert.Contains(t, msg, `Hook "lint-hook"`)
	assert.Contains(t, msg, "custom-lint --fix")
	assert.Contains(t, msg, "non-zero exit code 1")
	assert.Contains(t, msg, "warning: deprecated feature")
}

func TestHookErrorMessage_NoOutput(t *testing.T) {
	t.Parallel()

	err := util.ProcessExecutionError{
		Err:        stubExitErr{code: 3},
		Command:    "check",
		Args:       []string{"-strict"},
		WorkingDir: "/tmp",
	}

	msg := hookErrorMessage("my-hook", errors.New(err))
	assert.Contains(t, msg, `Hook "my-hook"`)
	assert.Contains(t, msg, "check -strict")
	assert.Contains(t, msg, "non-zero exit code 3")
}

func TestHookErrorMessage_TflintWrapped(t *testing.T) {
	t.Parallel()

	var output util.CmdOutput
	output.Stderr.WriteString("3 issue(s) found")

	processErr := util.ProcessExecutionError{
		Err:        stubExitErr{code: 2},
		Command:    "tflint",
		Args:       []string{"--config", ".tflint.hcl"},
		WorkingDir: "/tmp",
		Output:     output,
	}

	// Simulate the real tflint error chain: ErrorRunningTflint wraps ProcessExecutionError
	tflintErr := tflint.ErrorRunningTflint{
		Args: []string{"tflint", "--config", ".tflint.hcl"},
		Err:  errors.New(processErr),
	}

	msg := hookErrorMessage("tflint", errors.New(tflintErr))
	assert.Contains(t, msg, `Hook "tflint"`)
	assert.Contains(t, msg, "tflint --config .tflint.hcl")
	assert.Contains(t, msg, "non-zero exit code 2")
	assert.Contains(t, msg, "3 issue(s) found")
}

func TestHookErrorMessage_NonProcessError(t *testing.T) {
	t.Parallel()

	err := errors.New("exec: \"tflint\": executable file not found in $PATH")

	msg := hookErrorMessage("my-hook", err)
	assert.Equal(t, `Hook "my-hook" failed to execute: exec: "tflint": executable file not found in $PATH`, msg)
}

// FuzzHookErrorMessage pins the formatter against arbitrary inputs. The
// formatter dereferences ProcessExecutionError fields, so its contract is
// "must not panic on any field value." We exercise both wrapped and bare
// errors via a one-bit selector in the corpus.
func FuzzHookErrorMessage(f *testing.F) {
	type seed struct {
		hookName, command, errMsg, stderr, stdout string
		args                                      []string
		exitCode                                  int
		wrap                                      bool
	}

	seeds := []seed{
		{hookName: "lint", command: "tflint", args: []string{"--config", ".tflint.hcl"}, exitCode: 2, stderr: "3 issues", wrap: true},
		{hookName: "", command: "", args: nil, exitCode: 0, wrap: true},
		{hookName: "x", errMsg: "raw error", wrap: false},
		{hookName: "long", command: "go", args: []string{strings.Repeat("a", 1024)}, exitCode: 137, stdout: "out\nput", wrap: true},
		{hookName: "neg", command: "x", exitCode: -1, wrap: true},
	}

	for _, s := range seeds {
		args := strings.Join(s.args, "\x00")
		f.Add(s.hookName, s.command, args, s.errMsg, s.stderr, s.stdout, s.exitCode, s.wrap)
	}

	f.Fuzz(func(_ *testing.T,
		hookName, command, argsJoined, errMsg, stderr, stdout string,
		exitCode int, wrap bool,
	) {
		var args []string

		if argsJoined != "" {
			args = strings.Split(argsJoined, "\x00")
		}

		var feed error

		if wrap {
			var output util.CmdOutput

			output.Stderr.WriteString(stderr)
			output.Stdout.WriteString(stdout)

			feed = util.ProcessExecutionError{
				Err:        stubExitErr{code: exitCode},
				Command:    command,
				Args:       args,
				WorkingDir: "/tmp",
				Output:     output,
			}
		} else {
			feed = errors.New(errMsg)
		}

		// Contract: must not panic for any input.
		_ = hookErrorMessage(hookName, feed)
	})
}

// stubExitErr is a stand-in error that exposes an ExitStatus() so
// [util.GetExitCode] resolves to the encoded code. Both unit tests and
// the fuzz target use it instead of shelling out to produce a real
// *exec.ExitError.
type stubExitErr struct{ code int }

func (e stubExitErr) Error() string            { return fmt.Sprintf("exit status %d", e.code) }
func (e stubExitErr) ExitStatus() (int, error) { return e.code, nil }
