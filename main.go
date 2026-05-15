package main

import (
	"context"
	"os"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func main() {
	exitCode := tf.NewDetailedExitCodeMap()
	opts := options.NewTerragruntOptions()

	l := log.New(
		log.WithOutput(opts.Writers.ErrWriter),
		log.WithLevel(options.DefaultLogLevel),
		log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
	)

	if err := global.NewLogLevelFlag(l, opts, nil).Parse(os.Args); err != nil {
		l.Errorf("An error has occurred: %v", err)
		os.Exit(1)
	}

	reporter := panicreport.New(versionString(opts))

	defer reporter.PanicHandler(l, os.Args)

	cliApp := cli.NewApp(l, opts)
	ctx := setupContext(l, exitCode)
	err := cliApp.RunContext(ctx, os.Args)

	checkForErrorsAndExit(l, finalExitCode(exitCode, opts), reporter)(err)
}

// Private helper functions

func versionString(opts *options.TerragruntOptions) string {
	if opts != nil && opts.TerragruntVersion != nil {
		return opts.TerragruntVersion.String()
	}

	return ""
}

func finalExitCode(exitCode *tf.DetailedExitCodeMap, opts *options.TerragruntOptions) int {
	if opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode) {
		return exitCode.GetFinalDetailedExitCode()
	}

	return exitCode.GetFinalExitCode()
}

func checkForErrorsAndExit(l log.Logger, exitCode int, reporter *panicreport.Reporter) func(error) {
	return func(err error) {
		if err == nil {
			os.Exit(exitCode)
		}

		// Some panics are caught downstream (notably cty wraps panics from
		// HCL function implementations as function.PanicError) and surface
		// here as a returned error. Route those through the same crash
		// report UX as panics caught by the deferred PanicHandler above.
		if panicreport.IsPanic(err) {
			reporter.ReportPanic(l, err.Error(), nil, os.Args)
			os.Exit(panicreport.ExitCode)
		}

		exitCoder, exitCodeErr := util.GetExitCode(err)
		if exitCodeErr != nil {
			exitCoder = 1
		}

		l.Error(err.Error())

		if errStack := errors.ErrorStack(err); errStack != "" {
			l.Trace(errStack)
		}

		if explain := shell.ExplainError(err); len(explain) > 0 {
			l.Errorf("Suggested fixes: \n%s", explain)
		}

		os.Exit(exitCoder)
	}
}

func setupContext(l log.Logger, exitCode *tf.DetailedExitCodeMap) context.Context {
	ctx := context.Background()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	return log.ContextWithLogger(ctx, l)
}
