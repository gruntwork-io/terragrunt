package test_test

import (
	"bytes"
	"fmt"
	"os"
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

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.path, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := prepareGraphFixture(t)
			fixturePath := util.JoinPath(tmpEnvPath, testFixtureGraph)
			tmpModulePath := util.JoinPath(fixturePath, testCase.path)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt graph destroy --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-graph-root %s", tmpModulePath, tmpEnvPath))
			require.NoError(t, err)
			output := fmt.Sprintf("%v\n%v\n", stdout, stderr)

			for _, modulePath := range testCase.expectedModules {
				modulePath = filepath.Join(fixturePath, modulePath)

				relPath, err := filepath.Rel(tmpModulePath, modulePath)
				require.NoError(t, err)

				assert.Containsf(t, output, relPath+"\n", "Expected module %s to be in output: %s", relPath, output)
			}

			for _, modulePath := range testCase.notExpectedModules {
				modulePath = filepath.Join(fixturePath, modulePath)

				relPath, err := filepath.Rel(tmpModulePath, modulePath)
				require.NoError(t, err)

				assert.NotContainsf(t, output, "Module "+relPath+"\n", "Expected module %s must not to be in output: %s", relPath, output)
			}
		})
	}
}

func TestTerragruntApplyGraph(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path               string
		expectedModules    []string
		notExpectedModules []string
	}{
		{
			path:               "services/eks-service-3-v2",
			expectedModules:    []string{"services/eks-service-3-v2", "services/eks-service-3-v3"},
			notExpectedModules: []string{"lambda", "eks", "services/eks-service-3"},
		},
		{
			path:               "lambda",
			expectedModules:    []string{"lambda", "services/lambda-service-1", "services/lambda-service-2"},
			notExpectedModules: []string{"eks", "services/eks-service-1", "services/eks-service-2", "services/eks-service-3"},
		},
		{
			path:               "services/eks-service-5",
			expectedModules:    []string{"services/eks-service-5"},
			notExpectedModules: []string{"eks", "lambda", "services/eks-service-1", "services/eks-service-2", "services/eks-service-3"},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.path, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := prepareGraphFixture(t)
			fixturePath := util.JoinPath(tmpEnvPath, testFixtureGraph)
			tmpModulePath := util.JoinPath(fixturePath, testCase.path)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt graph apply --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-graph-root %s", tmpModulePath, tmpEnvPath))
			require.NoError(t, err)
			output := fmt.Sprintf("%v\n%v\n", stdout, stderr)

			for _, modulePath := range testCase.expectedModules {
				modulePath = filepath.Join(fixturePath, modulePath)

				relPath, err := filepath.Rel(tmpModulePath, modulePath)
				require.NoError(t, err)

				assert.Containsf(t, output, relPath+"\n", "Expected module %s to be in output: %s", relPath, output)
			}

			for _, modulePath := range testCase.notExpectedModules {
				modulePath = filepath.Join(fixturePath, modulePath)

				relPath, err := filepath.Rel(tmpModulePath, modulePath)
				require.NoError(t, err)

				assert.NotContainsf(t, output, "Module "+relPath+"\n", "Expected module %s must not to be in output: %s", relPath, output)
			}
		})
	}
}

func TestTerragruntGraphNonTerraformCommandExecution(t *testing.T) {
	t.Parallel()

	tmpEnvPath := prepareGraphFixture(t)
	tmpModulePath := util.JoinPath(tmpEnvPath, testFixtureGraph, "eks")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt graph render-json --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-graph-root %s", tmpModulePath, tmpEnvPath), &stdout, &stderr)
	require.NoError(t, err)

	// check that terragrunt_rendered.json is created in mod1/mod2/mod3
	for _, module := range []string{"services/eks-service-1", "eks"} {
		_, err = os.Stat(util.JoinPath(tmpEnvPath, testFixtureGraph, module, "terragrunt_rendered.json"))
		require.NoError(t, err)
	}
}

func prepareGraphFixture(t *testing.T) string {
	t.Helper()
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGraph)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureGraph)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt run-all apply --terragrunt-non-interactive --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	return tmpEnvPath
}
