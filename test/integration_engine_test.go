//go:build engine

package test

import (
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	TestFixtureLocalEngine    = "fixture-engine/local-engine"
	TestFixtureRemoteEngine   = "fixture-engine/remote-engine"
	TestFixtureOpenTofuEngine = "fixture-engine/opentofu-engine"
	TestFixtureOpenTofuRunAll = "fixture-engine/opentofu-run-all"

	EnvVarExperimental = "TG_EXPERIMENTAL_ENGINE"
)

var LocalEngineBinaryPath = "terragrunt-iac-engine-opentofu_" + testEngineVersion()

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

func TestEngineOpentofu(t *testing.T) {
	t.Setenv(EnvVarExperimental, "1")

	cleanupTerraformFolder(t, TestFixtureOpenTofuEngine)
	tmpEnvPath := copyEnvironment(t, TestFixtureOpenTofuEngine)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureOpenTofuEngine)

	stdout, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
	require.NoError(t, err)

	assert.Contains(t, stderr, "starting plugin:")
	assert.Contains(t, stderr, "plugin process exited:")
	assert.Contains(t, stdout, "OpenTofu has been successfully initialized")
	assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")
}

func TestEngineRunAllOpentofu(t *testing.T) {
	t.Setenv(EnvVarExperimental, "1")

	cleanupTerraformFolder(t, TestFixtureOpenTofuRunAll)
	tmpEnvPath := copyEnvironment(t, TestFixtureOpenTofuRunAll)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureOpenTofuRunAll)

	stdout, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all apply -no-color -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
	require.NoError(t, err)

	assert.Contains(t, stderr, "starting plugin:")
	assert.Contains(t, stderr, "plugin process exited:")
	assert.Contains(t, stdout, "OpenTofu has been successfully initialized")
	assert.Contains(t, stdout, "Your infrastructure matches the configuration.")
}

func TestEngineDownloadOverHttp(t *testing.T) {
	t.Setenv(EnvVarExperimental, "1")

	cleanupTerraformFolder(t, TestFixtureRemoteEngine)
	tmpEnvPath := copyEnvironment(t, TestFixtureRemoteEngine)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureRemoteEngine)

	platform := runtime.GOOS
	arch := runtime.GOARCH

	copyAndFillMapPlaceholders(t, util.JoinPath(TestFixtureRemoteEngine, "terragrunt.hcl"), util.JoinPath(rootPath, config.DefaultTerragruntConfigPath), map[string]string{
		"__hardcoded_url__": fmt.Sprintf("https://github.com/gruntwork-io/terragrunt-engine-opentofu/releases/download/v0.0.2/terragrunt-iac-engine-opentofu_rpc_v0.0.2_%s_%s.zip", platform, arch),
	})

	stdout, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
	require.NoError(t, err)

	assert.Contains(t, stderr, "starting plugin:")
	assert.Contains(t, stderr, "plugin process exited:")
	assert.Contains(t, stdout, "OpenTofu has been successfully initialized")
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

// testEngineVersion returns the version of the engine to be used in the test
func testEngineVersion() string {
	value, found := os.LookupEnv("ENGINE_VERSION")
	if !found {
		return "v0.0.1"
	}
	return value
}
