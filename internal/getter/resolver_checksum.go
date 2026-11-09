package getter

import (
	"context"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/redact"
)

// checksumFilePrefix marks a checksum value that go-getter reads from a file instead of the URL.
const checksumFilePrefix = "file:"

// ChecksumResolver keys a request by the checksum its URL pins; no remote probe runs and --cas-offline applies.
type ChecksumResolver struct {
	scheme   string
	checksum string
}

// NewChecksumResolver returns the resolver for a request on scheme whose URL carries checksum.
func NewChecksumResolver(scheme, checksum string) *ChecksumResolver {
	return &ChecksumResolver{scheme: scheme, checksum: checksum}
}

// Scheme returns the URL scheme of the request the resolver was built for.
func (r *ChecksumResolver) Scheme() string { return r.scheme }

// Pinned reports whether the checksum is a literal, which names content that cannot change upstream.
func (r *ChecksumResolver) Pinned(_ redact.URL) bool { return !r.fromFile() }

// Probe keys source by its literal checksum and URL, since the URL decides how the download is unpacked.
func (r *ChecksumResolver) Probe(_ context.Context, source redact.URL) (string, error) {
	if r.fromFile() {
		// The checksum is known only once the inner fetch reads the file, so every fetch downloads and verifies.
		return "", cas.ErrNoVersionMetadata
	}

	return cas.OpaqueKey(r.scheme, source.Reveal(), "checksum:"+strings.ToLower(r.checksum)), nil
}

// fromFile reports whether the checksum value is a file reference.
func (r *ChecksumResolver) fromFile() bool {
	return strings.HasPrefix(r.checksum, checksumFilePrefix)
}
