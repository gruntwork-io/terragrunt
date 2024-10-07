// Package remote contains code for configuring remote state storage.
package remote

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const initializedRemoteStateCacheName = "initializedRemoteStateCache"

// RemoteState is the configuration for Terraform remote state
// NOTE: If any attributes are added here, be sure to add it to remoteStateAsCty in config/config_as_cty.go
type RemoteState struct {
	Backend                       string                 `mapstructure:"backend" json:"Backend"`
	DisableInit                   bool                   `mapstructure:"disable_init" json:"DisableInit"`
	DisableDependencyOptimization bool                   `mapstructure:"disable_dependency_optimization" json:"DisableDependencyOptimization"`
	Generate                      *RemoteStateGenerate   `mapstructure:"generate" json:"Generate"`
	Config                        map[string]interface{} `mapstructure:"config" json:"Config"`
}

// map to store mutexes for each state bucket action
type stateAccess struct {
	mapAccess   sync.Mutex
	bucketLocks map[string]*sync.Mutex
}

var stateAccessLock = newStateAccess()

// initializedRemoteStateCache is a cache to store the result of a remote state initialization check.
// This is used to avoid checking to see if remote state needs to be initialized multiple times.
var initializedRemoteStateCache = cache.NewCache[bool](initializedRemoteStateCacheName)

func (state *RemoteState) String() string {
	return fmt.Sprintf(
		"RemoteState{Backend = %v, DisableInit = %v, DisableDependencyOptimization = %v, Generate = %v, Config = %v}",
		state.Backend,
		state.DisableInit,
		state.DisableDependencyOptimization,
		state.Generate,
		state.Config,
	)
}

// RemoteStateGenerate is code gen configuration for Terraform remote state.
type RemoteStateGenerate struct {
	Path     string `cty:"path" mapstructure:"path"`
	IfExists string `cty:"if_exists" mapstructure:"if_exists"`
}

type RemoteStateInitializer interface {
	// Return true if remote state needs to be initialized
	NeedsInitialization(remoteState *RemoteState, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) (bool, error)

	// Initialize the remote state
	Initialize(ctx context.Context, remoteState *RemoteState, terragruntOptions *options.TerragruntOptions) error

	// Return the config that should be passed on to terraform via -backend-config cmd line param
	// Allows the Backends to filter and/or modify the configuration given from the user
	GetTerraformInitArgs(config map[string]interface{}) map[string]interface{}
}

// TODO: initialization actions for other remote state backends can be added here
var remoteStateInitializers = map[string]RemoteStateInitializer{
	"s3":  S3Initializer{},
	"gcs": GCSInitializer{},
}

// FillDefaults fills in any default configuration for remote state
func (state *RemoteState) FillDefaults() {
	// Nothing to do
}

// Validate that the remote state is configured correctly
func (state *RemoteState) Validate() error {
	if state.Backend == "" {
		return errors.New(ErrRemoteBackendMissing)
	}

	return nil
}

// Initialize performs any actions necessary to initialize the remote state before it's used for storage. For example, if you're
// using S3 or GCS for remote state storage, this may create the bucket if it doesn't exist already.
func (state *RemoteState) Initialize(ctx context.Context, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Initializing remote state for the %s backend", state.Backend)

	initializer, hasInitializer := remoteStateInitializers[state.Backend]
	if hasInitializer {
		return initializer.Initialize(ctx, state, terragruntOptions)
	}

	return nil
}

// NeedsInit returns true if remote state needs to be configured. This will be the case when:
//
// 1. Remote state auto-initialization has been disabled
// 2. Remote state has not already been configured
// 3. Remote state has been configured, but with a different configuration
// 4. The remote state initializer for this backend type, if there is one, says initialization is necessary
func (state *RemoteState) NeedsInit(terragruntOptions *options.TerragruntOptions) (bool, error) {
	if terragruntOptions.DisableBucketUpdate {
		terragruntOptions.Logger.Debugf("Skipping remote state initialization due to %s flag", commands.TerragruntDisableBucketUpdateFlagName)
		return false, nil
	}

	parsedState, err := ParseTerraformStateFileFromLocation(state.Backend, state.Config, terragruntOptions.WorkingDir, terragruntOptions.DataDir())
	if err != nil {
		return false, err
	}

	if state.DisableInit {
		return false, nil
	}

	// Remote state not configured
	if parsedState == nil {
		return true, nil
	}

	if initializer, hasInitializer := remoteStateInitializers[state.Backend]; hasInitializer {
		// Remote state initializer says initialization is necessary
		return initializer.NeedsInitialization(state, parsedState.Backend, terragruntOptions)
	} else if parsedState.IsRemote() && state.DiffersFrom(parsedState.Backend, terragruntOptions) {
		// If there's no remote state initializer, then just compare the the config values
		return true, nil
	}

	return false, nil
}

// DiffersFrom returns true if this remote state is different than
// the given remote state that is currently being used by terraform.
func (state *RemoteState) DiffersFrom(existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) bool {
	if existingBackend.Type != state.Backend {
		terragruntOptions.Logger.Infof("Backend type has changed from %s to %s", existingBackend.Type, state.Backend)
		return true
	}

	if !terraformStateConfigEqual(existingBackend.Config, state.Config) {
		terragruntOptions.Logger.Debugf("Changed from %s to %s", existingBackend.Config, state.Config)
		return true
	}

	terragruntOptions.Logger.Debugf("Backend %s has not changed.", existingBackend.Type)

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

	existingConfigNonNil := copyExistingNotNullValues(existingConfig, newConfig)

	return reflect.DeepEqual(existingConfigNonNil, newConfig)
}

// Copy the non-nil values from the existingMap to a new map
func copyExistingNotNullValues(existingMap map[string]interface{}, newMap map[string]interface{}) map[string]interface{} {
	existingConfigNonNil := map[string]interface{}{}

	for existingKey, existingValue := range existingMap {
		newValue, newValueIsSet := newMap[existingKey]
		if existingValue == nil && !newValueIsSet {
			continue
		}
		// if newValue and existingValue are both maps, we need to recursively copy the non-nil values
		if existingValueMap, existingValueIsMap := existingValue.(map[string]interface{}); existingValueIsMap {
			if newValueMap, newValueIsMap := newValue.(map[string]interface{}); newValueIsMap {
				existingValue = copyExistingNotNullValues(existingValueMap, newValueMap)
			}
		}

		existingConfigNonNil[existingKey] = existingValue
	}

	return existingConfigNonNil
}

// ToTerraformInitArgs converts the RemoteState config into the
// format used by the terraform init command
func (state RemoteState) ToTerraformInitArgs() []string {
	config := state.Config

	if state.DisableInit {
		return []string{"-backend=false"}
	}

	if state.Generate != nil {
		// When in generate mode, we don't need to use `-backend-config` to initialize the remote state backend
		return []string{}
	}

	initializer, hasInitializer := remoteStateInitializers[state.Backend]
	if hasInitializer {
		// get modified config from backend, if backend exists
		config = initializer.GetTerraformInitArgs(state.Config)
	}

	var backendConfigArgs = make([]string, 0, len(config))

	for key, value := range config {
		arg := fmt.Sprintf("-backend-config=%s=%v", key, value)
		backendConfigArgs = append(backendConfigArgs, arg)
	}

	return backendConfigArgs
}

// GenerateTerraformCode generates the terraform code for configuring remote state backend.
func (state *RemoteState) GenerateTerraformCode(terragruntOptions *options.TerragruntOptions) error {
	if state.Generate == nil {
		return errors.New(ErrGenerateCalledWithNoGenerateAttr)
	}

	// Make sure to strip out terragrunt specific configurations from the config.
	config := state.Config

	initializer, hasInitializer := remoteStateInitializers[state.Backend]
	if hasInitializer {
		config = initializer.GetTerraformInitArgs(config)
	}

	// Convert the IfExists setting to the internal enum representation before calling generate.
	ifExistsEnum, err := codegen.GenerateConfigExistsFromString(state.Generate.IfExists)
	if err != nil {
		return err
	}

	configBytes, err := codegen.RemoteStateConfigToTerraformCode(state.Backend, config)
	if err != nil {
		return err
	}

	codegenConfig := codegen.GenerateConfig{
		Path:          state.Generate.Path,
		IfExists:      ifExistsEnum,
		IfExistsStr:   state.Generate.IfExists,
		Contents:      string(configBytes),
		CommentPrefix: codegen.DefaultCommentPrefix,
	}

	return codegen.WriteToFile(terragruntOptions, terragruntOptions.WorkingDir, codegenConfig)
}

// Custom errors
var (
	ErrRemoteBackendMissing             = errors.New("the remote_state.backend field cannot be empty")
	ErrGenerateCalledWithNoGenerateAttr = errors.New("generate code routine called when no generate attribute is configured")
)

type BucketCreationNotAllowed string

func (bucketName BucketCreationNotAllowed) Error() string {
	return fmt.Sprintf("Creation of remote state bucket %s is not allowed", string(bucketName))
}

func newStateAccess() *stateAccess {
	return &stateAccess{
		bucketLocks: make(map[string]*sync.Mutex),
	}
}

// fetchMutex - fetch mutex for specific bucket.
func (locks *stateAccess) fetchMutex(bucket string) *sync.Mutex {
	// only one go routine can access mutex map.
	locks.mapAccess.Lock()
	defer locks.mapAccess.Unlock()

	if locks.bucketLocks == nil {
		locks.bucketLocks = make(map[string]*sync.Mutex)
	}

	if _, ok := locks.bucketLocks[bucket]; !ok {
		locks.bucketLocks[bucket] = &sync.Mutex{}
	}

	return locks.bucketLocks[bucket]
}

// StateBucketUpdate - run state bucket initialization logic, maintaining a single logic execution per bucket simultaneously.
func (locks *stateAccess) StateBucketUpdate(bucket string, logic func() error) error {
	mutex := locks.fetchMutex(bucket)

	mutex.Lock()
	defer mutex.Unlock()

	return logic()
}
