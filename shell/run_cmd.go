// Package shell provides functions to run shell commands and Terraform commands.
package shell

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/engine"
	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// SignalForwardingDelay is the time to wait before forwarding the signal to the subcommand.
//
// The signal can be sent to the main process (only `terragrunt`) as well as the process group (`terragrunt` and `terraform`), for example:
// kill -INT <pid>  # sends SIGINT only to the main process
// kill -INT -<pid> # sends SIGINT to the process group
// Since we cannot know how the signal is sent, we should give `tofu`/`terraform` time to gracefully exit
// if it receives the signal directly from the shell, to avoid sending the second interrupt signal to `tofu`/`terraform`.
const SignalForwardingDelay = time.Second * 15

// RunCommand runs the given shell command.
func RunCommand(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, command string, args ...string) error {
	_, err := RunCommandWithOutput(ctx, l, opts, "", false, false, command, args...)

	return err
}

// RunCommandWithOutput runs the specified shell command with the specified arguments.
//
// Connect the command's stdin, stdout, and stderr to
// the currently running app. The command can be executed in a custom working directory by using the parameter
// `workingDir`. Terragrunt working directory will be assumed if empty string.
func RunCommandWithOutput(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	workingDir string,
	suppressStdout bool,
	needsPTY bool,
	command string,
	args ...string,
) (*util.CmdOutput, error) {
	var (
		output     = util.CmdOutput{}
		commandDir = workingDir
	)

	if workingDir == "" {
		commandDir = opts.WorkingDir
	}

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "run_"+command, map[string]any{
		"command": command,
		"args":    fmt.Sprintf("%v", args),
		"dir":     commandDir,
	}, func(ctx context.Context) error {
		l.Debugf("Running command: %s %s", command, strings.Join(args, " "))

		var (
			cmdStderr = io.MultiWriter(opts.ErrWriter, &output.Stderr)
			cmdStdout = io.MultiWriter(opts.Writer, &output.Stdout)
		)

		// Pass the traceparent to the child process if it is available in the context.
		traceParent := telemetry.TraceParentFromContext(ctx, opts.Telemetry)

		if traceParent != "" {
			l.Debugf("Setting trace parent=%q for command %s", traceParent, fmt.Sprintf("%s %v", command, args))
			opts.Env[telemetry.TraceParentEnv] = traceParent
		}

		if suppressStdout {
			l.Debugf("Command output will be suppressed.")

			cmdStdout = io.MultiWriter(&output.Stdout)
		}

		if command == opts.TFPath {
			// If the engine is enabled and the command is IaC executable, use the engine to run the command.
			if opts.Engine != nil && opts.EngineEnabled {
				l.Debugf("Using engine to run command: %s %s", command, strings.Join(args, " "))

				cmdOutput, err := engine.Run(ctx, l, &engine.ExecutionOptions{
					TerragruntOptions: opts,
					CmdStdout:         cmdStdout,
					CmdStderr:         cmdStderr,
					WorkingDir:        commandDir,
					SuppressStdout:    suppressStdout,
					AllocatePseudoTty: needsPTY,
					Command:           command,
					Args:              args,
				})
				if err != nil {
					return errors.New(err)
				}

				output = *cmdOutput

				return err
			}

			l.Debugf("Engine is not enabled, running command directly in %s", commandDir)
		}

		cmd := exec.Command(ctx, command, args...)
		cmd.Dir = commandDir
		cmd.Stdout = cmdStdout
		cmd.Stderr = cmdStderr
		cmd.Configure(
			exec.WithLogger(l),
			exec.WithUsePTY(needsPTY),
			exec.WithEnv(opts.Env),
			exec.WithForwardSignalDelay(SignalForwardingDelay),
		)

		if err := cmd.Start(); err != nil { //nolint:contextcheck
			err = util.ProcessExecutionError{
				Err:            err,
				Args:           args,
				Command:        command,
				WorkingDir:     cmd.Dir,
				DisableSummary: opts.LogDisableErrorSummary,
			}

			return errors.New(err)
		}

		cancelShutdown := cmd.RegisterGracefullyShutdown(ctx)
		defer cancelShutdown()

		if err := cmd.Wait(); err != nil {
			err = util.ProcessExecutionError{
				Err:            err,
				Args:           args,
				Command:        command,
				Output:         output,
				WorkingDir:     cmd.Dir,
				DisableSummary: opts.LogDisableErrorSummary,
			}

			return errors.New(err)
		}

		return nil
	})

	return &output, err
}
