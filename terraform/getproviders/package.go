package getproviders

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

var AvailablePlatforms []Platform = []Platform{
	{OS: "solaris", Arch: "amd64"},
	{OS: "openbsd", Arch: "386"},
	{OS: "openbsd", Arch: "arm"},
	{OS: "openbsd", Arch: "amd64"},
	{OS: "freebsd", Arch: "386"},
	{OS: "freebsd", Arch: "arm"},
	{OS: "freebsd", Arch: "amd64"},
	{OS: "linux", Arch: "386"},
	{OS: "linux", Arch: "arm"},
	{OS: "linux", Arch: "arm64"},
	{OS: "linux", Arch: "amd64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
	{OS: "windows", Arch: "386"},
	{OS: "windows", Arch: "amd64"},
}

// SigningKey represents a key used to sign packages from a registry, along with an optional trust signature from the registry operator. These are both in ASCII armored OpenPGP format.
type SigningKey struct {
	ASCIIArmor     string `json:"ascii_armor"`
	TrustSignature string `json:"trust_signature"`
}

type SigningKeyList struct {
	GPGPublicKeys []*SigningKey `json:"gpg_public_keys"`
}

type Version struct {
	Version   string     `json:"version"`
	Protocols []string   `json:"protocols"`
	Platforms []Platform `json:"platforms"`
}

type Platform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// Package represents the details of the Terraform provider.
type Package struct {
	Platform

	Protocols []string `json:"protocols,omitempty"`
	Filename  string   `json:"filename"`

	DownloadURL            string `json:"download_url"`
	SHA256SumsURL          string `json:"shasums_url,omitempty"`
	SHA256SumsSignatureURL string `json:"shasums_signature_url,omitempty"`

	SHA256Sum   string         `json:"shasum,omitempty"`
	SigningKeys SigningKeyList `json:"signing_keys,omitempty"`
}

func (provider *Package) Checksum() ([sha256.Size]byte, error) {
	var checksum [sha256.Size]byte

	if _, err := hex.Decode(checksum[:], []byte(provider.SHA256Sum)); err != nil {
		return checksum, errors.Errorf("registry response includes invalid SHA256 hash %q: %w", provider.SHA256Sum, err)
	}
	return checksum, nil
}

func (provider *Package) FetchSignature(ctx context.Context) ([]byte, error) {
	var signature = new(bytes.Buffer)

	if err := util.Fetch(ctx, provider.SHA256SumsSignatureURL, signature); err != nil {
		return nil, fmt.Errorf("failed to retrieve authentication checksums: %w", err)
	}

	return signature.Bytes(), nil
}

func (provider *Package) FetchSHA256Sums(ctx context.Context) ([]byte, error) {
	var document = new(bytes.Buffer)

	if err := util.Fetch(ctx, provider.SHA256SumsURL, document); err != nil {
		return nil, fmt.Errorf("failed to retrieve authentication checksums: %w", err)
	}

	return document.Bytes(), nil
}

func (provider *Package) FetchArchive(ctx context.Context, saveTo string) error {
	return util.FetchToFile(ctx, provider.DownloadURL, saveTo)
}
