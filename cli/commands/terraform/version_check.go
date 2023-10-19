package terraform

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/hashicorp/go-version"
)

// This uses the constraint syntax from https://github.com/hashicorp/go-version
// This version of Terragrunt was tested to work with Terraform 0.12.0 and above only
const DefaultTerraformVersionConstraint = ">= v0.12.0"

// TerraformVersionRegex verifies that terraform --version output is in one of the following formats:
// - Terraform v0.9.5-dev (cad024a5fe131a546936674ef85445215bbc4226+CHANGES)
// - Terraform v0.13.0-beta2
// - Terraform v0.12.27
// We only make sure the "v#.#.#" part is present in the output.
var TerraformVersionRegex = regexp.MustCompile(`^(.*?)\s(v?\d+\.\d+\.\d+).*`)

const versionParts = 3

// Check the version constraints of both terragrunt and terraform. Note that as a side effect this will set the
// following settings on terragruntOptions:
// - TerraformPath
// - TerraformVersion
// TODO: Look into a way to refactor this function to avoid the side effect.
func checkVersionConstraints(terragruntOptions *options.TerragruntOptions) error {
	partialTerragruntConfig, err := config.PartialParseConfigFile(
		terragruntOptions.TerragruntConfigPath,
		terragruntOptions,
		nil,
		[]config.PartialDecodeSectionType{config.TerragruntVersionConstraints},
	)
	if err != nil {
		return err
	}

	// Change the terraform binary path before checking the version
	// if the path is not changed from default and set in the config.
	if terragruntOptions.TerraformPath == options.DefaultWrappedPath && partialTerragruntConfig.TerraformBinary != "" {
		terragruntOptions.TerraformPath = partialTerragruntConfig.TerraformBinary
	}
	if err := PopulateTerraformVersion(terragruntOptions); err != nil {
		return err
	}

	terraformVersionConstraint := DefaultTerraformVersionConstraint
	if partialTerragruntConfig.TerraformVersionConstraint != "" {
		terraformVersionConstraint = partialTerragruntConfig.TerraformVersionConstraint
	}
	if err := CheckTerraformVersion(terraformVersionConstraint, terragruntOptions); err != nil {
		return err
	}

	if partialTerragruntConfig.TerragruntVersionConstraint != "" {
		if err := CheckTerragruntVersion(partialTerragruntConfig.TerragruntVersionConstraint, terragruntOptions); err != nil {
			return err
		}
	}
	return nil
}

// Populate the currently installed version of Terraform into the given terragruntOptions
func PopulateTerraformVersion(terragruntOptions *options.TerragruntOptions) error {
	// Discard all log output to make sure we don't pollute stdout or stderr with this extra call to '--version'
	terragruntOptionsCopy := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	terragruntOptionsCopy.Writer = io.Discard
	terragruntOptionsCopy.ErrWriter = io.Discard

	// Remove any TF_CLI_ARGS before version checking. These are appended to
	// the arguments supplied on the command line and cause issues when running
	// the --version command.
	// https://www.terraform.io/docs/commands/environment-variables.html#tf_cli_args-and-tf_cli_args_name
	for key := range terragruntOptionsCopy.Env {
		if strings.HasPrefix(key, "TF_CLI_ARGS") {
			delete(terragruntOptionsCopy.Env, key)
		}
	}

	output, err := shell.RunTerraformCommandWithOutput(terragruntOptionsCopy, "--version")
	if err != nil {
		return err
	}

	terraformVersion, err := parseTerraformVersion(output.Stdout)
	if err != nil {
		return err
	}

	tfImplementation, err := parseTerraformImplementationType(output.Stdout)
	if err != nil {
		return err
	}

	terragruntOptions.TerraformVersion = terraformVersion
	terragruntOptions.TerraformImplementation = tfImplementation

	if tfImplementation == options.UnknownImpl {
		terragruntOptions.TerraformImplementation = options.TerraformImpl
		terragruntOptions.Logger.Warnf("Failed to identify Terraform implementation, fallback to terraform version: %s", terraformVersion)
	} else {
		terragruntOptions.Logger.Debugf("%s version: %s", tfImplementation, terraformVersion)
	}
	return nil
}

// Check that the currently installed Terraform version works meets the specified version constraint and return an error
// if it doesn't
func CheckTerraformVersion(constraint string, terragruntOptions *options.TerragruntOptions) error {
	return checkTerraformVersionMeetsConstraint(terragruntOptions.TerraformVersion, constraint)
}

// Check that the currently running Terragrunt version meets the specified version constraint and return an error
// if it doesn't
func CheckTerragruntVersion(constraint string, terragruntOptions *options.TerragruntOptions) error {
	return checkTerragruntVersionMeetsConstraint(terragruntOptions.TerragruntVersion, constraint)
}

// Check that the current version of Terragrunt meets the specified constraint and return an error if it doesn't
func checkTerragruntVersionMeetsConstraint(currentVersion *version.Version, constraint string) error {
	versionConstraint, err := version.NewConstraint(constraint)
	if err != nil {
		return err
	}

	if !versionConstraint.Check(currentVersion) {
		return errors.WithStackTrace(InvalidTerragruntVersion{CurrentVersion: currentVersion, VersionConstraints: versionConstraint})
	}

	return nil
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
	matches := TerraformVersionRegex.FindStringSubmatch(versionCommandOutput)

	if len(matches) != versionParts {
		return nil, errors.WithStackTrace(InvalidTerraformVersionSyntax(versionCommandOutput))
	}

	return version.NewVersion(matches[2])
}

// parseTerraformImplementationType - Parse terraform implementation from --version command output
func parseTerraformImplementationType(versionCommandOutput string) (options.TerraformImplementationType, error) {
	matches := TerraformVersionRegex.FindStringSubmatch(versionCommandOutput)

	if len(matches) != versionParts {
		return options.UnknownImpl, errors.WithStackTrace(InvalidTerraformVersionSyntax(versionCommandOutput))
	}
	rawType := strings.ToLower(matches[1])
	switch rawType {
	case "terraform":
		return options.TerraformImpl, nil
	case "opentofu":
		return options.OpenTofuImpl, nil
	default:
		return options.UnknownImpl, nil
	}
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

type InvalidTerragruntVersion struct {
	CurrentVersion     *version.Version
	VersionConstraints version.Constraints
}

func (err InvalidTerraformVersion) Error() string {
	return fmt.Sprintf("The currently installed version of Terraform (%s) is not compatible with the version Terragrunt requires (%s).", err.CurrentVersion.String(), err.VersionConstraints.String())
}

func (err InvalidTerragruntVersion) Error() string {
	return fmt.Sprintf("The currently installed version of Terragrunt (%s) is not compatible with the version constraint requiring (%s).", err.CurrentVersion.String(), err.VersionConstraints.String())
}
