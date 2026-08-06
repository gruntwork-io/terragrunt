package getter_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	upstream "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// TestObjectDst pins how an object key maps onto a destination path. The
// prefix is stripped and the remainder keeps its layout, so a prefix download
// reproduces the bucket's tree rather than flattening it.
func TestObjectDst(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		dst    string
		prefix string
		key    string
		want   string
	}{
		{
			name:   "nested key keeps its layout below the prefix",
			dst:    "/out",
			prefix: "modules/vpc",
			key:    "modules/vpc/sub/main.tf",
			want:   filepath.Join("/out", "sub", "main.tf"),
		},
		{
			name:   "prefix with trailing slash does not leave a leading separator",
			dst:    "/out",
			prefix: "modules/vpc/",
			key:    "modules/vpc/main.tf",
			want:   filepath.Join("/out", "main.tf"),
		},
		{
			name:   "key equal to the prefix falls back to the base name",
			dst:    "/out",
			prefix: "modules/vpc/main.tf",
			key:    "modules/vpc/main.tf",
			want:   filepath.Join("/out", "main.tf"),
		},
		{
			name:   "empty prefix keeps the whole key",
			dst:    "/out",
			prefix: "",
			key:    "a/b/c.tf",
			want:   filepath.Join("/out", "a", "b", "c.tf"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, getter.ObjectDst(tc.dst, tc.prefix, tc.key))
		})
	}
}

// TestWriteGetterObject pins that a downloaded object lands on the supplied
// filesystem with its parent directories created, and that the request's
// umask reaches the resulting file mode.
func TestWriteGetterObject(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	dst := filepath.Join("out", "nested", "main.tf")

	req := &upstream.Request{}

	err := getter.WriteGetterObject(
		fsys,
		req,
		dst,
		io.NopCloser(strings.NewReader("hello")),
	)
	require.NoError(t, err)

	got, err := vfs.ReadFile(fsys, dst)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))
}

// TestWriteGetterObjectAppliesUmask pins that the request's umask masks the
// mode the object is written with, matching what go-getter's own copy helper
// does for the getters this replaced.
func TestWriteGetterObjectAppliesUmask(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	dst := filepath.Join("out", "main.tf")

	req := &upstream.Request{Umask: 0o022}

	require.NoError(t, getter.WriteGetterObject(
		fsys,
		req,
		dst,
		io.NopCloser(strings.NewReader("x")),
	))

	info, err := fsys.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

// TestWriteGetterObjectClosesBody pins that the body is closed even when the
// copy target cannot be created, so a failed download does not leak the
// SDK's response connection back into the pool unread.
func TestWriteGetterObjectClosesBody(t *testing.T) {
	t.Parallel()

	body := &closeTrackingReader{Reader: strings.NewReader("x")}

	fsys := openFailFS{FS: vfs.NewMemMapFS()}

	err := getter.WriteGetterObject(fsys, &upstream.Request{}, "out/main.tf", body)
	require.ErrorIs(t, err, errOpenFail)
	assert.True(t, body.closed, "body must be closed even when the write fails")
}

var errOpenFail = errors.New("open refused")

// openFailFS fails every file creation so the write path can be exercised
// without a real filesystem that refuses writes.
type openFailFS struct {
	vfs.FS
}

func (openFailFS) OpenFile(string, int, os.FileMode) (vfs.File, error) {
	return nil, errOpenFail
}

type closeTrackingReader struct {
	io.Reader
	closed bool
}

func (r *closeTrackingReader) Close() error {
	r.closed = true
	return nil
}

// TestResetGetterDst pins that a directory download clears a stale tree from
// a previous run instead of merging into it.
func TestResetGetterDst(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()

	stale := filepath.Join("out", "stale.tf")
	require.NoError(t, fsys.MkdirAll(filepath.Dir(stale), 0o755))
	require.NoError(t, vfs.WriteFile(fsys, stale, []byte("old"), 0o644))

	require.NoError(t, getter.ResetGetterDst(fsys, &upstream.Request{Dst: "out"}))

	exists, err := vfs.FileExists(fsys, stale)
	require.NoError(t, err)
	assert.False(t, exists, "a stale tree must not survive into the new download")
}
