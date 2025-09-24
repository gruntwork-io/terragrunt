package run

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"encoding/hex"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
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

// CheckVersionConstraints checks the version constraints of both terragrunt and terraform. Note that as a side effect this will set the
// following settings on terragruntOptions:
// - TerraformPath
// - TerraformVersion
// - FeatureFlags
// TODO: Look into a way to refactor this function to avoid the side effect.
func CheckVersionConstraints(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions) (log.Logger, error) {
	partialTerragruntConfig, err := getTerragruntConfig(ctx, l, terragruntOptions)
	if err != nil {
		return l, err
	}

	// If the TFPath is not explicitly set, use the TFPath from the config if it is set.
	if !terragruntOptions.TFPathExplicitlySet && partialTerragruntConfig.TerraformBinary != "" {
		terragruntOptions.TFPath = partialTerragruntConfig.TerraformBinary
	}

	l, err = PopulateTFVersion(ctx, l, terragruntOptions)
	if err != nil {
		return l, err
	}

	terraformVersionConstraint := DefaultTerraformVersionConstraint
	if partialTerragruntConfig.TerraformVersionConstraint != "" {
		terraformVersionConstraint = partialTerragruntConfig.TerraformVersionConstraint
	}

	if err := CheckTerraformVersion(terraformVersionConstraint, terragruntOptions); err != nil {
		return l, err
	}

	if partialTerragruntConfig.TerragruntVersionConstraint != "" {
		if err := CheckTerragruntVersion(partialTerragruntConfig.TerragruntVersionConstraint, terragruntOptions); err != nil {
			return l, err
		}
	}

	if partialTerragruntConfig.FeatureFlags != nil {
		// update feature flags for evaluation
		for _, flag := range partialTerragruntConfig.FeatureFlags {
			flagName := flag.Name

			defaultValue, err := flag.DefaultAsString()
			if err != nil {
				return l, err
			}

			if _, exists := terragruntOptions.FeatureFlags.Load(flagName); !exists {
				terragruntOptions.FeatureFlags.Store(flagName, defaultValue)
			}
		}
	}

	return l, nil
}

// PopulateTFVersion populates the currently installed version of OpenTofuTerraform into the given terragruntOptions.
//
// The caller also gets a copy of the logger with the config path set.
func PopulateTFVersion(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (log.Logger, error) {
	versionCache := GetRunVersionCache(ctx)
	cacheKey := computeVersionFilesCacheKey(opts.WorkingDir, opts.VersionManagerFileName)
	l.Debugf("using cache key for version files: %s", cacheKey)

	if cachedOutput, found := versionCache.Get(ctx, cacheKey); found {
		tfImplementation, terraformVersion, err := parseVersionFromCache(cachedOutput)
		if err != nil {
			return l, err
		}

		opts.TerraformVersion = terraformVersion
		opts.TerraformImplementation = tfImplementation

		return l, nil
	}

	l, terraformVersion, tfImplementation, err := GetTFVersion(ctx, l, opts)
	if err != nil {
		return l, err
	}

	// Save output to cache using minimal format
	cacheData := formatVersionForCache(tfImplementation, terraformVersion)
	versionCache.Put(ctx, cacheKey, cacheData)

	opts.TerraformVersion = terraformVersion
	opts.TerraformImplementation = tfImplementation

	return l, nil
}

// formatVersionForCache formats the implementation and version for the cache
func formatVersionForCache(implementation options.TerraformImplementationType, version *version.Version) string {
	var implStr string

	switch implementation {
	case options.TerraformImpl:
		implStr = "terraform"
	case options.OpenTofuImpl:
		implStr = "opentofu"
	case options.UnknownImpl:
		implStr = "unknown"
	}

	return fmt.Sprintf("%s:%s", implStr, version.String())
}

// parseVersionFromCache parses the cache format back to implementation and version for options
func parseVersionFromCache(cachedData string) (options.TerraformImplementationType, *version.Version, error) {
	const expectedParts = 2

	parts := strings.SplitN(cachedData, ":", expectedParts)
	if len(parts) != expectedParts {
		return options.UnknownImpl, nil, errors.New(InvalidTerraformVersionSyntax(cachedData))
	}

	implStr := strings.ToLower(parts[0])
	versionStr := parts[1]

	var implementation options.TerraformImplementationType

	switch implStr {
	case "terraform":
		implementation = options.TerraformImpl
	case "opentofu":
		implementation = options.OpenTofuImpl
	default:
		implementation = options.UnknownImpl
	}

	version, err := version.NewVersion(versionStr)
	if err != nil {
		return options.UnknownImpl, nil, err
	}

	return implementation, version, nil
}

// GetTFVersion checks the OpenTofu/Terraform version directly without using cache.
// This function can be used independently when you need to check the version without
// populating or using the version cache.
func GetTFVersion(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (log.Logger, *version.Version, options.TerraformImplementationType, error) {
	l, optsCopy, err := opts.CloneWithConfigPath(l, opts.TerragruntConfigPath)
	if err != nil {
		return l, nil, options.UnknownImpl, err
	}

	optsCopy.Writer = io.Discard
	optsCopy.ErrWriter = io.Discard

	for key := range optsCopy.Env {
		if strings.HasPrefix(key, "TF_CLI_ARGS") {
			delete(optsCopy.Env, key)
		}
	}

	output, err := tf.RunCommandWithOutput(ctx, l, optsCopy, tf.FlagNameVersion)
	if err != nil {
		return l, nil, options.UnknownImpl, err
	}

	terraformVersion, err := ParseTerraformVersion(output.Stdout.String())
	if err != nil {
		return l, nil, options.UnknownImpl, err
	}

	tfImplementation, err := parseTerraformImplementationType(output.Stdout.String())
	if err != nil {
		return l, nil, options.UnknownImpl, err
	}

	if tfImplementation == options.UnknownImpl {
		tfImplementation = options.TerraformImpl

		l.Warnf("Failed to identify Terraform implementation, fallback to terraform version: %s", terraformVersion)
	} else {
		l.Debugf("%s version: %s", tfImplementation, terraformVersion)
	}

	return l, terraformVersion, tfImplementation, nil
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

// Helper to compute a cache key from the checksums of provided files
func computeVersionFilesCacheKey(workingDir string, versionFiles []string) string {
	var hashes []string

	for _, file := range versionFiles {
		path, err := util.SanitizePath(workingDir, file)
		if err != nil {
			continue
		}

		if util.FileExists(path) {
			hash, err := util.FileSHA256(path)
			if err == nil {
				// We use `file` as part of the cache key because the `path` becomes an absolute path after sanitization.
				// Without implementing a full "mock filesystem", this would be difficult to test currently.
				// Note: This approach may potentially create duplicate cache files in some edge cases.
				hashes = append(hashes, file+":"+hex.EncodeToString(hash))
			}
		}
	}

	cacheKey := "no-version-files"

	if len(hashes) != 0 {
		cacheKey = strings.Join(hashes, "|")
	}

	return util.EncodeBase64Sha1(cacheKey)
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
