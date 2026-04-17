package scaffold_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/stretchr/testify/assert"
)

func TestBuildSourceURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		originalURL string
		resolvedURL string
		expected    string
	}{
		{
			name:        "catalog URL gets ref from resolved",
			originalURL: "github.com/gruntwork-io/terragrunt-scale-catalog//modules/azure/resource-group",
			resolvedURL: "git::https://github.com/gruntwork-io/terragrunt-scale-catalog.git//modules/azure/resource-group?ref=v1.10.2",
			expected:    "github.com/gruntwork-io/terragrunt-scale-catalog//modules/azure/resource-group?ref=v1.10.2",
		},
		{
			name:        "original already has ref",
			originalURL: "github.com/gruntwork-io/repo//modules/foo?ref=v2.0.0",
			resolvedURL: "git::https://github.com/gruntwork-io/repo.git//modules/foo?ref=v1.0.0",
			expected:    "github.com/gruntwork-io/repo//modules/foo?ref=v2.0.0",
		},
		{
			name:        "no ref in resolved URL",
			originalURL: "github.com/gruntwork-io/repo//modules/foo",
			resolvedURL: "git::https://github.com/gruntwork-io/repo.git//modules/foo",
			expected:    "github.com/gruntwork-io/repo//modules/foo",
		},
		{
			name:        "original with existing query params gets ref appended",
			originalURL: "github.com/gruntwork-io/repo//modules/foo?depth=1",
			resolvedURL: "git::https://github.com/gruntwork-io/repo.git//modules/foo?depth=1&ref=v1.0.0",
			expected:    "github.com/gruntwork-io/repo//modules/foo?depth=1&ref=v1.0.0",
		},
		{
			name:        "git:: prefixed original preserved",
			originalURL: "git::https://github.com/gruntwork-io/repo.git//modules/foo",
			resolvedURL: "git::https://github.com/gruntwork-io/repo.git//modules/foo?ref=v1.0.0",
			expected:    "git::https://github.com/gruntwork-io/repo.git//modules/foo?ref=v1.0.0",
		},
		{
			name:        "root module without subdirectory",
			originalURL: "github.com/gruntwork-io/repo",
			resolvedURL: "git::https://github.com/gruntwork-io/repo.git?ref=v3.0.0",
			expected:    "github.com/gruntwork-io/repo?ref=v3.0.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := scaffold.BuildSourceURL(tc.originalURL, tc.resolvedURL)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestExtractQueryParam(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		rawURL   string
		param    string
		expected string
	}{
		{
			name:     "ref from go-getter URL",
			rawURL:   "git::https://github.com/org/repo.git//modules/foo?ref=v1.0.0",
			param:    "ref",
			expected: "v1.0.0",
		},
		{
			name:     "ref among multiple params",
			rawURL:   "git::https://github.com/org/repo.git//modules/foo?depth=1&ref=v2.0.0",
			param:    "ref",
			expected: "v2.0.0",
		},
		{
			name:     "no query string",
			rawURL:   "git::https://github.com/org/repo.git//modules/foo",
			param:    "ref",
			expected: "",
		},
		{
			name:     "param not present",
			rawURL:   "git::https://github.com/org/repo.git//modules/foo?depth=1",
			param:    "ref",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := scaffold.ExtractQueryParam(tc.rawURL, tc.param)
			assert.Equal(t, tc.expected, result)
		})
	}
}
