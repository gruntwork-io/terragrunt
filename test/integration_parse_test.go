//go:build parse

// These tests consume so much memory that they cause the CI runner to crash.
// As a result, we have to run them on their own.
//
// In the future, we should make improvements to parsing so that this isn't necessary.

package test_test

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var knownBadFiles = []string{
	"fixtures/catalog/local-template/.boilerplate/terragrunt.hcl",
	"fixtures/disabled/unit-disabled/terragrunt.hcl",
	"fixtures/hcl-filter/validate/semantic-error/incomplete-block/terragrunt.hcl",
	"fixtures/hcl-filter/validate/semantic-error/missing-value/terragrunt.hcl",
	"fixtures/hcl-filter/validate/stacks/syntax-error/stack2/terragrunt.stack.hcl",
	"fixtures/hcl-filter/validate/syntax-error/invalid-char/terragrunt.hcl",
	"fixtures/hcl-filter/validate/syntax-error/invalid-key/terragrunt.hcl",
	"fixtures/hclfmt-errors/dangling-attribute/terragrunt.hcl",
	"fixtures/hclfmt-errors/invalid-character/terragrunt.hcl",
	"fixtures/hclfmt-errors/invalid-key/terragrunt.hcl",
	"fixtures/hclvalidate/second/a/terragrunt.hcl",
	"fixtures/parsing/exposed-include-with-deprecated-inputs/compcommon.hcl",
	"fixtures/scaffold/with-shell-and-hooks/.boilerplate/terragrunt.hcl",
	"fixtures/scaffold/with-shell-commands/.boilerplate/terragrunt.hcl",
	"fixtures/stacks/errors/unknown-value/units/bad-unit/terragrunt.hcl",
	// Files that require AWS credentials (will fail/timeout without them)
	"fixtures/assume-role/external-id-with-comma/terragrunt.hcl",
	"fixtures/assume-role/external-id/terragrunt.hcl",
	"fixtures/auth-provider-cmd/creds-for-dependency/dependency/terragrunt.hcl",
	"fixtures/auth-provider-cmd/oidc/terragrunt.hcl",
	"fixtures/auth-provider-cmd/remote-state-w-oidc/terragrunt.hcl",
	"fixtures/auth-provider-cmd/remote-state/terragrunt.hcl",
	"fixtures/get-aws-account-alias/terragrunt.hcl",
	"fixtures/get-aws-caller-identity/terragrunt.hcl",
	"fixtures/read-config/iam_role_in_file/terragrunt.hcl",
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

			// Skip known bad files early to avoid timeouts (e.g., AWS credential errors)
			if slices.Contains(knownBadFiles, file) {
				t.Skip("Skipping known bad file")
			}

			dir := filepath.Dir(file)

			opts, err := options.NewTerragruntOptionsForTest(dir)
			require.NoError(t, err)

			opts.Experiments.ExperimentMode()

			l := logger.CreateLogger()

			ctx, pctx := config.NewParsingContext(
				context.TODO(), // Using context.TODO() instead of t.Context() here because we end up storing way too much in context otherwise.
				l,
				opts,
			)

			cfg, _ := config.ParseConfigFile(ctx, pctx, l, file, nil)

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
		{name: "find", command: "terragrunt find --no-color"},
		{name: "list", command: "terragrunt list --no-color"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				tt.command,
			)
			require.NoError(t, err)

			// stderr can be non-empty if there are deprecations
			t.Logf("stderr: %s", stderr)
			assert.NotEmpty(t, stdout)

			fields := strings.Fields(stdout)

			aDepLine := 0
			bDepLine := 0

			for i, field := range fields {
				if field == "fixtures/find/dag/a-dependent" {
					aDepLine = i
				}

				if field == "fixtures/find/dag/b-dependency" {
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
		{name: "find", command: "terragrunt find --no-color --dag"},
		{name: "list", command: "terragrunt list --no-color --dag"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				tt.command,
			)
			require.NoError(t, err)

			// stderr can be non-empty if there are deprecations
			t.Logf("stderr: %s", stderr)
			assert.NotEmpty(t, stdout)

			fields := strings.Fields(stdout)

			// Find positions of all fixtures in the output
			aDepLine := -1
			bDepLine := -1
			cMixedLine := -1
			dDepsLine := -1

			for i, field := range fields {
				switch field {
				case "fixtures/find/dag/a-dependent":
					aDepLine = i
				case "fixtures/find/dag/b-dependency":
					bDepLine = i
				case "fixtures/find/dag/c-mixed-deps":
					cMixedLine = i
				case "fixtures/find/dag/d-dependencies-only":
					dDepsLine = i
				}
			}

			// Verify DAG ordering:
			// b-dependency (no deps) should come first
			// a-dependent (depends on b) should come after b
			// d-dependencies-only (depends on b) should come after b
			// c-mixed-deps (depends on a and d) should come last
			assert.Greater(t, aDepLine, bDepLine, "a-dependent should come after b-dependency")
			assert.Greater(t, dDepsLine, bDepLine, "d-dependencies-only should come after b-dependency")
			assert.Greater(t, cMixedLine, aDepLine, "c-mixed-deps should come after a-dependent")
			assert.Greater(t, cMixedLine, dDepsLine, "c-mixed-deps should come after d-dependencies-only")
		})
	}
}

func TestParseFindListAllComponentsWithDAGAndExternal(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name    string
		command string
	}{
		{name: "find", command: "terragrunt find --no-color --dag --external"},
		{name: "list", command: "terragrunt list --no-color --dag --external"},
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

			fields := strings.Fields(stdout)

			// Find positions of all fixtures in the output
			aDepLine := -1
			bDepLine := -1
			cMixedLine := -1
			dDepsLine := -1

			for i, field := range fields {
				switch field {
				case "fixtures/find/dag/a-dependent":
					aDepLine = i
				case "fixtures/find/dag/b-dependency":
					bDepLine = i
				case "fixtures/find/dag/c-mixed-deps":
					cMixedLine = i
				case "fixtures/find/dag/d-dependencies-only":
					dDepsLine = i
				}
			}

			// Verify DAG ordering for the core dependencies
			// The exact ordering may vary with external dependencies included,
			// but the basic dependency relationship should hold
			if aDepLine >= 0 && bDepLine >= 0 {
				assert.Greater(t, aDepLine, bDepLine, "a-dependent should come after b-dependency")
			}

			if dDepsLine >= 0 && bDepLine >= 0 {
				assert.Greater(t, dDepsLine, bDepLine, "d-dependencies-only should come after b-dependency")
			}

			if cMixedLine >= 0 && aDepLine >= 0 && dDepsLine >= 0 {
				assert.Greater(t, cMixedLine, aDepLine, "c-mixed-deps should come after a-dependent")
				assert.Greater(t, cMixedLine, dDepsLine, "c-mixed-deps should come after d-dependencies-only")
			}
		})
	}
}
