package cas_test

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCASGetterMode(t *testing.T) {
	t.Parallel()

	g := newTestCASGetter(t, &cas.CloneOptions{})

	testURL, err := url.Parse("https://github.com/gruntwork-io/terragrunt")
	require.NoError(t, err)

	mode, err := g.Mode(t.Context(), testURL)
	require.NoError(t, err)
	assert.Equal(t, getter.ModeDir, mode)
}

func TestCASGetterGetFile(t *testing.T) {
	t.Parallel()

	g := newTestCASGetter(t, &cas.CloneOptions{})

	err := g.GetFile(t.Context(), &getter.Request{})
	require.Error(t, err)
	assert.ErrorIs(t, err, cas.ErrGetFileNotSupported)
}

func TestCASGetterDetect(t *testing.T) {
	t.Parallel()

	g := newTestCASGetter(t, &cas.CloneOptions{})

	tmp := helpers.TmpDirWOSymlinks(t)

	require.NoError(t, vfs.WriteFile(g.Venv.FS, filepath.Join(tmp, "fake-module", "main.tf"), []byte(""), 0644))

	tests := []struct {
		expectedErr error
		name        string
		src         string
		pwd         string
	}{
		{
			name: "GitHub repository",
			src:  "github.com/gruntwork-io/terragrunt",
			pwd:  tmp,
		},
		{
			name: "HTTPS URL repository",
			src:  "git::https://github.com/gruntwork-io/terragrunt",
			pwd:  tmp,
		},
		{
			name:        "Invalid URL",
			src:         "not-a-valid-url",
			pwd:         tmp,
			expectedErr: getter.ErrDirectoryNotFound,
		},
		{
			name: "Local directory",
			src:  "./fake-module",
			pwd:  tmp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &getter.Request{
				Src: tt.src,
				Pwd: tt.pwd,
			}

			ok, err := g.Detect(req)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
				assert.True(t, ok)
			}
		})
	}
}

func TestCASGetterGet(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	opts := &cas.CloneOptions{
		Depth: -1,
	}

	l := logger.CreateLogger()

	g := getter.NewCASGetter(l, c, v, opts)
	client := getter.Client{
		Getters: []getter.Getter{g},
	}

	tests := []struct {
		name string
		url  string
	}{
		{
			name: "clone via getter with ref",
			url:  "git::" + repoURL + "?ref=main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

			res, err := client.Get(
				t.Context(),
				&getter.Request{
					Src: tt.url,
					Dst: tmpDir,
				},
			)
			require.NoError(t, err)

			assert.Equal(t, tmpDir, res.Dst)
		})
	}
}

func TestCASGetterLocalDir(t *testing.T) {
	t.Parallel()

	tmp := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tmp, "store")

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	opts := &cas.CloneOptions{
		Branch: "main",
	}

	l := logger.CreateLogger()

	g := getter.NewCASGetter(l, c, v, opts)

	fakeModule := filepath.Join(tmp, "fake-module")
	fakeModuleSubdir := filepath.Join(fakeModule, "subdir")

	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(fakeModule, "main.tf"), []byte(""), 0644))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(fakeModuleSubdir, "subfile.tf"), []byte(""), 0644))

	fakeDest := filepath.Join(tmp, "fake-dest")

	req := &getter.Request{
		Src: fakeModule,
		Dst: fakeDest,
		Pwd: tmp,
	}

	ok, err := g.Detect(req)
	require.NoError(t, err)
	assert.True(t, ok)

	assert.True(t, req.Copy)

	err = g.Get(t.Context(), req)
	require.NoError(t, err)

	// Default-path materialization clears the write bit so the destination
	// cannot poison the shared CAS store via shared inodes.
	stat, err := os.Stat(filepath.Join(fakeDest, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o444), stat.Mode())

	stat, err = os.Stat(filepath.Join(fakeDest, "subdir", "subfile.tf"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o444), stat.Mode())
}

// newTestCASGetter constructs a CASGetter wired to a fresh on-disk CAS
// store and a real logger, so tests can exercise any method on the getter
// without worrying about which ones happen to dereference which fields.
func newTestCASGetter(t *testing.T, opts *cas.CloneOptions) *getter.CASGetter {
	t.Helper()

	c, err := cas.New(cas.WithStorePath(filepath.Join(helpers.TmpDirWOSymlinks(t), "store")))
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	return getter.NewCASGetter(logger.CreateLogger(), c, v, opts)
}
