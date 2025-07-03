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
func RunCommand(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, args ...string) error {
	_, err := RunCommandWithOutput(ctx, l, opts, args...)

	return err
}

// RunCommandWithOutput runs the given Terraform command, writing its stdout/stderr to the terminal AND returning stdout/stderr to this
// method's caller
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
