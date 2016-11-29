package remote

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/options"
	"reflect"
)

// Configuration for Terraform remote state
type RemoteState struct {
	Backend string            `hcl:"backend"`
	Config  map[string]string `hcl:"config"`
}

type RemoteStateInitializer func(map[string]string, *options.TerragruntOptions) error

// TODO: initialization actions for other remote state backends can be added here
var remoteStateInitializers = map[string]RemoteStateInitializer {
	"s3": InitializeRemoteStateS3,
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

	return nil
}

// Perform any actions necessary to initialize the remote state before it's used for storage. For example, if you're
// using S3 for remote state storage, this may create the S3 bucket if it doesn't exist already.
func (remoteState *RemoteState) Initialize(terragruntOptions *options.TerragruntOptions) error {
	initializer, hasInitializer := remoteStateInitializers[remoteState.Backend]
	if hasInitializer {
		return initializer(remoteState.Config, terragruntOptions)
	}

	return nil
}

// Configure Terraform remote state
func (remoteState RemoteState) ConfigureRemoteState(terragruntOptions *options.TerragruntOptions) error {
	shouldConfigure, err := shouldConfigureRemoteState(remoteState, terragruntOptions)
	if err != nil {
		return err
	}

	if shouldConfigure {
		terragruntOptions.Logger.Printf("Initializing remote state for the %s backend", remoteState.Backend)
		if err := remoteState.Initialize(terragruntOptions); err != nil {
			return err
		}

		terragruntOptions.Logger.Printf("Configuring remote state for the %s backend", remoteState.Backend)
		return shell.RunShellCommand(terragruntOptions, "terraform", remoteState.toTerraformRemoteConfigArgs()...)
	}

	return nil
}

// Returns true if remote state needs to be configured. This will be the case when:
//
// 1. Remote state has not already been configured
// 2. Remote state has been configured, but for a different backend type, and the user confirms it's OK to overwrite it.
func shouldConfigureRemoteState(remoteStateFromTerragruntConfig RemoteState, terragruntOptions *options.TerragruntOptions) (bool, error) {
	state, err := ParseTerraformStateFileFromDefaultLocations()
	if err != nil {
		return false, err
	}

	if state != nil && state.IsRemote() {
		return shouldOverrideExistingRemoteState(state.Remote, remoteStateFromTerragruntConfig, terragruntOptions)
	} else {
		return true, nil
	}
}

// Check if the remote state that is already configured matches the one specified in the Terragrunt config. If it does,
// return false to indicate remote state does not need to be configured again. If it doesn't, prompt the user whether
// we should override the existing remote state setting.
func shouldOverrideExistingRemoteState(existingRemoteState *TerraformStateRemote, remoteStateFromTerragruntConfig RemoteState, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if existingRemoteState.Type != remoteStateFromTerragruntConfig.Backend {
		prompt := fmt.Sprintf("WARNING: Terraform remote state is already configured, but for backend %s, whereas your .terragrunt file specifies %s. Overwrite?", existingRemoteState.Type, remoteStateFromTerragruntConfig.Backend)
		return shell.PromptUserForYesNo(prompt, terragruntOptions)
	}

	if !reflect.DeepEqual(existingRemoteState.Config, remoteStateFromTerragruntConfig.Config) {
		prompt := fmt.Sprintf("WARNING: Terraform remote state is already configured for backend %s with config %v, but your .terragrunt file specifies config %v. Overwrite?", existingRemoteState.Type, existingRemoteState.Config, remoteStateFromTerragruntConfig.Config)
		return shell.PromptUserForYesNo(prompt, terragruntOptions)
	}

	terragruntOptions.Logger.Printf("Remote state is already configured for backend %s", existingRemoteState.Type)
	return false, nil
}

// Convert the RemoteState config into the format used by Terraform
func (remoteState RemoteState) toTerraformRemoteConfigArgs() []string {
	baseArgs := []string{"remote", "config", "-backend", remoteState.Backend}

	backendConfigArgs := []string{}
	for key, value := range remoteState.Config {
		arg := fmt.Sprintf("-backend-config=%s=%s", key, value)
		backendConfigArgs = append(backendConfigArgs, arg)
	}

	return append(baseArgs, backendConfigArgs...)
}

var RemoteBackendMissing = fmt.Errorf("The remoteState.backend field cannot be empty")
