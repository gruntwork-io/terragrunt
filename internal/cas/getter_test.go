package cas_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCASGetterMode(t *testing.T) {
	t.Parallel()

	g := cas.NewCASGetter(nil, nil, &cas.CloneOptions{})
	testURL, err := url.Parse("https://github.com/gruntwork-io/terragrunt")
	require.NoError(t, err)

	mode, err := g.Mode(context.Background(), testURL)
	require.NoError(t, err)
	assert.Equal(t, getter.ModeDir, mode)
}

func TestCASGetterGetFile(t *testing.T) {
	t.Parallel()

	g := cas.NewCASGetter(nil, nil, &cas.CloneOptions{})
	err := g.GetFile(context.Background(), &getter.Request{})
	require.Error(t, err)
	assert.Equal(t, "GetFile not implemented", err.Error())
}

func TestCASGetterDetect(t *testing.T) {
	t.Parallel()

	g := cas.NewCASGetter(nil, nil, &cas.CloneOptions{})

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
			t.Parallel()

			req := &getter.Request{
				Src: tt.src,
				Pwd: tt.pwd,
			}
			ok, err := g.Detect(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, ok)
		})
	}
}

func TestCASGetterGet(t *testing.T) {
	t.Parallel()

	c, err := cas.New(cas.Options{})
	require.NoError(t, err)

	opts := &cas.CloneOptions{
		Branch: "main",
	}

	l := log.New()

	g := cas.NewCASGetter(&l, c, opts)
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
			t.Parallel()

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
