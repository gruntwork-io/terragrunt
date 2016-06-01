package remote

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"strings"
)

func TestToTerraformRemoteConfigArgs(t *testing.T) {
	t.Parallel()

	remoteState := RemoteState{
		Backend: "s3",
		BackendConfigs: map[string]string {
			"encrypted": "true",
			"bucket": "my-bucket",
			"key": "terraform.tfstate",
			"region": "us-east-1",
		},
	}
	args := remoteState.toTerraformRemoteConfigArgs()

	assertRemoteConfigArgsEqual(t, args, "remote config -backend s3 -backend-config=encrypted=true -backend-config=bucket=my-bucket -backend-config=key=terraform.tfstate -backend-config=region=us-east-1")
}

func TestToTerraformRemoteConfigArgsNoBackendConfigs(t *testing.T) {
	t.Parallel()

	remoteState := RemoteState{Backend: "s3"}
	args := remoteState.toTerraformRemoteConfigArgs()

	assertRemoteConfigArgsEqual(t, args, "remote config -backend s3")
}

func assertRemoteConfigArgsEqual(t *testing.T, actualArgs []string, expectedArgs string) {
	expected := strings.Split(expectedArgs, " ")
	assert.Len(t, actualArgs, len(expected))

	for _, expectedArg := range expected {
		assert.Contains(t, actualArgs, expectedArg)
	}
}