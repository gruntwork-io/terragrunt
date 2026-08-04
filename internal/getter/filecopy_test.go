package getter_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tggetter "github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	getter "github.com/hashicorp/go-getter/v2"
)

// TestFileCopyGetterHandlesSpacesInLocalPaths pins that a local source whose
// path contains spaces still resolves. The URL reaching the getter carries
// that path percent-encoded (`file:///projects/my%20local%20module`), so
// recovering the filesystem path from it is the getter's job, and it has to
// arrive at the same answer go-getter's own FileGetter would.
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
		name string
		src  string
		dst  string
		want string
		mode getter.Mode
	}{
		{
			name: "single file",
			src:  "file://" + filepath.Join(srcDir, "main.tf"),
			dst:  filepath.Join(base, "out-file", "main.tf"),
			mode: getter.ModeFile,
			want: filepath.Join(base, "out-file", "main.tf"),
		},
		{
			name: "directory",
			src:  "file://" + srcDir,
			dst:  filepath.Join(base, "out-dir"),
			mode: getter.ModeDir,
			want: filepath.Join(base, "out-dir", "main.tf"),
		},
		{
			name: "mode decided by the getter",
			src:  "file://" + srcDir,
			dst:  filepath.Join(base, "out-any"),
			mode: getter.ModeAny,
			want: filepath.Join(base, "out-any", "main.tf"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

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
