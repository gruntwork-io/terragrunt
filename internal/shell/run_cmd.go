// Package shell provides functions to run shell commands and Terraform commands.
package shell

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
)

// SignalForwardingDelay is the time to wait before forwarding the signal to the subcommand.
//
// The signal can be sent to the main process (only `terragrunt`) as well as the process group (`terragrunt` and `terraform`), for example:
// kill -INT <pid>  # sends SIGINT only to the main process
// kill -INT -<pid> # sends SIGINT to the process group
// Since we cannot know how the signal is sent, we should give `tofu`/`terraform` time to gracefully exit
// if it receives the signal directly from the shell, to avoid sending the second interrupt signal to `tofu`/`terraform`.
const SignalForwardingDelay = time.Second * 15

// ShellOptions contains the per-invocation configuration needed to run shell
// commands. The shell environment and stdout/stderr writers travel separately
// via the venv parameter; they are not stored here.
type ShellOptions struct {
	EngineOptions *engine.EngineOptions
	EngineConfig  *engine.EngineConfig
	Telemetry     *telemetry.Options

	RootWorkingDir         string
	WorkingDir             string
	TFPath                 string
	Experiments            experiment.Experiments
	Headless               bool
	ForwardTFStdout        bool
	LogShowAbsPaths        bool
	LogDisableErrorSummary bool
}

// NewShellOptions creates ShellOptions with sensible defaults. Telemetry is
// always non-nil; TRACEPARENT is seeded from env when present. Use the With*
// methods to override any field.
//
// env carries the venv-mediated shell environment; callers pass
// [github.com/gruntwork-io/terragrunt/internal/venv.Venv.Env] so trace context
// propagation stays consistent with the rest of the venv-mediated I/O.
func NewShellOptions(env map[string]string) *ShellOptions {
	opts := &ShellOptions{
		Telemetry: &telemetry.Options{},
	}

	if tp := env[telemetry.TraceParentEnv]; tp != "" {
		opts.Telemetry.TraceParent = tp
	}

	return opts
}

// WithWorkingDir sets the working directory for command execution.
func (o *ShellOptions) WithWorkingDir(dir string) *ShellOptions {
	o.WorkingDir = dir

	return o
}

// SetTraceParent explicitly overrides the TRACEPARENT value used for trace context propagation.
func (o *ShellOptions) SetTraceParent(tp string) *ShellOptions {
	if o.Telemetry == nil {
		o.Telemetry = &telemetry.Options{}
	}

	o.Telemetry.TraceParent = tp

	return o
}

// WithTelemetry sets the full telemetry options, replacing the defaults from the constructor.
func (o *ShellOptions) WithTelemetry(t *telemetry.Options) *ShellOptions {
	if t != nil {
		o.Telemetry = t
	}

	return o
}

// WithEngine sets the engine configuration and options.
func (o *ShellOptions) WithEngine(cfg *engine.EngineConfig, opts *engine.EngineOptions) *ShellOptions {
	o.EngineConfig = cfg
	o.EngineOptions = opts

	return o
}

// WithTFPath sets the path to the Terraform/OpenTofu binary.
func (o *ShellOptions) WithTFPath(path string) *ShellOptions {
	o.TFPath = path

	return o
}

// WithRootWorkingDir sets the root working directory used in error messages.
func (o *ShellOptions) WithRootWorkingDir(dir string) *ShellOptions {
	o.RootWorkingDir = dir

	return o
}

// WithExperiments sets the active experiments.
func (o *ShellOptions) WithExperiments(exp experiment.Experiments) *ShellOptions {
	o.Experiments = exp

	return o
}

// WithHeadless sets the headless mode flag.
func (o *ShellOptions) WithHeadless(h bool) *ShellOptions {
	o.Headless = h

	return o
}

// WithForwardTFStdout sets the flag to forward TF stdout.
func (o *ShellOptions) WithForwardTFStdout(f bool) *ShellOptions {
	o.ForwardTFStdout = f

	return o
}

// NoEngine returns true if the user explicitly disabled the engine via --no-engine.
// Returns false when EngineOptions is nil (default: don't disable), letting the
// other guards (EngineConfig != nil, experiment enabled) decide whether to run.
func (o *ShellOptions) NoEngine() bool {
	return o.EngineOptions != nil && o.EngineOptions.NoEngine
}

// RunCommand runs the given shell command. The shell environment and
// stdout/stderr writers come from v; tests can substitute a venv whose Exec
// is a [vexec.NewMemExec] so external binaries are never forked.
func RunCommand(ctx context.Context, l log.Logger, v *venv.Venv, runOpts *ShellOptions, command string, args ...string) error {
	_, err := RunCommandWithOutput(ctx, l, v, runOpts, "", false, false, command, args...)

	return err
}

// RunCommandWithOutput runs the specified shell command and captures
// stdout/stderr in addition to streaming. The command can be executed in a
// custom working directory by using the parameter `workingDir`; the
// ShellOptions working directory is assumed if empty.
func RunCommandWithOutput(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	runOpts *ShellOptions,
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
		commandDir = runOpts.WorkingDir
	}

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "run_"+filepath.Base(command), map[string]any{
		"binary":      filepath.Base(command),
		"binary_path": command,
		"args":        fmt.Sprintf("%v", args),
		"dir":         commandDir,
	}, func(ctx context.Context) error {
		runErr := runCommand(ctx, l, v, runOpts, RunCommandOptions{
			CommandDir:     commandDir,
			SuppressStdout: suppressStdout,
			NeedsPTY:       needsPTY,
			Command:        command,
			Args:           args,
			Output:         &output,
		})

		if span := trace.SpanFromContext(ctx); span.IsRecording() {
			exitCode := 0

			if runErr != nil {
				if code, codeErr := util.GetExitCode(runErr); codeErr == nil {
					exitCode = code
				} else {
					exitCode = -1
				}
			}

			span.SetAttributes(
				attribute.Int("exit_code", exitCode),
				attribute.Int("stdout_bytes", output.Stdout.Len()),
				attribute.Int("stderr_bytes", output.Stderr.Len()),
			)
		}

		return runErr
	})

	return &output, err
}

// RunCommandOptions groups the per-invocation parameters for runCommand,
// keeping the function signature short and call sites readable.
type RunCommandOptions struct {
	Output         *util.CmdOutput
	CommandDir     string
	Command        string
	Args           []string
	SuppressStdout bool
	NeedsPTY       bool
}

// runCommand contains the actual subprocess execution logic, separated to keep
// RunCommandWithOutput focused on telemetry framing.
func runCommand(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	runOpts *ShellOptions,
	cmdOpts RunCommandOptions,
) error {
	l.Debugf("Running command: %s %s", cmdOpts.Command, strings.Join(cmdOpts.Args, " "))

	// Default nil venv writers to io.Discard so callers that construct a
	// venv without explicit writers (most test setups) don't trip
	// io.MultiWriter on the nil entry.
	outWriter := v.Writers.Writer
	if outWriter == nil {
		outWriter = io.Discard
	}

	errWriter := v.Writers.ErrWriter
	if errWriter == nil {
		errWriter = io.Discard
	}

	var (
		cmdStderr = io.MultiWriter(errWriter, &cmdOpts.Output.Stderr)
		cmdStdout = io.MultiWriter(outWriter, &cmdOpts.Output.Stdout)
	)

	// Pass the traceparent to the child process if it is available in the context.
	if traceParent := telemetry.TraceParentFromContext(ctx, runOpts.Telemetry); traceParent != "" {
		l.Debugf("Setting trace parent=%q for command %s", traceParent, fmt.Sprintf("%s %v", cmdOpts.Command, cmdOpts.Args))
		v.Env[telemetry.TraceParentEnv] = traceParent
	}

	if cmdOpts.SuppressStdout {
		l.Debugf("Command output will be suppressed.")

		cmdStdout = io.MultiWriter(&cmdOpts.Output.Stdout)
	}

	if cmdOpts.Command == runOpts.TFPath {
		// If the engine is enabled and the command is IaC executable, use the engine to run the command.
		if runOpts.EngineConfig != nil && runOpts.Experiments.Evaluate(experiment.IacEngine) && !runOpts.NoEngine() {
			l.Debugf("Using engine to run command: %s %s", cmdOpts.Command, strings.Join(cmdOpts.Args, " "))

			cmdOutput, err := engine.Run(ctx, l, v.Exec, &engine.ExecutionOptions{
				Writers: writer.Writers{
					Writer:    writer.NewWrappedWriter(cmdStdout, v.Writers.Writer),
					ErrWriter: writer.NewWrappedWriter(cmdStderr, v.Writers.ErrWriter),
				},
				LogShowAbsPaths:        runOpts.LogShowAbsPaths,
				LogDisableErrorSummary: runOpts.LogDisableErrorSummary,
				EngineOptions:          runOpts.EngineOptions,
				EngineConfig:           runOpts.EngineConfig,
				Env:                    v.Env,
				WorkingDir:             cmdOpts.CommandDir,
				RootWorkingDir:         runOpts.RootWorkingDir,
				Command:                cmdOpts.Command,
				Args:                   cmdOpts.Args,
				Headless:               runOpts.Headless,
				ForwardTFStdout:        runOpts.ForwardTFStdout,
				SuppressStdout:         cmdOpts.SuppressStdout,
				AllocatePseudoTty:      cmdOpts.NeedsPTY,
			})
			if err != nil {
				return errors.New(err)
			}

			*cmdOpts.Output = *cmdOutput

			return err
		}
	}

	cmd := exec.Command(ctx, v.Exec, cmdOpts.Command, cmdOpts.Args...)
	cmd.SetDir(cmdOpts.CommandDir)
	cmd.SetStdout(cmdStdout)
	cmd.SetStderr(cmdStderr)
	cmd.Configure(
		exec.WithUsePTY(cmdOpts.NeedsPTY),
		exec.WithEnv(v.Env),
		exec.WithForwardSignalDelay(SignalForwardingDelay),
	)

	// Save/restore console mode around subprocess — Windows subprocesses can reset it.
	savedConsole := exec.SaveConsoleState()
	defer savedConsole.Restore()

	if err := cmd.Start(l); err != nil { //nolint:contextcheck // context already passed to exec.Command
		err = util.ProcessExecutionError{
			Err:             err,
			Args:            cmdOpts.Args,
			Command:         cmdOpts.Command,
			WorkingDir:      cmd.Dir(),
			RootWorkingDir:  runOpts.RootWorkingDir,
			LogShowAbsPaths: runOpts.LogShowAbsPaths,
			DisableSummary:  runOpts.LogDisableErrorSummary,
		}

		return errors.New(err)
	}

	cancelShutdown := cmd.RegisterGracefullyShutdown(ctx, l)
	defer cancelShutdown()

	if err := cmd.Wait(); err != nil {
		err = util.ProcessExecutionError{
			Err:             err,
			Args:            cmdOpts.Args,
			Command:         cmdOpts.Command,
			Output:          *cmdOpts.Output,
			WorkingDir:      cmd.Dir(),
			RootWorkingDir:  runOpts.RootWorkingDir,
			LogShowAbsPaths: runOpts.LogShowAbsPaths,
			DisableSummary:  runOpts.LogDisableErrorSummary,
		}

		return errors.New(err)
	}

	return nil
}
