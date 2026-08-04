package getter

import (
	"errors"
	"fmt"
)

// The errors raised while resolving OCI credentials, from the OpenTofu CLI config and from ambient Docker config.

// ErrOCIHelperMalformedOutput reports a credential helper whose output is not valid JSON.
var ErrOCIHelperMalformedOutput = errors.New("oci credential helper returned malformed output")

// ErrOCIAmbientConfigFile reports a docker_style_config_files entry that cannot be read.
var ErrOCIAmbientConfigFile = errors.New("cannot read docker_style_config_files entry")

// ErrOCIInvalidHelperName reports a helper name that is empty or contains a path separator.
var ErrOCIInvalidHelperName = errors.New(
	"credential helper name must be the suffix of a docker-credential-<name> program on PATH, not a path",
)

// ErrOCIMissingCredentialStyle reports an oci_credentials block configuring no credential at all.
var ErrOCIMissingCredentialStyle = errors.New(
	"oci_credentials block must configure basic auth, OAuth tokens, or a helper",
)

// ErrOCIMultipleCredentialStyles reports a block configuring more than one credential style.
var ErrOCIMultipleCredentialStyles = errors.New("oci_credentials block must configure at most one credential style")

// ErrOCIIncompleteBasicCredential reports an oci_credentials block missing a username or password.
var ErrOCIIncompleteBasicCredential = errors.New("oci_credentials basic auth requires both a username and a password")

// ErrOCIIncompleteOAuthCredential reports an oci_credentials block missing an access or refresh token.
var ErrOCIIncompleteOAuthCredential = errors.New(
	"oci_credentials oauth requires both an access_token and a refresh_token",
)

// ErrOCISubdirNotFound reports a module subdir matching nothing in the extracted archive.
var ErrOCISubdirNotFound = errors.New("module subdir not found in the archive")

// ErrOCISubdirAmbiguous reports a module subdir glob matching more than one path in the archive.
var ErrOCISubdirAmbiguous = errors.New("module subdir matches multiple paths in the archive")

// ErrOCIHelperWithRepositoryPath reports a helper on a repository-scoped label, which tofu rejects.
var ErrOCIHelperWithRepositoryPath = errors.New(
	"oci_credentials docker_credentials_helper cannot be used with a repository path",
)

// ErrOCIAmbientFilesWithoutDiscovery reports docker_style_config_files set while ambient discovery is off.
var ErrOCIAmbientFilesWithoutDiscovery = errors.New(
	"oci_default_credentials docker_style_config_files requires discover_ambient_credentials to be enabled",
)

// ErrOCIDuplicateDefaultBlock reports a second oci_default_credentials block, which tofu rejects.
var ErrOCIDuplicateDefaultBlock = errors.New("at most one oci_default_credentials block is allowed")

// ErrOCIDuplicateRepoBlock reports two oci_credentials blocks sharing a label, which tofu rejects.
var ErrOCIDuplicateRepoBlock = errors.New("duplicate oci_credentials block")

// ErrOCIEmptyCredentialValue reports a credential argument set to an empty string.
var ErrOCIEmptyCredentialValue = errors.New("oci_credentials values must not be empty")

// ErrOCILabelNotRepositoryAddress reports a label that is not a bare registry domain and repository path.
var ErrOCILabelNotRepositoryAddress = errors.New(
	"oci_credentials label must be a registry domain with an optional repository path, without a URL scheme",
)

// OCICredentialHelperError reports a helper failure, carrying stderr diagnostics but never the secret stdout.
type OCICredentialHelperError struct {
	Err      error
	Helper   string
	Registry string
	Stderr   string
}

func (err OCICredentialHelperError) Error() string {
	if err.Stderr != "" {
		return fmt.Sprintf("oci credential helper %q for %s: %v: %s", err.Helper, err.Registry, err.Err, err.Stderr)
	}

	return fmt.Sprintf("oci credential helper %q for %s: %v", err.Helper, err.Registry, err.Err)
}

func (err OCICredentialHelperError) Unwrap() error {
	return err.Err
}

// OCIAmbientConfigFileError names the docker_style_config_files entry that could not be read.
type OCIAmbientConfigFileError struct {
	Err  error
	Path string
}

func (err OCIAmbientConfigFileError) Error() string {
	return fmt.Sprintf("%s %s: %v", ErrOCIAmbientConfigFile, err.Path, err.Err)
}

// Unwrap exposes both the class sentinel and the read failure, so either matches with errors.Is.
func (err OCIAmbientConfigFileError) Unwrap() []error {
	return []error{ErrOCIAmbientConfigFile, err.Err}
}

// OCIDuplicateRepoBlockError names the oci_credentials label declared twice, and the file redeclaring it.
type OCIDuplicateRepoBlockError struct {
	Label string
	Path  string
}

func (err OCIDuplicateRepoBlockError) Error() string {
	return fmt.Sprintf("%s %q in %s", ErrOCIDuplicateRepoBlock, err.Label, err.Path)
}

func (err OCIDuplicateRepoBlockError) Unwrap() error {
	return ErrOCIDuplicateRepoBlock
}

// OCIDuplicateDefaultBlockError names the file declaring a second oci_default_credentials block.
type OCIDuplicateDefaultBlockError struct {
	Path string
}

func (err OCIDuplicateDefaultBlockError) Error() string {
	if err.Path == "" {
		return ErrOCIDuplicateDefaultBlock.Error()
	}

	return fmt.Sprintf("%s: %s", ErrOCIDuplicateDefaultBlock, err.Path)
}

func (err OCIDuplicateDefaultBlockError) Unwrap() error {
	return ErrOCIDuplicateDefaultBlock
}
