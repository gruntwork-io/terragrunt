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

			got, err := getter.ObjectDst(tc.dst, tc.prefix, tc.key)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestObjectDstRejectsEscapingKeys pins that a key climbing out of the
// destination is refused rather than written wherever it points.
func TestObjectDstRejectsEscapingKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prefix string
		key    string
	}{
		{
			name:   "key climbs above the destination",
			prefix: "modules/vpc",
			key:    "modules/vpc/../../../etc/passwd",
		},
		{
			name:   "key outside the prefix climbs from the whole key",
			prefix: "modules/vpc",
			key:    "../../etc/passwd",
		},
		{
			name:   "single parent segment still escapes",
			prefix: "modules/vpc/",
			key:    "modules/vpc/../sibling.tf",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := getter.ObjectDst("/out", tc.prefix, tc.key)
			require.ErrorIs(t, err, getter.ErrObjectEscapesDst)
		})
	}
}

// TestObjectMode pins the rule both getters decide file-vs-directory with,
// siblings sharing the object's name as a prefix included.
func TestObjectMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		listed   string
		object   string
		want     upstream.Mode
		wantDone bool
	}{
		{
			name:     "exact match is a file",
			listed:   "modules/vpc",
			object:   "modules/vpc",
			want:     upstream.ModeFile,
			wantDone: true,
		},
		{
			name:     "name below the object is a directory",
			listed:   "modules/vpc/main.tf",
			object:   "modules/vpc",
			want:     upstream.ModeDir,
			wantDone: true,
		},
		{
			name:     "directory placeholder is a directory",
			listed:   "modules/vpc/",
			object:   "modules/vpc",
			want:     upstream.ModeDir,
			wantDone: true,
		},
		{
			name:     "name below a trailing-slash object is a directory",
			listed:   "modules/vpc/main.tf",
			object:   "modules/vpc/",
			want:     upstream.ModeDir,
			wantDone: true,
		},
		{
			name:     "trailing-slash object matching its own placeholder is a directory",
			listed:   "modules/vpc/",
			object:   "modules/vpc/",
			want:     upstream.ModeDir,
			wantDone: true,
		},
		{
			name:   "sibling sharing the prefix decides nothing",
			listed: "modules/vpc-old/main.tf",
			object: "modules/vpc",
		},
		{
			name:   "sibling of a trailing-slash object decides nothing",
			listed: "modules/vpc-old/main.tf",
			object: "modules/vpc/",
		},
		{
			name:   "sibling with no separator decides nothing",
			listed: "modules/vpcfoo",
			object: "modules/vpc",
		},
		{
			name:   "unrelated name decides nothing",
			listed: "other/main.tf",
			object: "modules/vpc",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, done := getter.ObjectMode(tc.listed, tc.object)
			assert.Equal(t, tc.wantDone, done)
			assert.Equal(t, tc.want, got)
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

// TestListPrefix pins the prefix a directory download lists under. Mode
// decides a key names a directory by finding keys below `<key>/`, so the
// listing has to use that same trailing separator or it would also match a
// sibling key the requested one is a prefix of.
func TestListPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "bare key gains a separator",
			key:  "modules",
			want: "modules/",
		},
		{
			name: "nested key gains a separator",
			key:  "modules/vpc/sub",
			want: "modules/vpc/sub/",
		},
		{
			name: "key already ending in a separator is unchanged",
			key:  "modules/vpc/",
			want: "modules/vpc/",
		},
		{
			name: "only one trailing separator is collapsed",
			key:  "modules/vpc//",
			want: "modules/vpc//",
		},
		{
			name: "empty key lists the whole bucket",
			key:  "",
			want: "/",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, getter.ListPrefix(tc.key))
		})
	}
}

// TestCloseOnSuccess pins which error survives when closing the client fails.
// A close error only reaches the caller when the operation itself succeeded,
// so the reason a download failed is never displaced by the cleanup.
func TestCloseOnSuccess(t *testing.T) {
	t.Parallel()

	errPrimary := errors.New("download failed")
	errClose := errors.New("close failed")

	tests := []struct {
		primary  error
		closeErr error
		want     error
		name     string
	}{
		{
			name: "clean close on a successful operation reports nothing",
		},
		{
			name:     "close error surfaces when the operation succeeded",
			closeErr: errClose,
			want:     errClose,
		},
		{
			name:    "primary error survives a clean close",
			primary: errPrimary,
			want:    errPrimary,
		},
		{
			name:     "primary error outranks a close error",
			primary:  errPrimary,
			closeErr: errClose,
			want:     errPrimary,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := &stubCloser{err: tc.closeErr}

			err := tc.primary
			getter.CloseOnSuccess(c, &err)

			assert.True(t, c.closed, "the closer must run whatever the operation returned")

			if tc.want == nil {
				require.NoError(t, err)
				return
			}

			require.ErrorIs(t, err, tc.want)

			if tc.primary != nil && tc.closeErr != nil {
				require.NotErrorIs(t, err, tc.closeErr,
					"a close error must not be joined onto the error that explains the failure")
			}
		})
	}
}

type stubCloser struct {
	err    error
	closed bool
}

func (c *stubCloser) Close() error {
	c.closed = true
	return c.err
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

	exists, err = vfs.FileExists(fsys, "out")
	require.NoError(t, err)
	assert.True(t, exists, "the destination must exist after a reset")
}
