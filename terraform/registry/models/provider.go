package models

import (
	"fmt"
	"net/url"
	"path"
)

type Provider struct {
	RegistryName string
	Namespace    string
	Name         string
	Version      string
	OS           string
	Arch         string

	DownloadURL *url.URL
}

func (provider *Provider) PackageURL() *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   provider.RegistryName,
		Path:   path.Join("/v1/providers", provider.Namespace, provider.Name, provider.Version, "download", provider.OS, provider.Arch),
	}
}

func (provider *Provider) VersionURL() *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   provider.RegistryName,
		Path:   path.Join("/v1/providers", provider.Namespace, provider.Name, "versions"),
	}
}

func (provider *Provider) Platform() string {
	return fmt.Sprintf("%s_%s", provider.OS, provider.Arch)
}

func (provider *Provider) String() string {
	return path.Join(provider.RegistryName, provider.Namespace, provider.Name, provider.Version)
}

func (provider *Provider) Match(target *Provider) bool {
	if (provider.RegistryName == "" || target.RegistryName == "" || provider.RegistryName == target.RegistryName) &&
		(provider.Namespace == "" || target.Namespace == "" || provider.Namespace == target.Namespace) &&
		(provider.Name == "" || target.Name == "" || provider.Name == target.Name) &&
		(provider.Version == "" || target.Version == "" || provider.Version == target.Version) &&
		(provider.OS == "" || target.OS == "" || provider.OS == target.OS) &&
		(provider.Arch == "" || target.Arch == "" || provider.Arch == target.Arch) &&
		(provider.DownloadURL == nil || target.DownloadURL == nil || provider.DownloadURL.String() == target.DownloadURL.String()) {
		return true
	}
	return false
}
