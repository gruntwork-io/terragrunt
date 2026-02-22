package tf

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"slices"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	logwriter "github.com/gruntwork-io/terragrunt/pkg/log/writer"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/mattn/go-isatty"
)

const (
	// tfLogMsgPrefix is a message prefix that is prepended to each TF_LOG output lines when the output is integrated in TG log, for example:
	//
	// TF_LOG: using github.com/zclconf/go-cty v1.14.3
	// TF_LOG: Go runtime version: go1.22.1
	tfLogMsgPrefix = "TF_LOG: "

	logMsgSeparator = "\n"

	defaultWriterOptionsLen = 2
)

// Commands that implement a REPL need a pseudo TTY when run as a subprocess in order for the readline properties to be
// preserved. This is a list of terraform commands that have this property, which is used to determine if terragrunt
// should allocate a ptty when running that terraform command.
var commandsThatNeedPty = []string{
	CommandNameConsole,
}

// TFOptions contains the configuration needed to run TF commands.
type TFOptions struct {
	Writers      writer.Writers
	ShellOptions *shell.ShellOptions

	// TerragruntOptions is the full options struct. It is needed when the
	// TerraformCommandHook fires (e.g. for provider caching) because the hook
	// implementation requires full access to TerragruntOptions.
	// Callers that only need basic TF command execution may leave this nil,
	// as long as the TerraformCommandHook is not set in the context.
	TerragruntOptions *options.TerragruntOptions

	OriginalTerragruntConfigPath string
	JSONLogFormat                bool
}

// TFOptionsFromOpts constructs RunOptions from TerragruntOptions.
func TFOptionsFromOpts(opts *options.TerragruntOptions) *TFOptions {
	return &TFOptions{
		Writers:                      opts.Writers,
		JSONLogFormat:                opts.JSONLogFormat,
		OriginalTerragruntConfigPath: opts.OriginalTerragruntConfigPath,
		ShellOptions:                 shell.RunOptionsFromOpts(opts),
		TerragruntOptions:            opts,
	}
}

// RunCommand runs the given Terraform command.
func RunCommand(ctx context.Context, l log.Logger, runOpts *TFOptions, args ...string) error {
	_, err := RunCommandWithOutput(ctx, l, runOpts, args...)

	return err
}

// RunCommandWithOutput runs the given Terraform command, writing its stdout/stderr to the terminal AND returning stdout/stderr to this
// method's caller
func RunCommandWithOutput(ctx context.Context, l log.Logger, runOpts *TFOptions, args ...string) (*util.CmdOutput, error) {
	args = clihelper.Args(args).Normalize(clihelper.SingleDashFlag)

	if fn := TerraformCommandHookFromContext(ctx); fn != nil {
		return fn(ctx, l, runOpts.TerragruntOptions, args)
	}

	needsPTY, err := isCommandThatNeedsPty(args)
	if err != nil {
		return nil, err
	}

	shellOpts := runOpts.ShellOptions
	if !runOpts.ShellOptions.ForwardTFStdout {
		// Copy the shell opts to avoid mutating the caller's struct.
		shellOptsCopy := *shellOpts
		shellOpts = &shellOptsCopy

		outWriter, errWriter := logTFOutput(l, runOpts, args)
		shellOpts.Writers.Writer = outWriter
		shellOpts.Writers.ErrWriter = errWriter
	}

	output, err := shell.RunCommandWithOutput(ctx, l, shellOpts, "", false, needsPTY, runOpts.ShellOptions.TFPath, args...)

	hasDetailedExitCode := slices.Contains(args, FlagNameDetailedExitCode)
	if hasDetailedExitCode {
		code := 0

		if err != nil {
			code, _ = util.GetExitCode(err)
		}

		if exitCode := DetailedExitCodeFromContext(ctx); exitCode != nil {
			exitCode.Set(filepath.Dir(runOpts.OriginalTerragruntConfigPath), code)
		}

		if code != 1 {
			return output, nil
		}
	}

	return output, err
}

func logTFOutput(l log.Logger, runOpts *TFOptions, args clihelper.Args) (io.Writer, io.Writer) {
	var (
		originalOutWriter           = writer.NewOriginalWriter(runOpts.Writers.Writer)
		originalErrWriter           = writer.NewOriginalWriter(runOpts.Writers.ErrWriter)
		outWriter         io.Writer = originalOutWriter
		errWriter         io.Writer = originalErrWriter
	)

	logger := l.
		WithField(placeholders.TFPathKeyName, filepath.Base(runOpts.ShellOptions.TFPath)).
		WithField(placeholders.TFCmdArgsKeyName, args.Slice()).
		WithField(placeholders.TFCmdKeyName, args.CommandName())

	if runOpts.JSONLogFormat && !args.Normalize(clihelper.SingleDashFlag).Contains(FlagNameJSON) {
		wrappedOut := buildOutWriter(
			logger,
			runOpts.ShellOptions.Headless,
			outWriter,
			errWriter,
		)
		wrappedErr := buildErrWriter(
			logger,
			runOpts.ShellOptions.Headless,
			errWriter,
		)

		outWriter = writer.NewWrappedWriter(wrappedOut, originalOutWriter)
		errWriter = writer.NewWrappedWriter(wrappedErr, originalErrWriter)
	} else if !shouldForceForwardTFStdout(args) {
		wrappedOut := buildOutWriter(
			logger,
			runOpts.ShellOptions.Headless,
			outWriter,
			errWriter,
			logwriter.WithMsgSeparator(logMsgSeparator),
		)
		wrappedErr := buildErrWriter(
			logger,
			runOpts.ShellOptions.Headless,
			errWriter,
			logwriter.WithMsgSeparator(logMsgSeparator),
			logwriter.WithParseFunc(ParseLogFunc(tfLogMsgPrefix, false)),
		)

		outWriter = writer.NewWrappedWriter(wrappedOut, originalOutWriter)
		errWriter = writer.NewWrappedWriter(wrappedErr, originalErrWriter)
	}

	return outWriter, errWriter
}

// isCommandThatNeedsPty returns true if the sub command of terraform we are running requires a pty.
func isCommandThatNeedsPty(args []string) (bool, error) {
	if len(args) == 0 || !slices.Contains(commandsThatNeedPty, args[0]) {
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
func shouldForceForwardTFStdout(args clihelper.Args) bool {
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

	if slices.ContainsFunc(tfFlags, args.Normalize(clihelper.SingleDashFlag).Contains) {
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
func buildOutWriter(l log.Logger, headless bool, outWriter, errWriter io.Writer, writerOptions ...logwriter.Option) io.Writer {
	logLevel := log.StdoutLevel

	if headless {
		logLevel = log.InfoLevel
		outWriter = errWriter
	}

	opts := make([]logwriter.Option, 0, defaultWriterOptionsLen+len(writerOptions))
	opts = append(opts,
		logwriter.WithLogger(l.WithOptions(log.WithOutput(outWriter))),
		logwriter.WithDefaultLevel(logLevel),
	)
	opts = append(opts, writerOptions...)

	return logwriter.New(opts...)
}

// buildErrWriter returns the writer for the command's stderr.
//
// When Terragrunt is running in Headless mode, we want to forward
// any stderr to the ERROR log level, otherwise, we want to forward
// stderr to the STDERR log level.
//
// Also accepts any additional writer options desired.
func buildErrWriter(l log.Logger, headless bool, errWriter io.Writer, writerOptions ...logwriter.Option) io.Writer {
	logLevel := log.StderrLevel

	if headless {
		logLevel = log.ErrorLevel
	}

	opts := make([]logwriter.Option, 0, defaultWriterOptionsLen+len(writerOptions))
	opts = append(opts,
		logwriter.WithLogger(l.WithOptions(log.WithOutput(errWriter))),
		logwriter.WithDefaultLevel(logLevel),
	)
	opts = append(opts, writerOptions...)

	return logwriter.New(opts...)
}
