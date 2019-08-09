package config

import (
	"fmt"
	"path/filepath"

	"github.com/hashicorp/hcl2/hcl"
	"github.com/hashicorp/hcl2/hclparse"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
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

func PartialParseConfigFile(
	filename string,
	terragruntOptions *options.TerragruntOptions,
	include *IncludeConfig,
	decodeList []string,
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
// controlled by the function parameters.
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
	decodeList []string,
) (*TerragruntConfig, error) {
	// Parse the HCL string into an AST body that can be decoded multiple times later without having to re-parse
	parser := hclparse.NewParser()
	file, err := parseHcl(parser, configString, filename)
	if err != nil {
		return nil, err
	}

	// Evaluate all the expressions in the locals block separately and generate the variables list to use in the
	// evaluation context.
	locals, err := evaluateLocalsBlock(terragruntOptions, parser, file, filename)
	if err != nil {
		return nil, err
	}

	// Decode just the `include` block, and verify that it's allowed here
	terragruntInclude, err := decodeAsTerragruntInclude(file, filename, terragruntOptions, locals)
	if err != nil {
		return nil, err
	}
	includeForDecode, err := getIncludedConfigForDecode(terragruntInclude, terragruntOptions, includeFromChild)
	if err != nil {
		return nil, err
	}

	output := TerragruntConfig{IsPartial: true}

	for _, decode := range decodeList {
		switch decode {
		case "dependencies":
			decoded := terragruntDependencies{}
			err := decodeHcl(file, filename, &decoded, terragruntOptions, includeForDecode, locals)
			if err != nil {
				return nil, err
			}
			output.Dependencies = decoded.Dependencies
		case "terraform":
			decoded := terragruntTerraform{}
			err := decodeHcl(file, filename, &decoded, terragruntOptions, includeForDecode, locals)
			if err != nil {
				return nil, err
			}
			output.Terraform = decoded.Terraform
		case "flags":
			decoded := terragruntFlags{}
			err := decodeHcl(file, filename, &decoded, terragruntOptions, includeForDecode, locals)
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

func partialParseIncludedConfig(includedConfig *IncludeConfig, terragruntOptions *options.TerragruntOptions, decodeList []string) (*TerragruntConfig, error) {
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
	locals map[string]cty.Value,
) (*terragruntInclude, error) {
	terragruntInclude := terragruntInclude{}
	err := decodeHcl(file, filename, &terragruntInclude, terragruntOptions, nil, locals)
	if err != nil {
		return nil, err
	}
	return &terragruntInclude, nil
}

// Custom error types

type InvalidPartialBlockName struct {
	name string
}

func (err InvalidPartialBlockName) Error() string {
	return fmt.Sprintf("Unrecognized partial block name %s. This is most likely an error in terragrunt. Please file a bug report on the project repository.", err.name)
}
