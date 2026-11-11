// Package models provides the data structures used to represent Terraform providers and their details.
package models

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	semver "github.com/gruntwork-io/terragrunt/internal/semver"
)

// Providers is a list of provider address patterns.
type Providers []*Provider

// ParseProviders parses each of strs with [ParseProvider].
func ParseProviders(strs ...string) Providers {
	var providers Providers

	for _, str := range strs {
		if provider := ParseProvider(str); provider != nil {
			providers = append(providers, provider)
		}
	}

	return providers
}

// Find returns the first pattern in providers that matches target, or nil when none does.
func (providers Providers) Find(target *Provider) *Provider {
	for _, provider := range providers {
		if provider.Match(target) {
			return provider
		}
	}

	return nil
}

// SigningKey represents a key used to sign packages from a registry, along with an optional trust signature from the registry operator. These are both in ASCII armored OpenPGP format.
type SigningKey struct {
	ASCIIArmor     string `json:"ascii_armor"`
	TrustSignature string `json:"trust_signature"`
}

// SigningKeyList is the `signing_keys` object of a registry platform response.
type SigningKeyList struct {
	GPGPublicKeys []*SigningKey `json:"gpg_public_keys"`
}

// Keys maps the ASCII-armored form of each key in list to its trust signature.
func (list SigningKeyList) Keys() map[string]string {
	keys := make(map[string]string)

	for _, key := range list.GPGPublicKeys {
		keys[key.ASCIIArmor] = key.TrustSignature
	}

	return keys
}

// Versions is the list of versions a registry or mirror offers for one provider.
type Versions []*Version

// Version is one provider version in a registry versions response, with the platforms it supports.
type Version struct {
	Version   string    `json:"version"`
	Protocols []string  `json:"protocols"`
	Platforms Platforms `json:"platforms"`
}

func (version Version) String() string {
	return fmt.Sprintf("%s/%s/%s", version.Version, version.Protocols, version.Platforms)
}

// FilterValid returns only versions with valid semver strings that conform to
// the Terraform registry protocol (no "v" prefix, no empty strings).
// The second return value contains the invalid version strings that were filtered out.
func (versions Versions) FilterValid() (Versions, []string) {
	valid := make(Versions, 0, len(versions))
	invalid := make([]string, 0, len(versions))

	for _, v := range versions {
		if v.Version == "" || strings.HasPrefix(v.Version, "v") {
			invalid = append(invalid, v.Version)
			continue
		}

		if _, err := semver.Parse(v.Version); err != nil {
			invalid = append(invalid, v.Version)
			continue
		}

		valid = append(valid, v)
	}

	return valid, invalid
}

// Platforms is the list of platforms a provider version supports.
type Platforms []*Platform

// Platform is an operating system and CPU architecture pair.
type Platform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

func (platform Platform) String() string {
	return fmt.Sprintf("%s/%s", platform.OS, platform.Arch)
}

// PlatformPackage carries the per-platform hash and size data exposed by the
// OpenTofu provider registry. Consumers can populate `.terraform.lock.hcl`
// with hashes for every supported platform from a single response, without
// downloading each provider archive.
type PlatformPackage struct {
	Hashes      []string `json:"hashes"`
	PackageSize int64    `json:"package_size,omitempty"`
}

// PackageOrigin is the kind of installation source that described a provider
// package.
type PackageOrigin int

const (
	// OriginRegistry is a provider registry. The provider registry protocol
	// requires its download response to name the archive's checksum and the
	// document that lists it.
	OriginRegistry PackageOrigin = iota

	// OriginMirror is a network or filesystem mirror. The mirror protocol has
	// no checksum document.
	OriginMirror
)

// ResponseBody represents the details of the OpenTofu/Terraform provider received from a registry.
type ResponseBody struct {
	Packages map[string]*PlatformPackage `json:"packages,omitempty"`
	Platform
	Filename               string         `json:"filename"`
	DownloadURL            string         `json:"download_url"`
	SHA256SumsURL          string         `json:"shasums_url,omitempty"`
	SHA256SumsSignatureURL string         `json:"shasums_signature_url,omitempty"`
	SHA256Sum              string         `json:"shasum,omitempty"`
	Protocols              []string       `json:"protocols,omitempty"`
	SigningKeys            SigningKeyList `json:"signing_keys"`
	Origin                 PackageOrigin  `json:"-"`
}

// ResolveRelativeReferences returns a copy of body whose download, SHA256SUMS, and signature URLs
// are resolved against base. A relative path without a leading slash is joined to the path of base.
func (body *ResponseBody) ResolveRelativeReferences(base *url.URL) *ResponseBody {
	clone := *body
	clone.DownloadURL = resolveLink(base, body.DownloadURL)
	clone.SHA256SumsSignatureURL = resolveLink(base, body.SHA256SumsSignatureURL)
	clone.SHA256SumsURL = resolveLink(base, body.SHA256SumsURL)

	return &clone
}

// Provider represents the details of the Terraform provider.
type Provider struct {
	*ResponseBody

	RegistryName string
	Namespace    string
	Name         string
	Version      string
	OS           string
	Arch         string

	// OriginalConstraints holds the version constraints from the module's required_providers block
	OriginalConstraints string
}

// ParseProvider parses a provider address pattern of the form `registry/namespace/name`,
// `namespace/name`, or `registry`. A `*` part, or a part the pattern leaves out, matches any value.
func ParseProvider(str string) *Provider {
	parts := strings.Split(str, "/")
	for i := range parts {
		if parts[i] == "*" {
			parts[i] = ""
		}
	}

	const twoVals = 2

	switch {
	case len(parts) == twoVals:
		return &Provider{
			Namespace: parts[0],
			Name:      parts[1],
		}
	case len(parts) > twoVals:
		return &Provider{
			RegistryName: parts[0],
			Namespace:    parts[1],
			Name:         parts[2],
		}
	}

	return &Provider{
		RegistryName: parts[0],
	}
}

func (provider *Provider) String() string {
	if provider.Version != "" {
		return fmt.Sprintf(
			"%s/%s/%s v%s",
			provider.RegistryName,
			provider.Namespace,
			provider.Name,
			provider.Version,
		)
	}

	return fmt.Sprintf("%s/%s/%s", provider.RegistryName, provider.Namespace, provider.Name)
}

// Platform returns the `os_arch` name of the platform of provider, such as `linux_amd64`.
func (provider *Provider) Platform() string {
	return fmt.Sprintf("%s_%s", provider.OS, provider.Arch)
}

// Address returns the `registry/namespace/name` source address of provider.
func (provider *Provider) Address() string {
	return path.Join(provider.RegistryName, provider.Namespace, provider.Name)
}

// Constraints returns the version constraints from the module's `required_providers` block, or an empty string when it has none.
func (provider *Provider) Constraints() string {
	return provider.OriginalConstraints
}

// Match returns true if all defined provider properties are matched.
func (provider *Provider) Match(target *Provider) bool {
	registryNameMatch := provider.RegistryName == "" || target.RegistryName == "" ||
		provider.RegistryName == target.RegistryName
	namespaceMatch := provider.Namespace == "" || target.Namespace == "" ||
		provider.Namespace == target.Namespace
	nameMatch := provider.Name == "" || target.Name == "" || provider.Name == target.Name
	osMatch := provider.OS == "" || target.OS == "" || provider.OS == target.OS
	archMatch := provider.Arch == "" || target.Arch == "" || provider.Arch == target.Arch
	downloadURLMatch := provider.ResponseBody == nil || provider.DownloadURL == "" ||
		target.DownloadURL == "" ||
		provider.DownloadURL == target.DownloadURL

	if registryNameMatch && namespaceMatch && nameMatch && osMatch && archMatch &&
		downloadURLMatch {
		return true
	}

	return false
}
