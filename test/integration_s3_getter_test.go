//go:build aws

package test_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/require"

	tggetter "github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

// TestAwsS3GetterModes downloads from a real S3 bucket in each shape a
// `source` can name: an object, a prefix, and a prefix written with a
// trailing separator. Real S3 supplies the listing order and the directory
// placeholder that the mode scan reads, which no stub can vouch for.
func TestAwsS3GetterModes(t *testing.T) {
	t.Parallel()

	region := helpers.TerraformRemoteStateS3Region
	bucket := provisionS3GetterLayout(t, region, cloudGetterLayout)

	tests := []struct {
		want map[string]string
		name string
		key  string
	}{
		{
			name: "key naming an object downloads that object",
			key:  "modules/vpc",
			want: map[string]string{"vpc": "vpc-object"},
		},
		{
			name: "prefix without a trailing separator downloads the tree",
			key:  "modules/app",
			want: map[string]string{"main.tf": "app-main"},
		},
		{
			name: "prefix with a trailing separator downloads the tree",
			key:  "modules/vpc/",
			want: map[string]string{"main.tf": "main", "sub/nested.tf": "nested"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.OSVenv()
			client := &tggetter.Client{Getters: []tggetter.Getter{tggetter.NewS3Getter(v)}}
			dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "module")

			_, err := client.Get(t.Context(), &tggetter.Request{
				Src:     "s3::https://s3-" + region + ".amazonaws.com/" + bucket + "/" + tc.key,
				Dst:     dst,
				GetMode: tggetter.ModeAny,
			})
			require.NoError(t, err)

			assertCloudDownloadTree(t, v, dst, tc.want)
		})
	}
}

// TestAwsS3GetterRejectsEscapingKey pins that a key climbing out of the
// destination is refused against a real bucket, where the key travels through
// S3's own listing rather than a fixture.
func TestAwsS3GetterRejectsEscapingKey(t *testing.T) {
	t.Parallel()

	region := helpers.TerraformRemoteStateS3Region
	bucket := provisionS3GetterLayout(t, region, map[string]string{"modules/esc/keep.tf": "keep"})

	escaping := "modules/esc/../../../escape.tf"

	client := helpers.CreateS3ClientForTest(t, region)

	_, err := client.PutObject(t.Context(), &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(escaping),
		Body:   strings.NewReader("escape"),
	})
	if err != nil {
		t.Skipf("S3 refused the key %q, so it cannot reach a download: %v", escaping, err)
	}

	v := venv.OSVenv()
	getterClient := &tggetter.Client{Getters: []tggetter.Getter{tggetter.NewS3Getter(v)}}

	_, err = getterClient.Get(t.Context(), &tggetter.Request{
		Src:     "s3::https://s3-" + region + ".amazonaws.com/" + bucket + "/modules/esc/",
		Dst:     filepath.Join(helpers.TmpDirWOSymlinks(t), "module"),
		GetMode: tggetter.ModeAny,
	})
	require.ErrorIs(t, err, tggetter.ErrObjectEscapesDst)
}

// provisionS3GetterLayout creates a throwaway bucket in region holding every
// key in layout, registers cleanup, and returns the bucket name.
func provisionS3GetterLayout(t *testing.T, region string, layout map[string]string) string {
	t.Helper()

	bucket := "terragrunt-getter-test-" + strings.ToLower(helpers.UniqueID())

	client := helpers.CreateS3ClientForTest(t, region)

	_, err := client.CreateBucket(t.Context(), &s3.CreateBucketInput{
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

	for key, body := range layout {
		_, err := client.PutObject(t.Context(), &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			Body:   strings.NewReader(body),
		})
		require.NoError(t, err, "putting %s", key)
	}

	return bucket
}
