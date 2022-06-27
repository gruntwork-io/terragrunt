package remote

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

/**
 * Test for s3, also tests that the terragrunt-specific options are not passed on to terraform
 */
func TestToTerraformInitArgs(t *testing.T) {
	t.Parallel()

	remoteState := RemoteState{
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
				"name":    "Terraform state storage",
				"service": "Terraform"},

			"skip_bucket_versioning": true,

			"shared_credentials_file": "my-file",
			"force_path_style":        true,
		},
	}
	args := remoteState.ToTerraformInitArgs()

	// must not contain s3_bucket_tags or dynamodb_table_tags or skip_bucket_versioning
	assertTerraformInitArgsEqual(t, args, "-backend-config=encrypt=true -backend-config=bucket=my-bucket -backend-config=key=terraform.tfstate -backend-config=region=us-east-1 -backend-config=force_path_style=true -backend-config=shared_credentials_file=my-file")
}

func TestToTerraformInitArgsForGCS(t *testing.T) {
	t.Parallel()

	remoteState := RemoteState{
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

			"credentials": "my-file",
		},
	}
	args := remoteState.ToTerraformInitArgs()

	// must not contain project, location gcs_bucket_labels or skip_bucket_versioning
	assertTerraformInitArgsEqual(t, args, "-backend-config=bucket=my-bucket -backend-config=prefix=terraform.tfstate -backend-config=credentials=my-file")
}

func TestToTerraformInitArgsUnknownBackend(t *testing.T) {
	t.Parallel()

	remoteState := RemoteState{
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

	remoteState := RemoteState{
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

	remoteStates := []RemoteState{
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
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	testCases := []struct {
		name            string
		existingBackend TerraformBackend
		stateFromConfig RemoteState
		shouldOverride  bool
	}{
		{"both empty", TerraformBackend{}, RemoteState{}, false},
		{"same backend type value", TerraformBackend{Type: "s3"}, RemoteState{Backend: "s3"}, false},
		{"different backend type values", TerraformBackend{Type: "s3"}, RemoteState{Backend: "atlas"}, true},
		{
			"identical S3 configs",
			TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			false,
		}, {
			"identical GCS configs",
			TerraformBackend{
				Type:   "gcs",
				Config: map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			},
			RemoteState{
				Backend: "gcs",
				Config:  map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			},
			false,
		}, {
			"different s3 bucket values",
			TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"bucket": "different", "key": "bar", "region": "us-east-1"},
			},
			true,
		}, {
			"different gcs bucket values",
			TerraformBackend{
				Type:   "gcs",
				Config: map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			},
			RemoteState{
				Backend: "gcs",
				Config:  map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "different", "prefix": "bar"},
			},
			true,
		}, {
			"different s3 key values",
			TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"bucket": "foo", "key": "different", "region": "us-east-1"},
			},
			true,
		}, {
			"different gcs prefix values",
			TerraformBackend{
				Type:   "gcs",
				Config: map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			},
			RemoteState{
				Backend: "gcs",
				Config:  map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "different"},
			},
			true,
		}, {
			"different s3 region values",
			TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"bucket": "foo", "key": "bar", "region": "different"},
			},
			true,
		}, {
			"different gcs location values",
			TerraformBackend{
				Type:   "gcs",
				Config: map[string]interface{}{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			},
			RemoteState{
				Backend: "gcs",
				Config:  map[string]interface{}{"project": "foo-123456", "location": "different", "bucket": "foo", "prefix": "bar"},
			},
			true,
		},
		{
			"different boolean values and boolean conversion",
			TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"something": "true"},
			},
			RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"something": false},
			},
			true,
		},
		{
			"different gcs boolean values and boolean conversion",
			TerraformBackend{
				Type:   "gcs",
				Config: map[string]interface{}{"something": "true"},
			},
			RemoteState{
				Backend: "gcs",
				Config:  map[string]interface{}{"something": false},
			},
			true,
		},
		{
			"null values ignored",
			TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"something": "foo", "set-to-nil-should-be-ignored": nil},
			},
			RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"something": "foo"},
			},
			false,
		},
		{
			"gcs null values ignored",
			TerraformBackend{
				Type:   "gcs",
				Config: map[string]interface{}{"something": "foo", "set-to-nil-should-be-ignored": nil},
			},
			RemoteState{
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
			shouldOverride := testCase.stateFromConfig.differsFrom(&testCase.existingBackend, terragruntOptions)
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
