//go:build parse

// This test consumes so much memory that it causes the CI runner to crash.
// As a result, we have to run it on its own.

package test_test

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var knownBadFiles = []string{
	"fixtures/hclvalidate/second/a/terragrunt.hcl",
	"fixtures/hclfmt-errors/dangling-attribute/terragrunt.hcl",
	"fixtures/hclfmt-errors/invalid-character/terragrunt.hcl",
	"fixtures/hclfmt-errors/invalid-key/terragrunt.hcl",
	"fixtures/disabled/unit-disabled/terragrunt.hcl",
}

func TestParseAllFixtureFiles(t *testing.T) {
	t.Parallel()

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

			// Suggest garbage collection to free up memory.
			// Parsing config files can be memory intensive, and we don't need the config
			// files in memory after we've parsed them.
			runtime.GC()
		})
	}
}

func TestParseFindListAllComponents(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name    string
		command string
	}{
		{name: "find", command: "terragrunt find --experiment cli-redesign --no-color"},
		{name: "list", command: "terragrunt list --experiment cli-redesign --no-color"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				tt.command,
			)
			require.NoError(t, err)

			assert.Empty(t, stderr)
			assert.NotEmpty(t, stdout)

			lines := strings.Split(stdout, "\n")

			aDepLine := 0
			bDepLine := 0

			for i, line := range lines {
				if line == "fixtures/find/dag/a-dependency" {
					aDepLine = i
				}

				if line == "fixtures/find/dag/b-dependency" {
					bDepLine = i
				}
			}

			assert.Less(t, aDepLine, bDepLine)
		})
	}
}

func TestParseFindListAllComponentsWithDAG(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name    string
		command string
	}{
		{name: "find", command: "terragrunt find --experiment cli-redesign --no-color --dag"},
		{name: "list", command: "terragrunt list --experiment cli-redesign --no-color --dag"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				tt.command,
			)
			require.NoError(t, err)

			assert.NotEmpty(t, stderr)
			assert.NotEmpty(t, stdout)

			lines := strings.Split(stdout, "\n")

			aDepLine := 0
			bDepLine := 0

			for i, line := range lines {
				if line == "fixtures/find/dag/a-dependency" {
					aDepLine = i
				}

				if line == "fixtures/find/dag/b-dependency" {
					bDepLine = i
				}
			}

			assert.Greater(t, aDepLine, bDepLine)
		})
	}
}

func TestParseFindListAllComponentsWithDAGAndExternal(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name    string
		command string
	}{
		{name: "find", command: "terragrunt find --experiment cli-redesign --no-color --dag --external"},
		{name: "list", command: "terragrunt list --experiment cli-redesign --no-color --dag --external"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				tt.command,
			)
			require.NoError(t, err)

			assert.NotEmpty(t, stderr)
			assert.NotEmpty(t, stdout)

			lines := strings.Split(stdout, "\n")

			aDepLine := 0
			bDepLine := 0

			for i, line := range lines {
				if line == "fixtures/find/dag/a-dependency" {
					aDepLine = i
				}

				if line == "fixtures/find/dag/b-dependency" {
					bDepLine = i
				}
			}

			assert.Greater(t, aDepLine, bDepLine)
		})
	}
}
