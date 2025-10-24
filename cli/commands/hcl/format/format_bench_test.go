package format_test

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	logformat "github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terratest/modules/files"
)

func BenchmarkFormatOriginalPath(b *testing.B) {
	tmpPath, err := files.CopyFolderToTemp("./testdata/fixtures", b.Name(), func(path string) bool { return true })
	if err != nil {
		b.Fatalf("Failed to copy fixtures: %v", err)
	}
	defer os.RemoveAll(tmpPath)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	if err != nil {
		b.Fatalf("Failed to create options: %v", err)
	}

	tgOptions.WorkingDir = tmpPath
	tgOptions.HclExclude = []string{".history"}
	tgOptions.FilterQueries = []string{}
	tgOptions.Experiments = experiment.NewExperiments()
	tgOptions.Writer = io.Discard
	tgOptions.ErrWriter = io.Discard

	formatter := logformat.NewFormatter(logformat.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)
	l := log.New(log.WithOutput(io.Discard), log.WithLevel(log.ErrorLevel), log.WithFormatter(formatter))
	ctx := context.Background()

	err = format.Run(ctx, l, tgOptions)
	if err != nil {
		b.Fatalf("Failed to pre-format files: %v", err)
	}

	tgOptions.Check = true

	for b.Loop() {
		err := format.Run(ctx, l, tgOptions)
		if err != nil {
			b.Fatalf("format.Run failed: %v", err)
		}
	}
}

func BenchmarkFormatDiscoveryPath(b *testing.B) {
	tmpPath, err := files.CopyFolderToTemp("./testdata/fixtures", b.Name(), func(path string) bool { return true })
	if err != nil {
		b.Fatalf("Failed to copy fixtures: %v", err)
	}
	defer os.RemoveAll(tmpPath)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	if err != nil {
		b.Fatalf("Failed to create options: %v", err)
	}

	tgOptions.WorkingDir = tmpPath
	tgOptions.HclExclude = []string{".history"}
	tgOptions.FilterQueries = []string{}
	tgOptions.Experiments = experiment.NewExperiments()
	tgOptions.Writer = io.Discard
	tgOptions.ErrWriter = io.Discard

	err = tgOptions.Experiments.EnableExperiment(experiment.FilterFlag)
	if err != nil {
		b.Fatalf("Failed to enable FilterFlag experiment: %v", err)
	}

	formatter := logformat.NewFormatter(logformat.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)
	l := log.New(log.WithOutput(io.Discard), log.WithLevel(log.ErrorLevel), log.WithFormatter(formatter))
	ctx := context.Background()

	err = format.Run(ctx, l, tgOptions)
	if err != nil {
		b.Fatalf("Failed to pre-format files: %v", err)
	}

	tgOptions.Check = true

	for b.Loop() {
		err := format.Run(ctx, l, tgOptions)
		if err != nil {
			b.Fatalf("format.Run failed: %v", err)
		}
	}
}
