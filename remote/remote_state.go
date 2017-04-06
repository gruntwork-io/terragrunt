package remote

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

// File name used to inject temporary configuration information to terraform
const temporaryConfigFile = "temp_terragrunt_config.tf"

// NewRemoteStateFormatTerraformVersion : The terraform version that implements new format for remote state
const NewRemoteStateFormatTerraformVersion = "0.9.0"

// TerraformVersion : Version of the terraform tool
var TerraformVersion string

// Configuration for Terraform remote state
type RemoteState struct {
	Backend string            `hcl:"backend"`
	Config  map[string]string `hcl:"config"`
}

func (state *RemoteState) String() string {
	return fmt.Sprintf("RemoteState{Backend = %v, Config = %v}", state.Backend, state.Config)
}

type RemoteStateInitializer func(map[string]string, *options.TerragruntOptions) error

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

	// Retreive the terraform version
	version, err := getTerraformVersion(terragruntOptions)
	if err != nil {
		return err
	}

	// Check if we should add a temporary config file
	if version >= NewRemoteStateFormatTerraformVersion {
		if err = remoteState.addTemporaryConfigFile(terragruntOptions); err != nil {
			return err
		}
	}

	if shouldConfigure {
		terragruntOptions.Logger.Printf("Initializing remote state for the %s backend", remoteState.Backend)
		if err = remoteState.Initialize(terragruntOptions); err != nil {
			return err
		}

		terragruntOptions.Logger.Printf("Configuring remote state for the %s backend", remoteState.Backend)

		options := []string{"init"}
		if version < NewRemoteStateFormatTerraformVersion {
			// Legacy remote state management (Before terraform v0.9.0)
			options = remoteState.toTerraformRemoteConfigArgs()
		}

		return shell.RunShellCommand(terragruntOptions, terragruntOptions.TerraformPath, options...)
	}

	return nil
}

// AddTemporaryConfigFile :
// Add a temporary .tf file that inject required terraform configuration for remote state
func (remoteState RemoteState) addTemporaryConfigFile(terragruntOptions *options.TerragruntOptions) error {
	// Inject a temporary file to configure the remote state backend
	var config string
	for key, value := range remoteState.Config {
		config += fmt.Sprintf("    %s = \"%s\"\n", key, value)
	}
	text := fmt.Sprintf("terraform {\n  backend \"%s\" {\n%s  }\n}\n", remoteState.Backend, config)
	outputFile, err := os.Create(temporaryConfigFile)
	if err != nil {
		return err
	}
	outputFile.WriteString(text)
	outputFile.Close()
	return nil
}

// RemoveTemporaryConfigFile :
// Erase the temporary file that stores the remote state configuration
// The error is ignored if the file no longer exists
func (remoteState RemoteState) RemoveTemporaryConfigFile() {
	os.Remove(temporaryConfigFile)
}

// Get the terraform version
func getTerraformVersion(terragruntOptions *options.TerragruntOptions) (version string, err error) {
	if TerraformVersion == "" {
		if out, err := exec.Command(terragruntOptions.TerraformPath, "version").Output(); err == nil {
			TerraformVersion = strings.Fields(string(out))[1][1:]
		}
	}
	version = TerraformVersion
	return
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
		return shouldOverrideExistingRemoteState(state, remoteStateFromTerragruntConfig, terragruntOptions)
	}
	return true, nil
}

// Check if the remote state that is already configured matches the one specified in the Terragrunt config. If it does,
// return false to indicate remote state does not need to be configured again. If it doesn't, prompt the user whether
// we should override the existing remote state setting.
func shouldOverrideExistingRemoteState(existingState *TerraformState, remoteStateFromTerragruntConfig RemoteState, terragruntOptions *options.TerragruntOptions) (bool, error) {
	version, err := getTerraformVersion(terragruntOptions)
	if err != nil {
		return false, err
	}

	if existingState.Terraform_Version != nil && *existingState.Terraform_Version < version {
		prompt := fmt.Sprintf("WARNING: Terraform remote state is already configured, but for an older version of terraform (v%s), you currently run (v%s). Overwrite?", *existingState.Terraform_Version, version)
		if answer, _ := shell.PromptUserForYesNo(prompt, terragruntOptions); answer {
			ReplaceRemoteStateFile()
		}
		return true, nil
	}

	if existingState.Remote.Type != remoteStateFromTerragruntConfig.Backend {
		prompt := fmt.Sprintf("WARNING: Terraform remote state is already configured, but for backend %s, whereas your Terragrunt configuration specifies %s. Overwrite?", existingState.Remote.Type, remoteStateFromTerragruntConfig.Backend)
		return shell.PromptUserForYesNo(prompt, terragruntOptions)
	}

	if !reflect.DeepEqual(existingState.Remote.Config, remoteStateFromTerragruntConfig.Config) {
		prompt := fmt.Sprintf("WARNING: Terraform remote state is already configured for backend %s with config %v, but your Terragrunt configuration specifies config %v. Overwrite?", existingState.Remote.Type, existingState.Remote.Config, remoteStateFromTerragruntConfig.Config)
		return shell.PromptUserForYesNo(prompt, terragruntOptions)
	}

	terragruntOptions.Logger.Printf("Remote state is already configured for backend %s", existingState.Remote.Type)
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
