//go:build engine
// +build engine

package test

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

const (
	TestFixtureLocalEngine = "fixture-engine/local-engine"
)

func TestEngineInvocation(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TestFixtureLocalEngine)
	tmpEnvPath := copyEnvironment(t, TestFixtureLocalEngine)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureLocalEngine)

	stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
	require.NoError(t, err)

	assert.Contains(t, stdout, "Initializing provider plugins...")
	assert.Contains(t, stdout, "test_input_value_from_terragrunt")
}
