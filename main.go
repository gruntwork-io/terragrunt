package main

import (
	"os"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func main() {
	os.Exit(run())
}

func run() int {
	opts := options.NewTerragruntOptions()
	l := log.New(
		log.WithOutput(opts.Writers.ErrWriter),
		log.WithLevel(options.DefaultLogLevel),
		log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
	)

	reporter := log.NewPanicReporter()
	defer reporter.PanicHandler(l, opts.VersionString, os.Args)

	if err := global.NewLogLevelFlag(l, opts, nil).Parse(os.Args); err != nil {
		l.Errorf("An error has occurred: %v", err)
		return 1
	}

	return cli.NewApp(l, opts).RunWithExitCode(os.Args, tf.NewDetailedExitCodeMap(), reporter)
}
