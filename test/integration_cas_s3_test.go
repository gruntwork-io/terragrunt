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

	tgcas "github.com/gruntwork-io/terragrunt/internal/cas"
	tggetter "github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// TestAwsCASS3ChecksumProbe exercises CASGetter end-to-end against a
// real S3 bucket. The PutObject sets a SHA-256 checksum so the
// resolver's preferred content-addressed path runs; on a second
// CASGetter request CAS materializes from the local store without
// re-downloading the archive.
func TestAwsCASS3ChecksumProbe(t *testing.T) {
	t.Parallel()

	region := helpers.TerraformRemoteStateS3Region
	bucket := "terragrunt-cas-test-" + strings.ToLower(helpers.UniqueID())
	key := "modules/example.tar.gz"

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

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	g := tggetter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{}, tggetter.WithDefaultGenericDispatch())
	client := &tggetter.Client{Getters: []tggetter.Getter{g}}

	// Legacy regional path-style URL: the bare go-getter s3.Getter's
	// parseUrl only handles 3-part hostnames. Modern virtual-host
	// URLs (`bucket.s3.region.amazonaws.com`, 5 parts) and modern
	// path-style URLs (`s3.region.amazonaws.com`, 4 parts) both fail
	// the bare getter's len(hostParts) != 3 check. `s3-region.amazonaws.com`
	// is the form both the bare getter and our S3Resolver's parseS3URL
	// recognize, so the test URL stays compatible with both.
	src := "s3::https://s3-" + region + ".amazonaws.com/" + bucket + "/" + key

	first := filepath.Join(helpers.TmpDirWOSymlinks(t), "first")
	_, err = client.Get(t.Context(), &tggetter.Request{
		Src:     src,
		Dst:     first,
		GetMode: tggetter.ModeAny,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(first, "main.tf"))

	second := filepath.Join(helpers.TmpDirWOSymlinks(t), "second")
	_, err = client.Get(t.Context(), &tggetter.Request{
		Src:     src,
		Dst:     second,
		GetMode: tggetter.ModeAny,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(second, "main.tf"))
}
