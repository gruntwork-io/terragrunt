//go:build gcp

package test_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/require"

	tggetter "github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

// TestGcpGCSGetterModes downloads from a real GCS bucket in each shape a
// `source` can name: an object, a prefix, and a prefix written with a
// trailing separator. Real GCS supplies the listing order and the directory
// placeholder that the mode scan reads, which no stub can vouch for.
func TestGcpGCSGetterModes(t *testing.T) {
	t.Parallel()

	bucket := provisionGCSGetterLayout(t, cloudGetterLayout)

	tests := []struct {
		want   map[string]string
		name   string
		object string
	}{
		{
			name:   "name matching an object downloads that object",
			object: "modules/vpc",
			want:   map[string]string{"vpc": "vpc-object"},
		},
		{
			name:   "prefix without a trailing separator downloads the tree",
			object: "modules/app",
			want:   map[string]string{"main.tf": "app-main"},
		},
		{
			name:   "prefix with a trailing separator downloads the tree",
			object: "modules/vpc/",
			want:   map[string]string{"main.tf": "main", "sub/nested.tf": "nested"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.OSVenv()
			client := &tggetter.Client{Getters: []tggetter.Getter{tggetter.NewGCSGetter(v)}}
			dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "module")

			_, err := client.Get(t.Context(), &tggetter.Request{
				Src:     "gcs::https://www.googleapis.com/storage/v1/" + bucket + "/" + tc.object,
				Dst:     dst,
				GetMode: tggetter.ModeAny,
			})
			require.NoError(t, err)

			assertCloudDownloadTree(t, v, dst, tc.want)
		})
	}
}

// TestGcpGCSGetterRejectsEscapingKey pins that an object name climbing out of
// the destination is refused against a real bucket, where the name travels
// through the GCS listing rather than a fixture.
func TestGcpGCSGetterRejectsEscapingKey(t *testing.T) {
	t.Parallel()

	bucket := provisionGCSGetterLayout(t, map[string]string{"modules/esc/keep.tf": "keep"})

	escaping := "modules/esc/../../../escape.tf"
	if err := putGCSObject(t, bucket, escaping, "escape"); err != nil {
		t.Skipf("GCS refused the name %q, so it cannot reach a download: %v", escaping, err)
	}

	v := venv.OSVenv()
	client := &tggetter.Client{Getters: []tggetter.Getter{tggetter.NewGCSGetter(v)}}

	_, err := client.Get(t.Context(), &tggetter.Request{
		Src:     "gcs::https://www.googleapis.com/storage/v1/" + bucket + "/modules/esc/",
		Dst:     filepath.Join(helpers.TmpDirWOSymlinks(t), "module"),
		GetMode: tggetter.ModeAny,
	})
	require.ErrorIs(t, err, tggetter.ErrObjectEscapesDst)
}

// putGCSObject writes body to bucket/object and returns the failure rather
// than ending the test, so a caller can tell a name GCS refuses outright from
// a download that mishandled one.
func putGCSObject(t *testing.T, bucket, object, body string) error {
	t.Helper()

	c, err := storage.NewClient(t.Context())
	if err != nil {
		return err
	}

	defer func() {
		if err := c.Close(); err != nil {
			t.Logf("close GCS client: %v", err)
		}
	}()

	w := c.Bucket(bucket).Object(object).NewWriter(t.Context())

	if _, err := w.Write([]byte(body)); err != nil {
		return err
	}

	return w.Close()
}

// provisionGCSGetterLayout creates a throwaway bucket holding every object in
// layout, registers cleanup, and returns the bucket name.
func provisionGCSGetterLayout(t *testing.T, layout map[string]string) string {
	t.Helper()

	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		t.Skip("GOOGLE_CLOUD_PROJECT not set; skipping real-GCP test")
	}

	bucket := "terragrunt-getter-test-" + strings.ToLower(helpers.UniqueID())

	createGCSBucket(t, project, terraformRemoteStateGcpRegion, bucket)
	t.Cleanup(func() { deleteGCSBucket(t, bucket) })

	for object, body := range layout {
		uploadGCSObjectForCAS(t, bucket, object, []byte(body))
	}

	return bucket
}
