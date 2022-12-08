package cli

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/mattn/go-zglob"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const awsProviderPatchHelp = `
   Usage: terragrunt aws-provider-patch [OPTIONS]

   Description:
   Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018)
   
   Options:
   --terragrunt-override-attr	A key=value attribute to override in a provider block as part of the aws-provider-patch command. May be specified multiple times.
`

// applyAwsProviderPatch finds all Terraform modules nested in the current code (i.e., in the .terraform/modules
// folder), looks for provider "aws" { ... } blocks in those modules, and overwrites the attributes in those provider
// blocks with the attributes specified in terragrntOptions.
//
// For example, if were running Terragrunt against code that contained a module:
//
// module "example" {
//   source = "<URL>"
// }
//
// When you run 'init', Terraform would download the code for that module into .terraform/modules. This function would
// scan that module code for provider blocks:
//
// provider "aws" {
//    region = var.aws_region
// }
//
// And if AwsProviderPatchOverrides in terragruntOptions was set to map[string]string{"region": "us-east-1"}, then this
// method would update the module code to:
//
// provider "aws" {
//    region = "us-east-1"
// }
//
// This is a temporary workaround for a Terraform bug (https://github.com/hashicorp/terraform/issues/13018) where
// any dynamic values in nested provider blocks are not handled correctly when you call 'terraform import', so by
// temporarily hard-coding them, we can allow 'import' to work.
func applyAwsProviderPatch(terragruntOptions *options.TerragruntOptions) error {
	if len(terragruntOptions.AwsProviderPatchOverrides) == 0 {
		return errors.WithStackTrace(MissingOverrides(optTerragruntOverrideAttr))
	}

	terraformFilesInModules, err := findAllTerraformFilesInModules(terragruntOptions)
	if err != nil {
		return err
	}

	for _, terraformFile := range terraformFilesInModules {
		terragruntOptions.Logger.Debugf("Looking at file %s", terraformFile)
		originalTerraformFileContents, err := util.ReadFileAsString(terraformFile)
		if err != nil {
			return err
		}

		updatedTerraformFileContents, codeWasUpdated, err := patchAwsProviderInTerraformCode(originalTerraformFileContents, terraformFile, terragruntOptions.AwsProviderPatchOverrides)
		if err != nil {
			return err
		}

		if codeWasUpdated {
			terragruntOptions.Logger.Debugf("Patching AWS provider in %s", terraformFile)
			if err := util.WriteFileWithSamePermissions(terraformFile, terraformFile, []byte(updatedTerraformFileContents)); err != nil {
				return err
			}
		}
	}

	return nil
}

// The format we expect in the .terraform/modules/modules.json file
type TerraformModulesJson struct {
	Modules []TerraformModule
}

type TerraformModule struct {
	Key    string
	Source string
	Dir    string
}

// findAllTerraformFiles returns all Terraform source files within the modules being used by this Terragrunt
// configuration. To be more specific, it only returns the source files downloaded for module "xxx" { ... } blocks into
// the .terraform/modules folder; it does NOT return Terraform files for the top-level (AKA "root") module.
//
// NOTE: this method only supports *.tf files right now. Terraform code defined in *.json files is not currently
// supported.
func findAllTerraformFilesInModules(terragruntOptions *options.TerragruntOptions) ([]string, error) {
	// Terraform downloads modules into the .terraform/modules folder. Unfortunately, it downloads not only the module
	// into that folder, but the entire repo it's in, which can contain lots of other unrelated code we probably don't
	// want to touch. To find the paths to the actual modules, we read the modules.json file in that folder, which is
	// a manifest file Terraform uses to track where the modules are within each repo. Note that this is an internal
	// API, so the way we parse/read this modules.json file may break in future Terraform versions. Note that we
	// can't use the official HashiCorp code to parse this file, as it's marked internal:
	// https://github.com/hashicorp/terraform/blob/master/internal/modsdir/manifest.go
	modulesJsonPath := util.JoinPath(terragruntOptions.DataDir(), "modules", "modules.json")

	if !util.FileExists(modulesJsonPath) {
		return nil, nil
	}

	modulesJsonContents, err := ioutil.ReadFile(modulesJsonPath)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	var terraformModulesJson TerraformModulesJson
	if err := json.Unmarshal(modulesJsonContents, &terraformModulesJson); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	var terraformFiles []string

	for _, module := range terraformModulesJson.Modules {
		if module.Key != "" && module.Dir != "" {
			moduleAbsPath := module.Dir
			if !filepath.IsAbs(moduleAbsPath) {
				moduleAbsPath = util.JoinPath(terragruntOptions.WorkingDir, moduleAbsPath)
			}

			// Ideally, we'd use a builtin Go library like filepath.Glob here, but per https://github.com/golang/go/issues/11862,
			// the current go implementation doesn't support treating ** as zero or more directories, just zero or one.
			// So we use a third-party library.
			matches, err := zglob.Glob(fmt.Sprintf("%s/**/*.tf", moduleAbsPath))
			if err != nil {
				return nil, errors.WithStackTrace(err)
			}

			terraformFiles = append(terraformFiles, matches...)
		}
	}

	return terraformFiles, nil
}

// patchAwsProviderInTerraformCode looks for provider "aws" { ... } blocks in the given Terraform code and overwrites
// the attributes in those provider blocks with the given attributes. It returns the new Terraform code and a boolean
// true if that code was updated.
//
// For example, if you passed in the following Terraform code:
//
// provider "aws" {
//    region = var.aws_region
// }
//
// And you set attributesToOverride to map[string]string{"region": "us-east-1"}, then this method will return:
//
// provider "aws" {
//    region = "us-east-1"
// }
//
// This is a temporary workaround for a Terraform bug (https://github.com/hashicorp/terraform/issues/13018) where
// any dynamic values in nested provider blocks are not handled correctly when you call 'terraform import', so by
// temporarily hard-coding them, we can allow 'import' to work.
func patchAwsProviderInTerraformCode(terraformCode string, terraformFilePath string, attributesToOverride map[string]string) (string, bool, error) {
	if len(attributesToOverride) == 0 {
		return terraformCode, false, nil
	}

	hclFile, err := hclwrite.ParseConfig([]byte(terraformCode), terraformFilePath, hcl.InitialPos)
	if err != nil {
		return "", false, errors.WithStackTrace(err)
	}

	codeWasUpdated := false

	for _, block := range hclFile.Body().Blocks() {
		if block.Type() == "provider" && len(block.Labels()) == 1 && block.Labels()[0] == "aws" {
			for key, value := range attributesToOverride {
				attributeOverridden, err := overrideAttributeInBlock(block, key, value)
				if err != nil {
					return string(hclFile.Bytes()), codeWasUpdated, err
				}
				codeWasUpdated = codeWasUpdated || attributeOverridden
			}
		}
	}

	return string(hclFile.Bytes()), codeWasUpdated, nil
}

// Override the attribute specified in the given key to the given value in a Terraform block: that is, if the attribute
// is already set, then update its value to the new value; if the attribute is not already set, do nothing. This method
// returns true if an attribute was overridden and false if nothing was changed.
//
// Note that you can set attributes within nested blocks by using a dot syntax similar to Terraform addresses: e.g.,
// "<NESTED_BLOCK>.<KEY>".
//
// Examples:
//
// Assume that block1 is:
//
// provider "aws" {
//   region = var.aws_region
//   assume_role {
//     role_arn = var.role_arn
//   }
// }
//
// If you call:
//
// overrideAttributeInBlock(block1, "region", "eu-west-1")
// overrideAttributeInBlock(block1, "assume_role.role_arn", "foo")
//
// The result would be:
//
// provider "aws" {
//   region = "eu-west-1"
//   assume_role {
//     role_arn = "foo"
//   }
// }
//
// Assume block2 is:
//
// provider "aws" {}
//
// If you call:
//
// overrideAttributeInBlock(block2, "region", "eu-west-1")
// overrideAttributeInBlock(block2, "assume_role.role_arn", "foo")
//
//
// The result would be:
//
// provider "aws" {}
//
// Returns an error if the provided value is not valid json.
func overrideAttributeInBlock(block *hclwrite.Block, key string, value string) (bool, error) {
	body, attr := traverseBlock(block, strings.Split(key, "."))
	if body == nil || body.GetAttribute(attr) == nil {
		// We didn't find an existing block or attribute, so there's nothing to override
		return false, nil
	}

	// The cty library requires concrete types, but since the value is user provided, we don't have a way to know the
	// underlying type. Additionally, the provider block themselves don't give us the typing information either unless
	// we maintain a mapping of all possible provider configurations (which is unmaintainable). To handle this, we
	// assume the user provided input is json, and convert to cty that way.
	valueBytes := []byte(value)
	ctyType, err := ctyjson.ImpliedType(valueBytes)
	if err != nil {
		// Wrap error in a custom error type that has better error messaging to the user.
		returnErr := TypeInferenceErr{value: value, underlyingErr: err}
		return false, errors.WithStackTrace(returnErr)
	}
	ctyVal, err := ctyjson.Unmarshal(valueBytes, ctyType)
	if err != nil {
		// Wrap error in a custom error type that has better error messaging to the user.
		returnErr := MalformedJSONValErr{value: value, underlyingErr: err}
		return false, errors.WithStackTrace(returnErr)
	}

	body.SetAttributeValue(attr, ctyVal)
	return true, nil
}

// Given a Terraform block and slice of keys, return the body of the block that is indicated by the keys, and the
// attribute to set within that body. If the slice is of length one, this method returns the body of the current block
// and the one entry in the slice. However, if the slice contains multiple values, those indicate nested blocks, so
// this method will recursively descend into those blocks and return the body of the final one and the final entry in
// the slice to set on it. If a nested block is specified that doesn't actually exist, this method returns a nil body
// and empty string for the attribute.
//
// Examples:
//
// Assume block is:
//
// provider "aws" {
//   region = var.aws_region
//   assume_role {
//     role_arn = var.role_arn
//   }
// }
//
// traverseBlock(block, []string{"region"})
//   => returns (<body of the current block>, "region")
//
// traverseBlock(block, []string{"assume_role", "role_arn"})
//   => returns (<body of the nested assume_role block>, "role_arn")
//
// traverseBlock(block, []string{"foo"})
//   => returns (nil, "")
//
// traverseBlock(block, []string{"assume_role", "foo"})
//   => returns (nil, "")
func traverseBlock(block *hclwrite.Block, keyParts []string) (*hclwrite.Body, string) {
	if block == nil {
		return nil, ""
	}

	if len(keyParts) < 2 {
		return block.Body(), strings.Join(keyParts, "")
	}

	blockName := keyParts[0]
	return traverseBlock(block.Body().FirstMatchingBlock(blockName, nil), keyParts[1:])
}

// Custom error types

type MissingOverrides string

func (err MissingOverrides) Error() string {
	return fmt.Sprintf("You must specify at least one provider attribute to override via the --%s option.", string(err))
}

type TypeInferenceErr struct {
	value         string
	underlyingErr error
}

func (err TypeInferenceErr) Error() string {
	val := err.value
	return fmt.Sprintf(`Could not determine underlying type of JSON string %s. This usually happens when the JSON string is malformed, or if the value is not properly quoted (e.g., "%s"). Underlying error: %s`, val, val, err.underlyingErr)
}

type MalformedJSONValErr struct {
	value         string
	underlyingErr error
}

func (err MalformedJSONValErr) Error() string {
	val := err.value
	return fmt.Sprintf(`Error unmarshaling JSON string %s. This usually happens when the JSON string is malformed, or if the value is not properly quoted (e.g., "%s"). Underlying error: %s`, val, val, err.underlyingErr)
}
