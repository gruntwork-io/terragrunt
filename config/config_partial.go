package config

import (
	"fmt"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
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
	TerragruntFlags
)

// terragruntInclude is a struct that can be used to only decode the include block.
type terragruntInclude struct {
	Include *IncludeConfig `hcl:"include,block"`
	Remain  hcl.Body       `hcl:",remain"`
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

// terragruntFlags is a struct that can be used to only decode the flag attributes (skip and prevent_destroy)
type terragruntFlags struct {
	PreventDestroy *bool    `hcl:"prevent_destroy,attr"`
	Skip           *bool    `hcl:"skip,attr"`
	Remain         hcl.Body `hcl:",remain"`
}

// terragruntDependency is a struct that can be used to only decode the dependency blocks in the terragrunt config
type terragruntDependency struct {
	Dependencies []Dependency `hcl:"dependency,block"`
	Remain       hcl.Body     `hcl:",remain"`
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
) (*cty.Value, *terragruntInclude, *IncludeConfig, error) {
	// Decode just the `include` block, and verify that it's allowed here
	terragruntInclude, err := decodeAsTerragruntInclude(
		hclFile,
		filename,
		terragruntOptions,
		EvalContextExtensions{},
	)
	if err != nil {
		return nil, nil, nil, err
	}
	includeForDecode, err := getIncludedConfigForDecode(terragruntInclude, terragruntOptions, includeFromChild)
	if err != nil {
		return nil, nil, nil, err
	}

	// Evaluate all the expressions in the locals block separately and generate the variables list to use in the
	// evaluation context.
	locals, err := evaluateLocalsBlock(terragruntOptions, parser, hclFile, filename, includeForDecode)
	if err != nil {
		return nil, nil, nil, err
	}
	localsAsCty, err := convertValuesMapToCtyVal(locals)
	if err != nil {
		return nil, nil, nil, err
	}

	return &localsAsCty, terragruntInclude, includeForDecode, nil
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
	localsAsCty, terragruntInclude, includeForDecode, err := DecodeBaseBlocks(terragruntOptions, parser, file, filename, includeFromChild)
	if err != nil {
		return nil, err
	}

	// Initialize evaluation context extensions from base blocks.
	contextExtensions := EvalContextExtensions{
		Locals:  localsAsCty,
		Include: includeForDecode,
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
				output.PreventDestroy = *decoded.PreventDestroy
			}
			if decoded.Skip != nil {
				output.Skip = *decoded.Skip
			}

		default:
			return nil, InvalidPartialBlockName{decode}
		}
	}

	// If this file includes another, parse and merge the partial blocks.  Otherwise just return this config.
	if terragruntInclude.Include != nil {
		includedConfig, err := partialParseIncludedConfig(terragruntInclude.Include, terragruntOptions, decodeList)
		if err != nil {
			return nil, err
		}
		return mergeConfigWithIncludedConfig(&output, includedConfig, terragruntOptions)
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

// This decodes only the `include` block of a terragrunt config, so its value can be used while decoding the rest of the
// config.
// For consistency, `include` in the call to `decodeHcl` is always assumed to be nil.
// Either it really is nil (parsing the child config), or it shouldn't be used anyway (the parent config shouldn't have
// an include block)
func decodeAsTerragruntInclude(
	file *hcl.File,
	filename string,
	terragruntOptions *options.TerragruntOptions,
	extensions EvalContextExtensions,
) (*terragruntInclude, error) {
	terragruntInclude := terragruntInclude{}
	err := decodeHcl(file, filename, &terragruntInclude, terragruntOptions, extensions)
	if err != nil {
		return nil, err
	}
	return &terragruntInclude, nil
}

// Custom error types

type InvalidPartialBlockName struct {
	sectionCode PartialDecodeSectionType
}

func (err InvalidPartialBlockName) Error() string {
	return fmt.Sprintf("Unrecognized partial block code %d. This is most likely an error in terragrunt. Please file a bug report on the project repository.", err.sectionCode)
}
