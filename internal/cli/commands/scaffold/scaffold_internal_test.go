package scaffold

import (
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseModuleURLTFRRoundTrip verifies that a tfr:// registry source
// survives parseModuleURL without a "Failed to parse url" warning path and
// without shelling out to git for a tag, keeping the ?version= query param
// intact end to end. See https://github.com/gruntwork-io/terragrunt/issues/3677.
func TestParseModuleURLTFRRoundTrip(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions()

	moduleURL := "tfr:///terraform-aws-modules/eks/aws?version=20.31.4"

	resolved, err := parseModuleURL(t.Context(), l, venv.Venv{}, opts, map[string]any{}, moduleURL)
	require.NoError(t, err)
	assert.Equal(t, "20.31.4", ExtractQueryParam(resolved, "version"))
}

func TestRewriteModuleURLSkipsRegexForNonGitSSHRewrite(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions()

	testCases := []struct {
		vars      map[string]any
		name      string
		moduleURL string
	}{
		{
			name:      "tfr scheme, default SourceUrlType",
			moduleURL: "tfr://registry.opentofu.org/terraform-aws-modules/eks/aws",
			vars:      map[string]any{},
		},
		{
			name:      "tfr scheme, explicit git-https",
			moduleURL: "tfr://registry.opentofu.org/terraform-aws-modules/eks/aws",
			vars:      map[string]any{sourceURLTypeVar: sourceURLTypeHTTPS},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := rewriteModuleURL(l, opts, tc.vars, tc.moduleURL)
			require.NoError(t, err)
			assert.Equal(t, "tfr", result.Scheme)
		})
	}
}

func TestAddRefToModuleURLSkipsGitLookupForNonGitSchemes(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions()

	testCases := []struct {
		name   string
		rawURL string
	}{
		{name: "tfr", rawURL: "tfr://registry.opentofu.org/terraform-aws-modules/eks/aws?version=20.31.4"},
		{name: "https, no forced getter", rawURL: "https://example.com/module.zip"},
		{name: "oci", rawURL: "oci://example.com/module:1.0.0"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := url.Parse(tc.rawURL)
			require.NoError(t, err)

			before := parsed.String()

			// A zero-value venv.Venv is safe here: addRefToModuleURL must
			// return before it ever touches v by shelling out to git for
			// these schemes. If it didn't skip, RunCommandWithOutput would
			// panic or fail on the zero-value venv/writers well before any
			// git error message, which is a stronger signal than trying to
			// assert on log output.
			result, err := addRefToModuleURL(t.Context(), l, venv.Venv{}, opts, parsed, map[string]any{})
			require.NoError(t, err)
			assert.Equal(t, before, result.String())
		})
	}
}

func TestIsGitShapedScheme(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		scheme   string
		expected bool
	}{
		{"", true},
		{"file", true},
		{"git", true},
		{"ssh", true},
		{"git::https", true},
		{"git::ssh", true},
		{"tfr", false},
		{"s3", false},
		{"s3::https", false},
		{"gcs::https", false},
		{"http", false},
		{"https", false},
		{"oci", false},
	}

	for _, tc := range testCases {
		t.Run(tc.scheme, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, isGitShapedScheme(tc.scheme))
		})
	}
}
