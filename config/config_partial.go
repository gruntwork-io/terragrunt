package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// PartialDecodeSectionType is an enum that is used to list out which blocks/sections of the terragrunt config should be
// decoded in a partial decode.
type PartialDecodeSectionType int

const (
	DependenciesBlock PartialDecodeSectionType = iota
	DependencyBlock
	TerraformBlock
	TerraformSource
	TerragruntFlags
	TerragruntVersionConstraints
	RemoteStateBlock
)

// terragruntIncludeMultiple is a struct that can be used to only decode the include block with labels.
type terragruntIncludeMultiple struct {
	Include []IncludeConfig `hcl:"include,block"`
	Remain  hcl.Body        `hcl:",remain"`
}

// terragruntDependencies is a struct that can be used to only decode the dependencies block.
type terragruntDependencies struct {
	Dependencies *ModuleDependencies `hcl:"dependencies,block"`
	Remain       hcl.Body            `hcl:",remain"`
}

// terragruntTerraform is a struct that can be used to only decode the terraform block
type terragruntTerraform struct {
	Terraform *TerraformConfig `hcl:"terraform,block"`
	Remain    hcl.Body         `hcl:",remain"`
}

// terragruntTerraformSource is a struct that can be used to only decode the terraform block, and only the source
// attribute.
type terragruntTerraformSource struct {
	Terraform *terraformConfigSourceOnly `hcl:"terraform,block"`
	Remain    hcl.Body                   `hcl:",remain"`
}

// terraformConfigSourceOnly is a struct that can be used to decode only the source attribute of the terraform block.
type terraformConfigSourceOnly struct {
	Source *string  `hcl:"source,attr"`
	Remain hcl.Body `hcl:",remain"`
}

// terragruntFlags is a struct that can be used to only decode the flag attributes (skip and prevent_destroy)
type terragruntFlags struct {
	IamRole        *string  `hcl:"iam_role,attr"`
	PreventDestroy *bool    `hcl:"prevent_destroy,attr"`
	Skip           *bool    `hcl:"skip,attr"`
	Remain         hcl.Body `hcl:",remain"`
}

// terragruntVersionConstraints is a struct that can be used to only decode the attributes related to constraining the
// versions of terragrunt and terraform.
type terragruntVersionConstraints struct {
	TerragruntVersionConstraint *string  `hcl:"terragrunt_version_constraint,attr"`
	TerraformVersionConstraint  *string  `hcl:"terraform_version_constraint,attr"`
	TerraformBinary             *string  `hcl:"terraform_binary,attr"`
	Remain                      hcl.Body `hcl:",remain"`
}

// terragruntDependency is a struct that can be used to only decode the dependency blocks in the terragrunt config
type terragruntDependency struct {
	Dependencies []Dependency `hcl:"dependency,block"`
	Remain       hcl.Body     `hcl:",remain"`
}

// terragruntRemoteState is a struct that can be used to only decode the remote_state blocks in the terragrunt config
type terragruntRemoteState struct {
	RemoteState *remoteStateConfigFile `hcl:"remote_state,block"`
	Remain      hcl.Body               `hcl:",remain"`
}

// DecodeBaseBlocks takes in a parsed HCL2 file and decodes the base blocks. Base blocks are blocks that should always
// be decoded even in partial decoding, because they provide bindings that are necessary for parsing any block in the
// file. Currently base blocks are:
// - locals
// - include
func DecodeBaseBlocks(
	terragruntOptions *options.TerragruntOptions,
	parser *hclparse.Parser,
	hclFile *hcl.File,
	filename string,
	includeFromChild *IncludeConfig,
) (*cty.Value, *TrackInclude, error) {
	// Decode just the `include` and `import` blocks, and verify that it's allowed here
	terragruntIncludeList, err := decodeAsTerragruntInclude(
		hclFile,
		filename,
		terragruntOptions,
		EvalContextExtensions{},
	)
	if err != nil {
		return nil, nil, err
	}

	trackInclude, err := getTrackInclude(terragruntIncludeList, includeFromChild, terragruntOptions)
	if err != nil {
		return nil, nil, err
	}

	// Evaluate all the expressions in the locals block separately and generate the variables list to use in the
	// evaluation context.
	locals, err := evaluateLocalsBlock(
		terragruntOptions,
		parser,
		hclFile,
		filename,
		trackInclude,
	)
	if err != nil {
		return nil, trackInclude, err
	}
	localsAsCty, err := convertValuesMapToCtyVal(locals)
	if err != nil {
		return nil, trackInclude, err
	}

	return &localsAsCty, trackInclude, nil
}

func PartialParseConfigFile(
	filename string,
	terragruntOptions *options.TerragruntOptions,
	include *IncludeConfig,
	decodeList []PartialDecodeSectionType,
) (*TerragruntConfig, error) {
	configString, err := util.ReadFileAsString(filename)
	if err != nil {
		return nil, err
	}

	config, err := PartialParseConfigString(configString, terragruntOptions, include, filename, decodeList)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// ParitalParseConfigString partially parses and decodes the provided string. Which blocks/attributes to decode is
// controlled by the function parameter decodeList. These blocks/attributes are parsed and set on the output
// TerragruntConfig. Valid values are:
// - DependenciesBlock: Parses the `dependencies` block in the config
// - DependencyBlock: Parses the `dependency` block in the config
// - TerraformBlock: Parses the `terraform` block in the config
// - TerragruntFlags: Parses the boolean flags `prevent_destroy` and `skip` in the config
// - TerragruntVersionConstraints: Parses the attributes related to constraining terragrunt and terraform versions in
//                                 the config.
// - RemoteStateBlock: Parses the `remote_state` block in the config
// Note that the following blocks are always decoded:
// - locals
// - include
// Note also that the following blocks are never decoded in a partial parse:
// - inputs
func PartialParseConfigString(
	configString string,
	terragruntOptions *options.TerragruntOptions,
	includeFromChild *IncludeConfig,
	filename string,
	decodeList []PartialDecodeSectionType,
) (*TerragruntConfig, error) {
	// Parse the HCL string into an AST body that can be decoded multiple times later without having to re-parse
	parser := hclparse.NewParser()
	file, err := parseHcl(parser, configString, filename)
	if err != nil {
		return nil, err
	}

	// Decode just the Base blocks. See the function docs for DecodeBaseBlocks for more info on what base blocks are.
	localsAsCty, trackInclude, err := DecodeBaseBlocks(terragruntOptions, parser, file, filename, includeFromChild)
	if err != nil {
		return nil, err
	}

	// Initialize evaluation context extensions from base blocks.
	contextExtensions := EvalContextExtensions{
		Locals:       localsAsCty,
		TrackInclude: trackInclude,
	}

	output := TerragruntConfig{IsPartial: true}

	// Now loop through each requested block / component to decode from the terragrunt config, decode them, and merge
	// them into the output TerragruntConfig struct.
	for _, decode := range decodeList {
		switch decode {
		case DependenciesBlock:
			decoded := terragruntDependencies{}
			err := decodeHcl(file, filename, &decoded, terragruntOptions, contextExtensions)
			if err != nil {
				return nil, err
			}

			// If we already decoded some dependencies, merge them in. Otherwise, set as the new list.
			if output.Dependencies != nil {
				output.Dependencies.Merge(decoded.Dependencies)
			} else {
				output.Dependencies = decoded.Dependencies
			}

		case TerraformBlock:
			decoded := terragruntTerraform{}
			err := decodeHcl(file, filename, &decoded, terragruntOptions, contextExtensions)
			if err != nil {
				return nil, err
			}
			output.Terraform = decoded.Terraform

		case TerraformSource:
			decoded := terragruntTerraformSource{}
			err := decodeHcl(file, filename, &decoded, terragruntOptions, contextExtensions)
			if err != nil {
				return nil, err
			}
			if decoded.Terraform != nil {
				output.Terraform = &TerraformConfig{Source: decoded.Terraform.Source}
			}

		case DependencyBlock:
			decoded := terragruntDependency{}
			err := decodeHcl(file, filename, &decoded, terragruntOptions, contextExtensions)
			if err != nil {
				return nil, err
			}
			output.TerragruntDependencies = decoded.Dependencies

			// Convert dependency blocks into module depenency lists. If we already decoded some dependencies,
			// merge them in. Otherwise, set as the new list.
			dependencies := dependencyBlocksToModuleDependencies(decoded.Dependencies)
			if output.Dependencies != nil {
				output.Dependencies.Merge(dependencies)
			} else {
				output.Dependencies = dependencies
			}

		case TerragruntFlags:
			decoded := terragruntFlags{}
			err := decodeHcl(file, filename, &decoded, terragruntOptions, contextExtensions)
			if err != nil {
				return nil, err
			}
			if decoded.PreventDestroy != nil {
				output.PreventDestroy = decoded.PreventDestroy
			}
			if decoded.Skip != nil {
				output.Skip = *decoded.Skip
			}
			if decoded.IamRole != nil {
				output.IamRole = *decoded.IamRole
			}

		case TerragruntVersionConstraints:
			decoded := terragruntVersionConstraints{}
			err := decodeHcl(file, filename, &decoded, terragruntOptions, contextExtensions)
			if err != nil {
				return nil, err
			}
			if decoded.TerragruntVersionConstraint != nil {
				output.TerragruntVersionConstraint = *decoded.TerragruntVersionConstraint
			}
			if decoded.TerraformVersionConstraint != nil {
				output.TerraformVersionConstraint = *decoded.TerraformVersionConstraint
			}
			if decoded.TerraformBinary != nil {
				output.TerraformBinary = *decoded.TerraformBinary
			}

		case RemoteStateBlock:
			decoded := terragruntRemoteState{}
			err := decodeHcl(file, filename, &decoded, terragruntOptions, contextExtensions)
			if err != nil {
				return nil, err
			}
			if decoded.RemoteState != nil {
				remoteState, err := decoded.RemoteState.toConfig()
				if err != nil {
					return nil, err
				}
				output.RemoteState = remoteState
			}

		default:
			return nil, InvalidPartialBlockName{decode}
		}
	}

	// If this file includes another, parse and merge the partial blocks.  Otherwise just return this config.
	if len(trackInclude.CurrentList) > 0 {
		return handleIncludePartial(&output, trackInclude, terragruntOptions, decodeList)
	}
	return &output, nil
}

func partialParseIncludedConfig(includedConfig *IncludeConfig, terragruntOptions *options.TerragruntOptions, decodeList []PartialDecodeSectionType) (*TerragruntConfig, error) {
	if includedConfig.Path == "" {
		return nil, errors.WithStackTrace(IncludedConfigMissingPath(terragruntOptions.TerragruntConfigPath))
	}

	includePath := includedConfig.Path

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(filepath.Dir(terragruntOptions.TerragruntConfigPath), includePath)
	}

	return PartialParseConfigFile(
		includePath,
		terragruntOptions,
		includedConfig,
		decodeList,
	)
}

// This decodes only the `include` blocks of a terragrunt config, so its value can be used while decoding the rest of
// the config.
// For consistency, `include` in the call to `decodeHcl` is always assumed to be nil. Either it really is nil (parsing
// the child config), or it shouldn't be used anyway (the parent config shouldn't have an include block).
//
// We take a two pass approach to parsing include blocks to support include blocks without a label. Ideally we can parse
// include blocks with and without labels in a single pass, but the HCL parser is fairly restrictive when it comes to
// parsing blocks with labels, requiring the exact number of expected labels in the parsing step.
// To handle this restriction, we first see if there are any include blocks without any labels, and if there is, we
// modify it in the file object to inject the label as "".
func decodeAsTerragruntInclude(
	file *hcl.File,
	filename string,
	terragruntOptions *options.TerragruntOptions,
	extensions EvalContextExtensions,
) ([]IncludeConfig, error) {
	updatedBytes, isUpdated, err := updateBareIncludeBlock(file, filename)
	if err != nil {
		return nil, err
	}
	if isUpdated {
		// Code was updated, so we need to reparse the new updated contents. This is necessarily because the blocks
		// returned by hclparse does not support editing, and so we have to go through hclwrite, which leads to a
		// different AST representation.
		file, err = parseHcl(hclparse.NewParser(), string(updatedBytes), filename)
		if err != nil {
			return nil, err
		}
	}

	tgInc := terragruntIncludeMultiple{}
	if err := decodeHcl(file, filename, &tgInc, terragruntOptions, extensions); err != nil {
		return nil, err
	}
	return tgInc.Include, nil
}

// updateBareIncludeBlock searches the parsed terragrunt contents for a bare include block (include without a label),
// and convert it to one with empty string as the label. This is necessary because the hcl parser is strictly enforces
// label counts when parsing out labels with a go struct.
//
// Returns the updated contents, a boolean indicated whether anything changed, and an error (if any).
func updateBareIncludeBlock(file *hcl.File, filename string) ([]byte, bool, error) {
	if filepath.Ext(filename) == ".json" {
		return updateBareIncludeBlockJSON(file.Bytes)
	}

	hclFile, err := hclwrite.ParseConfig(file.Bytes, filename, hcl.InitialPos)
	if err != nil {
		return nil, false, errors.WithStackTrace(err)
	}

	codeWasUpdated := false
	for _, block := range hclFile.Body().Blocks() {
		if block.Type() == "include" && len(block.Labels()) == 0 {
			if codeWasUpdated {
				return nil, false, errors.WithStackTrace(MultipleBareIncludeBlocksErr{})
			}
			block.SetLabels([]string{""})
			codeWasUpdated = true
		}
	}
	return hclFile.Bytes(), codeWasUpdated, nil
}

// updateBareIncludeBlockJSON implements the logic for updateBareIncludeBlock when the terragrunt.hcl configuration is
// encoded in json. The json version of this function is fairly complex due to the flexibility in how the blocks are
// encoded. That is, all of the following are valid encodings of a terragrunt.hcl.json file that has a bare include
// block:
//
// Case 1: a single include block as top level:
// {
//   "include": {
//     "path": "foo"
//   }
// }
//
// Case 2: a single include block in list:
// {
//   "include": [
//     {"path": "foo"}
//   ]
// }
//
// Case 3: mixed bare and labeled include block as list:
// {
//   "include": [
//     {"path": "foo"},
//     {
//       "labeled": {"path": "bar"}
//     }
//   ]
// }
//
// For simplicity of implementation, we focus on handling Case 1 and 2, and ignore Case 3. If we see Case 3, we will
// error out. Instead, the user should handle this case explicitly using the object encoding instead of list encoding:
// {
//   "include": {
//     "": {"path": "foo"},
//     "labeled": {"path": "bar"}
//   }
// }
// If the multiple include blocks are encoded in this way in the json configuration, nothing needs to be done by this
// function.
func updateBareIncludeBlockJSON(fileBytes []byte) ([]byte, bool, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal(fileBytes, &parsed); err != nil {
		return nil, false, errors.WithStackTrace(err)
	}
	includeBlock, hasKey := parsed["include"]
	if !hasKey {
		// No include block, so don't do anything
		return fileBytes, false, nil
	}
	switch typed := includeBlock.(type) {
	case []interface{}:
		if len(typed) == 0 {
			// No include block, so don't do anything
			return nil, false, nil
		} else if len(typed) > 1 {
			// Could be multiple bare includes, or Case 3. We simplify the handling of this case by erroring out,
			// ignoring the possibility of Case 3, which, while valid HCL encoding, is too complex to detect and handle
			// here. Instead we will recommend users use the object encoding.
			return nil, false, errors.WithStackTrace(MultipleBareIncludeBlocksErr{})
		}

		// Make sure this is Case 2, and not Case 3 with a single labeled block. If Case 2, update to inject the labeled
		// version. Otherwise, return without modifying.
		singleBlock := typed[0]
		if jsonIsIncludeBlock(singleBlock) {
			return updateSingleBareIncludeInParsedJSON(parsed, singleBlock)
		}
		return nil, false, nil
	case map[string]interface{}:
		if len(typed) == 0 {
			// No include block, so don't do anything
			return nil, false, nil
		}

		// We will only update the include block if we detect the object to represent an include block. Otherwise, the
		// blocks are labeled so we can pass forward to the tg parser step.
		if jsonIsIncludeBlock(typed) {
			return updateSingleBareIncludeInParsedJSON(parsed, typed)
		}
		return nil, false, nil
	}

	return nil, false, errors.WithStackTrace(IncludeIsNotABlockErr{parsed: includeBlock})
}

// updateBareIncludeInParsedJSON replaces the include attribute into a block with the label "" in the json. Note that we
// can directly assign to the map with the single "" key without worrying about the possibility of other include blocks
// since we will only call this function if there is only one include block, and that is a bare block with no labels.
func updateSingleBareIncludeInParsedJSON(parsed map[string]interface{}, newVal interface{}) ([]byte, bool, error) {
	parsed["include"] = map[string]interface{}{"": newVal}
	updatedBytes, err := json.Marshal(parsed)
	return updatedBytes, true, errors.WithStackTrace(err)
}

// jsonIsIncludeBlock checks if the arbitrary json data is the include block. The data is determined to be an include
// block if:
// - It is an object
// - Has the 'path' attribute
// - The 'path' attribute is a string
func jsonIsIncludeBlock(jsonData interface{}) bool {
	typed, isMap := jsonData.(map[string]interface{})
	if isMap {
		pathAttr, hasPath := typed["path"]
		if hasPath {
			_, pathIsString := pathAttr.(string)
			return pathIsString
		}
		return false
	}
	return false
}

// Custom error types

type InvalidPartialBlockName struct {
	sectionCode PartialDecodeSectionType
}

func (err InvalidPartialBlockName) Error() string {
	return fmt.Sprintf("Unrecognized partial block code %d. This is most likely an error in terragrunt. Please file a bug report on the project repository.", err.sectionCode)
}

type MultipleBareIncludeBlocksErr struct{}

func (err MultipleBareIncludeBlocksErr) Error() string {
	return "Multiple bare include blocks (include blocks without label) is not supported."
}

type IncludeIsNotABlockErr struct {
	parsed interface{}
}

func (err IncludeIsNotABlockErr) Error() string {
	return fmt.Sprintf("Parsed include is not a block: %v", err.parsed)
}
