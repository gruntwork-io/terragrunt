//go:build awsoidc

// These tests aren't hooked up to CI right now, as
// we'll soon be moving to a new CI system (GitHub Actions)
// and we don't want to add complexity to the migration by handling
// both CircleCI and GitHub Actions at the same time.

package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// This is the environment variable that GitHub Actions injects into the environment.
	// We use this to detect if we are running in GitHub Actions.
	githubActionsEnvVar = "GITHUB_ACTIONS"
	// Environment variable from GitHub Actions containing the URL to request the OIDC token.
	actionsIDTokenRequestURLEnvVar = "ACTIONS_ID_TOKEN_REQUEST_URL"
	// Environment variable from GitHub Actions containing the bearer token to authenticate the OIDC token request.
	actionsIDTokenRequestTokenEnvVar = "ACTIONS_ID_TOKEN_REQUEST_TOKEN"

	// This is a fixture that tests the assume-role-web-identity-file flag.
	testFixtureAssumeRoleWebIdentityFile = "fixtures/assume-role-web-identity/file-path"
)

func TestAwsAssumeRoleWebIdentityFile(t *testing.T) {
	// t.Parallel() cannot be used together with t.Setenv()
	// t.Parallel()

	token := fetchGitHubOIDCToken(t)

	// These tests need to be run without the static key + secret
	// used by most AWS tests here.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	os.Unsetenv("AWS_SECRET_ACCESS_KEY")

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAssumeRoleWebIdentityFile)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAssumeRoleWebIdentityFile)

	originalTerragruntConfigPath := util.JoinPath(testFixtureAssumeRoleWebIdentityFile, "terragrunt.hcl")
	tmpTerragruntConfigFile := util.JoinPath(testPath, "terragrunt.hcl")
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	role := os.Getenv("AWS_TEST_OIDC_ROLE_ARN")
	require.NotEmpty(t, role)

	tokenFile := t.TempDir() + "/oidc-token"
	require.NoError(t, os.WriteFile(tokenFile, []byte(token), 0400))

	defer func() {
		helpers.DeleteS3Bucket(
			t,
			helpers.TerraformRemoteStateS3Region,
			s3BucketName,
			options.WithIAMRoleARN(role),
			options.WithIAMWebIdentityToken(token),
		)
	}()

	helpers.CopyAndFillMapPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, map[string]string{
		"__FILL_IN_BUCKET_NAME__":              s3BucketName,
		"__FILL_IN_REGION__":                   helpers.TerraformRemoteStateS3Region,
		"__FILL_IN_ASSUME_ROLE__":              role,
		"__FILL_IN_IDENTITY_TOKEN_FILE_PATH__": tokenFile,
	})

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --backend-bootstrap --log-level trace --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	output := fmt.Sprintf("%s %s", stderr.String(), stdout.String())
	assert.Contains(t, output, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")
}

func TestAwsAssumeRoleWebIdentityFlag(t *testing.T) {
	// t.Parallel() cannot be used together with t.Setenv()
	// t.Parallel()

	token := fetchGitHubOIDCToken(t)

	// These tests need to be run without the static key + secret
	// used by most AWS tests here.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	os.Unsetenv("AWS_SECRET_ACCESS_KEY")

	tmp := t.TempDir()

	emptyTerragruntConfigPath := filepath.Join(tmp, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(emptyTerragruntConfigPath, []byte(""), 0400))

	emptyMainTFPath := filepath.Join(tmp, "main.tf")
	require.NoError(t, os.WriteFile(emptyMainTFPath, []byte(""), 0400))

	roleARN := os.Getenv("AWS_TEST_OIDC_ROLE_ARN")
	require.NotEmpty(t, roleARN)

	helpers.RunTerragrunt(t, "terragrunt apply --non-interactive --log-level trace --working-dir "+tmp+" --iam-assume-role "+roleARN+" --iam-assume-role-web-identity-token "+token)
}

func TestAwsReadTerragruntAuthProviderCmdWithOIDC(t *testing.T) {
	// t.Parallel() cannot be used together with t.Setenv()
	// t.Parallel()

	token := fetchGitHubOIDCToken(t)

	t.Setenv("OIDC_TOKEN", token)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	oidcPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "oidc")
	helpers.CleanupTerraformFolder(t, oidcPath)
	mockAuthCmd := filepath.Join(oidcPath, "mock-auth-cmd.sh")

	helpers.RunTerragrunt(t, fmt.Sprintf(`terragrunt apply -auto-approve --non-interactive --working-dir %s --auth-provider-cmd %s`, oidcPath, mockAuthCmd))
}

func TestAwsReadTerragruntAuthProviderCmdWithOIDCRemoteState(t *testing.T) {
	// t.Parallel() cannot be used together with t.Setenv()
	// t.Parallel()

	token := fetchGitHubOIDCToken(t)

	// These tests need to be run without the static key + secret
	// used by most AWS tests here.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	os.Unsetenv("AWS_SECRET_ACCESS_KEY")

	t.Setenv("OIDC_TOKEN", token)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	remoteStateOIDCPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "remote-state-w-oidc")
	helpers.CleanupTerraformFolder(t, remoteStateOIDCPath)
	mockAuthCmd := filepath.Join(remoteStateOIDCPath, "mock-auth-cmd.sh")

	// Create a temporary terragrunt config with actual values
	tmpTerragruntConfigFile := util.JoinPath(remoteStateOIDCPath, "terragrunt.hcl")
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	role := os.Getenv("AWS_TEST_OIDC_ROLE_ARN")
	require.NotEmpty(t, role)

	defer func() {
		helpers.DeleteS3Bucket(
			t,
			helpers.TerraformRemoteStateS3Region,
			s3BucketName,
			options.WithIAMRoleARN(role),
			options.WithIAMWebIdentityToken(token),
		)
	}()

	helpers.CopyAndFillMapPlaceholders(t, tmpTerragruntConfigFile, tmpTerragruntConfigFile, map[string]string{
		"__FILL_IN_BUCKET_NAME__": s3BucketName,
		"__FILL_IN_REGION__":      helpers.TerraformRemoteStateS3Region,
	})

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt apply --working-dir %s --auth-provider-cmd %s --non-interactive --backend-bootstrap --log-level trace",
			remoteStateOIDCPath,
			mockAuthCmd,
		),
	)
	require.NoError(t, err)
}

// oidcTokenResponse defines the structure of the JSON response from GitHub's OIDC token endpoint.
type oidcTokenResponse struct {
	Value string `json:"value"`
}

// fetchGitHubOIDCToken retrieves the OIDC token from the GitHub Actions environment by calling the token request URL.
// It skips the test if not running in a GitHub Actions environment or if the required environment variables are not set.
// It uses t.Fatalf if any part of the token fetching process fails after the initial checks.
func fetchGitHubOIDCToken(t *testing.T) string {
	t.Helper()

	if os.Getenv(githubActionsEnvVar) != "true" {
		t.Skipf("Skipping test because it's not running in a GitHub Actions environment (expected %s=true)", githubActionsEnvVar)
	}

	requestURL := os.Getenv(actionsIDTokenRequestURLEnvVar)
	if requestURL == "" {
		t.Skipf("Skipping test: Environment variable %s must be set in GitHub Actions to fetch OIDC token.", actionsIDTokenRequestURLEnvVar)
	}

	requestToken := os.Getenv(actionsIDTokenRequestTokenEnvVar)
	if requestToken == "" {
		t.Skipf("Skipping test: Environment variable %s must be set in GitHub Actions to fetch OIDC token.", actionsIDTokenRequestTokenEnvVar)
	}

	client := &http.Client{}
	postReqBody := strings.NewReader(`{"aud": "sts.amazonaws.com"}`)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, requestURL, postReqBody)
	require.NoError(t, err, "Failed to create OIDC token request to %s", requestURL)

	req.Header.Set("Authorization", "Bearer "+requestToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to execute OIDC token request to %s", requestURL)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			t.Fatalf("OIDC token request to %s failed with status %s. Additionally, failed to read response body: %v", requestURL, resp.Status, readErr)
		}
		t.Fatalf("OIDC token request to %s failed with status %s. Response: %s", requestURL, resp.Status, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read OIDC token response body from %s", requestURL)

	var tokenResp oidcTokenResponse
	err = json.Unmarshal(body, &tokenResp)
	require.NoError(t, err, "Failed to unmarshal OIDC token response JSON from %s. Response: %s", requestURL, string(body))

	require.NotEmpty(t, tokenResp.Value, "OIDC token 'value' field is empty in response from %s. Response: %s", requestURL, string(body))
	return tokenResp.Value
}
