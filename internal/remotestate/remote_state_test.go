package remotestate_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/stretchr/testify/assert"
)

/**
 * Test for s3, also tests that the terragrunt-specific options are not passed on to terraform
 */
func TestGetTFInitArgs(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "s3",
		BackendConfig: map[string]any{
			"encrypt": true,
			"bucket":  "my-bucket",
			"key":     "terraform.tfstate",
			"region":  "us-east-1",

			"s3_bucket_tags": map[string]any{
				"team":    "team name",
				"name":    "Terraform state storage",
				"service": "Terraform"},

			"dynamodb_table_tags": map[string]any{
				"team":    "team name",
				"name":    "Terraform lock table",
				"service": "Terraform"},

			"accesslogging_bucket_tags": map[string]any{
				"team":    "team name",
				"name":    "Terraform access log storage",
				"service": "Terraform"},

			"skip_bucket_versioning": true,

			"shared_credentials_file": "my-file",
			"force_path_style":        true,
		},
	}
	args := remotestate.New(cfg).GetTFInitArgs()

	// must not contain s3_bucket_tags or dynamodb_table_tags or accesslogging_bucket_tags or skip_bucket_versioning
	assertTerraformInitArgsEqual(t, args, "-backend-config=encrypt=true -backend-config=bucket=my-bucket -backend-config=key=terraform.tfstate -backend-config=region=us-east-1 -backend-config=force_path_style=true -backend-config=shared_credentials_file=my-file")
}

func TestGetTFInitArgsForGCS(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "gcs",
		BackendConfig: map[string]any{
			"project":  "my-project-123456",
			"location": "US",
			"bucket":   "my-bucket",
			"prefix":   "terraform.tfstate",

			"gcs_bucket_labels": map[string]any{
				"team":    "team name",
				"name":    "Terraform state storage",
				"service": "Terraform"},

			"skip_bucket_versioning": true,

			"credentials":  "my-file",
			"access_token": "xxxxxxxx",
		},
	}
	args := remotestate.New(cfg).GetTFInitArgs()

	// must not contain project, location gcs_bucket_labels or skip_bucket_versioning
	assertTerraformInitArgsEqual(t, args, "-backend-config=bucket=my-bucket -backend-config=prefix=terraform.tfstate -backend-config=credentials=my-file -backend-config=access_token=xxxxxxxx")
}

func TestGetTFInitArgsUnknownBackend(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "s4",
		BackendConfig: map[string]any{
			"encrypt": true,
			"bucket":  "my-bucket",
			"key":     "terraform.tfstate",
			"region":  "us-east-1"},
	}
	args := remotestate.New(cfg).GetTFInitArgs()

	// no Backend initializer available, but command line args should still be passed on
	assertTerraformInitArgsEqual(t, args, "-backend-config=encrypt=true -backend-config=bucket=my-bucket -backend-config=key=terraform.tfstate -backend-config=region=us-east-1")
}

func TestGetTFInitArgsInitDisabled(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "s3",
		DisableInit: true,
		BackendConfig: map[string]any{
			"encrypt": true,
			"bucket":  "my-bucket",
			"key":     "terraform.tfstate",
			"region":  "us-east-1"},
	}
	args := remotestate.New(cfg).GetTFInitArgs()

	assertTerraformInitArgsEqual(t, args, "-backend=false")
}

func TestGetTFInitArgsNoBackendConfigs(t *testing.T) {
	t.Parallel()

	cfgs := []*remotestate.Config{
		{BackendName: "s3"},
		{BackendName: "gcs"},
	}

	for _, cfg := range cfgs {
		args := remotestate.New(cfg).GetTFInitArgs()
		assert.Empty(t, args)
	}
}

func assertTerraformInitArgsEqual(t *testing.T, actualArgs []string, expectedArgs string) {
	t.Helper()

	expected := strings.Split(expectedArgs, " ")
	assert.Len(t, actualArgs, len(expected))

	for _, expectedArg := range expected {
		assert.Contains(t, actualArgs, expectedArg)
	}
}
