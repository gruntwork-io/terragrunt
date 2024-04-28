package models

import (
	"fmt"
	"net/url"
	"path"

	"github.com/gruntwork-io/terragrunt/terraform/getproviders"
)

// Provider represents the details of the Terraform provider.
type Provider struct {
	*getproviders.Package

	RegistryName string
	Namespace    string
	Name         string
	Version      string
	OS           string
	Arch         string
}

func NewProviderFromDownloadURL(downloadURL string) *Provider {
	return &Provider{
		Package: &getproviders.Package{
			DownloadURL: downloadURL,
		},
	}
}

// VersionURL returns the URL used to query the all Versions for a single provider.
// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-all-versions-for-a-single-provider
func (provider *Provider) VersionURL() *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   provider.RegistryName,
		Path:   path.Join("/v1/providers", provider.Namespace, provider.Name, "versions"),
	}
}

// PlatformURL returns the URL used to query the all platforms for a single version.
// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-all-platforms-for-a-single-version
func (provider *Provider) PlatformURL() *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   provider.RegistryName,
		Path:   path.Join("/v1/providers", provider.Namespace, provider.Name, provider.Version, "download", provider.OS, provider.Arch),
	}
}

func (provider *Provider) String() string {
	return fmt.Sprintf("%s/%s/%s v%s", provider.RegistryName, provider.Namespace, provider.Name, provider.Version)
}

func (provider *Provider) Platform() string {
	return fmt.Sprintf("%s_%s", provider.OS, provider.Arch)
}

func (provider *Provider) Address() string {
	return path.Join(provider.RegistryName, provider.Namespace, provider.Name)
}

// Match returns true if all defined provider properties are matched.
func (provider *Provider) Match(target *Provider) bool {
	if (provider.RegistryName == "" || target.RegistryName == "" || provider.RegistryName == target.RegistryName) &&
		(provider.Namespace == "" || target.Namespace == "" || provider.Namespace == target.Namespace) &&
		(provider.Name == "" || target.Name == "" || provider.Name == target.Name) &&
		(provider.Version == "" || target.Version == "" || provider.Version == target.Version) &&
		(provider.OS == "" || target.OS == "" || provider.OS == target.OS) &&
		(provider.Arch == "" || target.Arch == "" || provider.Arch == target.Arch) &&
		(provider.DownloadURL == "" || target.DownloadURL == "" || provider.DownloadURL == target.DownloadURL) {
		return true
	}
	return false
}
