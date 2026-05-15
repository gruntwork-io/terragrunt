package getter

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/gruntwork-io/terragrunt/internal/cas"
)

// ErrS3UnrecognizedURL is returned when an amazonaws.com URL does not match
// a supported S3 path-style or legacy virtual-host shape.
var ErrS3UnrecognizedURL = errors.New("not a recognized S3 URL")

// ErrS3ModernPathStyleUnsupported is returned for `s3.<region>.amazonaws.com`
// URLs. The upstream go-getter/s3 v2 Getter rejects them, so the resolver
// rejects them too to keep probe success aligned with fetch success.
var ErrS3ModernPathStyleUnsupported = errors.New("modern path-style S3 URL not supported (use s3-<region>.amazonaws.com instead)")

// ErrS3CompatibleUnrecognizedURL is returned when a non-amazonaws.com URL
// does not have the host/<bucket>/<key> path shape required for S3-compatible
// services.
var ErrS3CompatibleUnrecognizedURL = errors.New("not a recognized S3-compatible URL")

// s3ResolverTimeout caps the HeadObject call so a slow remote can't stall
// CAS dispatch.
const s3ResolverTimeout = 10 * time.Second

// Host-part counts for AWS S3 URL forms.
// Path style: `<region>.amazonaws.com`.
// Virtual-host style: `<bucket>.<region>.amazonaws.com`.
const (
	s3HostPartsPathStyle  = 3
	s3HostPartsVHostStyle = 4
	// s3URLPathSegments is the count produced by splitting `/bucket/key`
	// on "/" with limit 3: ["", "bucket", "key"]. Used as a validation
	// gate before indexing.
	s3URLPathSegments = 3
)

// S3API is the subset of *s3.Client a resolver depends on. Exported
// so tests can inject a fake.
type S3API interface {
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

// S3Resolver is a [cas.SourceResolver] for objects in Amazon S3 and
// S3-compatible services.
//
// Supported URL forms (constrained by the upstream go-getter/s3/v2
// Getter, whose parseUrl enforces a 3-part `amazonaws.com` hostname):
//
//	https://s3.amazonaws.com/<bucket>/<key>           (global path-style)
//	https://s3-<region>.amazonaws.com/<bucket>/<key>  (legacy regional path-style)
//	https://<host>/<bucket>/<key>?region=<region>     (S3-compatible service)
//
// Modern virtual-host URLs (`<bucket>.s3.<region>.amazonaws.com`,
// 5-part) and modern path-style URLs (`s3.<region>.amazonaws.com`,
// 4-part) are rejected by both the bare getter and this resolver. Use
// the legacy regional form above.
type S3Resolver struct {
	// NewClient builds an S3 client per request. Nil means the resolver
	// uses the AWS SDK default config (env, profile, IMDS) with a
	// region derived from the URL.
	NewClient func(ctx context.Context, region string) (S3API, error)
}

// NewS3Resolver returns a resolver wired to the default AWS SDK config.
func NewS3Resolver() *S3Resolver { return &S3Resolver{} }

// Scheme returns "s3".
func (r *S3Resolver) Scheme() string { return "s3" }

// Probe runs HeadObject with ChecksumMode=ENABLED and returns a
// cache key from the strongest available token. The cascade prefers
// content-addressed checksums (cross-URL dedupe) over the opaque ETag
// (URL-scoped):
//
//	x-amz-checksum-sha256
//	x-amz-checksum-crc64nvme
//	x-amz-checksum-sha1
//	x-amz-checksum-crc32c
//	x-amz-checksum-crc32
//	ETag
//
// The ETag stays opaque even for single-part objects: multipart ETag
// `<md5>-<n>` is not a content hash. Network or AWS errors surface as
// [cas.ErrNoVersionMetadata].
func (r *S3Resolver) Probe(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse S3 URL %s: %w", rawURL, err)
	}

	target, err := parseS3URL(u)
	if err != nil {
		return "", fmt.Errorf("parse S3 URL %s: %w", rawURL, err)
	}

	ctx, cancel := context.WithTimeout(ctx, s3ResolverTimeout)
	defer cancel()

	client, err := r.client(ctx, target.Region)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	input := &s3.HeadObjectInput{
		Bucket:       aws.String(target.Bucket),
		Key:          aws.String(target.Key),
		ChecksumMode: types.ChecksumModeEnabled,
	}

	// The bare S3 getter forwards ?version= as GetObject's VersionId,
	// so HeadObject must too. Without this, the probe describes the
	// current version while the fetch downloads a different one.
	if target.Version != "" {
		input.VersionId = aws.String(target.Version)
	}

	out, err := client.HeadObject(ctx, input)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	return pickS3CacheKey(rawURL, out)
}

// client returns the S3 client for region, using the AWS SDK config
// chain when r.NewClient is unset.
func (r *S3Resolver) client(ctx context.Context, region string) (S3API, error) {
	if r.NewClient != nil {
		return r.NewClient(ctx, region)
	}

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg), nil
}

// pickS3CacheKey walks the checksum cascade and returns the cache key
// for the first match. ErrNoVersionMetadata signals an empty head with
// no checksum and no ETag.
func pickS3CacheKey(rawURL string, head *s3.HeadObjectOutput) (string, error) {
	if head == nil {
		return "", cas.ErrNoVersionMetadata
	}

	if v := strPtr(head.ChecksumSHA256); v != "" {
		return cas.ContentKey("sha256", v), nil
	}

	if v := strPtr(head.ChecksumCRC64NVME); v != "" {
		return cas.ContentKey("crc64nvme", v), nil
	}

	if v := strPtr(head.ChecksumSHA1); v != "" {
		return cas.ContentKey("sha1", v), nil
	}

	if v := strPtr(head.ChecksumCRC32C); v != "" {
		return cas.ContentKey("crc32c", v), nil
	}

	if v := strPtr(head.ChecksumCRC32); v != "" {
		return cas.ContentKey("crc32", v), nil
	}

	if etag := strings.TrimSpace(strPtr(head.ETag)); etag != "" {
		if normalized := normalizeETag(etag); normalized != "" {
			return cas.OpaqueKey("s3", rawURL, normalized), nil
		}
	}

	return "", cas.ErrNoVersionMetadata
}

// s3Target is the parsed form of an S3 URL: AWS region, bucket, object
// key, and the optional ?version= selector for versioned objects.
type s3Target struct {
	Region  string
	Bucket  string
	Key     string
	Version string
}

// parseS3URL extracts an [s3Target] from an S3 URL in any of the forms
// go-getter accepts. Returns an error if the URL is unrecognizable.
func parseS3URL(u *url.URL) (s3Target, error) {
	version := u.Query().Get("version")

	if strings.Contains(u.Host, "amazonaws.com") {
		hostParts := strings.Split(u.Host, ".")
		switch len(hostParts) {
		case s3HostPartsPathStyle:
			// Path-style: <region>.amazonaws.com/<bucket>/<key>.
			// hostParts[0] must identify S3 (exactly "s3" or the
			// "s3-<region>" legacy regional prefix); otherwise the
			// host belongs to a different AWS service (e.g.
			// iam.amazonaws.com) and we must not silently parse it
			// as path-style S3 with a bogus region.
			region, ok := s3RegionFromHostLabel(hostParts[0])
			if !ok {
				return s3Target{}, fmt.Errorf("%w: %q", ErrS3UnrecognizedURL, u.String())
			}

			pathParts := strings.SplitN(u.Path, "/", s3URLPathSegments)
			if len(pathParts) != s3URLPathSegments {
				return s3Target{}, fmt.Errorf("%w: %q", ErrS3UnrecognizedURL, u.String())
			}

			return s3Target{Region: region, Bucket: pathParts[1], Key: pathParts[2], Version: version}, nil
		case s3HostPartsVHostStyle:
			// hostParts[0] == "s3" is the modern path-style
			// (`s3.<region>.amazonaws.com`), which the upstream
			// go-getter/s3 v2 Getter rejects. Reject at probe time too
			// so the failure mode matches the fetcher's rather than
			// silently misparsing bucket="s3".
			if hostParts[0] == "s3" {
				return s3Target{}, fmt.Errorf("%w: %q", ErrS3ModernPathStyleUnsupported, u.String())
			}

			// Legacy virtual-host style: <bucket>.s3[-<region>].amazonaws.com/<key>.
			// hostParts[1] must identify S3 the same way as the
			// path-style case, otherwise the host belongs to a
			// non-S3 service (e.g. bucket.iam.amazonaws.com).
			region, ok := s3RegionFromHostLabel(hostParts[1])
			if !ok {
				return s3Target{}, fmt.Errorf("%w: %q", ErrS3UnrecognizedURL, u.String())
			}

			return s3Target{
				Region:  region,
				Bucket:  hostParts[0],
				Key:     strings.TrimPrefix(u.Path, "/"),
				Version: version,
			}, nil
		}

		return s3Target{}, fmt.Errorf("%w: %q", ErrS3UnrecognizedURL, u.String())
	}

	// S3-compatible service: host/<bucket>/<key>?region=<region>
	pathParts := strings.SplitN(u.Path, "/", s3URLPathSegments)
	if len(pathParts) != s3URLPathSegments {
		return s3Target{}, fmt.Errorf("%w: %q", ErrS3CompatibleUnrecognizedURL, u.String())
	}

	region := u.Query().Get("region")
	if region == "" {
		region = "us-east-1"
	}

	return s3Target{Region: region, Bucket: pathParts[1], Key: pathParts[2], Version: version}, nil
}

// s3RegionFromHostLabel parses an S3-identifying host label and
// returns the AWS region it encodes. The exact label "s3" is the
// global path-style endpoint and maps to us-east-1. A label of the
// form "s3-<region>" maps to that region. Any other label is rejected
// (ok = false) so non-S3 amazonaws.com hosts (iam, sts, ec2, ...) do
// not silently parse as S3 with a bogus region.
func s3RegionFromHostLabel(label string) (region string, ok bool) {
	if label == "s3" {
		return "us-east-1", true
	}

	if region, ok := strings.CutPrefix(label, "s3-"); ok && region != "" {
		return region, true
	}

	return "", false
}

// strPtr safely dereferences a *string.
func strPtr(p *string) string {
	if p == nil {
		return ""
	}

	return *p
}
