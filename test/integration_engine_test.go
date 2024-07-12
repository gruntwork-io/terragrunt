//go:build engine

package test

import (
	"fmt"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	LocalEngineBinaryPath  = "terragrunt-iac-engine-opentofu_v0.0.1"
	TestFixtureLocalEngine = "fixture-engine/local-engine"

	EnvVarExperimental = "TG_EXPERIMENTAL_ENGINE"
)

func TestEnginePlan(t *testing.T) {
	rootPath := setupLocalEngine(t)

	stdout, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
	require.NoError(t, err)

	assert.Contains(t, stderr, LocalEngineBinaryPath+": plugin address")
	assert.Contains(t, stderr, "starting plugin:")
	assert.Contains(t, stderr, "plugin process exited:")
	assert.Contains(t, stdout, "1 to add, 0 to change, 0 to destroy.")
}

func TestEngineApply(t *testing.T) {
	rootPath := setupLocalEngine(t)

	stdout, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
	require.NoError(t, err)

	assert.Contains(t, stderr, LocalEngineBinaryPath+": plugin address")
	assert.Contains(t, stderr, "starting plugin:")
	assert.Contains(t, stderr, "plugin process exited:")
	assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")
}

func setupLocalEngine(t *testing.T) string {
	t.Setenv(EnvVarExperimental, "1")

	cleanupTerraformFolder(t, TestFixtureLocalEngine)
	tmpEnvPath := copyEnvironment(t, TestFixtureLocalEngine)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureLocalEngine)

	// get pwd
	pwd, err := os.Getwd()
	require.NoError(t, err)

	copyAndFillMapPlaceholders(t, util.JoinPath(TestFixtureLocalEngine, "terragrunt.hcl"), util.JoinPath(rootPath, config.DefaultTerragruntConfigPath), map[string]string{
		"__engine_source__": pwd + "/../" + LocalEngineBinaryPath,
	})
	return rootPath
}
