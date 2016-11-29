package remote

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/gruntwork-io/terragrunt/options"
)

func TestToTerraformRemoteConfigArgs(t *testing.T) {
	t.Parallel()

	remoteState := RemoteState{
		Backend: "s3",
		Config: map[string]string{
			"encrypt": "true",
			"bucket":  "my-bucket",
			"key":     "terraform.tfstate",
			"region":  "us-east-1",
		},
	}
	args := remoteState.toTerraformRemoteConfigArgs()

	assertRemoteConfigArgsEqual(t, args, "remote config -backend s3 -backend-config=encrypt=true -backend-config=bucket=my-bucket -backend-config=key=terraform.tfstate -backend-config=region=us-east-1")
}

func TestToTerraformRemoteConfigArgsNoBackendConfigs(t *testing.T) {
	t.Parallel()

	remoteState := RemoteState{Backend: "s3"}
	args := remoteState.toTerraformRemoteConfigArgs()

	assertRemoteConfigArgsEqual(t, args, "remote config -backend s3")
}

func TestShouldOverrideExistingRemoteState(t *testing.T) {
	t.Parallel()

	terragruntOptions := options.NewTerragruntOptionsForTest("remote_state_test")

	testCases := []struct {
		existingState   TerraformStateRemote
		stateFromConfig RemoteState
		shouldOverride  bool
	}{
		{TerraformStateRemote{}, RemoteState{}, false},
		{TerraformStateRemote{Type: "s3"}, RemoteState{Backend: "s3"}, false},
		{TerraformStateRemote{Type: "s3"}, RemoteState{Backend: "atlas"}, true},
		{
			TerraformStateRemote{
				Type: "s3",
				Config: map[string]string{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			RemoteState{
				Backend: "s3",
				Config: map[string]string{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			false,
		},{
			TerraformStateRemote{
				Type: "s3",
				Config: map[string]string{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			RemoteState{
				Backend: "s3",
				Config: map[string]string{"bucket": "different", "key": "bar", "region": "us-east-1"},
			},
			true,
		},{
			TerraformStateRemote{
				Type: "s3",
				Config: map[string]string{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			RemoteState{
				Backend: "s3",
				Config: map[string]string{"bucket": "foo", "key": "different", "region": "us-east-1"},
			},
			true,
		},{
			TerraformStateRemote{
				Type: "s3",
				Config: map[string]string{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			},
			RemoteState{
				Backend: "s3",
				Config: map[string]string{"bucket": "foo", "key": "bar", "region": "different"},
			},
			true,
		},
	}

	for _, testCase := range testCases {
		shouldOverride, err := shouldOverrideExistingRemoteState(&testCase.existingState, testCase.stateFromConfig, terragruntOptions)
		assert.Nil(t, err, "Unexpected error: %v", err)
		assert.Equal(t, testCase.shouldOverride, shouldOverride, "Expect shouldOverrideExistingRemoteState to return %t but got %t for existingRemoteState %v and remoteStateFromTerragruntConfig %v", testCase.shouldOverride, shouldOverride, testCase.existingState, testCase.stateFromConfig)
	}
}

func assertRemoteConfigArgsEqual(t *testing.T, actualArgs []string, expectedArgs string) {
	expected := strings.Split(expectedArgs, " ")
	assert.Len(t, actualArgs, len(expected))

	for _, expectedArg := range expected {
		assert.Contains(t, actualArgs, expectedArg)
	}
}
