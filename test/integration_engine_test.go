//go:build engine
// +build engine

package test

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

const (
	LocalEngineBinaryPath  = "terragrunt-iac-engine-opentofu_v0.0.1"
	TestFixtureLocalEngine = "fixture-engine/local-engine"
)

func TestEngineInvocation(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TestFixtureLocalEngine)
	tmpEnvPath := copyEnvironment(t, TestFixtureLocalEngine)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureLocalEngine)

	// get pwd
	pwd, err := os.Getwd()
	require.NoError(t, err)

	copyAndFillMapPlaceholders(t, util.JoinPath(TestFixtureLocalEngine, "terragrunt.hcl"), util.JoinPath(rootPath, "terragrunt.hcl"), map[string]string{
		"__engine_source__": pwd + "/../" + LocalEngineBinaryPath,
	})

	stdout, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
	require.NoError(t, err)

	assert.Contains(t, stdout+" "+stderr, pwd)
	assert.Contains(t, stderr, LocalEngineBinaryPath+": plugin address: address=")
	assert.Contains(t, stdout, "Initializing provider plugins...")
	assert.Contains(t, stdout, "test_input_value_from_terragrunt")

	assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")
}
