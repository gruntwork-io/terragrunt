package terraform

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/hashicorp/go-version"
)

// DefaultTerraformVersionConstraint uses the constraint syntax from https://github.com/hashicorp/go-version
// This version of Terragrunt was tested to work with Terraform 0.12.0 and above only
const DefaultTerraformVersionConstraint = ">= v0.12.0"

// TerraformVersionRegex verifies that terraform --version output is in one of the following formats:
// - OpenTofu v1.6.0-dev
// - Terraform v0.9.5-dev (cad024a5fe131a546936674ef85445215bbc4226+CHANGES)
// - Terraform v0.13.0-beta2
// - Terraform v0.12.27
// We only make sure the "v#.#.#" part is present in the output.
var TerraformVersionRegex = regexp.MustCompile(`^(\S+)\s(v?\d+\.\d+\.\d+)`)

const versionParts = 3

// Check the version constraints of both terragrunt and terraform. Note that as a side effect this will set the
// following settings on terragruntOptions:
// - TerraformPath
// - TerraformVersion
// - FeatureFlags
// TODO: Look into a way to refactor this function to avoid the side effect.
func checkVersionConstraints(ctx context.Context, terragruntOptions *options.TerragruntOptions) error {
	configContext := config.NewParsingContext(ctx, terragruntOptions).WithDecodeList(
		config.TerragruntVersionConstraints, config.FeatureFlagsBlock)

	// TODO: See if we should be ignore this lint error
	partialTerragruntConfig, err := config.PartialParseConfigFile( //nolint: contextcheck
		configContext,
		terragruntOptions.TerragruntConfigPath,
		nil,
	)
	if err != nil {
		return err
	}

	// Change the terraform binary path before checking the version
	// if the path is not changed from default and set in the config.
	if terragruntOptions.TerraformPath == options.DefaultWrappedPath && partialTerragruntConfig.TerraformBinary != "" {
		terragruntOptions.TerraformPath = partialTerragruntConfig.TerraformBinary
	}

	if err := PopulateTerraformVersion(ctx, terragruntOptions); err != nil {
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

	if partialTerragruntConfig.FeatureFlags != nil {
		// update feature flags for evaluation
		for _, flag := range partialTerragruntConfig.FeatureFlags {
			flagName := flag.Name
			defaultValue, err := flag.DefaultAsString()

			if err != nil {
				return err
			}

			if _, exists := terragruntOptions.FeatureFlags.Load(flagName); !exists {
				terragruntOptions.FeatureFlags.Store(flagName, defaultValue)
			}
		}
	}

	return nil
}

// PopulateTerraformVersion populates the currently installed version of Terraform into the given terragruntOptions.
func PopulateTerraformVersion(ctx context.Context, terragruntOptions *options.TerragruntOptions) error {
	// Discard all log output to make sure we don't pollute stdout or stderr with this extra call to '--version'
	terragruntOptionsCopy, err := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return err
	}

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

	output, err := shell.RunTerraformCommandWithOutput(ctx, terragruntOptionsCopy, "--version")
	if err != nil {
		return err
	}

	terraformVersion, err := ParseTerraformVersion(output.Stdout.String())
	if err != nil {
		return err
	}

	tfImplementation, err := parseTerraformImplementationType(output.Stdout.String())
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

// CheckTerraformVersion checks that the currently installed Terraform version works meets the specified version constraint and return an error
// if it doesn't
func CheckTerraformVersion(constraint string, terragruntOptions *options.TerragruntOptions) error {
	return CheckTerraformVersionMeetsConstraint(terragruntOptions.TerraformVersion, constraint)
}

// CheckTerragruntVersion checks that the currently running Terragrunt version meets the specified version constraint and return an error
// if it doesn't
func CheckTerragruntVersion(constraint string, terragruntOptions *options.TerragruntOptions) error {
	return CheckTerragruntVersionMeetsConstraint(terragruntOptions.TerragruntVersion, constraint)
}

// CheckTerragruntVersionMeetsConstraint checks that the current version of Terragrunt meets the specified constraint and return an error if it doesn't
func CheckTerragruntVersionMeetsConstraint(currentVersion *version.Version, constraint string) error {
	versionConstraint, err := version.NewConstraint(constraint)
	if err != nil {
		return err
	}

	checkedVersion := currentVersion

	if currentVersion.Prerelease() != "" {
		// The logic in hashicorp/go-version is such that it will not consider a prerelease version to be
		// compatible with a constraint that does not have a prerelease version. This is not the behavior we want
		// for Terragrunt, so we strip the prerelease version before checking the constraint.
		//
		// https://github.com/hashicorp/go-version/issues/130
		checkedVersion = currentVersion.Core()
	}

	if !versionConstraint.Check(checkedVersion) {
		return errors.New(InvalidTerragruntVersion{CurrentVersion: currentVersion, VersionConstraints: versionConstraint})
	}

	return nil
}

// CheckTerraformVersionMeetsConstraint checks that the current version of Terraform meets the specified constraint and return an error if it doesn't
func CheckTerraformVersionMeetsConstraint(currentVersion *version.Version, constraint string) error {
	versionConstraint, err := version.NewConstraint(constraint)
	if err != nil {
		return err
	}

	if !versionConstraint.Check(currentVersion) {
		return errors.New(InvalidTerraformVersion{CurrentVersion: currentVersion, VersionConstraints: versionConstraint})
	}

	return nil
}

// ParseTerraformVersion parses the output of the terraform --version command
func ParseTerraformVersion(versionCommandOutput string) (*version.Version, error) {
	matches := TerraformVersionRegex.FindStringSubmatch(versionCommandOutput)

	if len(matches) != versionParts {
		return nil, errors.New(InvalidTerraformVersionSyntax(versionCommandOutput))
	}

	return version.NewVersion(matches[2])
}

// parseTerraformImplementationType - Parse terraform implementation from --version command output
func parseTerraformImplementationType(versionCommandOutput string) (options.TerraformImplementationType, error) {
	matches := TerraformVersionRegex.FindStringSubmatch(versionCommandOutput)

	if len(matches) != versionParts {
		return options.UnknownImpl, errors.New(InvalidTerraformVersionSyntax(versionCommandOutput))
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
	return "Unable to parse Terraform version output: " + string(err)
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
