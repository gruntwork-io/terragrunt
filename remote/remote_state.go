package remote

import (
	"fmt"
	"reflect"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

// Configuration for Terraform remote state
type RemoteState struct {
	Backend     string
	DisableInit bool
	Config      map[string]interface{}
}

func (remoteState *RemoteState) String() string {
	return fmt.Sprintf("RemoteState{Backend = %v, DisableInit = %v, Config = %v}", remoteState.Backend, remoteState.DisableInit, remoteState.Config)
}

type RemoteStateInitializer interface {
	// Return true if remote state needs to be initialized
	NeedsInitialization(remoteState *RemoteState, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) (bool, error)

	// Initialize the remote state
	Initialize(remoteState *RemoteState, terragruntOptions *options.TerragruntOptions) error

	// Return the config that should be passed on to terraform via -backend-config cmd line param
	// Allows the Backends to filter and/or modify the configuration given from the user
	GetTerraformInitArgs(config map[string]interface{}) map[string]interface{}
}

// TODO: initialization actions for other remote state backends can be added here
var remoteStateInitializers = map[string]RemoteStateInitializer{
	"s3":  S3Initializer{},
	"gcs": GCSInitializer{},
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
// using S3 or GCS for remote state storage, this may create the bucket if it doesn't exist already.
func (remoteState *RemoteState) Initialize(terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Initializing remote state for the %s backend", remoteState.Backend)
	initializer, hasInitializer := remoteStateInitializers[remoteState.Backend]
	if hasInitializer {
		return initializer.Initialize(remoteState, terragruntOptions)
	}

	return nil
}

// Returns true if remote state needs to be configured. This will be the case when:
//
// 1. Remote state has not already been configured
// 2. Remote state has been configured, but with a different configuration
// 3. The remote state initializer for this backend type, if there is one, says initialization is necessary
func (remoteState *RemoteState) NeedsInit(terragruntOptions *options.TerragruntOptions) (bool, error) {
	state, err := ParseTerraformStateFileFromLocation(remoteState.Backend, remoteState.Config, terragruntOptions.WorkingDir, terragruntOptions.DataDir())
	if err != nil {
		return false, err
	}

	if remoteState.DisableInit {
		return false, nil
	}

	// Remote state not configured
	if state == nil {
		return true, nil
	}

	if initializer, hasInitializer := remoteStateInitializers[remoteState.Backend]; hasInitializer {
		// Remote state initializer says initialization is necessary
		return initializer.NeedsInitialization(remoteState, state.Backend, terragruntOptions)
	} else if state.IsRemote() && remoteState.differsFrom(state.Backend, terragruntOptions) {
		// If there's no remote state initializer, then just compare the the config values
		return true, nil
	}

	return false, nil
}

// Returns true if this remote state is different than the given remote state that is currently being used by terraform.
func (remoteState *RemoteState) differsFrom(existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) bool {
	if existingBackend.Type != remoteState.Backend {
		terragruntOptions.Logger.Printf("Backend type has changed from %s to %s", existingBackend.Type, remoteState.Backend)
		return true
	}

	if !terraformStateConfigEqual(existingBackend.Config, remoteState.Config) {
		terragruntOptions.Logger.Printf("Backend config has changed from %s to %s", existingBackend.Config, remoteState.Config)
		return true
	}

	terragruntOptions.Logger.Printf("Backend %s has not changed.", existingBackend.Type)
	return false
}

// Return true if the existing config from a .tfstate file is equal to the new config from the user's backend
// configuration. Under the hood, this method does a reflect.DeepEqual check, but with one twist: we strip out any
// null values in the existing config. This is because Terraform >= 0.12 stores ALL possible keys for a given backend
// in the .tfstate file, even if the user hasn't configured that key, in which case the value will be null, and cause
// reflect.DeepEqual to fail.
func terraformStateConfigEqual(existingConfig map[string]interface{}, newConfig map[string]interface{}) bool {
	if existingConfig == nil {
		return newConfig == nil
	}

	existingConfigNonNil := map[string]interface{}{}
	for existingKey, existingValue := range existingConfig {
		_, newValueIsSet := newConfig[existingKey]
		if existingValue == nil && !newValueIsSet {
			continue
		}
		existingConfigNonNil[existingKey] = existingValue
	}

	return reflect.DeepEqual(existingConfigNonNil, newConfig)
}

// Convert the RemoteState config into the format used by the terraform init command
func (remoteState RemoteState) ToTerraformInitArgs() []string {
	config := remoteState.Config
	if remoteState.DisableInit {
		return []string{"-backend=false"}
	}

	initializer, hasInitializer := remoteStateInitializers[remoteState.Backend]
	if hasInitializer {
		// get modified config from backend, if backend exists
		config = initializer.GetTerraformInitArgs(remoteState.Config)
	}

	var backendConfigArgs []string = nil

	for key, value := range config {
		arg := fmt.Sprintf("-backend-config=%s=%v", key, value)
		backendConfigArgs = append(backendConfigArgs, arg)
	}

	return backendConfigArgs
}

var RemoteBackendMissing = fmt.Errorf("The remote_state.backend field cannot be empty")
