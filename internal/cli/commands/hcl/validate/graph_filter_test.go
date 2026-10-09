package validate_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runValidateWithFilter runs hcl validate over files with a single filter query.
func runValidateWithFilter(t *testing.T, files map[string]string, query string) (string, error) {
	t.Helper()

	root := venvtest.Root("/repo")
	v := venvtest.New().WithFS(venvtest.NewFS(t, root, files))
	l := logger.CreateLogger()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, config.DefaultTerragruntConfigPath))
	require.NoError(t, err)

	opts.Filters, err = filter.ParseFilterQueries(l, []string{query})
	require.NoError(t, err)

	return root, validate.RunValidate(t.Context(), l, v, opts)
}

// TestRunValidateGraphFilterReportsMissingDependencyOnce pins that a dependency graph filter leaves a missing
// `dependency` target to the error the declaring unit already raises.
func TestRunValidateGraphFilterReportsMissingDependencyOnce(t *testing.T) {
	t.Parallel()

	_, err := runValidateWithFilter(t, map[string]string{
		"app/terragrunt.hcl": "dependency \"db\" {\n  config_path = \"../db\"\n}\n",
	}, "{./**}...")

	_, ok := errors.AsType[config.DependencyConfigNotFound](err)
	require.True(t, ok, "unexpected error %v", err)

	_, ok = errors.AsType[config.TerragruntConfigNotFoundError](err)
	assert.False(t, ok, "unexpected error %v", err)

	_, ok = errors.AsType[*validate.MissingDependencyError](err)
	assert.False(t, ok, "unexpected error %v", err)
}

// TestRunValidateGraphFilterNamesDependentOfMissingDependency pins that a dependency graph filter names the
// dependent of a `dependencies` path whose directory exists without a unit configuration.
func TestRunValidateGraphFilterNamesDependentOfMissingDependency(t *testing.T) {
	t.Parallel()

	root, err := runValidateWithFilter(t, map[string]string{
		"api/terragrunt.hcl": "dependencies {\n  paths = [\"../cache\"]\n}\n",
		"cache/main.tf":      "",
	}, "{./**}...")

	missing, ok := errors.AsType[*validate.MissingDependencyError](err)
	require.True(t, ok, "unexpected error %v", err)

	assert.Equal(t, filepath.Join(root, "cache"), missing.Path)
	assert.Equal(t, []string{filepath.Join(root, "api")}, missing.Dependents)

	_, ok = errors.AsType[config.TerragruntConfigNotFoundError](err)
	assert.False(t, ok, "unexpected error %v", err)
}

// TestRunValidateFilterKeepsUnparseableUnitError pins that a unit discovery cannot parse still fails
// validation when the filter leaves it out of the validated set.
func TestRunValidateFilterKeepsUnparseableUnitError(t *testing.T) {
	t.Parallel()

	_, err := runValidateWithFilter(t, map[string]string{
		"shared.hcl":         "locals {}\n",
		"ok/terragrunt.hcl":  "locals {\n  shared = read_terragrunt_config(\"../shared.hcl\")\n}\n",
		"bad/terragrunt.hcl": "include \"root\" {\n  path = \"../absent.hcl\"\n}\n",
	}, "reading=shared.hcl")

	_, ok := errors.AsType[config.IncludeConfigNotFoundError](err)
	require.True(t, ok, "unexpected error %v", err)
}
