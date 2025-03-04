package cas

import (
	"context"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCASGetter(t *testing.T) {
	cas := &CAS{}
	opts := CloneOptions{}

	g := NewCASGetter(nil, cas, opts)

	assert.NotNil(t, g)
	assert.Nil(t, g.Logger)
	assert.Equal(t, cas, g.CAS)
	assert.Equal(t, opts, g.Opts)
	assert.Len(t, g.Detectors, 4) // GitHub, Git, BitBucket, and GitLab detectors
}

func TestCASGetterMode(t *testing.T) {
	g := NewCASGetter(nil, nil, CloneOptions{})
	testURL, err := url.Parse("https://github.com/gruntwork-io/terragrunt")
	require.NoError(t, err)

	mode, err := g.Mode(context.Background(), testURL)
	assert.NoError(t, err)
	assert.Equal(t, getter.ModeDir, mode)
}

func TestCASGetterGetFile(t *testing.T) {
	g := NewCASGetter(nil, nil, CloneOptions{})
	err := g.GetFile(context.Background(), &getter.Request{})
	assert.Error(t, err)
	assert.Equal(t, "GetFile not implemented", err.Error())
}

func TestCASGetterDetect(t *testing.T) {
	g := NewCASGetter(nil, nil, CloneOptions{})

	tests := []struct {
		name     string
		src      string
		pwd      string
		expected bool
	}{
		{
			name:     "GitHub repository",
			src:      "github.com/gruntwork-io/terragrunt",
			expected: true,
		},
		{
			name:     "Invalid URL",
			src:      "not-a-valid-url",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &getter.Request{
				Src: tt.src,
				Pwd: tt.pwd,
			}
			ok, err := g.Detect(req)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, ok)
		})
	}
}

func TestCASGetterGet(t *testing.T) {
	cas, err := New(Options{})
	require.NoError(t, err)

	opts := CloneOptions{
		Branch: "main",
	}

	l := log.New()

	g := NewCASGetter(&l, cas, opts)
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
			name:      "Basic URL without ref",
			url:       "github.com/gruntwork-io/terragrunt",
			expectRef: "",
		},
		{
			name:      "URL with ref parameter",
			url:       "github.com/gruntwork-io/terragrunt?ref=v0.75.0",
			expectRef: "v0.75.0",
		},
		{
			name:      "URL as SSH",
			url:       "git@github.com:gruntwork-io/terragrunt.git",
			expectRef: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			res, err := client.Get(
				context.TODO(),
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
