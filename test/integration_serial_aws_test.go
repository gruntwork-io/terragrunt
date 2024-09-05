//go:build aws

package test_test

import (
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/util"
)

func TestTerragruntParallelism(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		parallelism            int
		numberOfModules        int
		timeToDeployEachModule time.Duration
		expectedTimings        []int
	}{
		{1, 10, 5 * time.Second, []int{5, 10, 15, 20, 25, 30, 35, 40, 45, 50}},
		{3, 10, 5 * time.Second, []int{5, 5, 5, 10, 10, 10, 15, 15, 15, 20}},
		{5, 10, 5 * time.Second, []int{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}},
	}
	for _, tc := range testCases {

		t.Run(fmt.Sprintf("parallelism=%d numberOfModules=%d timeToDeployEachModule=%v expectedTimings=%v", tc.parallelism, tc.numberOfModules, tc.timeToDeployEachModule, tc.expectedTimings), func(t *testing.T) {
			t.Parallel()

			testTerragruntParallelism(t, tc.parallelism, tc.numberOfModules, tc.timeToDeployEachModule, tc.expectedTimings)
		})
	}
}

func TestReadTerragruntAuthProviderCmdRemoteState(t *testing.T) {
	cleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := copyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "remote-state")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	defer deleteS3Bucket(t, TerraformRemoteStateS3Region, s3BucketName)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", TerraformRemoteStateS3Region)

	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	credsConfig := util.JoinPath(rootPath, "creds.config")

	copyAndFillMapPlaceholders(t, credsConfig, credsConfig, map[string]string{
		"__FILL_AWS_ACCESS_KEY_ID__":     accessKeyID,
		"__FILL_AWS_SECRET_ACCESS_KEY__": secretAccessKey,
	})

	runTerragrunt(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-auth-provider-cmd %s", rootPath, mockAuthCmd))
}

func TestReadTerragruntAuthProviderCmdCredsForDependency(t *testing.T) {
	cleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := copyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "creds-for-dependency")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	dependencyCredsConfig := util.JoinPath(rootPath, "dependency", "creds.config")
	copyAndFillMapPlaceholders(t, dependencyCredsConfig, dependencyCredsConfig, map[string]string{
		"__FILL_AWS_ACCESS_KEY_ID__":     accessKeyID,
		"__FILL_AWS_SECRET_ACCESS_KEY__": secretAccessKey,
	})

	dependentCredsConfig := util.JoinPath(rootPath, "dependent", "creds.config")
	copyAndFillMapPlaceholders(t, dependentCredsConfig, dependentCredsConfig, map[string]string{
		"__FILL_AWS_ACCESS_KEY_ID__":     accessKeyID,
		"__FILL_AWS_SECRET_ACCESS_KEY__": secretAccessKey,
	})
	runTerragrunt(t, fmt.Sprintf("terragrunt run-all apply --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-auth-provider-cmd %s", rootPath, mockAuthCmd))
}

// NOTE: the following test asserts precise timing for determining parallelism. As such, it can not be run in parallel
// with all the other tests as the system load could impact the duration in which the parallel terragrunt goroutines
// run.

func testTerragruntParallelism(t *testing.T, parallelism int, numberOfModules int, timeToDeployEachModule time.Duration, expectedTimings []int) {
	t.Helper()

	output, testStart, err := testRemoteFixtureParallelism(t, parallelism, numberOfModules, timeToDeployEachModule)
	require.NoError(t, err)

	// parse output and sort the times, the regex captures a string in the format time.RFC3339 emitted by terraform's timestamp function
	regex, err := regexp.Compile(`out = "([-:\w]+)"`)
	require.NoError(t, err)

	matches := regex.FindAllStringSubmatch(output, -1)
	assert.Len(t, matches, numberOfModules)

	var deploymentTimes = make([]int, 0, len(matches))
	for _, match := range matches {
		parsedTime, err := time.Parse(time.RFC3339, match[1])
		require.NoError(t, err)
		deploymentTime := int(parsedTime.Unix()) - testStart
		deploymentTimes = append(deploymentTimes, deploymentTime)
	}
	sort.Ints(deploymentTimes)

	// the reported times are skewed (running terragrunt/terraform apply adds a little bit of overhead)
	// we apply a simple scaling algorithm on the times based on the last expected time and the last actual time
	scalingFactor := float64(deploymentTimes[0]) / float64(expectedTimings[0])
	// find max skew time deploymentTimes vs expectedTimings
	for i := 1; i < len(deploymentTimes); i++ {
		factor := float64(deploymentTimes[i]) / float64(expectedTimings[i])
		if factor > scalingFactor {
			scalingFactor = factor
		}
	}
	scaledTimes := make([]float64, len(deploymentTimes))
	for i, deploymentTime := range deploymentTimes {
		scaledTimes[i] = float64(deploymentTime) / scalingFactor
	}

	t.Logf("Parallelism test numberOfModules=%d p=%d expectedTimes=%v deploymentTimes=%v scaledTimes=%v scaleFactor=%f", numberOfModules, parallelism, expectedTimings, deploymentTimes, scaledTimes, scalingFactor)
	maxDiffInSeconds := 5.0 * scalingFactor
	for i, scaledTime := range scaledTimes {
		difference := math.Abs(scaledTime - float64(expectedTimings[i]))
		assert.LessOrEqual(t, difference, maxDiffInSeconds, "Expected timing %d but got %f", expectedTimings[i], scaledTime)
	}
}

func testRemoteFixtureParallelism(t *testing.T, parallelism int, numberOfModules int, timeToDeployEachModule time.Duration) (string, int, error) {
	t.Helper()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())

	// copy the template `numberOfModules` times into the app
	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}
	for i := 0; i < numberOfModules; i++ {
		err := util.CopyFolderContents(testFixtureParallelism, tmpEnvPath, ".terragrunt-test", nil)
		if err != nil {
			return "", 0, err
		}
		err = os.Rename(
			path.Join(tmpEnvPath, "template"),
			path.Join(tmpEnvPath, "app"+strconv.Itoa(i)))
		if err != nil {
			return "", 0, err
		}
	}

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := tmpEnvPath

	// forces plugin download & initialization (no parallelism control)
	runTerragrunt(t, fmt.Sprintf("terragrunt plan-all --terragrunt-non-interactive --terragrunt-working-dir %s -var sleep_seconds=%d", environmentPath, timeToDeployEachModule/time.Second))
	// apply all with parallelism set
	// NOTE: we can't run just apply-all and not plan-all because the time to initialize the plugins skews the results of the test
	testStart := int(time.Now().Unix())
	t.Logf("apply-all start time = %d, %s", testStart, time.Now().Format(time.RFC3339))
	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-parallelism %d --terragrunt-non-interactive --terragrunt-working-dir %s -var sleep_seconds=%d", parallelism, environmentPath, timeToDeployEachModule/time.Second))

	// read the output of all modules 1 by 1 sequence, parallel reads mix outputs and make output complicated to parse
	outputParallelism := 1
	// Call runTerragruntCommandWithOutput directly because this command contains failures (which causes runTerragruntRedirectOutput to abort) but we don't care.
	stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt output-all -no-color --terragrunt-forward-tf-stdout --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-parallelism %d", environmentPath, outputParallelism))
	if err != nil {
		return "", 0, err
	}

	return stdout, testStart, nil
}
