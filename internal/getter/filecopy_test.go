package getter_test

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tggetter "github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	getter "github.com/hashicorp/go-getter/v2"
)

// fileSourceURL renders a filesystem path the way Terragrunt hands a local
// source to the getter client: tf.ToSourceURL parses the detector's
// `file://<path>` output into a *url.URL and the client is given its String(),
// so any space arrives percent-encoded. Building it through url.URL keeps the
// result a valid URI on Windows too, where the path is `C:/...` rather than
// `/...` and needs a slash ahead of the drive letter (RFC 8089) or the URL
// carries the drive as its host.
func fileSourceURL(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	u := url.URL{Scheme: "file", Path: p}

	return u.String()
}

// TestFileCopyGetterHandlesSpacesInLocalPaths pins that a local source whose
// path contains spaces still resolves. Recovering the filesystem path from the
// URL is the getter's job, and it has to arrive at the same answer go-getter's
// own FileGetter would — from the percent-encoded form Terragrunt produces and
// from the raw form a hand-written `file://` source carries, which url.Parse
// leaves in RawPath.
func TestFileCopyGetterHandlesSpacesInLocalPaths(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	srcDir := filepath.Join(base, "my local module")

	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# hi\n"), 0o644))

	client := &getter.Client{
		Getters: []getter.Getter{tggetter.NewFileCopyGetter(vfs.NewOSFS())},
	}

	testCases := []struct {
		name          string
		src           string
		dst           string
		want          string
		skipOnWindows string
		mode          getter.Mode
	}{
		{
			name: "single file",
			src:  fileSourceURL(filepath.Join(srcDir, "main.tf")),
			dst:  filepath.Join(base, "out-file", "main.tf"),
			mode: getter.ModeFile,
			want: filepath.Join(base, "out-file", "main.tf"),
		},
		{
			name: "directory",
			src:  fileSourceURL(srcDir),
			dst:  filepath.Join(base, "out-dir"),
			mode: getter.ModeDir,
			want: filepath.Join(base, "out-dir", "main.tf"),
		},
		{
			name: "mode decided by the getter",
			src:  fileSourceURL(srcDir),
			dst:  filepath.Join(base, "out-any"),
			mode: getter.ModeAny,
			want: filepath.Join(base, "out-any", "main.tf"),
		},
		{
			name:          "unencoded spaces",
			src:           helpers.FileURL(srcDir),
			dst:           filepath.Join(base, "out-raw"),
			mode:          getter.ModeAny,
			want:          filepath.Join(base, "out-raw", "main.tf"),
			skipOnWindows: "Skipping on Windows: a drive path with an unencoded space loses its drive letter",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.skipOnWindows != "" && helpers.IsWindows() {
				t.Skip(tc.skipOnWindows)
			}

			_, err := client.Get(context.Background(), &getter.Request{
				Src:     tc.src,
				Dst:     tc.dst,
				GetMode: tc.mode,
			})
			require.NoError(t, err)

			contents, err := os.ReadFile(tc.want)
			require.NoError(t, err)
			assert.Equal(t, "# hi\n", string(contents))
		})
	}
}
