package remotestate

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/util"
)

// TODO: this file could be changed to use the Terraform Go code to read state files, but that code is relatively
// complicated and doesn't seem to be designed for standalone use. Fortunately, the .tfstate format is a fairly simple
// JSON format, so hopefully this simple parsing code will not be a maintenance burden.

// DefaultPathToLocalStateFile is the default path to the tfstate file when storing Terraform state locally.
const DefaultPathToLocalStateFile = "terraform.tfstate"

// DefaultPathToRemoteStateFile is the default folder location for the local copy of the state file when using remote
// state storage in Terraform.
const DefaultPathToRemoteStateFile = "terraform.tfstate"

// TerraformState - represents the structure of the Terraform .tfstate file.
type TerraformState struct {
	Backend *TerraformBackend      `json:"Backend"`
	Modules []TerraformStateModule `json:"Modules"`
	Version int                    `json:"Version"`
	Serial  int                    `json:"Serial"`
}

// TerraformBackend represents the structure of the "backend" section in the Terraform .tfstate file.
type TerraformBackend struct {
	Config map[string]any `json:"Config"`
	Type   string         `json:"Type"`
}

// TerraformStateModule represents the structure of a "module" section in the Terraform .tfstate file.
type TerraformStateModule struct {
	Outputs   map[string]any `json:"Outputs"`
	Resources map[string]any `json:"Resources"`
	Path      []string       `json:"Path"`
}

// IsRemote returns true if this Terraform state is configured for remote state storage.
func (state *TerraformState) IsRemote() bool {
	return state != nil && state.Backend != nil && state.Backend.Type != "local"
}

// ParseTerraformStateFileFromLocation parses the Terraform .tfstate file. If a local backend is used then search
// the given path, or return nil if the file is missing. If the backend is not local then parse the Terraform .tfstate
// file from the location specified by workingDir. If no location is specified, search the current
// directory. If the file doesn't exist at any of the default locations, return nil.
func ParseTerraformStateFileFromLocation(backend string, config backend.Config, workingDir, dataDir string) (*TerraformState, error) {
	if stateFile := config.Path(); backend == "local" && stateFile != "" && util.FileExists(stateFile) {
		return ParseTerraformStateFile(stateFile)
	}

	if util.FileExists(util.JoinPath(dataDir, DefaultPathToRemoteStateFile)) {
		return ParseTerraformStateFile(util.JoinPath(dataDir, DefaultPathToRemoteStateFile))
	}

	if util.FileExists(util.JoinPath(workingDir, DefaultPathToLocalStateFile)) {
		return ParseTerraformStateFile(util.JoinPath(workingDir, DefaultPathToLocalStateFile))
	}

	return nil, nil
}

// ParseTerraformStateFile parses the Terraform .tfstate file located at the specified path.
func ParseTerraformStateFile(path string) (*TerraformState, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New(CantParseTerraformStateFileError{Path: path, UnderlyingErr: err})
	}

	state, err := ParseTerraformState(bytes)
	if err != nil {
		return nil, errors.New(CantParseTerraformStateFileError{Path: path, UnderlyingErr: err})
	}

	return state, nil
}

// ParseTerraformState parses the Terraform state file data from the provided byte slice.
func ParseTerraformState(terraformStateData []byte) (*TerraformState, error) {
	terraformState := &TerraformState{}

	if len(terraformStateData) == 0 {
		return terraformState, nil
	}

	if err := json.Unmarshal(terraformStateData, terraformState); err != nil {
		return nil, errors.New(err)
	}

	return terraformState, nil
}

// CantParseTerraformStateFileError error that occurs when we can't parse the Terraform state file.
type CantParseTerraformStateFileError struct {
	UnderlyingErr error
	Path          string
}

func (err CantParseTerraformStateFileError) Error() string {
	return fmt.Sprintf("Error parsing Terraform state file %s: %s", err.Path, err.UnderlyingErr.Error())
}
