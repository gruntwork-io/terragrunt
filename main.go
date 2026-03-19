package main

import (
	"context"
	"fmt"
	"os"
	"runtime/pprof"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

var cpuProfileCleanup func()

func stopCPUProfileIfRunning() {
	if cpuProfileCleanup != nil {
		cpuProfileCleanup()
		cpuProfileCleanup = nil
	}
}

// The main entrypoint for Terragrunt
func main() {
	exitCode := tf.NewDetailedExitCodeMap()

	opts := options.NewTerragruntOptions()

	l := log.New(
		log.WithOutput(opts.Writers.ErrWriter),
		log.WithLevel(options.DefaultLogLevel),
		log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
	)

	// Immediately parse the `TG_LOG_LEVEL` environment variable, e.g. to set the TRACE level.
	if err := global.NewLogLevelFlag(l, opts, nil).Parse(os.Args); err != nil {
		l.Error(err.Error())
		os.Exit(1)
	}

	// Start CPU profiling if TG_CPU_PROFILE is set
	if cpuProfile := os.Getenv(tf.EnvNameTGCPUProfile); cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err != nil {
			l.Error(fmt.Sprintf("Could not create CPU profile: %v", err))
			os.Exit(1)
		}

		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			l.Error(fmt.Sprintf("Could not start CPU profile: %v", err))
			os.Exit(1)
		}

		cpuProfileCleanup = func() {
			pprof.StopCPUProfile()
			f.Close()
		}
	}

	defer func() {
		if opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode) {
			errors.Recover(checkForErrorsAndExit(l, exitCode.GetFinalDetailedExitCode()))
			return
		}

		errors.Recover(checkForErrorsAndExit(l, exitCode.GetFinalExitCode()))
	}()

	app := cli.NewApp(l, opts)

	ctx := setupContext(l, exitCode)
	err := app.RunContext(ctx, os.Args)

	if opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode) {
		checkForErrorsAndExit(l, exitCode.GetFinalDetailedExitCode())(err)

		return
	}

	checkForErrorsAndExit(l, exitCode.GetFinalExitCode())(err)
}

// If there is an error, display it in the console and exit with a non-zero exit code. Otherwise, exit 0.
func checkForErrorsAndExit(l log.Logger, exitCode int) func(error) {
	return func(err error) {
		stopCPUProfileIfRunning()

		if err == nil {
			os.Exit(exitCode)
		}

		l.Error(err.Error())

		if errStack := errors.ErrorStack(err); errStack != "" {
			l.Trace(errStack)
		}

		// exit with the underlying error code
		exitCoder, exitCodeErr := util.GetExitCode(err)
		if exitCodeErr != nil {
			exitCoder = 1
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
