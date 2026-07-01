package main

import (
	"os"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/version"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func main() {
	os.Exit(run())
}

func run() (code int) {
	// Restore the parent shell's console mode on the way out. On Windows, PrepareConsole
	// enables virtual terminal input/processing on the console Terragrunt shares with the
	// parent shell; without restoring it, shells such as Nushell are left rendering arrow
	// keys as raw escape sequences. No-op on non-Windows platforms.
	originalConsole := exec.SaveConsoleState()
	defer originalConsole.Restore()

	opts := options.NewTerragruntOptions()
	l := log.New(
		log.WithOutput(opts.Writers.ErrWriter),
		log.WithLevel(options.DefaultLogLevel),
		log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
	)

	reporter := panicreport.New()
	// Recover panics here so main owns os.Exit and any future main-level defers still run.
	defer func() {
		if reporter.PanicHandler(recover(), l, version.GetVersion, os.Args) {
			code = 1
		}
	}()

	if err := global.NewLogLevelFlag(l, opts, nil).Parse(os.Args); err != nil {
		l.Errorf("An error has occurred: %v", err)
		return 1
	}

	return cli.NewApp(l, opts, venv.OSVenv()).RunWithExitCode(os.Args, tf.NewDetailedExitCodeMap(), reporter)
}
