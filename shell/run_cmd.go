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
func RunCommand(ctx context.Context, opts *options.TerragruntOptions, command string, args ...string) error {
	_, err := RunCommandWithOutput(ctx, opts, "", false, false, command, args...)

	return err
}

// RunCommandWithOutput runs the specified shell command with the specified arguments.
//
// Connect the command's stdin, stdout, and stderr to
// the currently running app. The command can be executed in a custom working directory by using the parameter
// `workingDir`. Terragrunt working directory will be assumed if empty string.
func RunCommandWithOutput(
	ctx context.Context,
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

	err := telemetry.Telemetry(ctx, opts, "run_"+command, map[string]any{
		"command": command,
		"args":    fmt.Sprintf("%v", args),
		"dir":     commandDir,
	}, func(childCtx context.Context) error {
		opts.Logger.Debugf("Running command: %s %s", command, strings.Join(args, " "))

		var (
			cmdStderr = io.MultiWriter(opts.ErrWriter, &output.Stderr)
			cmdStdout = io.MultiWriter(opts.Writer, &output.Stdout)
		)

		if suppressStdout {
			opts.Logger.Debugf("Command output will be suppressed.")

			cmdStdout = io.MultiWriter(&output.Stdout)
		}

		if command == opts.TerraformPath {
			// If the engine is enabled and the command is IaC executable, use the engine to run the command.
			if opts.Engine != nil && opts.EngineEnabled {
				opts.Logger.Debugf("Using engine to run command: %s %s", command, strings.Join(args, " "))

				cmdOutput, err := engine.Run(ctx, &engine.ExecutionOptions{
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

			opts.Logger.Debugf("Engine is not enabled, running command directly in %s", commandDir)
		}

		cmd := exec.Command(command, args...)
		cmd.Dir = commandDir
		cmd.Stdout = cmdStdout
		cmd.Stderr = cmdStderr
		cmd.Configure(
			exec.WithLogger(opts.Logger),
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
