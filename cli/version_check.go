package cli

import (
	"fmt"
	"io/ioutil"
	"regexp"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/hashicorp/go-version"
)

// The terraform --version output is of the format: Terraform v0.9.5-dev (cad024a5fe131a546936674ef85445215bbc4226+CHANGES)
// where -dev and (commitid+CHANGES) is for custom builds or if TF_LOG is set for debug purposes
var TERRAFORM_VERSION_REGEX = regexp.MustCompile("Terraform (v?[\\d\\.]+)(?:-dev)?(?: .+)?")

// Populate the currently installed version of Terraform into the given terragruntOptions
func PopulateTerraformVersion(terragruntOptions *options.TerragruntOptions) error {
	// Discard all log output to make sure we don't pollute stdout or stderr with this extra call to '--version'
	terragruntOptionsCopy := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	terragruntOptionsCopy.Writer = ioutil.Discard
	terragruntOptionsCopy.ErrWriter = ioutil.Discard

	output, err := shell.RunTerraformCommandWithOutput(terragruntOptionsCopy, "--version")
	if err != nil {
		return err
	}

	terraformVersion, err := parseTerraformVersion(output.Stdout)
	if err != nil {
		return err
	}

	terragruntOptions.TerraformVersion = terraformVersion
	return nil
}

// Check that the currently installed Terraform version works meets the specified version constraint and return an error
// if it doesn't
func CheckTerraformVersion(constraint string, terragruntOptions *options.TerragruntOptions) error {
	return checkTerraformVersionMeetsConstraint(terragruntOptions.TerraformVersion, constraint)
}

// Check that the current version of Terraform meets the specified constraint and return an error if it doesn't
func checkTerraformVersionMeetsConstraint(currentVersion *version.Version, constraint string) error {
	versionConstraint, err := version.NewConstraint(constraint)
	if err != nil {
		return err
	}

	if !versionConstraint.Check(currentVersion) {
		return errors.WithStackTrace(InvalidTerraformVersion{CurrentVersion: currentVersion, VersionConstraints: versionConstraint})
	}

	return nil
}

// Parse the output of the terraform --version command
func parseTerraformVersion(versionCommandOutput string) (*version.Version, error) {
	matches := TERRAFORM_VERSION_REGEX.FindStringSubmatch(versionCommandOutput)

	if len(matches) != 2 {
		return nil, errors.WithStackTrace(InvalidTerraformVersionSyntax(versionCommandOutput))
	}

	return version.NewVersion(matches[1])
}

// Custom error types

type InvalidTerraformVersionSyntax string

func (err InvalidTerraformVersionSyntax) Error() string {
	return fmt.Sprintf("Unable to parse Terraform version output: %s", string(err))
}

type InvalidTerraformVersion struct {
	CurrentVersion     *version.Version
	VersionConstraints version.Constraints
}

func (err InvalidTerraformVersion) Error() string {
	return fmt.Sprintf("The currently installed version of Terraform (%s) is not compatible with the version Terragrunt requires (%s).", err.CurrentVersion.String(), err.VersionConstraints.String())
}
