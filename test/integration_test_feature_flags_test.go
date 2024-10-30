package test_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/util"
)

const (
	testSimpleFlag = "fixtures/feature-flags/simple-flag"
)

func TestFeatureFlagDefaults(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := copyEnvironment(t, testSimpleFlag)
	rootPath := util.JoinPath(tmpEnvPath, testSimpleFlag)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "test", outputs["string_feature_flag"].Value)
	assert.EqualValues(t, 666, outputs["int_feature_flag"].Value)
	assert.Equal(t, false, outputs["bool_feature_flag"].Value)

}
