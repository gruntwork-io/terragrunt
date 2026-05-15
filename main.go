package main

import (
	"os"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func main() {
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

	reporter := panicreport.New(opts.VersionString())
	defer reporter.PanicHandler(l, os.Args)

	cli.NewApp(l, opts).RunAndExit(os.Args, tf.NewDetailedExitCodeMap(), reporter)
}
