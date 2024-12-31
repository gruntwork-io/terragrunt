// Package shell provides functions to run shell commands and Terraform commands.
package shell

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/gruntwork-io/terragrunt/engine"
	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/log/writer"
	"github.com/gruntwork-io/terragrunt/terraform"

	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/gruntwork-io/go-commons/collections"
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

const (
	// tfLogMsgPrefix is a message prefix that is prepended to each TF_LOG output lines when the output is integrated in TG log, for example:
	//
	// TF_LOG: using github.com/zclconf/go-cty v1.14.3
	// TF_LOG: Go runtime version: go1.22.1
	tfLogMsgPrefix = "TF_LOG: "

	logMsgSeparator = "\n"
)

// Commands that implement a REPL need a pseudo TTY when run as a subprocess in order for the readline properties to be
// preserved. This is a list of terraform commands that have this property, which is used to determine if terragrunt
// should allocate a ptty when running that terraform command.
var terraformCommandsThatNeedPty = []string{
	terraform.CommandNameConsole,
}

// RunTerraformCommand runs the given Terraform command.
func RunTerraformCommand(ctx context.Context, opts *options.TerragruntOptions, args ...string) error {
	_, err := RunTerraformCommandWithOutput(ctx, opts, args...)

	return err
}

// RunTerraformCommandWithOutput runs the given Terraform command, writing its stdout/stderr to the terminal AND returning stdout/stderr to this
// method's caller
func RunTerraformCommandWithOutput(ctx context.Context, opts *options.TerragruntOptions, args ...string) (*util.CmdOutput, error) {
	needsPTY, err := isTerraformCommandThatNeedsPty(args)
	if err != nil {
		return nil, err
	}

	output, err := RunShellCommandWithOutput(ctx, opts, "", false, needsPTY, opts.TerraformPath, args...)

	if err != nil && util.ListContainsElement(args, terraform.FlagNameDetailedExitCode) {
		code, _ := util.GetExitCode(err)
		if exitCode := DetailedExitCodeFromContext(ctx); exitCode != nil {
			exitCode.Set(code)
		}

		if code != 1 {
			return output, nil
		}
	}

	return output, err
}

// RunShellCommand runs the given shell command.
func RunShellCommand(ctx context.Context, opts *options.TerragruntOptions, command string, args ...string) error {
	_, err := RunShellCommandWithOutput(ctx, opts, "", false, false, command, args...)

	return err
}

// RunShellCommandWithOutput runs the specified shell command with the specified arguments.
//
// Connect the command's stdin, stdout, and stderr to
// the currently running app. The command can be executed in a custom working directory by using the parameter
// `workingDir`. Terragrunt working directory will be assumed if empty string.
func RunShellCommandWithOutput(
	ctx context.Context,
	opts *options.TerragruntOptions,
	workingDir string,
	suppressStdout bool,
	needsPTY bool,
	command string,
	args ...string,
) (*util.CmdOutput, error) {
	if command == opts.TerraformPath {
		if fn := TerraformCommandHookFromContext(ctx); fn != nil {
			return fn(ctx, opts, args)
		}
	}

	var (
		output     = util.CmdOutput{}
		commandDir = workingDir
	)

	if workingDir == "" {
		commandDir = opts.WorkingDir
	}

	err := telemetry.Telemetry(ctx, opts, "run_"+command, map[string]interface{}{
		"command": command,
		"args":    fmt.Sprintf("%v", args),
		"dir":     commandDir,
	}, func(childCtx context.Context) error {
		opts.Logger.Debugf("Running command: %s %s", command, strings.Join(args, " "))

		var (
			outWriter = opts.Writer
			errWriter = opts.ErrWriter
		)

		if command == opts.TerraformPath && !opts.ForwardTFStdout {
			logger := opts.Logger.
				WithField(placeholders.TFPathKeyName, filepath.Base(opts.TerraformPath)).
				WithField(placeholders.TFCmdArgsKeyName, args).
				WithField(placeholders.TFCmdKeyName, cli.Args(args).CommandName())

			if opts.JSONLogFormat && !cli.Args(args).Normalize(cli.SingleDashFlag).Contains(terraform.FlagNameJSON) {
				outWriter = buildOutWriter(
					opts,
					logger,
					outWriter,
					errWriter,
				)

				errWriter = buildErrWriter(
					opts,
					logger,
					errWriter,
				)
			} else if !shouldForceForwardTFStdout(args) {
				outWriter = buildOutWriter(
					opts,
					logger,
					outWriter,
					errWriter,
					writer.WithMsgSeparator(logMsgSeparator),
				)

				errWriter = buildErrWriter(
					opts,
					logger,
					errWriter,
					writer.WithMsgSeparator(logMsgSeparator),
					writer.WithParseFunc(terraform.ParseLogFunc(tfLogMsgPrefix, false)),
				)
			}
		}

		var (
			cmdStderr = io.MultiWriter(errWriter, &output.Stderr)
			cmdStdout = io.MultiWriter(outWriter, &output.Stdout)
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

// buildOutWriter returns the writer for the command's stdout.
//
// When Terragrunt is running in Headless mode, we want to forward
// any stdout to the INFO log level, otherwise, we want to forward
// stdout to the STDOUT log level.
//
// Also accepts any additional writer options desired.
func buildOutWriter(opts *options.TerragruntOptions, logger log.Logger, outWriter, errWriter io.Writer, writerOptions ...writer.Option) io.Writer {
	logLevel := log.StdoutLevel

	if opts.Headless {
		logLevel = log.InfoLevel
		outWriter = errWriter
	}

	options := []writer.Option{
		writer.WithLogger(logger.WithOptions(log.WithOutput(outWriter))),
		writer.WithDefaultLevel(logLevel),
	}
	options = append(options, writerOptions...)

	return writer.New(options...)
}

// buildErrWriter returns the writer for the command's stderr.
//
// When Terragrunt is running in Headless mode, we want to forward
// any stderr to the ERROR log level, otherwise, we want to forward
// stderr to the STDERR log level.
//
// Also accepts any additional writer options desired.
func buildErrWriter(opts *options.TerragruntOptions, logger log.Logger, errWriter io.Writer, writerOptions ...writer.Option) io.Writer {
	logLevel := log.StderrLevel

	if opts.Headless {
		logLevel = log.ErrorLevel
	}

	options := []writer.Option{
		writer.WithLogger(logger.WithOptions(log.WithOutput(errWriter))),
		writer.WithDefaultLevel(logLevel),
	}
	options = append(options, writerOptions...)

	return writer.New(options...)
}

// isTerraformCommandThatNeedsPty returns true if the sub command of terraform we are running requires a pty.
func isTerraformCommandThatNeedsPty(args []string) (bool, error) {
	if len(args) == 0 || !util.ListContainsElement(terraformCommandsThatNeedPty, args[0]) {
		return false, nil
	}

	fi, err := os.Stdin.Stat()
	if err != nil {
		return false, errors.New(err)
	}

	// if there is data in the stdin, then the terraform console is used in non-interactive mode, for example `echo "1 + 5" | terragrunt console`.
	if fi.Size() > 0 {
		return false, nil
	}

	// if the stdin is not a terminal, then the terraform console is used in non-interactive mode
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return false, nil
	}

	return true, nil
}

// shouldForceForwardTFStdout returns true if at least one of the conditions is met, args contains the `-json` flag or the `output` or `state` command.
func shouldForceForwardTFStdout(args cli.Args) bool {
	tfCommands := []string{
		terraform.CommandNameOutput,
		terraform.CommandNameState,
		terraform.CommandNameVersion,
		terraform.CommandNameConsole,
	}

	tfFlags := []string{
		terraform.FlagNameJSON,
		terraform.FlagNameVersion,
		terraform.FlagNameHelpLong,
		terraform.FlagNameHelpShort,
	}

	for _, flag := range tfFlags {
		if args.Normalize(cli.SingleDashFlag).Contains(flag) {
			return true
		}
	}

	return collections.ListContainsElement(tfCommands, args.CommandName())
}
