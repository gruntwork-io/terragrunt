package remote

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

// TODO: this file could be changed to use the Terraform Go code to read state files, but that code is relatively
// complicated and doesn't seem to be designed for standalone use. Fortunately, the .tfstate format is a fairly simple
// JSON format, so hopefully this simple parsing code will not be a maintenance burden.

// When storing Terraform state locally, this is the default path to the tfstate file
const DEFAULT_PATH_TO_LOCAL_STATE_FILE = "terraform.tfstate"

// When using remote state storage, Terraform keeps a local copy of the state file in this folder
const DEFAULT_PATH_TO_REMOTE_STATE_FILE = "terraform.tfstate"

// The structure of the Terraform .tfstate file
type TerraformState struct {
	Version int
	Serial  int
	Backend *TerraformBackend
	Modules []TerraformStateModule
}

// The structure of the "backend" section of the Terraform .tfstate file
type TerraformBackend struct {
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
	return state.Backend != nil && state.Backend.Type != "local"
}

// Parses the Terraform .tfstate file. If a local backend is used then search the given path, or
// return nil if the file is missing. If the backend is not local then parse the Terraform .tfstate
// file from the location specified by workingDir. If no location is specified, search the current
// directory. If the file doesn't exist at any of the default locations, return nil.
func ParseTerraformStateFileFromLocation(backend string, config map[string]interface{}, workingDir, dataDir string) (*TerraformState, error) {
	stateFile, ok := config["path"].(string)
	if backend == "local" && ok && util.FileExists(stateFile) {
		return ParseTerraformStateFile(stateFile)
	} else if util.FileExists(util.JoinPath(dataDir, DEFAULT_PATH_TO_REMOTE_STATE_FILE)) {
		return ParseTerraformStateFile(util.JoinPath(dataDir, DEFAULT_PATH_TO_REMOTE_STATE_FILE))
	} else if util.FileExists(util.JoinPath(workingDir, DEFAULT_PATH_TO_LOCAL_STATE_FILE)) {
		return ParseTerraformStateFile(util.JoinPath(workingDir, DEFAULT_PATH_TO_LOCAL_STATE_FILE))
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
