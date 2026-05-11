package getter

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"

	"github.com/gruntwork-io/terragrunt/internal/cas"
)

// gcsResolverTimeout caps the Attrs call so a slow remote can't stall CAS
// dispatch.
const gcsResolverTimeout = 10 * time.Second

// gcsCanonicalPathSegments is the segment count produced by splitting
// `storage/<version>/<bucket>/<object>` on "/" with limit 4.
const gcsCanonicalPathSegments = 4

// ErrGCSMissingBucket is returned when a gs:// URL has no host segment.
var ErrGCSMissingBucket = errors.New("missing bucket in GCS URL")

// ErrGCSMissingObject is returned when a GCS URL names a bucket but no object.
var ErrGCSMissingObject = errors.New("missing object in GCS URL")

// ErrGCSUnrecognizedURL is returned when an http(s) URL does not match the
// canonical /storage/<version>/<bucket>/<object> shape.
var ErrGCSUnrecognizedURL = errors.New("not a recognized GCS URL")

// ErrGCSUnsupportedScheme is returned when the URL scheme is neither gs nor http(s).
var ErrGCSUnsupportedScheme = errors.New("unsupported GCS URL scheme")

// GCSObject is the subset of *storage.ObjectHandle a resolver uses.
type GCSObject interface {
	Attrs(ctx context.Context) (*storage.ObjectAttrs, error)
}

// GCSClient is the subset of *storage.Client a resolver uses.
type GCSClient interface {
	Object(bucket, object string) GCSObject
	Close() error
}

// GCSResolver is a [cas.SourceResolver] for objects in Google Cloud
// Storage.
type GCSResolver struct {
	// NewClient builds a GCS client per request. Nil means
	// [storage.NewClient] with the ambient application default
	// credentials.
	NewClient func(ctx context.Context) (GCSClient, error)
}

// NewGCSResolver returns a resolver wired to the ambient ADC.
func NewGCSResolver() *GCSResolver { return &GCSResolver{} }

// Scheme returns "gcs".
func (r *GCSResolver) Scheme() string { return "gcs" }

// Probe reads object metadata via ObjectHandle.Attrs and returns a
// content-addressed cache key from MD5 (when present) or CRC32C
// (always populated by GCS). Errors surface as
// [cas.ErrNoVersionMetadata].
func (r *GCSResolver) Probe(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse GCS URL %s: %w", rawURL, err)
	}

	bucket, object, err := parseGCSURL(u)
	if err != nil {
		return "", fmt.Errorf("parse GCS URL %s: %w", rawURL, err)
	}

	ctx, cancel := context.WithTimeout(ctx, gcsResolverTimeout)
	defer cancel()

	client, err := r.client(ctx)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	key, probeErr := r.pickGCSCacheKeyFromAttrs(ctx, client, bucket, object)

	// Close errors only surface on the success path. On probe
	// failure the primary error already explains the outcome, and
	// joining a close error would mask the sentinel
	// (ErrNoVersionMetadata) callers test for.
	closeErr := client.Close()

	if probeErr != nil {
		return "", probeErr
	}

	if closeErr != nil {
		return "", fmt.Errorf("close GCS client: %w", closeErr)
	}

	return key, nil
}

// pickGCSCacheKeyFromAttrs fetches object metadata through client and
// returns the cascade-derived cache key. ErrNoVersionMetadata signals
// either a failed Attrs call or a nil attrs payload.
func (r *GCSResolver) pickGCSCacheKeyFromAttrs(
	ctx context.Context,
	client GCSClient,
	bucket, object string,
) (string, error) {
	attrs, err := client.Object(bucket, object).Attrs(ctx)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	return pickGCSCacheKey(attrs)
}

func (r *GCSResolver) client(ctx context.Context) (GCSClient, error) {
	if r.NewClient != nil {
		return r.NewClient(ctx)
	}

	c, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	return &storageClientAdapter{c: c}, nil
}

// pickGCSCacheKey walks the cascade MD5 → CRC32C and returns the
// cache key for the first match. CRC32C participates whenever MD5 is
// absent; its value zero is a real checksum (the empty object is the
// canonical example, but other byte sequences also collide on 0), not
// a "missing" sentinel. Older code gated CRC32C on `!= 0` and
// silently downgraded zero-CRC32C content to URL-scoped opaque keys.
//
// GCS populates CRC32C for every object the SDK returns Attrs for, so
// the cascade does not need an ETag fallback in practice. A nil attrs
// payload (only reachable from a fake test client or an SDK regression)
// surfaces as ErrNoVersionMetadata so the caller falls back to content
// hashing.
func pickGCSCacheKey(attrs *storage.ObjectAttrs) (string, error) {
	if attrs == nil {
		return "", cas.ErrNoVersionMetadata
	}

	if len(attrs.MD5) > 0 {
		return cas.ContentKey("md5", hex.EncodeToString(attrs.MD5)), nil
	}

	return cas.ContentKey("crc32c", strconv.FormatUint(uint64(attrs.CRC32C), 16)), nil
}

// parseGCSURL extracts bucket and object from either canonical form.
// Accepts `https://www.googleapis.com/storage/v1/<bucket>/<object>` and
// `gs://<bucket>/<object>`. URLs that name a bucket but no object are
// rejected at parse time so callers do not pay a doomed Attrs round
// trip for an SDK-shaped not-found error.
func parseGCSURL(u *url.URL) (bucket, object string, err error) {
	switch strings.ToLower(u.Scheme) {
	case "gs":
		object = strings.TrimPrefix(u.Path, "/")
		bucket = u.Host

		if bucket == "" {
			return "", "", fmt.Errorf("%w: %q", ErrGCSMissingBucket, u.String())
		}

		if object == "" {
			return "", "", fmt.Errorf("%w: %q", ErrGCSMissingObject, u.String())
		}

		return bucket, object, nil
	case "http", "https":
		// Canonical: /storage/<version>/<bucket>/<object...>
		parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", gcsCanonicalPathSegments)
		if len(parts) < gcsCanonicalPathSegments || parts[0] != "storage" {
			return "", "", fmt.Errorf("%w: %q", ErrGCSUnrecognizedURL, u.String())
		}

		if parts[3] == "" {
			return "", "", fmt.Errorf("%w: %q", ErrGCSMissingObject, u.String())
		}

		return parts[2], parts[3], nil
	}

	return "", "", fmt.Errorf("%w: %q", ErrGCSUnsupportedScheme, u.Scheme)
}

// storageClientAdapter narrows *storage.Client to the GCSClient interface.
type storageClientAdapter struct {
	c *storage.Client
}

func (a *storageClientAdapter) Object(bucket, object string) GCSObject {
	return a.c.Bucket(bucket).Object(object)
}

func (a *storageClientAdapter) Close() error { return a.c.Close() }
