package test_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureGraph = "fixtures/graph"
)

func TestTerragruntDestroyGraph(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path               string
		expectedModules    []string
		notExpectedModules []string
	}{
		{
			path:               "eks",
			expectedModules:    []string{"services/eks-service-3-v3", "services/eks-service-3-v2", "services/eks-service-3", "services/eks-service-4", "services/eks-service-5", "services/eks-service-2-v2", "services/eks-service-2", "services/eks-service-1"},
			notExpectedModules: []string{"lambda", "services/lambda-service-1", "services/lambda-service-2"},
		},
		{
			path:               "services/lambda-service-1",
			expectedModules:    []string{"services/lambda-service-2"},
			notExpectedModules: []string{"lambda"},
		},
		{
			path:               "services/eks-service-3",
			expectedModules:    []string{"services/eks-service-3-v2", "services/eks-service-4", "services/eks-service-3-v3"},
			notExpectedModules: []string{"eks", "services/eks-service-1", "services/eks-service-2"},
		},
		{
			path:               "services/lambda-service-2",
			expectedModules:    []string{"services/lambda-service-2"},
			notExpectedModules: []string{"services/lambda-service-1", "lambda"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := prepareGraphFixture(t)
			fixturePath := util.JoinPath(tmpEnvPath, testFixtureGraph)
			tmpModulePath := util.JoinPath(fixturePath, tc.path)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --graph destroy --non-interactive --working-dir %s --graph-root %s", tmpModulePath, tmpEnvPath))
			require.NoError(t, err)

			output := fmt.Sprintf("%v\n%v\n", stdout, stderr)

			for _, modulePath := range tc.expectedModules {
				modulePath = filepath.Join(fixturePath, modulePath)

				relPath, err := filepath.Rel(tmpEnvPath, modulePath)
				require.NoError(t, err)

				assert.Containsf(t, output, relPath+"\n", "Expected module %s to be in output: %s", relPath, output)
			}

			for _, modulePath := range tc.notExpectedModules {
				modulePath = filepath.Join(fixturePath, modulePath)

				relPath, err := filepath.Rel(tmpEnvPath, modulePath)
				require.NoError(t, err)

				assert.NotContainsf(t, output, "Unit "+relPath+"\n", "Expected module %s must not to be in output: %s", relPath, output)
			}
		})
	}
}

func TestTerragruntApplyGraph(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args               string
		path               string
		expectedModules    []string
		notExpectedModules []string
	}{
		{
			args:               "run --graph apply --non-interactive --working-dir %s --graph-root %s",
			path:               "lambda",
			expectedModules:    []string{"lambda", "services/lambda-service-1", "services/lambda-service-2"},
			notExpectedModules: []string{"eks", "services/eks-service-1", "services/eks-service-2", "services/eks-service-3"},
		},
		{
			args:               "run apply --graph --non-interactive --working-dir %s --graph-root %s",
			path:               "services/eks-service-5",
			expectedModules:    []string{"services/eks-service-5"},
			notExpectedModules: []string{"eks", "lambda", "services/eks-service-1", "services/eks-service-2", "services/eks-service-3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := prepareGraphFixture(t)
			fixturePath := util.JoinPath(tmpEnvPath, testFixtureGraph)
			tmpModulePath := util.JoinPath(fixturePath, tc.path)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt "+tc.args, tmpModulePath, tmpEnvPath))
			require.NoError(t, err)

			output := fmt.Sprintf("%v\n%v\n", stdout, stderr)

			for _, modulePath := range tc.expectedModules {
				modulePath = filepath.Join(fixturePath, modulePath)

				relPath, err := filepath.Rel(tmpEnvPath, modulePath)
				require.NoError(t, err)

				assert.Containsf(t, output, relPath+"\n", "Expected module %s to be in output: %s", relPath, output)
			}

			for _, modulePath := range tc.notExpectedModules {
				modulePath = filepath.Join(fixturePath, modulePath)

				relPath, err := filepath.Rel(tmpEnvPath, modulePath)
				require.NoError(t, err)

				assert.NotContainsf(t, output, "Unit "+relPath+"\n", "Expected module %s must not to be in output: %s", relPath, output)
			}
		})
	}
}

func prepareGraphFixture(t *testing.T) string {
	t.Helper()
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGraph)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureGraph)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	return tmpEnvPath
}
