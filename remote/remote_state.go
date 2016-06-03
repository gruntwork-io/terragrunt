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
	shouldConfigure, err := shouldConfigureRemoteState(remoteState)
	if err != nil {
		return err
	}

	if shouldConfigure {
		util.Logger.Printf("Configuring remote state for the %s backend", remoteState.Backend)
		return shell.RunShellCommand("terraform", remoteState.toTerraformRemoteConfigArgs()...)
	}

	return nil
}

// Returns true if remote state needs to be configured. This will be the case when:
//
// 1. Remote state has not already been configured
// 2. Remote state has been configured, but for a different backend type, and the user confirms it's OK to overwrite it.
func shouldConfigureRemoteState(remoteStateFromTerragruntConfig RemoteState) (bool, error) {
	state, err := ParseTerraformStateFileFromDefaultLocations()
	if err != nil {
		return false, err
	}

	if state.IsRemote() {
		return shouldOverrideExistingRemoteState(state.Remote, remoteStateFromTerragruntConfig)
	} else {
		return true, nil
	}
}

// Check if the remote state that is already configured matches the one specified in the Terragrunt config. If it does,
// return false to indicate remote state does not need to be configured again. If it doesn't, prompt the user whether
// we should override the existing remote state setting.
func shouldOverrideExistingRemoteState(existingRemoteState *TerraformStateRemote, remoteStateFromTerragruntConfig RemoteState) (bool, error) {
	if existingRemoteState.Type == remoteStateFromTerragruntConfig.Backend {
		util.Logger.Printf("Remote state is already configured for backend %s", existingRemoteState.Type)
		return false, nil
	} else {
		return shell.PromptUserForYesNo(fmt.Sprintf("WARNING: Terraform remote state is already configured, but for backend %s, whereas your Terragrunt configuration specifies %s. Overwrite?", existingRemoteState.Type, remoteStateFromTerragruntConfig.Backend))
	}
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