//go:build aws && tofu

package test_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExampleLiveStacks(t *testing.T) {
	uniqueID := strings.ToLower(helpers.UniqueID())

	awsCfg, err := awshelper.NewAWSConfigBuilder().Build(t.Context(), createLogger())
	require.NoError(t, err, "Error creating AWS config")

	stsClient := sts.NewFromConfig(awsCfg)

	identity, err := stsClient.GetCallerIdentity(t.Context(), &sts.GetCallerIdentityInput{})
	require.NoError(t, err, "Error getting AWS caller identity")

	accountID := *identity.Account

	t.Setenv("EX_APP_PREFIX", "tg-test-"+uniqueID+"-")
	t.Setenv("EX_BUCKET_PREFIX", "tg-test-"+uniqueID+"-")
	t.Setenv("EX_NON_PROD_ACCOUNT_ID", accountID)
	t.Setenv("EX_PROD_ACCOUNT_ID", accountID)

	tmpDir := helpers.TmpDirWOSymlinks(t)
	repoDir := filepath.Join(tmpDir, "live-stacks-example")

	helpers.ExecWithTestLogger(
		t,
		tmpDir,
		"git",
		"clone",
		"https://github.com/gruntwork-io/terragrunt-infrastructure-live-stacks-example.git",
		repoDir,
	)
	helpers.ExecWithTestLogger(t, repoDir, "git", "checkout", "3649bd95c93074e6a3742bc5122e411505c24c3a")

	helpers.ExecWithTestLogger(t, repoDir, "mise", "trust")
	helpers.ExecWithTestLogger(t, repoDir, "mise", "install")

	stdout, _ := helpers.ExecWithMiseAndCaptureOutput(t, repoDir, "terragrunt", "--version")
	require.Contains(t, stdout, "terragrunt")

	stdout, _ = helpers.ExecWithMiseAndCaptureOutput(t, repoDir, "tofu", "--version")
	require.Contains(t, stdout, "OpenTofu")

	region := "us-east-1"
	stateBucketName := fmt.Sprintf("tg-test-%s-terragrunt-example-tf-state-non-prod-%s", uniqueID, region)

	defer helpers.DeleteS3Bucket(t, region, stateBucketName)

	stackDir := filepath.Join(repoDir, "non-prod", "us-east-1", "stateful-lambda-service")

	defer func() {
		t.Log("Running destroy")
		helpers.ExecWithMiseAndTestLogger(
			t,
			stackDir,
			"terragrunt",
			"run",
			"--all",
			"--non-interactive",
			"--",
			"destroy",
			"-auto-approve",
		)
	}()

	t.Log("Running apply")
	helpers.ExecWithMiseAndTestLogger(
		t,
		stackDir,
		"terragrunt",
		"run",
		"--all",
		"--non-interactive",
		"--backend-bootstrap",
		"--",
		"apply",
	)

	serviceDir := filepath.Join(stackDir, ".terragrunt-stack", "service")
	stdoutOutput, _ := helpers.ExecWithMiseAndCaptureOutput(t, serviceDir, "terragrunt", "output", "-json")

	var outputs map[string]helpers.TerraformOutput
	require.NoError(t, json.Unmarshal([]byte(stdoutOutput), &outputs))

	functionURLOutput, ok := outputs["function_url"]
	require.True(t, ok, "Expected 'function_url' output to be defined")

	functionURL, ok := functionURLOutput.Value.(string)
	require.True(t, ok, "Expected 'function_url' to be a string")
	require.NotEmpty(t, functionURL, "function_url should not be empty")

	t.Logf("Lambda function URL: %s", functionURL)

	t.Log("GET initial count")

	const (
		maxRetries = 5
		retryDelay = 5 * time.Second
	)

	var initialCount float64

	for i := range maxRetries {
		req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodGet, functionURL, nil)
		require.NoError(t, reqErr)

		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			var body map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			resp.Body.Close()

			countVal, exists := body["count"]
			require.True(t, exists, "Expected 'count' field in response JSON")

			initialCount, ok = countVal.(float64)
			require.True(t, ok, "Expected 'count' to be a number")

			t.Logf("Initial count: %v", initialCount)

			break
		}

		if resp != nil {
			resp.Body.Close()
		}

		t.Logf("GET attempt %d/%d: err=%v, retrying in %s", i+1, maxRetries, err, retryDelay)
		time.Sleep(retryDelay)

		require.Less(t, i, maxRetries-1, "Failed to reach Lambda function URL after %d retries: %v", maxRetries, err)
	}

	t.Log("POST to increment count")

	postReq, err := http.NewRequestWithContext(t.Context(), http.MethodPost, functionURL, nil)
	require.NoError(t, err)

	postResp, err := http.DefaultClient.Do(postReq)
	require.NoError(t, err, "POST request failed")

	require.Equal(t, http.StatusOK, postResp.StatusCode, "Expected HTTP 200 from POST")

	var postBody map[string]any
	require.NoError(t, json.NewDecoder(postResp.Body).Decode(&postBody))
	postResp.Body.Close()

	postCount, exists := postBody["count"]
	require.True(t, exists, "Expected 'count' field in POST response JSON")

	postCountVal, ok := postCount.(float64)
	require.True(t, ok, "Expected 'count' to be a number in POST response")

	assert.Equal(t, int(initialCount)+1, int(postCountVal), "Expected count to increment by 1 after POST")
	t.Logf("Post-increment count: %v", postCountVal)

	t.Log("GET to verify persisted count")

	getReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet, functionURL, nil)
	require.NoError(t, err)

	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err, "GET request failed")

	require.Equal(t, http.StatusOK, getResp.StatusCode, "Expected HTTP 200 from final GET")

	var getBody map[string]any
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&getBody))
	getResp.Body.Close()

	finalCount, exists := getBody["count"]
	require.True(t, exists, "Expected 'count' field in final GET response JSON")

	finalCountVal, ok := finalCount.(float64)
	require.True(t, ok, "Expected 'count' to be a number in final GET response")

	assert.Equal(t, int(postCountVal), int(finalCountVal), "Expected final GET count to match POST count")
	t.Logf("Final verified count: %v", finalCountVal)
}
