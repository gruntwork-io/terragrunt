package config_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDependencyOutputFromInitFolderExportsInputsAsEnvVars pins that the fast path for an
// already-init-ed dependency sets a TF_VAR_ entry for each of that dependency's inputs.
func TestDependencyOutputFromInitFolderExportsInputsAsEnvVars(t *testing.T) {
	t.Parallel()

	outputEnv := runDependencyOutputFastPath(t, `
inputs = {
  passphrase = "not-a-real-passphrase"
}
`)

	assert.Contains(t, outputEnv, "TF_VAR_passphrase=not-a-real-passphrase")
}

// TestDependencyOutputFromInitFolderSkipsUnresolvableInputs pins that the fast path exports no
// TF_VAR_* entries when it cannot evaluate the dependency's `inputs`. Unknown values decode as
// empty strings, so exporting only the resolvable subset would give `tofu output` a wrong value
// for the rest.
func TestDependencyOutputFromInitFolderSkipsUnresolvableInputs(t *testing.T) {
	t.Parallel()

	outputEnv := runDependencyOutputFastPath(t, `
dependency "vault" {
  config_path = "../vault"
}

inputs = {
  static     = "yes"
  passphrase = dependency.vault.outputs.secret
}
`)

	for _, entry := range outputEnv {
		assert.False(
			t,
			strings.HasPrefix(entry, "TF_VAR_"),
			"unresolvable inputs must export no variables, got %s",
			entry,
		)
	}
}

// runDependencyOutputFastPath parses a unit that reads an output of a dependency whose config
// includes the given body, and requires that output to resolve. The dependency is already init-ed,
// so the fast path fires, and the returned slice is the environment its `tofu output` ran with.
func runDependencyOutputFastPath(t *testing.T, depConfigBody string) []string {
	t.Helper()

	l := logger.CreateLogger()
	v := venvtest.NewWithOSFS()

	rootDir := t.TempDir()
	appDir := filepath.Join(rootDir, "app")
	depDir := filepath.Join(rootDir, "dep")

	require.NoError(t, v.FS.MkdirAll(appDir, 0755))
	require.NoError(t, v.FS.MkdirAll(depDir, 0755))

	appConfigPath := filepath.Join(appDir, config.DefaultTerragruntConfigPath)
	require.NoError(t, vfs.WriteFile(v.FS, appConfigPath, []byte(`
dependency "dep" {
  config_path = "../dep"
}

inputs = {
  from_dep = dependency.dep.outputs.foo
}
`), 0644))

	depConfigPath := filepath.Join(depDir, config.DefaultTerragruntConfigPath)
	require.NoError(t, vfs.WriteFile(v.FS, depConfigPath, []byte(`
remote_state {
  backend = "local"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }

  config = {
    path = "terraform.tfstate"
  }
}
`+depConfigBody), 0644))

	var outputEnv []string

	v = v.WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if !slices.Contains(inv.Args, tf.CommandNameOutput) {
			return vexec.Result{}
		}

		outputEnv = slices.Clone(inv.Env)

		return vexec.Result{Stdout: []byte(`{"foo":{"value":"bar","type":"string"}}`)}
	})

	// The fast path fires only once the dependency's source has been init-ed, which Terragrunt
	// detects by the .terraform directory in the cache dir that source resolves to.
	_, depDownloadDir := util.DefaultWorkingAndDownloadDirs(depConfigPath)

	depSource, err := tf.NewSource(l, v.FS, ".", depDownloadDir, depDir, false)
	require.NoError(t, err)
	require.NoError(t, v.FS.MkdirAll(filepath.Join(depSource.WorkingDir, ".terraform"), 0755))

	ctx, pctx := newTestParsingContext(t, v, appConfigPath)

	cfg, err := config.ParseConfigFile(ctx, pctx, l, appConfigPath, nil)
	require.NoError(t, err)
	require.Equal(t, "bar", cfg.Inputs["from_dep"])

	return outputEnv
}
