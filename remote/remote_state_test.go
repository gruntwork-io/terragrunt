package remote_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/**
 * Test for s3, also tests that the terragrunt-specific options are not passed on to terraform
 */
func TestToTerraformInitArgs(t *testing.T) {
	t.Parallel()

	remoteState := remote.RemoteState{
		Backend: "s3",
		Config: map[string]interface{}{
			"encrypt": true,
			"bucket":  "my-bucket",
			"key":     "terraform.tfstate",
			"region":  "us-east-1",

			"s3_bucket_tags": map[string]interface{}{
				"team":    "team name",
				"name":    "Terraform state storage",
				"service": "Terraform"},

			"dynamodb_table_tags": map[string]interface{}{
				"team":    "team name",
				"name":    "Terraform lock table",
				"service": "Terraform"},

			"accesslogging_bucket_tags": map[string]interface{}{
				"team":    "team name",
				"name":    "Terraform access log storage",
				"service": "Terraform"},

			"skip_bucket_versioning": true,

			"shared_credentials_file": "my-file",
			"force_path_style":        true,
		},
	}
	args := remoteState.ToTerraformInitArgs()

	// must not contain s3_bucket_tags or dynamodb_table_tags or accesslogging_bucket_tags or skip_bucket_versioning
	assertTerraformInitArgsEqual(t, args, "-backend-config=encrypt=true -backend-config=bucket=my-bucket -backend-config=key=terraform.tfstate -backend-config=region=us-east-1 -backend-config=force_path_style=true -backend-config=shared_credentials_file=my-file")
}

func TestToTerraformInitArgsForGCS(t *testing.T) {
	t.Parallel()

	remoteState := remote.RemoteState{
		Backend: "gcs",
		Config: map[string]interface{}{
			"project":  "my-project-123456",
			"location": "US",
			"bucket":   "my-bucket",
			"prefix":   "terraform.tfstate",

			"gcs_bucket_labels": map[string]interface{}{
				"team":    "team name",
				"name":    "Terraform state storage",
				"service": "Terraform"},

			"skip_bucket_versioning": true,

			"credentials":  "my-file",
			"access_token": "xxxxxxxx",
		},
	}
	args := remoteState.ToTerraformInitArgs()

	// must not contain project, location gcs_bucket_labels or skip_bucket_versioning
	assertTerraformInitArgsEqual(t, args, "-backend-config=bucket=my-bucket -backend-config=prefix=terraform.tfstate -backend-config=credentials=my-file -backend-config=access_token=xxxxxxxx")
}

func TestToTerraformInitArgsUnknownBackend(t *testing.T) {
	t.Parallel()

	remoteState := remote.RemoteState{
		Backend: "s4",
		Config: map[string]interface{}{
			"encrypt": true,
			"bucket":  "my-bucket",
			"key":     "terraform.tfstate",
			"region":  "us-east-1"},
	}
	args := remoteState.ToTerraformInitArgs()

	// no Backend initializer available, but command line args should still be passed on
	assertTerraformInitArgsEqual(t, args, "-backend-config=encrypt=true -backend-config=bucket=my-bucket -backend-config=key=terraform.tfstate -backend-config=region=us-east-1")
}

func TestToTerraformInitArgsInitDisabled(t *testing.T) {
	t.Parallel()

	remoteState := remote.RemoteState{
		Backend:     "s3",
		DisableInit: true,
		Config: map[string]interface{}{
			"encrypt": true,
			"bucket":  "my-bucket",
			"key":     "terraform.tfstate",
			"region":  "us-east-1"},
	}
	args := remoteState.ToTerraformInitArgs()

	assertTerraformInitArgsEqual(t, args, "-backend=false")
}

func TestToTerraformInitArgsNoBackendConfigs(t *testing.T) {
	t.Parallel()

	remoteStates := []remote.RemoteState{
		{Backend: "s3"},
		{Backend: "gcs"},
	}

	for _, state := range remoteStates {
		args := state.ToTerraformInitArgs()
		assert.Empty(t, args)
	}
}

func TestDiffersFrom(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("remote_state_test")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	testCases := []struct {
		name            string
		existingBackend remote.TerraformBackend
		stateFromConfig remote.RemoteState
		shouldOverride  bool
	}{
		{"both empty", remote.TerraformBackend{}, remote.RemoteState{}, false},
		{"same backend type value", remote.TerraformBackend{Type: "s3"}, remote.RemoteState{Backend: "s3"}, false},
		{"different backend type values", remote.TerraformBackend{Type: "s3"}, remote.RemoteState{Backend: "atlas"}, true},
		{
			"identical S3 configs",
			remote.TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			remote.RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			false,
		}, {
			"identical GCS configs",
			remote.TerraformBackend{
				Type:   "gcs",
				Config: map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			},
			remote.RemoteState{
				Backend: "gcs",
				Config:  map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			},
			false,
		}, {
			"different s3 bucket values",
			remote.TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			remote.RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"bucket": "different", "key": "bar", "region": "us-east-1"},
			},
			true,
		}, {
			"different gcs bucket values",
			remote.TerraformBackend{
				Type:   "gcs",
				Config: map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			},
			remote.RemoteState{
				Backend: "gcs",
				Config:  map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "different", "prefix": "bar"},
			},
			true,
		}, {
			"different s3 key values",
			remote.TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			remote.RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"bucket": "foo", "key": "different", "region": "us-east-1"},
			},
			true,
		}, {
			"different gcs prefix values",
			remote.TerraformBackend{
				Type:   "gcs",
				Config: map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			},
			remote.RemoteState{
				Backend: "gcs",
				Config:  map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "different"},
			},
			true,
		}, {
			"different s3 region values",
			remote.TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			remote.RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"bucket": "foo", "key": "bar", "region": "different"},
			},
			true,
		}, {
			"different gcs location values",
			remote.TerraformBackend{
				Type:   "gcs",
				Config: map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			},
			remote.RemoteState{
				Backend: "gcs",
				Config:  map[string]interface{}{"project": "foo-123456", "location": "different", "bucket": "foo", "prefix": "bar"},
			},
			true,
		},
		{
			"different boolean values and boolean conversion",
			remote.TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"something": "true"},
			},
			remote.RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"something": false},
			},
			true,
		},
		{
			"different gcs boolean values and boolean conversion",
			remote.TerraformBackend{
				Type:   "gcs",
				Config: map[string]interface{}{"something": "true"},
			},
			remote.RemoteState{
				Backend: "gcs",
				Config:  map[string]interface{}{"something": false},
			},
			true,
		},
		{
			"null values ignored",
			remote.TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"something": "foo", "set-to-nil-should-be-ignored": nil},
			},
			remote.RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"something": "foo"},
			},
			false,
		},
		{
			"gcs null values ignored",
			remote.TerraformBackend{
				Type:   "gcs",
				Config: map[string]interface{}{"something": "foo", "set-to-nil-should-be-ignored": nil},
			},
			remote.RemoteState{
				Backend: "gcs",
				Config:  map[string]interface{}{"something": "foo"},
			},
			false,
		},
	}

	for _, testCase := range testCases {
		// Save the testCase in local scope so all the t.Run calls don't end up with the last item in the list
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			shouldOverride := testCase.stateFromConfig.DiffersFrom(&testCase.existingBackend, terragruntOptions)
			assert.Equal(t, testCase.shouldOverride, shouldOverride, "Expect differsFrom to return %t but got %t for existingRemoteState %v and remoteStateFromTerragruntConfig %v", testCase.shouldOverride, shouldOverride, testCase.existingBackend, testCase.stateFromConfig)
		})
	}
}

func assertTerraformInitArgsEqual(t *testing.T, actualArgs []string, expectedArgs string) {
	expected := strings.Split(expectedArgs, " ")
	assert.Len(t, actualArgs, len(expected))

	for _, expectedArg := range expected {
		assert.Contains(t, actualArgs, expectedArg)
	}
}
