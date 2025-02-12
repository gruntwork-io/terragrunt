//go:build aws

package test_test

import (
	"fmt"
	"math"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
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
	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}
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
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt plan-all --terragrunt-non-interactive --terragrunt-working-dir %s -var sleep_seconds=%d", environmentPath, timeToDeployEachModule/time.Second))
	// apply all with parallelism set
	// NOTE: we can't run just apply-all and not plan-all because the time to initialize the plugins skews the results of the test
	testStart := int(time.Now().Unix())
	t.Logf("apply-all start time = %d, %s", testStart, time.Now().Format(time.RFC3339))
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-parallelism %d --terragrunt-non-interactive --terragrunt-working-dir %s -var sleep_seconds=%d", parallelism, environmentPath, timeToDeployEachModule/time.Second))

	// read the output of all modules 1 by 1 sequence, parallel reads mix outputs and make output complicated to parse
	outputParallelism := 1
	// Call helpers.RunTerragruntCommandWithOutput directly because this command contains failures (which causes helpers.RunTerragruntRedirectOutput to abort) but we don't care.
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt output-all -no-color --terragrunt-forward-tf-stdout --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-parallelism %d", environmentPath, outputParallelism))
	if err != nil {
		return "", 0, err
	}

	return stdout, testStart, nil
}
