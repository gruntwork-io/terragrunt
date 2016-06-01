package remote

import (
	"encoding/json"
	"io/ioutil"
	"github.com/gruntwork-io/terragrunt/errors"
	"fmt"
	"github.com/gruntwork-io/terragrunt/util"
)

// TODO: this file could be changed to use the Terraform Go code to read state files, but that code is relatively
// complicated and doesn't seem to be designed for standalone use. Fortunately, the .tfstate format is a fairly simple
// JSON format, so hopefully this simple parsing code will not be a maintenance burden.

// When storing Terraform state locally, this is the default path to the tfstate file
const DEFAULT_PATH_TO_LOCAL_STATE_FILE = "terraform.tfstate"

// When using remote state storage, Terraform keeps a local copy of the state file in this folder
const DEFAULT_PATH_TO_REMOTE_STATE_FILE = ".terraform/terraform.tfstate"

// The structure of the Terraform .tfstate file
type TerraformState struct {
	Version int
	Serial  int
	Remote  *TerraformStateRemote
	Modules []TerraformStateModule
}

// The structure of the "remote" section of the Terraform .tfstate file
type TerraformStateRemote struct {
	Type   string
	Config map[string]interface{}
}

// The structure of a "module" section of the Terraform .tfstate file
type TerraformStateModule struct {
	Path      []string
	Outputs   map[string]interface{}
	Resources map[string]interface{}
}

// Return true if this Terraform state is configured for remote state storage
func (state *TerraformState) IsRemote() bool {
	return state.Remote != nil
}

// Parse the Terraform .tfstate file from its default locations. If the file doesn't exist at any of the default
// locations, return nil.
func ParseTerraformStateFileFromDefaultLocations() (*TerraformState, error) {
	if util.FileExists(DEFAULT_PATH_TO_LOCAL_STATE_FILE) {
		return ParseTerraformStateFile(DEFAULT_PATH_TO_LOCAL_STATE_FILE)
	} else if util.FileExists(DEFAULT_PATH_TO_REMOTE_STATE_FILE) {
		return ParseTerraformStateFile(DEFAULT_PATH_TO_REMOTE_STATE_FILE)
	} else {
		return nil, nil
	}
}

// Parse the Terraform .tfstate file at the given path
func ParseTerraformStateFile(path string) (*TerraformState, error) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.WithStackTrace(CantParseTerraformStateFile{Path: path, UnderlyingErr: err})
	}

	return parseTerraformState(bytes)
}

// Parse the Terraform state file data in the given byte slice
func parseTerraformState(terraformStateData []byte) (*TerraformState, error) {
	terraformState := &TerraformState{}

	if err := json.Unmarshal(terraformStateData, terraformState); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return terraformState, nil
}

type CantParseTerraformStateFile struct {
	Path          string
	UnderlyingErr error
}

func (err CantParseTerraformStateFile) Error() string {
	return fmt.Sprintf("Error parsing Terraform state file %s: %s", err.Path, err.UnderlyingErr.Error())
}