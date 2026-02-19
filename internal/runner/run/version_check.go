package run

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

// PopulateTFVersion determines the currently installed version of OpenTofu/Terraform.
// Returns the version and implementation type so the caller can write them back
// to whichever options struct it is using.
func PopulateTFVersion(ctx context.Context, l log.Logger, opts *Options) (log.Logger, *version.Version, tfimpl.Type, error) {
	versionCache := GetRunVersionCache(ctx)
	cacheKey := computeVersionFilesCacheKey(opts.WorkingDir, opts.VersionManagerFileName)
	l.Debugf("using cache key for version files: %s", cacheKey)

	if cachedOutput, found := versionCache.Get(ctx, cacheKey); found {
		tfImplementation, terraformVersion, err := parseVersionFromCache(cachedOutput)
		if err != nil {
			return l, nil, tfimpl.Unknown, err
		}

		return l, terraformVersion, tfImplementation, nil
	}

	l, terraformVersion, tfImplementation, err := GetTFVersion(ctx, l, opts)
	if err != nil {
		return l, nil, tfimpl.Unknown, err
	}

	cacheData := formatVersionForCache(tfImplementation, terraformVersion)
	versionCache.Put(ctx, cacheKey, cacheData)

	return l, terraformVersion, tfImplementation, nil
}

// formatVersionForCache formats the implementation and version for the cache
func formatVersionForCache(implementation tfimpl.Type, version *version.Version) string {
	var implStr string

	switch implementation {
	case tfimpl.Terraform:
		implStr = "terraform"
	case tfimpl.OpenTofu:
		implStr = "opentofu"
	case tfimpl.Unknown:
		implStr = "unknown"
	}

	return fmt.Sprintf("%s:%s", implStr, version.String())
}

// parseVersionFromCache parses the cache format back to implementation and version for options
func parseVersionFromCache(cachedData string) (tfimpl.Type, *version.Version, error) {
	const expectedParts = 2

	parts := strings.SplitN(cachedData, ":", expectedParts)
	if len(parts) != expectedParts {
		return tfimpl.Unknown, nil, errors.New(InvalidTerraformVersionSyntax(cachedData))
	}

	implStr := strings.ToLower(parts[0])
	versionStr := parts[1]

	var implementation tfimpl.Type

	switch implStr {
	case "terraform":
		implementation = tfimpl.Terraform
	case "opentofu":
		implementation = tfimpl.OpenTofu
	default:
		implementation = tfimpl.Unknown
	}

	version, err := version.NewVersion(versionStr)
	if err != nil {
		return tfimpl.Unknown, nil, err
	}

	return implementation, version, nil
}

// GetTFVersion checks the OpenTofu/Terraform version directly without using cache.
// This function can be used independently when you need to check the version without
// populating or using the version cache.
func GetTFVersion(ctx context.Context, l log.Logger, opts *Options) (log.Logger, *version.Version, tfimpl.Type, error) {
	_, optsCopy, err := opts.CloneWithConfigPath(l, opts.TerragruntConfigPath)
	if err != nil {
		return l, nil, tfimpl.Unknown, err
	}

	optsCopy.Writer = io.Discard
	optsCopy.ErrWriter = io.Discard

	for key := range optsCopy.Env {
		if strings.HasPrefix(key, "TF_CLI_ARGS") {
			delete(optsCopy.Env, key)
		}
	}

	output, err := tf.RunCommandWithOutput(ctx, l, optsCopy.tfRunOptions(), tf.FlagNameVersion)
	if err != nil {
		return l, nil, tfimpl.Unknown, err
	}

	terraformVersion, err := ParseTerraformVersion(output.Stdout.String())
	if err != nil {
		return l, nil, tfimpl.Unknown, err
	}

	tfImplementation, err := parseTerraformImplementationType(output.Stdout.String())
	if err != nil {
		return l, nil, tfimpl.Unknown, err
	}

	if tfImplementation == tfimpl.Unknown {
		tfImplementation = tfimpl.Terraform

		l.Warnf("Failed to identify Terraform implementation, fallback to terraform version: %s", terraformVersion)
	} else {
		l.Debugf("%s version: %s", tfImplementation, terraformVersion)
	}

	return l, terraformVersion, tfImplementation, nil
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
func parseTerraformImplementationType(versionCommandOutput string) (tfimpl.Type, error) {
	matches := TerraformVersionRegex.FindStringSubmatch(versionCommandOutput)

	if len(matches) != versionParts {
		return tfimpl.Unknown, errors.New(InvalidTerraformVersionSyntax(versionCommandOutput))
	}

	rawType := strings.ToLower(matches[1])
	switch rawType {
	case "terraform":
		return tfimpl.Terraform, nil
	case "opentofu":
		return tfimpl.OpenTofu, nil
	default:
		return tfimpl.Unknown, nil
	}
}

// Helper to compute a cache key from the checksums of provided files
func computeVersionFilesCacheKey(workingDir string, versionFiles []string) string {
	var hashes []string

	for _, file := range versionFiles {
		path := filepath.Join(workingDir, file)

		if !util.FileExists(path) {
			continue
		}

		sanitizedPath, err := util.SanitizePath(workingDir, file)
		if err != nil {
			sanitizedPath = path
		}

		hash, err := util.FileSHA256(sanitizedPath)
		if err == nil {
			hashes = append(hashes, file+":"+hex.EncodeToString(hash))
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
