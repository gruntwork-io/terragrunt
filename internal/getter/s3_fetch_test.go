package getter_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/getter"
)

// TestParseS3FetchURL pins how the s3 getter resolves a path-style URL into
// SDK inputs. Detect canonicalizes the AWS virtual-host and modern path-style
// forms first, so only `<host>/<bucket>/<key>` reaches this.
func TestParseS3FetchURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		raw          string
		wantRegion   string
		wantBucket   string
		wantKey      string
		wantVersion  string
		wantProfile  string
		wantEndpoint string
		wantCreds    bool
	}{
		{
			name:       "global path style defaults to us-east-1",
			raw:        "s3://s3.amazonaws.com/bucket/path/to/mod",
			wantRegion: "us-east-1",
			wantBucket: "bucket",
			wantKey:    "path/to/mod",
		},
		{
			name:       "legacy regional host carries its region",
			raw:        "s3://s3-eu-west-1.amazonaws.com/bucket/key",
			wantRegion: "eu-west-1",
			wantBucket: "bucket",
			wantKey:    "key",
		},
		{
			name:        "version query is preserved",
			raw:         "s3://s3.amazonaws.com/bucket/key?version=abc123",
			wantRegion:  "us-east-1",
			wantBucket:  "bucket",
			wantKey:     "key",
			wantVersion: "abc123",
		},
		{
			name:        "profile query is preserved",
			raw:         "s3://s3.amazonaws.com/bucket/key?aws_profile=dev",
			wantRegion:  "us-east-1",
			wantBucket:  "bucket",
			wantKey:     "key",
			wantProfile: "dev",
		},
		{
			name:       "s3-compatible host takes its region from the query",
			raw:        "https://minio.example.com/bucket/key?region=us-west-2",
			wantRegion: "us-west-2",
			wantBucket: "bucket",
			wantKey:    "key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse(tc.raw)
			require.NoError(t, err)

			got, err := getter.ParseS3FetchURL(u)
			require.NoError(t, err)

			assert.Equal(t, tc.wantRegion, got.Region)
			assert.Equal(t, tc.wantBucket, got.Bucket)
			assert.Equal(t, tc.wantKey, got.Key)
			assert.Equal(t, tc.wantVersion, got.Version)
			assert.Equal(t, tc.wantProfile, got.Profile)
			assert.Equal(t, tc.wantEndpoint, got.Endpoint)
			assert.Equal(t, tc.wantCreds, got.Creds != nil)
		})
	}
}

// TestParseS3FetchURLInURLCredentials pins the S3-compatible contract the
// RustFS integration test depends on: credentials in the query are also the
// signal to pin the endpoint to the URL's own host in path style, so the
// request reaches that service instead of real AWS.
func TestParseS3FetchURLInURLCredentials(t *testing.T) {
	t.Parallel()

	u, err := url.Parse(
		"http://127.0.0.1:9000/bucket/modules/example.tar.gz" +
			"?aws_access_key_id=key&aws_access_key_secret=secret",
	)
	require.NoError(t, err)

	got, err := getter.ParseS3FetchURL(u)
	require.NoError(t, err)

	assert.Equal(t, "bucket", got.Bucket)
	assert.Equal(t, "modules/example.tar.gz", got.Key)
	assert.Equal(t, "http://127.0.0.1:9000", got.Endpoint)
	require.NotNil(t, got.Creds)

	creds, err := got.Creds.Retrieve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "key", creds.AccessKeyID)
	assert.Equal(t, "secret", creds.SecretAccessKey)
}

// TestParseS3FetchURLRejectsIncomplete pins that a URL naming no key is
// rejected up front rather than turning into a bucket-wide prefix download.
func TestParseS3FetchURLRejectsIncomplete(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"s3://s3.amazonaws.com/bucket",
		"s3://s3.amazonaws.com/bucket/",
		"s3://s3.amazonaws.com/",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse(raw)
			require.NoError(t, err)

			_, err = getter.ParseS3FetchURL(u)
			require.ErrorIs(t, err, getter.ErrS3InvalidFetchURL)
		})
	}
}

// TestParseGCSFetchURL pins the canonical GCS form the upstream detector
// produces, and that anything else is rejected.
func TestParseGCSFetchURL(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://www.googleapis.com/storage/v1/bucket/path/to/mod")
	require.NoError(t, err)

	bucket, object, err := getter.ParseGCSFetchURL(u)
	require.NoError(t, err)
	assert.Equal(t, "bucket", bucket)
	assert.Equal(t, "path/to/mod", object)
}

func TestParseGCSFetchURLRejectsIncomplete(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://www.googleapis.com/storage/v1/bucket",
		"https://www.googleapis.com/notstorage/v1/bucket/key",
		"https://example.com/storage/v1/bucket/key",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse(raw)
			require.NoError(t, err)

			_, _, err = getter.ParseGCSFetchURL(u)
			require.ErrorIs(t, err, getter.ErrGCSInvalidFetchURL)
		})
	}
}
