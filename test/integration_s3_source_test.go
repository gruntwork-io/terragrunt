//go:build aws

package test_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	tggetter "github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/require"
)

// TestAwsS3SourceURLForms downloads a module archive through the default
// (non-CAS) client in each AWS S3 endpoint form a `source` URL can use.
// The forms differ only in how the hostname encodes bucket and region;
// all of them must fetch the same object.
func TestAwsS3SourceURLForms(t *testing.T) {
	t.Parallel()

	region := helpers.TerraformRemoteStateS3Region
	key := "modules/example.tar.gz"
	bucket := provisionS3ModuleArchive(t, region, key)

	tests := []struct {
		name string
		src  string
	}{
		{
			name: "virtual-host style",
			src:  "s3::https://" + bucket + ".s3." + region + ".amazonaws.com/" + key,
		},
		{
			name: "modern path-style",
			src:  "s3::https://s3." + region + ".amazonaws.com/" + bucket + "/" + key,
		},
		{
			name: "legacy regional path-style",
			src:  "s3::https://s3-" + region + ".amazonaws.com/" + bucket + "/" + key,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "module")

			_, err := tggetter.GetAny(t.Context(), dst, tt.src)
			require.NoError(t, err)
			require.FileExists(t, filepath.Join(dst, "main.tf"))
		})
	}
}

// provisionS3ModuleArchive creates a throwaway bucket in region holding
// the shared module archive under key, registers cleanup, and returns
// the bucket name. The PutObject sets a SHA-256 checksum so CAS
// resolver probes can exercise the content-addressed path against the
// same object.
func provisionS3ModuleArchive(t *testing.T, region, key string) string {
	t.Helper()

	bucket := "terragrunt-getter-test-" + strings.ToLower(helpers.UniqueID())

	s3Client := helpers.CreateS3ClientForTest(t, region)

	_, err := s3Client.CreateBucket(t.Context(), &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
		CreateBucketConfiguration: &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		if err := helpers.DeleteS3Bucket(t, region, bucket); err != nil {
			t.Logf("delete bucket %s: %v", bucket, err)
		}
	})

	body := makeModuleArchive(t)
	_, err = s3Client.PutObject(t.Context(), &s3.PutObjectInput{
		Bucket:            aws.String(bucket),
		Key:               aws.String(key),
		Body:              bytes.NewReader(body),
		ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
	})
	require.NoError(t, err)

	return bucket
}
