package remote

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// Configuration for Terraform remote state
type RemoteState struct {
	Backend string                 `hcl:"backend"`
	Config  map[string]interface{} `hcl:"config"`
}

func (remoteState *RemoteState) String() string {
	return fmt.Sprintf("RemoteState{Backend = %v, Config = %v}", remoteState.Backend, remoteState.Config)
}

type RemoteStateInitializer interface {
	// Return true if remote state needs to be initialized
	NeedsInitialization(config map[string]interface{}, terragruntOptions *options.TerragruntOptions) (bool, error)

	// Initialize the remote state
	Initialize(config map[string]interface{}, terragruntOptions *options.TerragruntOptions) error

	GetTerraformInitArgs(config map[string]interface{}) map[string]interface{}
}

// TODO: initialization actions for other remote state backends can be added here
var remoteStateInitializers = map[string]RemoteStateInitializer{
	"s3": S3Initializer{},
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
	terragruntOptions.Logger.Printf("Initializing remote state for the %s backend", remoteState.Backend)
	initializer, hasInitializer := remoteStateInitializers[remoteState.Backend]
	if hasInitializer {
		return initializer.Initialize(remoteState.Config, terragruntOptions)
	}

	return nil
}

// Returns true if remote state needs to be configured. This will be the case when:
//
// 1. Remote state has not already been configured
// 2. Remote state has been configured, but with a different configuration
// 3. The remote state initializer for this backend type, if there is one, says initialization is necessary
func (remoteState *RemoteState) NeedsInit(terragruntOptions *options.TerragruntOptions) (bool, error) {
	state, err := ParseTerraformStateFileFromLocation(remoteState.Backend, remoteState.Config, terragruntOptions.WorkingDir)
	if err != nil {
		return false, err
	}

	// Remote state not configured
	if state == nil {
		return true, nil
	}

	// Remote state configured, but with a different configuration
	if state.IsRemote() && remoteState.differsFrom(state.Backend, terragruntOptions) {
		return true, nil
	}

	// Remote state initializer says initialization is necessary
	initializer, hasInitializer := remoteStateInitializers[remoteState.Backend]
	if hasInitializer {
		return initializer.NeedsInitialization(remoteState.Config, terragruntOptions)
	}

	return false, nil
}

// Returns true if this remote state is different than the given remote state that is currently being used by terraform.
func (remoteState *RemoteState) differsFrom(existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) bool {
	if existingBackend.Type != remoteState.Backend {
		terragruntOptions.Logger.Printf("Backend type has changed from %s to %s", existingBackend.Type, remoteState.Backend)
		return true
	}

	// Terraform's `backend` configuration uses a boolean for the `encrypt` parameter. However, perhaps for backwards compatibility reasons,
	// Terraform stores that parameter as a string in the `terraform.tfstate` file. Therefore, we have to convert it accordingly, or `DeepEqual`
	// will fail.
	if util.KindOf(existingBackend.Config["encrypt"]) == reflect.String && util.KindOf(remoteState.Config["encrypt"]) == reflect.Bool {
		// If encrypt in remoteState is a bool and a string in existingBackend, DeepEqual will consider the maps to be different.
		// So we convert the value from string to bool to make them equivalent.
		if value, err := strconv.ParseBool(existingBackend.Config["encrypt"].(string)); err == nil {
			existingBackend.Config["encrypt"] = value
		} else {
			terragruntOptions.Logger.Printf("Remote state configuration encrypt contains invalid value %v, should be boolean.", existingBackend.Config["encrypt"])
		}
	}

	if !reflect.DeepEqual(existingBackend.Config, remoteState.Config) {
		terragruntOptions.Logger.Printf("Backend config has changed from %s to %s", existingBackend.Config, remoteState.Config)
		return true
	}

	terragruntOptions.Logger.Printf("Backend %s has not changed.", existingBackend.Type)
	return false
}

// Convert the RemoteState config into the format used by the terraform init command
func (remoteState RemoteState) ToTerraformInitArgs() []string {
	backendConfigArgs := []string{}

	initializer, hasInitializer := remoteStateInitializers[remoteState.Backend]
	if !hasInitializer {
		// return empty array
		return backendConfigArgs
	}

	var config = initializer.GetTerraformInitArgs(remoteState.Config)

	for key, value := range config {
		arg := fmt.Sprintf("-backend-config=%s=%v", key, value)
		backendConfigArgs = append(backendConfigArgs, arg)
	}

	return backendConfigArgs
}

var RemoteBackendMissing = fmt.Errorf("The remote_state.backend field cannot be empty")
