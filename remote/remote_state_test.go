package remote

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestToTerraformInitArgs(t *testing.T) {
	t.Parallel()

	remoteState := RemoteState{
		Backend: "s3",
		Config: map[string]interface{}{
			"encrypt": true,
			"bucket":  "my-bucket",
			"key":     "terraform.tfstate",
			"region":  "us-east-1",
		},
	}
	args := remoteState.ToTerraformInitArgs()

	assertTerraformInitArgsEqual(t, args, "-backend-config=encrypt=true -backend-config=bucket=my-bucket -backend-config=key=terraform.tfstate -backend-config=region=us-east-1")
}

func TestToTerraformInitArgsNoBackendConfigs(t *testing.T) {
	t.Parallel()

	remoteState := RemoteState{Backend: "s3"}
	args := remoteState.ToTerraformInitArgs()
	assert.Empty(t, args)
}

func TestDiffersFrom(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("remote_state_test")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	testCases := []struct {
		existingBackend TerraformBackend
		stateFromConfig RemoteState
		shouldOverride  bool
	}{
		{TerraformBackend{}, RemoteState{}, false},
		{TerraformBackend{Type: "s3"}, RemoteState{Backend: "s3"}, false},
		{TerraformBackend{Type: "s3"}, RemoteState{Backend: "atlas"}, true},
		{
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
			TerraformBackend{
				Type:   "s3",
				Config: map[string]interface{}{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			RemoteState{
				Backend: "s3",
				Config:  map[string]interface{}{"bucket": "foo", "key": "bar", "region": "different"},
			},
			true,
		},
	}

	for _, testCase := range testCases {
		shouldOverride := testCase.stateFromConfig.differsFrom(&testCase.existingBackend, terragruntOptions)
		assert.Equal(t, testCase.shouldOverride, shouldOverride, "Expect differsFrom to return %t but got %t for existingRemoteState %v and remoteStateFromTerragruntConfig %v", testCase.shouldOverride, shouldOverride, testCase.existingBackend, testCase.stateFromConfig)
	}
}

func assertTerraformInitArgsEqual(t *testing.T, actualArgs []string, expectedArgs string) {
	expected := strings.Split(expectedArgs, " ")
	assert.Len(t, actualArgs, len(expected))

	for _, expectedArg := range expected {
		assert.Contains(t, actualArgs, expectedArg)
	}
}
