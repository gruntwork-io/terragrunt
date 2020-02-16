package codegen

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"

	"github.com/hashicorp/hcl2/hclwrite"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

// An enum to represent valid values for if_exists
type GenerateConfigExists int

const (
	ExistsError GenerateConfigExists = iota
	ExistsSkip
	ExistsOverwrite
	ExistsUnknown
)

const (
	ExistsErrorStr     = "error"
	ExistsSkipStr      = "skip"
	ExistsOverwriteStr = "overwrite"
)

// Configuration for generating code
type GenerateConfig struct {
	Path     string
	IfExists GenerateConfigExists
	Contents string
}

// WriteToFile will generate a new file at the given target path with the given contents. If a file already exists at
// the target path, the behavior depends on the value of IfExists:
// - if ExistsError, return an error.
// - if ExistsSkip, do nothing and return
// - if ExistsOverwrite, overwrite the existing file
func WriteToFile(logger *log.Logger, basePath string, config GenerateConfig) error {
	// Figure out thee target path to generate the code in. If relative, merge with basePath.
	var targetPath string
	if filepath.IsAbs(config.Path) {
		targetPath = config.Path
	} else {
		targetPath = filepath.Join(basePath, config.Path)
	}

	targetFileExists := util.FileExists(targetPath)
	if targetFileExists && config.IfExists == ExistsError {
		return errors.WithStackTrace(GenerateFileExistsError{path: targetPath})
	} else if targetFileExists && config.IfExists == ExistsSkip {
		// Do nothing since file exists and skip was configured
		logger.Printf("The file path %s already exists and if_exists for code generation set to \"skip\". Will not regenerate file.", targetPath)
		return nil
	} else if targetFileExists {
		logger.Printf("The file path %s already exists and if_exists for code generation set to \"overwrite\". Regenerating file.", targetPath)
	} else if config.IfExists == ExistsUnknown {
		return errors.WithStackTrace(UnknownGenerateIfExistsVal{""})
	}

	if err := ioutil.WriteFile(targetPath, []byte(config.Contents), 0644); err != nil {
		return errors.WithStackTrace(err)
	}
	logger.Printf("Generated file %s.", targetPath)
	return nil
}

// Convert the arbitrary map that represents a remote state config into HCL code to configure that remote state.
func RemoteStateConfigToTerraformCode(backend string, config map[string]interface{}) ([]byte, error) {
	f := hclwrite.NewEmptyFile()
	backendBlock := f.Body().AppendNewBlock("terraform", nil).Body().AppendNewBlock("backend", []string{backend})
	backendBlockBody := backendBlock.Body()

	for key, val := range config {
		// Since we don't have the cty type information for the config and since config can be arbitrary, we cheat by using
		// json as an intermediate representation.
		jsonBytes, err := json.Marshal(val)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		var ctyVal ctyjson.SimpleJSONValue
		if err := ctyVal.UnmarshalJSON(jsonBytes); err != nil {
			return nil, errors.WithStackTrace(err)
		}

		backendBlockBody.SetAttributeValue(key, ctyVal.Value)
	}

	return f.Bytes(), nil
}

// GenerateConfigExistsFromString converst a string representation of if_exists into the enum, returning an error if it
// is not set to one of the known values.
func GenerateConfigExistsFromString(val string) (GenerateConfigExists, error) {
	switch val {
	case ExistsErrorStr:
		return ExistsError, nil
	case ExistsSkipStr:
		return ExistsSkip, nil
	case ExistsOverwriteStr:
		return ExistsOverwrite, nil
	}
	return ExistsUnknown, errors.WithStackTrace(UnknownGenerateIfExistsVal{val: val})
}

// Custom error types

type UnknownGenerateIfExistsVal struct {
	val string
}

func (err UnknownGenerateIfExistsVal) Error() string {
	if err.val != "" {
		return fmt.Sprintf("%s is not a valid value for generate if_exists", err.val)
	}
	return "Received unknown value for if_exists"
}

type GenerateFileExistsError struct {
	path string
}

func (err GenerateFileExistsError) Error() string {
	return fmt.Sprintf("Can not generate terraform file: %s already exists", err.path)
}
