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
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
)

func TestTerragruntParallelism(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedTimings        []int
		parallelism            int
		numberOfModules        int
		timeToDeployEachModule time.Duration
	}{
		{parallelism: 1, numberOfModules: 10, timeToDeployEachModule: 5 * time.Second, expectedTimings: []int{5, 10, 15, 20, 25, 30, 35, 40, 45, 50}},
		{parallelism: 3, numberOfModules: 10, timeToDeployEachModule: 5 * time.Second, expectedTimings: []int{5, 5, 5, 10, 10, 10, 15, 15, 15, 20}},
		{parallelism: 5, numberOfModules: 10, timeToDeployEachModule: 5 * time.Second, expectedTimings: []int{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}},
	}
	for _, tc := range testCases {

		t.Run(fmt.Sprintf("parallelism=%d numberOfModules=%d timeToDeployEachModule=%v expectedTimings=%v", tc.parallelism, tc.numberOfModules, tc.timeToDeployEachModule, tc.expectedTimings), func(t *testing.T) {
			t.Parallel()

			testTerragruntParallelism(t, tc.parallelism, tc.numberOfModules, tc.timeToDeployEachModule, tc.expectedTimings)
		})
	}
}

//nolint:paralleltest
func TestReadTerragruntAuthProviderCmdRemoteState(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "remote-state")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", helpers.TerraformRemoteStateS3Region)

	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	// I'm not sure why, but this test doesn't work with tenv
	os.Setenv("AWS_ACCESS_KEY_ID", "")     //nolint: tenv,usetesting
	os.Setenv("AWS_SECRET_ACCESS_KEY", "") //nolint: tenv,usetesting

	defer func() {
		os.Setenv("AWS_ACCESS_KEY_ID", accessKeyID)         //nolint: tenv,usetesting
		os.Setenv("AWS_SECRET_ACCESS_KEY", secretAccessKey) //nolint: tenv,usetesting
	}()

	credsConfig := util.JoinPath(rootPath, "creds.config")

	helpers.CopyAndFillMapPlaceholders(t, credsConfig, credsConfig, map[string]string{
		"__FILL_AWS_ACCESS_KEY_ID__":     accessKeyID,
		"__FILL_AWS_SECRET_ACCESS_KEY__": secretAccessKey,
	})

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt plan --non-interactive --working-dir %s --auth-provider-cmd %s", rootPath, mockAuthCmd))
}

func TestReadTerragruntAuthProviderCmdCredsForDependency(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "creds-for-dependency")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	dependencyCredsConfig := util.JoinPath(rootPath, "dependency", "creds.config")
	helpers.CopyAndFillMapPlaceholders(t, dependencyCredsConfig, dependencyCredsConfig, map[string]string{
		"__FILL_AWS_ACCESS_KEY_ID__":     accessKeyID,
		"__FILL_AWS_SECRET_ACCESS_KEY__": secretAccessKey,
	})

	dependentCredsConfig := util.JoinPath(rootPath, "dependent", "creds.config")
	helpers.CopyAndFillMapPlaceholders(t, dependentCredsConfig, dependentCredsConfig, map[string]string{
		"__FILL_AWS_ACCESS_KEY_ID__":     accessKeyID,
		"__FILL_AWS_SECRET_ACCESS_KEY__": secretAccessKey,
	})
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt run --all apply --non-interactive --working-dir %s --auth-provider-cmd %s", rootPath, mockAuthCmd))
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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	// copy the template `numberOfModules` times into the app
	tmpEnvPath := t.TempDir()
	for i := 0; i < numberOfModules; i++ {
		err := util.CopyFolderContents(createLogger(), testFixtureParallelism, tmpEnvPath, ".terragrunt-test", nil, nil)
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

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, "root.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := tmpEnvPath

	// forces plugin download & initialization (no parallelism control)
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt run --all plan --non-interactive --working-dir %s -var sleep_seconds=%d", environmentPath, timeToDeployEachModule/time.Second))

	// NOTE: we can't run just run --all apply and not run --all plan because the time to initialize the plugins skews the results of the test
	testStart := time.Now().Unix()
	t.Logf("run --all apply start time = %d, %s", testStart, time.Now().Format(time.RFC3339))
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt run --all apply --parallelism %d --non-interactive --working-dir %s -var sleep_seconds=%d", parallelism, environmentPath, timeToDeployEachModule/time.Second))

	// read the output of all modules 1 by 1 sequence, parallel reads mix outputs and make output complicated to parse
	outputParallelism := 1
	// Call helpers.RunTerragruntCommandWithOutput directly because this command contains failures (which causes helpers.RunTerragruntRedirectOutput to abort) but we don't care.
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all output -no-color --tf-forward-stdout --non-interactive --working-dir %s --parallelism %d", environmentPath, outputParallelism))
	if err != nil {
		return "", 0, err
	}

	return stdout, int(testStart), nil
}
