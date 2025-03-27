//go:build parse

// This test consumes so much memory that it causes the CI runner to crash.
// As a result, we have to run it on its own.

package test_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAllFixtureFiles(t *testing.T) {
	t.Parallel()

	knownBadFiles := []string{
		"fixtures/hclvalidate/second/a/terragrunt.hcl",
		"fixtures/hclfmt-errors/dangling-attribute/terragrunt.hcl",
		"fixtures/hclfmt-errors/invalid-character/terragrunt.hcl",
		"fixtures/hclfmt-errors/invalid-key/terragrunt.hcl",
		"fixtures/disabled/unit-disabled/terragrunt.hcl",
	}

	files := helpers.HCLFilesInDir(t, "fixtures")

	for _, file := range files {
		// Skip files in a .terragrunt-cache directory
		if strings.Contains(file, ".terragrunt-cache") {
			continue
		}

		t.Run(file, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Dir(file)

			opts, err := options.NewTerragruntOptionsForTest(dir)
			require.NoError(t, err)

			opts.Experiments.ExperimentMode()

			ctx := config.NewParsingContext(context.Background(), opts)

			cfg, _ := config.ParseConfigFile(ctx, file, nil)

			if slices.Contains(knownBadFiles, file) {
				assert.Nil(t, cfg)

				return
			}

			assert.NotNil(t, cfg)
		})
	}
}
