package remote

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"reflect"
	"strconv"
)

// Configuration for Terraform remote state
type RemoteState struct {
	Backend string                 `hcl:"backend"`
	Config  map[string]interface{} `hcl:"config"`
}

func (state *RemoteState) String() string {
	return fmt.Sprintf("RemoteState{Backend = %v, Config = %v}", state.Backend, state.Config)
}

type RemoteStateInitializer func(map[string]interface{}, *options.TerragruntOptions) error

// TODO: initialization actions for other remote state backends can be added here
var remoteStateInitializers = map[string]RemoteStateInitializer{
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
		return shell.RunTerraformCommandAndRedirectOutputToLogger(terragruntOptions, initCommand(remoteState)...)
	}

	return nil
}

// Returns true if remote state needs to be configured. This will be the case when:
//
// 1. Remote state has not already been configured
// 2. Remote state has been configured, but for a different backend type, and the user confirms it's OK to overwrite it.
func shouldConfigureRemoteState(remoteStateFromTerragruntConfig RemoteState, terragruntOptions *options.TerragruntOptions) (bool, error) {
	state, err := ParseTerraformStateFileFromLocation(terragruntOptions.WorkingDir)
	if err != nil {
		return false, err
	}

	if state != nil && state.IsRemote() {
		return shouldOverrideExistingRemoteState(state.Backend, remoteStateFromTerragruntConfig, terragruntOptions)
	} else {
		return true, nil
	}
}

// Check if the remote state that is already configured matches the one specified in the Terragrunt config. If it does,
// return false to indicate remote state does not need to be configured again. If it doesn't, prompt the user whether
// we should override the existing remote state setting.
func shouldOverrideExistingRemoteState(existingBackend *TerraformBackend, remoteStateFromTerragruntConfig RemoteState, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if existingBackend.Type != remoteStateFromTerragruntConfig.Backend {
		prompt := fmt.Sprintf("WARNING: Terraform remote state is already configured, but for backend %s, whereas your Terragrunt configuration specifies %s. Overwrite?", existingBackend.Type, remoteStateFromTerragruntConfig.Backend)
		return shell.PromptUserForYesNo(prompt, terragruntOptions)
	}

	// Terraform's `backend` configuration uses a boolean for the `encrypt` parameter. However, perhaps for backwards compatibility reasons,
	// Terraform stores that parameter as a string in the `terraform.tfstate` file. Therefore, we have to convert it accordingly, or `DeepEqual`
	// will fail.
	if util.KindOf(existingBackend.Config["encrypt"]) == reflect.String && util.KindOf(remoteStateFromTerragruntConfig.Config["encrypt"]) == reflect.Bool {
		// If encrypt in remoteStateFromTerragruntConfig is a bool and a string in existingBackend, DeepEqual will consider the maps to be different.
		// So we convert the value from string to bool to make them equivalent.
		if value, err := strconv.ParseBool(existingBackend.Config["encrypt"].(string)); err == nil {
			existingBackend.Config["encrypt"] = value
		} else {
			terragruntOptions.Logger.Printf("Remote state configuration encrypt contains invalid value %v, should be boolean.", existingBackend.Config["encrypt"])
		}
	}

	if !reflect.DeepEqual(existingBackend.Config, remoteStateFromTerragruntConfig.Config) {
		prompt := fmt.Sprintf("WARNING: Terraform remote state is already configured for backend %s with config %v, but your Terragrunt configuration specifies config %v. Overwrite?", existingBackend.Type, existingBackend.Config, remoteStateFromTerragruntConfig.Config)
		return shell.PromptUserForYesNo(prompt, terragruntOptions)
	}

	terragruntOptions.Logger.Printf("Remote state is already configured for backend %s", existingBackend.Type)
	return false, nil
}

func initCommand(remoteState RemoteState) []string {
	return append([]string{"init"}, remoteState.ToTerraformInitArgs()...)
}

// Convert the RemoteState config into the format used by the terraform init command
func (remoteState RemoteState) ToTerraformInitArgs() []string {
	backendConfigArgs := []string{}
	for key, value := range remoteState.Config {
		arg := fmt.Sprintf("-backend-config=%s=%v", key, value)
		backendConfigArgs = append(backendConfigArgs, arg)
	}

	return backendConfigArgs
}

var RemoteBackendMissing = fmt.Errorf("The remote_state.backend field cannot be empty")
