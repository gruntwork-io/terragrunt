package cli

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/hashicorp/go-version"
	"regexp"
)

// The terraform --version output is of the format: Terraform v0.9.3
var TERRAFORM_VERSION_REGEX = regexp.MustCompile("Terraform (.+)")

// Check that the currently installed Terraform version works meets the specified version constraint and return an error
// if it doesn't
func CheckTerraformVersion(constraint string, terragruntOptions *options.TerragruntOptions) error {
	currentVersion, err := getTerraformVersion(terragruntOptions)
	if err != nil {
		return err
	}

	return checkTerraformVersionMeetsConstraint(currentVersion, constraint)
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

// Get the currently installed version of Terraform
func getTerraformVersion(terragruntOptions *options.TerragruntOptions) (*version.Version, error) {
	output, err := shell.RunTerraformCommandAndCaptureOutput(terragruntOptions, "--version")
	if err != nil {
		return nil, err
	}

	return parseTerraformVersion(output)
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
