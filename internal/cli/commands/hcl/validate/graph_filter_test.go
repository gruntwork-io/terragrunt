package validate_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validateResult is what one hcl validate run over an in-memory repository produced.
type validateResult struct {
	err   error
	root  string
	diags diagnostic.Diagnostics
}

// validateRun describes one hcl validate run over an in-memory repository.
type validateRun struct {
	// files are the files of the repository, by path relative to its root.
	files map[string]string
	// queries are the filter queries the run is given.
	queries []string
	// strictControls are the names of the strict controls the run enables.
	strictControls []string
}

// runValidate runs hcl validate as run describes and decodes the diagnostics it writes.
func runValidate(t *testing.T, run validateRun) validateResult {
	t.Helper()

	var stdout bytes.Buffer

	root := venvtest.Root("/repo")
	v := venvtest.New().WithFS(venvtest.NewFS(t, root, run.files)).WithWriter(&stdout)
	l := logger.CreateLogger()

	opts, err := options.NewTerragruntOptionsForTest(
		filepath.Join(root, config.DefaultTerragruntConfigPath),
	)
	require.NoError(t, err)

	opts.HCLValidateJSONOutput = true

	opts.Filters, err = filter.ParseFilterQueries(l, run.queries)
	require.NoError(t, err)

	for _, name := range run.strictControls {
		require.NoError(t, opts.StrictControls.EnableControl(name))
	}

	result := validateResult{root: root, err: validate.RunValidate(config.WithCaches(t.Context()), l, v, opts)}

	if stdout.Len() > 0 {
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &result.diags))
	}

	return result
}

// requireMissingDependencyAt requires that the only diagnostic reports a missing dependency at line of file.
func requireMissingDependencyAt(
	t *testing.T,
	result validateResult,
	file string,
	line int,
	code, detail string,
) {
	t.Helper()

	require.Error(t, result.err)
	require.Len(t, result.diags, 1)

	diag := result.diags[0]

	assert.Equal(t, "Missing dependency", diag.Summary)
	assert.Equal(t, detail, diag.Detail)

	require.NotNil(t, diag.Range)
	assert.Equal(t, filepath.Join(result.root, file), diag.Range.Filename)
	assert.Equal(t, line, diag.Range.Start.Line)

	require.NotNil(t, diag.Snippet)
	assert.Equal(
		t,
		code,
		diag.Snippet.Code[diag.Snippet.HighlightStartOffset:diag.Snippet.HighlightEndOffset],
	)
}

// TestRunValidatePointsAtMissingDependency pins that a missing dependency is reported once, as a diagnostic
// on the expression that declares it, with and without a dependency graph filter.
func TestRunValidatePointsAtMissingDependency(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		files   map[string]string
		name    string
		file    string
		code    string
		missing string
		queries []string
		line    int
	}{
		{
			name: "dependency block",
			files: map[string]string{
				"app/terragrunt.hcl": "dependency \"db\" {\n  config_path = \"../db\"\n}\n",
			},
			file:    "app/terragrunt.hcl",
			line:    2,
			code:    `"../db"`,
			missing: "This directory does not exist.",
		},
		{
			name: "dependency block on a directory without a unit configuration",
			files: map[string]string{
				"app/terragrunt.hcl": "dependency \"db\" {\n  config_path = \"../db\"\n}\n",
				"db/main.tf":         "",
			},
			file:    "app/terragrunt.hcl",
			line:    2,
			code:    `"../db"`,
			missing: "This directory has no terragrunt.hcl.",
		},
		{
			name: "dependency block under a graph filter",
			files: map[string]string{
				"app/terragrunt.hcl": "dependency \"db\" {\n  config_path = \"../db\"\n}\n",
			},
			queries: []string{"{./**}..."},
			file:    "app/terragrunt.hcl",
			line:    2,
			code:    `"../db"`,
			missing: "This directory does not exist.",
		},
		{
			name: "dependencies path that is not a directory",
			files: map[string]string{
				"api/terragrunt.hcl": "dependencies {\n  paths = [\n    \"../vpc\",\n    \"../cache\",\n  ]\n}\n",
				"vpc/terragrunt.hcl": "",
			},
			queries: []string{"{./**}..."},
			file:    "api/terragrunt.hcl",
			line:    4,
			code:    `"../cache"`,
			missing: "This directory does not exist.",
		},
		{
			name: "dependencies path without a unit configuration",
			files: map[string]string{
				"api/terragrunt.hcl": "dependencies {\n  paths = [\"../cache\"]\n}\n",
				"cache/main.tf":      "",
			},
			queries: []string{"{./**}..."},
			file:    "api/terragrunt.hcl",
			line:    2,
			code:    `"../cache"`,
			missing: "This directory has no terragrunt.hcl.",
		},
		{
			name: "dependent left out of the validated set",
			files: map[string]string{
				"app/terragrunt.hcl": "dependency \"db\" {\n  config_path = \"../db\"\n}\n",
			},
			queries: []string{"^{./app}..."},
			file:    "app/terragrunt.hcl",
			line:    2,
			code:    `"../db"`,
			missing: "This directory does not exist.",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, validateRun{files: tc.files, queries: tc.queries})

			_, ok := errors.AsType[config.TerragruntConfigNotFoundError](result.err)
			assert.False(t, ok, "unexpected error %v", result.err)

			requireMissingDependencyAt(t, result, tc.file, tc.line, tc.code, tc.missing)
		})
	}
}

// TestRunValidateNamesDependentWhenDependencyIsNotSpelledOut pins that a missing dependency whose path the
// dependent computes is still reported with the dependent, as an error without a source position.
func TestRunValidateNamesDependentWhenDependencyIsNotSpelledOut(t *testing.T) {
	t.Parallel()

	result := runValidate(t, validateRun{
		files: map[string]string{
			"api/terragrunt.hcl": "dependencies {\n  paths = [\"${get_terragrunt_dir()}/../cache\"]\n}\n",
			"cache/main.tf":      "",
		},
		queries: []string{"{./**}..."},
	})

	missing, ok := errors.AsType[*validate.MissingDependencyError](result.err)
	require.True(t, ok, "unexpected error %v", result.err)

	assert.Equal(t, filepath.Join(result.root, "cache"), missing.Path)
	assert.Equal(t, []string{filepath.Join(result.root, "api")}, missing.Dependents)
	assert.Empty(t, result.diags)
}

// TestRunValidateGraphFilterAcceptsStackDependency pins that a dependency graph filter accepts a dependency
// on a stack directory whether or not the stack is generated, and still rejects one on a unit of the stack
// that is not generated.
func TestRunValidateGraphFilterAcceptsStackDependency(t *testing.T) {
	t.Parallel()

	const (
		stack     = "unit \"vpc\" {\n  source = \"../vpc\"\n  path   = \"vpc\"\n}\n"
		stackUnit = "net/.terragrunt-stack/vpc"
	)

	dependency := func(configPath string) string {
		return "dependency \"net\" {\n  config_path = \"" + configPath + "\"\n}\n"
	}

	tcs := []struct {
		files       map[string]string
		name        string
		query       string
		wantMissing string
	}{
		{
			name: "stack not generated",
			files: map[string]string{
				"app/terragrunt.hcl":       dependency("../net"),
				"net/terragrunt.stack.hcl": stack,
			},
			query: "{./app}...",
		},
		{
			name: "dependencies path on a stack not generated",
			files: map[string]string{
				"app/terragrunt.hcl":       "dependencies {\n  paths = [\"../net\"]\n}\n",
				"net/terragrunt.stack.hcl": stack,
			},
			query: "{./app}...",
		},
		{
			name: "stack generated",
			files: map[string]string{
				"app/terragrunt.hcl":          dependency("../net"),
				"net/terragrunt.stack.hcl":    stack,
				stackUnit + "/terragrunt.hcl": "",
			},
			query: "{./app}...",
		},
		{
			name: "unit of a stack not generated",
			files: map[string]string{
				"app/terragrunt.hcl":       dependency("../" + stackUnit),
				"net/terragrunt.stack.hcl": stack,
			},
			query:       "{./app}...",
			wantMissing: stackUnit,
		},
		{
			name: "unit of a stack not generated and dependent left out",
			files: map[string]string{
				"app/terragrunt.hcl":       dependency("../" + stackUnit),
				"net/terragrunt.stack.hcl": stack,
			},
			query:       "^{./app}...",
			wantMissing: stackUnit,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, validateRun{files: tc.files, queries: []string{tc.query}})
			if tc.wantMissing == "" {
				require.NoError(t, result.err)
				assert.Empty(t, result.diags)

				return
			}

			requireMissingDependencyAt(
				t, result, "app/terragrunt.hcl", 2, `"../`+tc.wantMissing+`"`,
				"This directory does not exist.",
			)
		})
	}
}

// TestRunValidateFilterKeepsUnparseableUnitError pins that a unit discovery cannot parse still fails
// validation when the filter leaves it out of the validated set.
func TestRunValidateFilterKeepsUnparseableUnitError(t *testing.T) {
	t.Parallel()

	result := runValidate(t, validateRun{
		files: map[string]string{
			"shared.hcl":         "locals {}\n",
			"ok/terragrunt.hcl":  "locals {\n  shared = read_terragrunt_config(\"../shared.hcl\")\n}\n",
			"bad/terragrunt.hcl": "include \"root\" {\n  path = \"../absent.hcl\"\n}\n",
		},
		queries: []string{"reading=shared.hcl"},
	})

	_, ok := errors.AsType[config.IncludeConfigNotFoundError](result.err)
	require.True(t, ok, "unexpected error %v", result.err)
}

// TestRunValidateShowsPathsRelativeToWorkingDir pins that the human output names the file of a diagnostic
// relative to the working directory discovery ran in.
func TestRunValidateShowsPathsRelativeToWorkingDir(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer

	root := venvtest.Root("/repo")
	v := venvtest.New().WithFS(venvtest.NewFS(t, root, map[string]string{
		"live/app/terragrunt.hcl": "dependency \"db\" {\n  config_path = \"../db\"\n}\n",
	})).WithWriter(&stdout)

	opts, err := options.NewTerragruntOptionsForTest(
		filepath.Join(root, "live", config.DefaultTerragruntConfigPath),
	)
	require.NoError(t, err)

	require.Error(t, validate.RunValidate(config.WithCaches(t.Context()), logger.CreateLogger(), v, opts))

	assert.Contains(t, stdout.String(), " --> "+filepath.Join("app", "terragrunt.hcl")+":2:17\n")
}

// TestRunValidateGraphFilterAcceptsJSONDependency pins that a dependency configured by terragrunt.hcl.json
// is not reported missing, since a `dependency` block resolves its `config_path` to that file too.
func TestRunValidateGraphFilterAcceptsJSONDependency(t *testing.T) {
	t.Parallel()

	result := runValidate(t, validateRun{
		files: map[string]string{
			"app/terragrunt.hcl":     "dependency \"db\" {\n  config_path = \"../db\"\n}\n",
			"db/terragrunt.hcl.json": "{}\n",
		},
		queries: []string{"{./app}..."},
	})

	for _, diag := range result.diags {
		assert.NotEqual(t, "Missing dependency", diag.Summary)
	}

	_, ok := errors.AsType[*validate.MissingDependencyError](result.err)
	assert.False(t, ok, "unexpected error %v", result.err)
}
