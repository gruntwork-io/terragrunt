package test_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
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
			fixturePath := filepath.Join(tmpEnvPath, testFixtureGraph)
			tmpModulePath := filepath.Join(fixturePath, tc.path)
			reportFile := filepath.Join(fixturePath, "report.json")

			_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --graph destroy --non-interactive --working-dir %s --graph-root %s --report-file %s --report-format json", tmpModulePath, tmpEnvPath, reportFile))
			require.NoError(t, err)

			require.FileExists(t, reportFile)
			runs, err := report.ParseJSONRunsFromFile(reportFile)
			require.NoError(t, err)

			expectedNames := make([]string, 0, len(tc.expectedModules))
			for _, modulePath := range tc.expectedModules {
				absPath := filepath.Join(fixturePath, modulePath)
				relPath, relErr := filepath.Rel(tmpEnvPath, absPath)
				require.NoError(t, relErr)

				expectedNames = append(expectedNames, relPath)
			}

			assert.ElementsMatch(t, expectedNames, runs.Names(), "Expected modules to match report")

			reportNames := runs.Names()

			for _, modulePath := range tc.notExpectedModules {
				absPath := filepath.Join(fixturePath, modulePath)
				notExpectedName, relErr := filepath.Rel(tmpEnvPath, absPath)
				require.NoError(t, relErr)
				assert.NotContains(t, reportNames, notExpectedName, "Expected module %s must not be in report", notExpectedName)
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
			fixturePath := filepath.Join(tmpEnvPath, testFixtureGraph)
			tmpModulePath := filepath.Join(fixturePath, tc.path)
			reportFile := filepath.Join(fixturePath, "report.json")

			_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt "+tc.args+" --report-file %s --report-format json", tmpModulePath, tmpEnvPath, reportFile))
			require.NoError(t, err)

			require.FileExists(t, reportFile)
			runs, err := report.ParseJSONRunsFromFile(reportFile)
			require.NoError(t, err)

			expectedNames := make([]string, 0, len(tc.expectedModules))
			for _, modulePath := range tc.expectedModules {
				absPath := filepath.Join(fixturePath, modulePath)
				relPath, relErr := filepath.Rel(tmpEnvPath, absPath)
				require.NoError(t, relErr)

				expectedNames = append(expectedNames, relPath)
			}

			assert.ElementsMatch(t, expectedNames, runs.Names(), "Expected modules to match report")

			reportNames := runs.Names()

			for _, modulePath := range tc.notExpectedModules {
				absPath := filepath.Join(fixturePath, modulePath)
				notExpectedName, relErr := filepath.Rel(tmpEnvPath, absPath)
				require.NoError(t, relErr)
				assert.NotContains(t, reportNames, notExpectedName, "Expected module %s must not be in report", notExpectedName)
			}
		})
	}
}

func prepareGraphFixture(t *testing.T) string {
	t.Helper()
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGraph)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureGraph)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	return tmpEnvPath
}
