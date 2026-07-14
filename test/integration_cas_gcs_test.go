//go:build gcp

package test_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/storage"

	tgcas "github.com/gruntwork-io/terragrunt/internal/cas"
	tggetter "github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// TestGcpCASGCSMD5Probe exercises CASGetter end-to-end against a
// real GCS bucket. The MD5 metadata GCS records for single-chunk
// uploads drives the content-addressed cache key; a second
// CASGetter request materializes from CAS without re-downloading.
func TestGcpCASGCSMD5Probe(t *testing.T) {
	t.Parallel()

	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		t.Skip("GOOGLE_CLOUD_PROJECT not set; skipping real-GCP test")
	}

	bucket := "terragrunt-cas-test-" + strings.ToLower(helpers.UniqueID())
	object := "modules/example.tar.gz"

	createGCSBucket(t, project, terraformRemoteStateGcpRegion, bucket)
	t.Cleanup(func() { deleteGCSBucket(t, bucket) })

	uploadGCSObjectForCAS(t, bucket, object, makeModuleArchive(t))

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	g := tggetter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{}, tggetter.WithDefaultGenericDispatch())
	client := &tggetter.Client{Getters: []tggetter.Getter{g}}

	// The bare v2 gcs.Getter's parseURL only recognizes
	// googleapis.com-hosted URLs; gs:// URLs land an empty bucket.
	src := "gcs::https://www.googleapis.com/storage/v1/" + bucket + "/" + object

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

// uploadGCSObjectForCAS writes body to bucket/object using the ambient
// application default credentials, the same path the resolver and the
// go-getter GCS fetcher take.
func uploadGCSObjectForCAS(t *testing.T, bucket, object string, body []byte) {
	t.Helper()

	c, err := storage.NewClient(t.Context())
	require.NoError(t, err)

	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Logf("close GCS client: %v", err)
		}
	})

	w := c.Bucket(bucket).Object(object).NewWriter(t.Context())

	_, err = w.Write(body)
	require.NoError(t, err)
	require.NoError(t, w.Close())
}
