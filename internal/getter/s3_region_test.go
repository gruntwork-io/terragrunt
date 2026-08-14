package getter_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/getter"
)

// awsRegions covers the shapes an AWS region name takes: the global default,
// an ordinary regional name, and the longer partition-qualified form.
var awsRegions = []string{"us-east-1", "us-west-2", "eu-west-1", "ap-south-1", "us-gov-west-1"}

// TestS3Region pins the region the fetch path resolves for each URL shape it
// accepts. Detect canonicalizes the AWS virtual-host forms to path style
// first, so only `s3[-<region>].amazonaws.com` and S3-compatible hosts reach
// this.
func TestS3Region(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "global path-style host is us-east-1",
			raw:  "s3://s3.amazonaws.com/bucket/key",
			want: "us-east-1",
		},
		{
			name: "legacy regional host carries its region",
			raw:  "s3://s3-eu-west-1.amazonaws.com/bucket/key",
			want: "eu-west-1",
		},
		{
			name: "region keeps every hyphenated segment",
			raw:  "s3://s3-us-gov-west-1.amazonaws.com/bucket/key",
			want: "us-gov-west-1",
		},
		{
			name: "port on an AWS host does not disturb the region",
			raw:  "https://s3-ap-south-1.amazonaws.com:443/bucket/key",
			want: "ap-south-1",
		},
		{
			name: "AWS host outranks a region query",
			raw:  "s3://s3-eu-west-1.amazonaws.com/bucket/key?region=us-west-2",
			want: "eu-west-1",
		},
		{
			name: "s3-compatible host takes the region from the query",
			raw:  "https://minio.example.com/bucket/key?region=us-west-2",
			want: "us-west-2",
		},
		{
			name: "s3-compatible host without a region falls back to us-east-1",
			raw:  "https://minio.example.com/bucket/key",
			want: "us-east-1",
		},
		{
			name: "empty region query falls back to us-east-1",
			raw:  "https://minio.example.com/bucket/key?region=",
			want: "us-east-1",
		},
		{
			name: "host and port of a local service falls back to us-east-1",
			raw:  "http://127.0.0.1:9000/bucket/key",
			want: "us-east-1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse(tc.raw)
			require.NoError(t, err)

			got, err := getter.S3Region(u)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestS3RegionRejectsUncanonicalizedAWSHosts pins that an AWS host carrying
// its region anywhere but the first label is rejected rather than guessed at.
// Those forms are Detect's job to rewrite, so one reaching the fetch path
// means the rewrite did not happen.
func TestS3RegionRejectsUncanonicalizedAWSHosts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "modern path style",
			raw:  "https://s3.us-west-2.amazonaws.com/bucket/key",
		},
		{
			name: "modern virtual-host style",
			raw:  "https://bucket.s3.us-west-2.amazonaws.com/key",
		},
		{
			name: "legacy virtual-host style",
			raw:  "https://bucket.s3-us-west-2.amazonaws.com/key",
		},
		{
			name: "no host label at all",
			raw:  "https://amazonaws.com/bucket/key",
		},
		{
			name: "host that merely embeds the AWS domain",
			raw:  "https://evil.notamazonaws.com.test/bucket/key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse(tc.raw)
			require.NoError(t, err)

			_, err = getter.S3Region(u)
			require.ErrorIs(t, err, getter.ErrS3InvalidFetchURL)
		})
	}
}

// TestS3RegionRejectsNonS3AWSHosts pins that the fetch path rejects the hosts
// the probe path does. An explicitly forced s3 URL reaches here, since Detect
// claims those without inspecting the host.
func TestS3RegionRejectsNonS3AWSHosts(t *testing.T) {
	t.Parallel()

	for _, host := range []string{"iam", "sts", "ec2"} {
		t.Run(host, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse("https://" + host + ".amazonaws.com/bucket/key")
			require.NoError(t, err)

			_, err = getter.S3Region(u)
			require.ErrorIs(t, err, getter.ErrS3InvalidFetchURL)
		})
	}
}

// TestS3RegionRedactsCredentials pins that a rejected URL does not carry the
// credentials it was given into the error text, which reaches logs.
func TestS3RegionRedactsCredentials(t *testing.T) {
	t.Parallel()

	u, err := url.Parse(
		"https://iam.amazonaws.com/bucket/key" +
			"?aws_access_key_id=AKIAEXAMPLE&aws_access_key_secret=super-secret",
	)
	require.NoError(t, err)

	_, err = getter.S3Region(u)
	require.ErrorIs(t, err, getter.ErrS3InvalidFetchURL)
	assert.NotContains(t, err.Error(), "super-secret")
	assert.NotContains(t, err.Error(), "AKIAEXAMPLE")
}

// TestS3RegionFromHostLabel pins which host labels identify S3. Anything else
// belongs to another AWS service and must not parse as S3 with a region taken
// from the label.
func TestS3RegionFromHostLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		label  string
		want   string
		wantOK bool
	}{
		{
			name:   "bare s3 is the global endpoint",
			label:  "s3",
			want:   "us-east-1",
			wantOK: true,
		},
		{
			name:   "regional prefix carries its region",
			label:  "s3-eu-west-1",
			want:   "eu-west-1",
			wantOK: true,
		},
		{
			name:   "region keeps every hyphenated segment",
			label:  "s3-us-gov-east-1",
			want:   "us-gov-east-1",
			wantOK: true,
		},
		{
			name:  "regional prefix with no region is rejected",
			label: "s3-",
		},
		{
			name:  "label that merely starts with s3 is rejected",
			label: "s3x",
		},
		{
			name:  "another AWS service is rejected",
			label: "iam",
		},
		{
			name:  "empty label is rejected",
			label: "",
		},
		{
			name:  "matching is case-sensitive",
			label: "S3",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := getter.S3RegionFromHostLabel(tc.label)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestS3HostLabelForRegion pins the host label a canonicalized URL is built
// with. us-east-1 stays on the global label so a region-less URL keeps
// probing the endpoint it named.
func TestS3HostLabelForRegion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		region string
		want   string
	}{
		{
			name:   "us-east-1 is the global label",
			region: "us-east-1",
			want:   "s3",
		},
		{
			name:   "any other region gets the regional label",
			region: "eu-west-1",
			want:   "s3-eu-west-1",
		},
		{
			name:   "region keeps every hyphenated segment",
			region: "us-gov-west-1",
			want:   "s3-us-gov-west-1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, getter.S3HostLabelForRegion(tc.region))
		})
	}
}

// TestS3HostLabelRegionRoundTrip pins that a host label built for a region
// parses back to that region, so a canonicalized URL probes the region the
// original URL named.
func TestS3HostLabelRegionRoundTrip(t *testing.T) {
	t.Parallel()

	for _, region := range awsRegions {
		t.Run(region, func(t *testing.T) {
			t.Parallel()

			got, ok := getter.S3RegionFromHostLabel(getter.S3HostLabelForRegion(region))
			require.True(t, ok)
			assert.Equal(t, region, got)
		})
	}
}

// TestS3RegionReadsCanonicalizedHosts is the contract between the two halves
// of an S3 download: Detect rewrites the host with [getter.S3HostLabelForRegion]
// and the fetch path reads it back with [getter.S3Region]. A region that
// survives one but not the other would download from the wrong endpoint.
func TestS3RegionReadsCanonicalizedHosts(t *testing.T) {
	t.Parallel()

	for _, region := range awsRegions {
		t.Run(region, func(t *testing.T) {
			t.Parallel()

			host := getter.S3HostLabelForRegion(region) + ".amazonaws.com"

			u, err := url.Parse("https://" + host + "/bucket/key")
			require.NoError(t, err)

			got, err := getter.S3Region(u)
			require.NoError(t, err)
			assert.Equal(t, region, got)
		})
	}
}
