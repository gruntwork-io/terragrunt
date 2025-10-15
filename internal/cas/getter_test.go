package cas_test

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCASGetterMode(t *testing.T) {
	t.Parallel()

	g := cas.NewCASGetter(nil, nil, &cas.CloneOptions{})
	testURL, err := url.Parse("https://github.com/gruntwork-io/terragrunt")
	require.NoError(t, err)

	mode, err := g.Mode(t.Context(), testURL)
	require.NoError(t, err)
	assert.Equal(t, getter.ModeDir, mode)
}

func TestCASGetterGetFile(t *testing.T) {
	t.Parallel()

	g := cas.NewCASGetter(nil, nil, &cas.CloneOptions{})
	err := g.GetFile(t.Context(), &getter.Request{})
	require.Error(t, err)
	assert.Equal(t, "GetFile not implemented", err.Error())
}

func TestCASGetterDetect(t *testing.T) {
	t.Parallel()

	g := cas.NewCASGetter(nil, nil, &cas.CloneOptions{})

	tmp := t.TempDir()

	os.MkdirAll(filepath.Join(tmp, "fake-module"), 0755)
	os.WriteFile(filepath.Join(tmp, "fake-module", "main.tf"), []byte(""), 0644)

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
			name:        "Invalid URL",
			src:         "not-a-valid-url",
			pwd:         tmp,
			expectedErr: cas.ErrDirectoryNotFound,
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

	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "store")

	c, err := cas.New(cas.Options{
		StorePath: storePath,
	})
	require.NoError(t, err)

	opts := &cas.CloneOptions{
		Branch: "main",
	}

	l := logger.CreateLogger()

	g := cas.NewCASGetter(l, c, opts)
	client := getter.Client{
		Getters: []getter.Getter{g},
	}

	tests := []struct {
		name      string
		url       string
		queryRef  string
		expectRef string
	}{
		{
			name:      "URL with ref parameter",
			url:       "github.com/gruntwork-io/terragrunt?ref=v0.75.0",
			expectRef: "v0.75.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()

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

	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "store")

	c, err := cas.New(cas.Options{
		StorePath: storePath,
	})
	require.NoError(t, err)

	opts := &cas.CloneOptions{
		Branch: "main",
	}

	l := logger.CreateLogger()

	g := cas.NewCASGetter(l, c, opts)

	fakeModule := filepath.Join(tmp, "fake-module")
	os.MkdirAll(fakeModule, 0755)

	fakeModuleSubdir := filepath.Join(fakeModule, "subdir")
	os.MkdirAll(fakeModuleSubdir, 0755)

	os.WriteFile(filepath.Join(fakeModule, "main.tf"), []byte(""), 0644)
	os.WriteFile(filepath.Join(fakeModuleSubdir, "subfile.tf"), []byte(""), 0644)

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

	stat, err := os.Stat(filepath.Join(fakeDest, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), stat.Mode())

	stat, err = os.Stat(filepath.Join(fakeDest, "subdir", "subfile.tf"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), stat.Mode())
}
