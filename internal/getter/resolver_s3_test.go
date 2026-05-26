package getter_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3Resolver_PrefersSHA256(t *testing.T) {
	t.Parallel()

	r := newS3ResolverWith(&fakeS3Head{out: &s3.HeadObjectOutput{
		ChecksumSHA256: aws.String("sha256-token"),
		ChecksumSHA1:   aws.String("sha1-token"),
		ETag:           aws.String(`"etag-token"`),
	}})

	got, err := r.Probe(t.Context(), "https://s3-us-east-1.amazonaws.com/bucket/key.tgz")
	require.NoError(t, err)
	assert.Equal(t, cas.ContentKey("sha256", "sha256-token"), got)
}

func TestS3Resolver_FallsThroughChecksumCascade(t *testing.T) {
	t.Parallel()

	// Each entry isolates a single checksum so a future reorder of
	// the cascade fails noisily instead of slipping past the
	// "strongest-and-weakest only" sentinels.
	tests := []struct {
		name string
		head *s3.HeadObjectOutput
		want string
	}{
		{
			name: "CRC64NVME only",
			head: &s3.HeadObjectOutput{
				ChecksumCRC64NVME: aws.String("crc64nvme-token"),
				ETag:              aws.String(`"etag-token"`),
			},
			want: cas.ContentKey("crc64nvme", "crc64nvme-token"),
		},
		{
			name: "SHA1 only",
			head: &s3.HeadObjectOutput{
				ChecksumSHA1: aws.String("sha1-token"),
				ETag:         aws.String(`"etag-token"`),
			},
			want: cas.ContentKey("sha1", "sha1-token"),
		},
		{
			name: "CRC32C only",
			head: &s3.HeadObjectOutput{
				ChecksumCRC32C: aws.String("crc32c-token"),
				ETag:           aws.String(`"etag-token"`),
			},
			want: cas.ContentKey("crc32c", "crc32c-token"),
		},
		{
			name: "CRC32 only",
			head: &s3.HeadObjectOutput{
				ChecksumCRC32: aws.String("crc32-token"),
				ETag:          aws.String(`"etag-token"`),
			},
			want: cas.ContentKey("crc32", "crc32-token"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newS3ResolverWith(&fakeS3Head{out: tt.head})

			got, err := r.Probe(t.Context(), "https://s3-us-east-1.amazonaws.com/bucket/key.tgz")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestS3Resolver_FallsBackToOpaqueETag(t *testing.T) {
	t.Parallel()

	r := newS3ResolverWith(&fakeS3Head{out: &s3.HeadObjectOutput{
		ETag: aws.String(`"etag-token"`),
	}})

	url := "https://s3-us-east-1.amazonaws.com/bucket/key.tgz"
	got, err := r.Probe(t.Context(), url)
	require.NoError(t, err)
	assert.Equal(t, cas.OpaqueKey("s3", url, "etag-token"), got)
}

func TestS3Resolver_MultipartETagStaysOpaque(t *testing.T) {
	t.Parallel()

	r := newS3ResolverWith(&fakeS3Head{out: &s3.HeadObjectOutput{
		ETag: aws.String(`"d41d8cd98f00b204e9800998ecf8427e-3"`),
	}})

	url := "https://s3-us-east-1.amazonaws.com/bucket/key.tgz"
	got, err := r.Probe(t.Context(), url)
	require.NoError(t, err)
	// Multipart ETag is treated opaquely, scoped to URL.
	assert.Equal(t, cas.OpaqueKey("s3", url, "d41d8cd98f00b204e9800998ecf8427e-3"), got)
}

func TestS3Resolver_HeadFailureSurfacesErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	r := newS3ResolverWith(&fakeS3Head{err: errors.New("transient AWS error")})

	_, err := r.Probe(t.Context(), "https://s3-us-east-1.amazonaws.com/bucket/key.tgz")
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

// TestS3Resolver_RejectsModernURLForms pins the upstream
// go-getter/s3/v2 limitation: modern virtual-host URLs
// (`<bucket>.s3.<region>.amazonaws.com`) and modern path-style URLs
// (`s3.<region>.amazonaws.com`) are rejected by the bare getter's
// parseUrl. The resolver tracks the bare getter's behavior so probe
// success aligns with fetch success. The fake fails the test if
// HeadObject is reached, since rejection has to happen at parse time
// rather than silently downgrade through a doomed HeadObject call.
func TestS3Resolver_RejectsModernURLForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		name    string
		url     string
	}{
		{
			wantErr: getter.ErrS3UnrecognizedURL,
			name:    "modern virtual-host style with 5 host parts",
			url:     "https://bucket.s3.us-west-2.amazonaws.com/modules/example.tar.gz",
		},
		{
			wantErr: getter.ErrS3ModernPathStyleUnsupported,
			name:    "modern path-style with 4 host parts",
			url:     "https://s3.us-west-2.amazonaws.com/bucket/modules/example.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newS3ResolverWith(&assertingS3Head{t: t})

			_, err := r.Probe(t.Context(), tt.url)
			require.ErrorIs(t, err, tt.wantErr,
				"parseS3URL must reject %s; upstream go-getter/s3/v2 also rejects it", tt.url)
			require.NotErrorIs(t, err, cas.ErrNoVersionMetadata,
				"rejection must come from parseS3URL, not from an empty HeadObject result")
		})
	}
}

type fakeS3Head struct {
	out      *s3.HeadObjectOutput
	err      error
	gotInput *s3.HeadObjectInput
}

func (f *fakeS3Head) HeadObject(_ context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	f.gotInput = in

	if f.err != nil {
		return nil, f.err
	}

	return f.out, nil
}

// TestS3Resolver_VersionedURLForwardsVersionIDToHeadObject pins
// probe/fetch alignment for versioned S3 objects. The upstream
// go-getter/s3/v2 Getter passes ?version= as VersionId on GetObject;
// without the same forwarding on HeadObject the probe describes the
// current version while the fetch downloads a different one, and the
// cache key derived from the probe's checksum no longer matches the
// fetched bytes.
func TestS3Resolver_VersionedURLForwardsVersionIDToHeadObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
	}{
		{
			name: "aws path-style with version",
			url:  "https://s3-us-east-1.amazonaws.com/bucket/key.tgz?version=abc123",
		},
		{
			name: "aws virtual-host style with version",
			url:  "https://bucket.s3-us-west-2.amazonaws.com/key.tgz?version=abc123",
		},
		{
			name: "s3-compatible with version",
			url:  "https://minio.example.com/bucket/key.tgz?version=abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			head := &fakeS3Head{out: &s3.HeadObjectOutput{
				ChecksumSHA256: aws.String("sha256-token"),
			}}
			r := newS3ResolverWith(head)

			_, err := r.Probe(t.Context(), tt.url)
			require.NoError(t, err)
			require.NotNil(t, head.gotInput)
			require.NotNil(t, head.gotInput.VersionId,
				"HeadObject must receive VersionId so the probe targets the same version the fetcher downloads")
			assert.Equal(t, "abc123", aws.ToString(head.gotInput.VersionId))
		})
	}
}

// TestS3Resolver_UnversionedURLOmitsVersionID pins that an absent
// ?version= leaves VersionId nil rather than passing an empty string,
// which would be a malformed HeadObject input.
func TestS3Resolver_UnversionedURLOmitsVersionID(t *testing.T) {
	t.Parallel()

	head := &fakeS3Head{out: &s3.HeadObjectOutput{
		ChecksumSHA256: aws.String("sha256-token"),
	}}
	r := newS3ResolverWith(head)

	_, err := r.Probe(t.Context(), "https://s3-us-east-1.amazonaws.com/bucket/key.tgz")
	require.NoError(t, err)
	require.NotNil(t, head.gotInput)
	assert.Nil(t, head.gotInput.VersionId,
		"unversioned URL must leave VersionId nil so HeadObject targets the current version")
}

// assertingS3Head fails the test if HeadObject is reached. Pin
// parse-time rejection: parseS3URL must filter the URL before any
// network call.
type assertingS3Head struct {
	t *testing.T
}

func (f *assertingS3Head) HeadObject(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	f.t.Fatalf("HeadObject must not be reached for an unsupported S3 URL form")
	return nil, nil
}

// TestS3Resolver_RejectsNonS3AmazonawsHosts pins that parseS3URL
// only claims hostnames whose first label identifies S3
// (`s3.amazonaws.com` and `s3-<region>.amazonaws.com`). Any other
// 3-part `*.amazonaws.com` host belongs to a different AWS service
// and must be rejected at parse time, not silently parsed with a
// bogus region.
//
// Before this fix the URL below parsed as path-style with
// region="iam", bucket="bucket", key="key", which caused a wasted
// HeadObject call against a non-S3 endpoint on every probe.
func TestS3Resolver_RejectsNonS3AmazonawsHosts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
	}{
		{name: "iam endpoint", url: "https://iam.amazonaws.com/bucket/key.tgz"},
		{name: "sts endpoint", url: "https://sts.amazonaws.com/bucket/key.tgz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newS3ResolverWith(&assertingS3Head{t: t})

			_, err := r.Probe(t.Context(), tt.url)
			require.ErrorIs(t, err, getter.ErrS3UnrecognizedURL,
				"parseS3URL must reject non-S3 amazonaws.com host %q", tt.url)
			require.NotErrorIs(t, err, cas.ErrNoVersionMetadata,
				"rejection must come from parseS3URL, not from a HeadObject call against the wrong service")
		})
	}
}

// TestS3Resolver_RejectsS3CompatibleURLWithoutKey pins parse-time
// rejection of an S3-compatible host that names a bucket but no key.
// Failing at parse time keeps the resolver from issuing a doomed
// HeadObject against a non-AWS endpoint.
func TestS3Resolver_RejectsS3CompatibleURLWithoutKey(t *testing.T) {
	t.Parallel()

	r := newS3ResolverWith(&assertingS3Head{t: t})

	_, err := r.Probe(t.Context(), "https://minio.example.com/bucket")
	require.ErrorIs(t, err, getter.ErrS3CompatibleUnrecognizedURL)
	require.NotErrorIs(t, err, cas.ErrNoVersionMetadata)
}

func newS3ResolverWith(head getter.S3API) *getter.S3Resolver {
	r := getter.NewS3Resolver()
	r.NewClient = func(_ context.Context, _ string) (getter.S3API, error) {
		return head, nil
	}

	return r
}
