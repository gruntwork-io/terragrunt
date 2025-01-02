// Package models provides the data structures used to represent Terraform providers and their details.
package models

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

type Providers []*Provider

func ParseProviders(strs ...string) Providers {
	var prvoiders Providers

	for _, str := range strs {
		if provider := ParseProvider(str); provider != nil {
			prvoiders = append(prvoiders, provider)
		}
	}

	return prvoiders
}

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

type SigningKeyList struct {
	GPGPublicKeys []*SigningKey `json:"gpg_public_keys"`
}

func (list SigningKeyList) Keys() map[string]string {
	keys := make(map[string]string)

	for _, key := range list.GPGPublicKeys {
		keys[key.ASCIIArmor] = key.TrustSignature
	}

	return keys
}

type Version struct {
	Version   string      `json:"version"`
	Protocols []string    `json:"protocols"`
	Platforms []*Platform `json:"platforms"`
}

type Platform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// ResponseBody represents the details of the Terraform provider received from a registry.
type ResponseBody struct {
	Platform

	Protocols []string `json:"protocols,omitempty"`
	Filename  string   `json:"filename"`

	DownloadURL            string `json:"download_url"`
	SHA256SumsURL          string `json:"shasums_url,omitempty"`
	SHA256SumsSignatureURL string `json:"shasums_signature_url,omitempty"`

	SHA256Sum   string         `json:"shasum,omitempty"`
	SigningKeys SigningKeyList `json:"signing_keys,omitempty"`
}

func (body ResponseBody) ResolveRelativeReferences(base *url.URL) *ResponseBody {
	body.DownloadURL = resolveRelativeReference(base, body.DownloadURL)
	body.SHA256SumsSignatureURL = resolveRelativeReference(base, body.SHA256SumsSignatureURL)
	body.SHA256SumsURL = resolveRelativeReference(base, body.SHA256SumsURL)

	return &body
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
}

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
	registryNameMatch := provider.RegistryName == "" || target.RegistryName == "" || provider.RegistryName == target.RegistryName
	namespaceMatch := provider.Namespace == "" || target.Namespace == "" || provider.Namespace == target.Namespace
	nameMatch := provider.Name == "" || target.Name == "" || provider.Name == target.Name
	osMatch := provider.OS == "" || target.OS == "" || provider.OS == target.OS
	archMatch := provider.Arch == "" || target.Arch == "" || provider.Arch == target.Arch
	downloadURLMatch := provider.ResponseBody == nil || provider.DownloadURL == "" || target.DownloadURL == "" || provider.DownloadURL == target.DownloadURL

	if registryNameMatch && namespaceMatch && nameMatch && osMatch && archMatch && downloadURLMatch {
		return true
	}

	return false
}
