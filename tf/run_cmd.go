// Package tf provides functions for running Terraform/OpenTofu commands.
//
// # TFOptions - Dedicated Options Type
//
// This package uses TFOptions, a dedicated options type that contains only
// the fields needed for terraform operations. This provides clean separation
// from TerragruntOptions and RunnerOptions.
//
// Preferred API (TFOptions-based):
//
//	tfOpts := &tf.TFOptions{
//	    TFPath:                  "/usr/bin/terraform",
//	    TerraformCliArgs:        []string{"apply"},
//	    WorkingDir:              "/path/to/dir",
//	    Writer:                  os.Stdout,
//	    ErrWriter:               os.Stderr,
//	    Env:                     map[string]string{},
//	    TerraformImplementation: options.TerraformImpl,
//	}
//	err := tf.RunCommandWithOptions(ctx, l, tfOpts, args...)
//
// # Migration Guide
//
// Three generations of functions exist:
//
//  1. Old (deprecated): RunCommand/RunCommandWithOutput - uses TerragruntOptions
//  2. Transitional (deprecated): RunCommandWithRunner/RunCommandWithOutputAndRunner - uses RunnerOptions
//  3. New (preferred): RunCommandWithOptions/RunCommandWithOutputAndOptions - uses TFOptions
//
// Migration examples:
//
//	// From TerragruntOptions (deprecated):
//	err := tf.RunCommand(ctx, l, opts, args...)
//
//	// From RunnerOptions (deprecated):
//	runnerOpts := runnertypes.FromTerragruntOptions(opts)
//	err := tf.RunCommandWithRunner(ctx, l, runnerOpts, args...)
//
//	// To TFOptions (preferred):
//	tfOpts := toTFOptions(runnerOpts)
//	err := tf.RunCommandWithOptions(ctx, l, tfOpts, args...)
//
// The old functions are maintained for backward compatibility but will be removed
// in a future release.
package tf

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"slices"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	runnertypes "github.com/gruntwork-io/terragrunt/internal/runner/types"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/log/writer"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/mattn/go-isatty"
)

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
var commandsThatNeedPty = []string{
	CommandNameConsole,
}

// RunCommand runs the given Terraform command.
//
// Deprecated: Use RunCommandWithRunner instead, which accepts RunnerOptions.
// This function is maintained for backward compatibility with external callers.
func RunCommand(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, args ...string) error {
	_, err := RunCommandWithOutput(ctx, l, opts, args...)

	return err
}

// RunCommandWithOutput runs the given Terraform command, writing its stdout/stderr to the terminal AND returning stdout/stderr to this
// method's caller.
//
// Deprecated: Use RunCommandWithOutputAndRunner instead, which accepts RunnerOptions.
// This function is maintained for backward compatibility with external callers.
func RunCommandWithOutput(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, args ...string) (*util.CmdOutput, error) {
	args = cli.Args(args).Normalize(cli.SingleDashFlag)

	if fn := TerraformCommandHookFromContext(ctx); fn != nil {
		return fn(ctx, l, opts, args)
	}

	needsPTY, err := isCommandThatNeedsPty(args)
	if err != nil {
		return nil, err
	}

	if !opts.ForwardTFStdout {
		opts = opts.Clone()
		opts.Writer, opts.ErrWriter = logTFOutput(l, opts, args)
	}

	output, err := shell.RunCommandWithOutput(ctx, l, opts, "", false, needsPTY, opts.TFPath, args...)

	if err != nil && util.ListContainsElement(args, FlagNameDetailedExitCode) {
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

//nolint:dupl // Intentional duplication for backward compatibility during migration
func logTFOutput(l log.Logger, opts *options.TerragruntOptions, args cli.Args) (io.Writer, io.Writer) {
	var (
		outWriter = opts.Writer
		errWriter = opts.ErrWriter
	)

	logger := l.
		WithField(placeholders.TFPathKeyName, filepath.Base(opts.TFPath)).
		WithField(placeholders.TFCmdArgsKeyName, args.Slice()).
		WithField(placeholders.TFCmdKeyName, args.CommandName())

	if opts.JSONLogFormat && !args.Normalize(cli.SingleDashFlag).Contains(FlagNameJSON) {
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
			writer.WithParseFunc(ParseLogFunc(tfLogMsgPrefix, false)),
		)
	}

	return outWriter, errWriter
}

// isCommandThatNeedsPty returns true if the sub command of terraform we are running requires a pty.
func isCommandThatNeedsPty(args []string) (bool, error) {
	if len(args) == 0 || !util.ListContainsElement(commandsThatNeedPty, args[0]) {
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
		CommandNameOutput,
		CommandNameState,
		CommandNameVersion,
		CommandNameConsole,
		CommandNameGraph,
	}

	tfFlags := []string{
		FlagNameJSON,
		FlagNameVersion,
		FlagNameHelpLong,
		FlagNameHelpShort,
	}

	if slices.ContainsFunc(tfFlags, args.Normalize(cli.SingleDashFlag).Contains) {
		return true
	}

	return collections.ListContainsElement(tfCommands, args.CommandName())
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

// RunCommandWithRunner runs the given Terraform command using RunnerOptions.
//
// Deprecated: Use RunCommandWithOptions instead, which accepts TFOptions.
// This transitional function accepts RunnerOptions but tf package now has its own options.
func RunCommandWithRunner(ctx context.Context, l log.Logger, runnerOpts *runnertypes.RunnerOptions, args ...string) error {
	_, err := RunCommandWithOutputAndRunner(ctx, l, runnerOpts, args...)
	return err
}

// RunCommandWithOutputAndRunner runs the given Terraform command using RunnerOptions,
// writing its stdout/stderr to the terminal AND returning stdout/stderr to this method's caller.
//
// Deprecated: Use RunCommandWithOutputAndOptions instead, which accepts TFOptions.
// This transitional function accepts RunnerOptions but tf package now has its own options.
func RunCommandWithOutputAndRunner(ctx context.Context, l log.Logger, runnerOpts *runnertypes.RunnerOptions, args ...string) (*util.CmdOutput, error) {
	args = cli.Args(args).Normalize(cli.SingleDashFlag)

	if fn := TerraformCommandHookFromContext(ctx); fn != nil {
		// Hook still expects TerragruntOptions, create minimal opts
		opts := &options.TerragruntOptions{
			TFPath:                  runnerOpts.TFPath,
			WorkingDir:              runnerOpts.WorkingDir,
			Writer:                  runnerOpts.Writer,
			ErrWriter:               runnerOpts.ErrWriter,
			Env:                     runnerOpts.Env,
			ForwardTFStdout:         runnerOpts.ForwardTFStdout,
			JSONLogFormat:           runnerOpts.JSONLogFormat,
			Headless:                runnerOpts.Headless,
			TerraformImplementation: runnerOpts.TerraformImplementation,
		}

		return fn(ctx, l, opts, args)
	}

	needsPTY, err := isCommandThatNeedsPty(args)
	if err != nil {
		return nil, err
	}

	if !runnerOpts.ForwardTFStdout {
		// Clone runnerOpts to avoid mutating the original
		runnerOpts = runnerOpts.Clone()
		runnerOpts.Writer, runnerOpts.ErrWriter = logTFOutputWithRunner(l, runnerOpts, args)
	}

	output, err := shell.RunCommandWithOutputAndRunner(ctx, l, runnerOpts, "", false, needsPTY, runnerOpts.TFPath, args...)

	if err != nil && util.ListContainsElement(args, FlagNameDetailedExitCode) {
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

// logTFOutputWithRunner configures log writers for TF output using RunnerOptions.
//
//nolint:dupl // Intentional duplication for backward compatibility during migration
func logTFOutputWithRunner(l log.Logger, runnerOpts *runnertypes.RunnerOptions, args cli.Args) (io.Writer, io.Writer) {
	var (
		outWriter = runnerOpts.Writer
		errWriter = runnerOpts.ErrWriter
	)

	logger := l.
		WithField(placeholders.TFPathKeyName, filepath.Base(runnerOpts.TFPath)).
		WithField(placeholders.TFCmdArgsKeyName, args.Slice()).
		WithField(placeholders.TFCmdKeyName, args.CommandName())

	if runnerOpts.JSONLogFormat && !args.Normalize(cli.SingleDashFlag).Contains(FlagNameJSON) {
		outWriter = buildOutWriterWithRunner(
			runnerOpts,
			logger,
			outWriter,
			errWriter,
		)

		errWriter = buildErrWriterWithRunner(
			runnerOpts,
			logger,
			errWriter,
		)
	} else if !shouldForceForwardTFStdout(args) {
		outWriter = buildOutWriterWithRunner(
			runnerOpts,
			logger,
			outWriter,
			errWriter,
			writer.WithMsgSeparator(logMsgSeparator),
		)

		errWriter = buildErrWriterWithRunner(
			runnerOpts,
			logger,
			errWriter,
			writer.WithMsgSeparator(logMsgSeparator),
			writer.WithParseFunc(ParseLogFunc(tfLogMsgPrefix, false)),
		)
	}

	return outWriter, errWriter
}

// buildOutWriterWithRunner returns the writer for the command's stdout using RunnerOptions.
func buildOutWriterWithRunner(runnerOpts *runnertypes.RunnerOptions, logger log.Logger, outWriter, errWriter io.Writer, writerOptions ...writer.Option) io.Writer {
	logLevel := log.StdoutLevel

	if runnerOpts.Headless {
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

// buildErrWriterWithRunner returns the writer for the command's stderr using RunnerOptions.
func buildErrWriterWithRunner(runnerOpts *runnertypes.RunnerOptions, logger log.Logger, errWriter io.Writer, writerOptions ...writer.Option) io.Writer {
	logLevel := log.StderrLevel

	if runnerOpts.Headless {
		logLevel = log.ErrorLevel
	}

	options := []writer.Option{
		writer.WithLogger(logger.WithOptions(log.WithOutput(errWriter))),
		writer.WithDefaultLevel(logLevel),
	}
	options = append(options, writerOptions...)

	return writer.New(options...)
}

// RunCommandWithOptions runs the given Terraform command using TFOptions.
// This is the preferred API for new code.
func RunCommandWithOptions(ctx context.Context, l log.Logger, tfOpts *TFOptions, args ...string) error {
	_, err := RunCommandWithOutputAndOptions(ctx, l, tfOpts, args...)
	return err
}

// RunCommandWithOutputAndOptions runs the given Terraform command using TFOptions,
// writing its stdout/stderr to the terminal AND returning stdout/stderr to this method's caller.
// This is the preferred API for new code.
func RunCommandWithOutputAndOptions(ctx context.Context, l log.Logger, tfOpts *TFOptions, args ...string) (*util.CmdOutput, error) {
	args = cli.Args(args).Normalize(cli.SingleDashFlag)

	if fn := TerraformCommandHookFromContext(ctx); fn != nil {
		// Hook still expects TerragruntOptions, create minimal opts
		opts := &options.TerragruntOptions{
			TFPath:                  tfOpts.TFPath,
			WorkingDir:              tfOpts.WorkingDir,
			Writer:                  tfOpts.Writer,
			ErrWriter:               tfOpts.ErrWriter,
			Env:                     tfOpts.Env,
			ForwardTFStdout:         tfOpts.ForwardTFStdout,
			JSONLogFormat:           tfOpts.JSONLogFormat,
			Headless:                tfOpts.Headless,
			TerraformImplementation: tfOpts.TerraformImplementation,
		}

		return fn(ctx, l, opts, args)
	}

	needsPTY, err := isCommandThatNeedsPty(args)
	if err != nil {
		return nil, err
	}

	if !tfOpts.ForwardTFStdout {
		// Clone tfOpts to avoid mutating the original
		tfOpts = tfOpts.Clone()
		tfOpts.Writer, tfOpts.ErrWriter = logTFOutputWithTFOptions(l, tfOpts, args)
	}

	// Convert TFOptions to RunnerOptions for shell package call
	// Shell package doesn't need to know about TFOptions
	runnerOpts := toRunnerOptions(tfOpts)

	output, err := shell.RunCommandWithOutputAndRunner(ctx, l, runnerOpts, "", false, needsPTY, tfOpts.TFPath, args...)

	if err != nil && util.ListContainsElement(args, FlagNameDetailedExitCode) {
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

// toRunnerOptions converts TFOptions to RunnerOptions for shell package calls.
// This is an internal conversion within tf package to call shell functions.
func toRunnerOptions(tfOpts *TFOptions) *runnertypes.RunnerOptions {
	if tfOpts == nil {
		return nil
	}

	return &runnertypes.RunnerOptions{
		TFPath:                  tfOpts.TFPath,
		TFPathExplicitlySet:     tfOpts.TFPathExplicitlySet,
		TerraformImplementation: tfOpts.TerraformImplementation,
		TerraformCliArgs:        tfOpts.TerraformCliArgs,
		WorkingDir:              tfOpts.WorkingDir,
		TerragruntConfigPath:    tfOpts.TerragruntConfigPath,
		DownloadDir:             tfOpts.DownloadDir,
		Writer:                  tfOpts.Writer,
		ErrWriter:               tfOpts.ErrWriter,
		Env:                     tfOpts.Env,
		ForwardTFStdout:         tfOpts.ForwardTFStdout,
		JSONLogFormat:           tfOpts.JSONLogFormat,
		Headless:                tfOpts.Headless,
		LogDisableErrorSummary:  tfOpts.LogDisableErrorSummary,
		Engine:                  tfOpts.Engine,
		EngineEnabled:           tfOpts.EngineEnabled,
		Telemetry:               tfOpts.Telemetry,
	}
}

// logTFOutputWithTFOptions configures log writers for TF output using TFOptions.
//
//nolint:dupl // Intentional duplication for backward compatibility during migration
func logTFOutputWithTFOptions(l log.Logger, tfOpts *TFOptions, args cli.Args) (io.Writer, io.Writer) {
	var (
		outWriter = tfOpts.Writer
		errWriter = tfOpts.ErrWriter
	)

	logger := l.
		WithField(placeholders.TFPathKeyName, filepath.Base(tfOpts.TFPath)).
		WithField(placeholders.TFCmdArgsKeyName, args.Slice()).
		WithField(placeholders.TFCmdKeyName, args.CommandName())

	if tfOpts.JSONLogFormat && !args.Normalize(cli.SingleDashFlag).Contains(FlagNameJSON) {
		outWriter = buildOutWriterWithTFOptions(
			tfOpts,
			logger,
			outWriter,
			errWriter,
		)

		errWriter = buildErrWriterWithTFOptions(
			tfOpts,
			logger,
			errWriter,
		)
	} else if !shouldForceForwardTFStdout(args) {
		outWriter = buildOutWriterWithTFOptions(
			tfOpts,
			logger,
			outWriter,
			errWriter,
			writer.WithMsgSeparator(logMsgSeparator),
		)

		errWriter = buildErrWriterWithTFOptions(
			tfOpts,
			logger,
			errWriter,
			writer.WithMsgSeparator(logMsgSeparator),
			writer.WithParseFunc(ParseLogFunc(tfLogMsgPrefix, false)),
		)
	}

	return outWriter, errWriter
}

// buildOutWriterWithTFOptions returns the writer for the command's stdout using TFOptions.
func buildOutWriterWithTFOptions(tfOpts *TFOptions, logger log.Logger, outWriter, errWriter io.Writer, writerOptions ...writer.Option) io.Writer {
	logLevel := log.StdoutLevel

	if tfOpts.Headless {
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

// buildErrWriterWithTFOptions returns the writer for the command's stderr using TFOptions.
func buildErrWriterWithTFOptions(tfOpts *TFOptions, logger log.Logger, errWriter io.Writer, writerOptions ...writer.Option) io.Writer {
	logLevel := log.StderrLevel

	if tfOpts.Headless {
		logLevel = log.ErrorLevel
	}

	options := []writer.Option{
		writer.WithLogger(logger.WithOptions(log.WithOutput(errWriter))),
		writer.WithDefaultLevel(logLevel),
	}
	options = append(options, writerOptions...)

	return writer.New(options...)
}
