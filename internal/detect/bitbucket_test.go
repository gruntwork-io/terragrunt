package detect_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/detect"
)

func TestBitBucketIgnoresNonBitBucketSources(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		src  string
	}{
		{name: "empty", src: ""},
		{name: "github shorthand", src: "github.com/gruntwork-io/terragrunt"},
		{name: "local path", src: "/abs/path/to/module"},
		{name: "host is a bitbucket suffix", src: "notbitbucket.org/owner/repo"},
		{name: "bare host without a path", src: "bitbucket.org"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, ok, err := new(detect.BitBucket).Detect(tc.src, "")
			require.NoError(t, err)
			assert.False(t, ok)
			assert.Empty(t, out)
		})
	}
}

func TestBitBucketRewritesShorthandToForcedGitURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		src         string
		expectedURL string
	}{
		{
			name:        "adds the .git suffix",
			src:         "bitbucket.org/atlassian/aws-ecr-push-image",
			expectedURL: "git::https://bitbucket.org/atlassian/aws-ecr-push-image.git",
		},
		{
			name:        "keeps an existing .git suffix",
			src:         "bitbucket.org/atlassian/aws-ecr-push-image.git",
			expectedURL: "git::https://bitbucket.org/atlassian/aws-ecr-push-image.git",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, ok, err := new(detect.BitBucket).Detect(tc.src, "")
			require.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, tc.expectedURL, out)
		})
	}
}

// TestBitBucketMatchesUpstreamRewrite pins the rewrite against go-getter's.
// Dropping that detector's API probe was only safe because the rewrite around
// it is unconditional, so the two have to stay identical.
func TestBitBucketMatchesUpstreamRewrite(t *testing.T) {
	t.Parallel()

	// Every shape upstream's detectHTTP puts through url.Parse and its .git
	// suffix check.
	sources := []string{
		"bitbucket.org/owner/repo",
		"bitbucket.org/owner/repo.git",
		"bitbucket.org/owner/repo/deep/path",
	}

	for _, src := range sources {
		out, ok, err := new(detect.BitBucket).Detect(src, "")
		require.NoError(t, err, "src=%s", src)
		require.True(t, ok, "src=%s", src)
		assert.Equal(t, "git::https://"+strings.TrimSuffix(src, ".git")+".git", out)
	}
}
