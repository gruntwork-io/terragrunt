package getter

import (
	"context"
	"net/url"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/redact"
)

// checksumFilePrefix marks a checksum value that names a file of checksums.
const checksumFilePrefix = "file:"

// ChecksumResolver keys a request by the checksum its URL pins; no remote probe runs, so it answers offline too.
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

// Pinned reports true for a literal checksum and false for a checksum file reference.
func (r *ChecksumResolver) Pinned(_ redact.URL) bool { return !r.fromFile() }

// ProbesLocally reports true: Probe reads only the request URL.
func (r *ChecksumResolver) ProbesLocally(_ redact.URL) bool { return true }

// Probe keys source by its checksum-free URL and lowercased checksum; a file reference has no key.
func (r *ChecksumResolver) Probe(_ context.Context, source redact.URL) (string, error) {
	if r.fromFile() {
		return "", cas.ErrNoVersionMetadata
	}

	return cas.OpaqueKey(r.scheme, withoutChecksum(source.Reveal()), "checksum:"+strings.ToLower(r.checksum)), nil
}

// fromFile reports whether the checksum value is a file reference.
func (r *ChecksumResolver) fromFile() bool {
	return strings.HasPrefix(r.checksum, checksumFilePrefix)
}

// withoutChecksum returns rawURL with its checksum query parameter removed.
func withoutChecksum(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	q := u.Query()
	q.Del(checksumQueryKey)
	u.RawQuery = q.Encode()

	return u.String()
}
