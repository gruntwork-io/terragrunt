package remote

import (
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/shell"
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
)

// Configuration for Terraform remote state
type RemoteState struct {
	Backend        string
	BackendConfigs map[string]string
}

// Fill in any default configuration for remote state
func (remoteState *RemoteState) FillDefaults() {
	// Nothing to do
}

// Validate that the remote state is configured correctly
func (remoteState *RemoteState) Validate() error {
	if remoteState.Backend == "" {
		return errors.WithStackTrace(RemoteBackendMissing)
	}

	// TODO: for the S3 backend, check that encryption is enabled
	// TODO: for the S3 backend, use the AWS API to verify the S3 bucket has versioning enabled

	return nil
}

// Configure Terraform remote state
func (remoteState RemoteState) ConfigureRemoteState() error {
	// TODO: skip this step if the tfstate shows that remote state is *already* configured
	util.Logger.Printf("Configuring remote state for the %s backend", remoteState.Backend)
	return shell.RunShellCommand("terraform", remoteState.toTerraformRemoteConfigArgs()...)

	return nil
}

// Convert the RemoteState config into the format used by Terraform
func (remoteState RemoteState) toTerraformRemoteConfigArgs() []string {
	baseArgs := []string{"remote", "config", "-backend", remoteState.Backend}

	backendConfigArgs := []string{}
	for key, value := range remoteState.BackendConfigs {
		arg := fmt.Sprintf("-backend-config=%s=%s", key, value)
		backendConfigArgs = append(backendConfigArgs, arg)
	}

	return append(baseArgs, backendConfigArgs...)
}

var RemoteBackendMissing = fmt.Errorf("The remoteState.backend field cannot be empty")