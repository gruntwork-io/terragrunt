package scaffold_test

import (
	"context"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commandRecorder records the commands a venv is asked to run, so a test can
// assert on whether a lookup was attempted at all rather than on log output.
type commandRecorder struct {
	names []string
	mu    sync.Mutex
}

func (r *commandRecorder) venv() *venv.Venv {
	return venvtest.New().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		r.mu.Lock()
		defer r.mu.Unlock()

		r.names = append(r.names, inv.Name)

		return vexec.Result{ExitCode: 1}
	})
}

func (r *commandRecorder) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.names...)
}

// TestParseModuleURLSkipsGitLookupForNonGitSchemes covers the scaffold half of
// https://github.com/gruntwork-io/terragrunt/issues/3677: a source git does
// not understand must round-trip untouched, without a `git ls-remote` that
// fails with `git: 'remote-tfr' is not a git command`.
func TestParseModuleURLSkipsGitLookupForNonGitSchemes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		vars      map[string]any
		name      string
		moduleURL string
	}{
		{
			name:      "registry source pinned to a version",
			moduleURL: "tfr:///terraform-aws-modules/eks/aws?version=20.31.4",
			vars:      map[string]any{},
		},
		{
			name:      "registry source with an explicit registry host",
			moduleURL: "tfr://registry.opentofu.org/terraform-aws-modules/eks/aws?version=20.31.4",
			vars:      map[string]any{},
		},
		{
			name:      "registry source asked for git-https",
			moduleURL: "tfr://registry.opentofu.org/terraform-aws-modules/eks/aws?version=20.31.4",
			vars:      map[string]any{"SourceUrlType": "git-https"},
		},
		{
			name:      "http archive",
			moduleURL: "https://example.com/module.zip",
			vars:      map[string]any{},
		},
		{
			name:      "oci source",
			moduleURL: "oci://example.com/module:1.0.0",
			vars:      map[string]any{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := &commandRecorder{}

			resolved, err := scaffold.ParseModuleURL(
				t.Context(),
				logger.CreateLogger(),
				recorder.venv(),
				options.NewTerragruntOptions(),
				tc.vars,
				tc.moduleURL,
			)
			require.NoError(t, err)
			assert.Equal(t, tc.moduleURL, resolved)
			assert.Empty(t, recorder.recorded())
		})
	}
}

// TestParseModuleURLLooksUpTagForGitSources is the other half of
// TestParseModuleURLSkipsGitLookupForNonGitSchemes: the git-shaped source it
// contrasts with still reaches the tag lookup, so the skip is scheme-driven
// rather than a lookup that stopped happening for everyone.
func TestParseModuleURLLooksUpTagForGitSources(t *testing.T) {
	t.Parallel()

	recorder := &commandRecorder{}

	moduleURL := "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs"

	resolved, err := scaffold.ParseModuleURL(
		t.Context(),
		logger.CreateLogger(),
		recorder.venv(),
		options.NewTerragruntOptions(),
		map[string]any{},
		moduleURL,
	)
	require.NoError(t, err)
	assert.Equal(t, moduleURL, resolved)
	assert.Contains(t, recorder.recorded(), "git")
}

// TestParseModuleURLLeavesRegistrySourceUnpinnedWhenRegistryUnreachable pins
// the non-fatal half of registry version resolution: an unreachable registry
// leaves the source unpinned instead of failing the scaffold, matching what a
// failed git tag lookup does.
func TestParseModuleURLLeavesRegistrySourceUnpinnedWhenRegistryUnreachable(t *testing.T) {
	t.Parallel()

	moduleURL := "tfr:///terraform-aws-modules/eks/aws"

	resolved, err := scaffold.ParseModuleURL(
		t.Context(),
		logger.CreateLogger(),
		venvtest.New(),
		options.NewTerragruntOptions(),
		map[string]any{},
		moduleURL,
	)
	require.NoError(t, err)
	assert.Equal(t, moduleURL, resolved)
}

func TestIsGitShapedScheme(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		scheme   string
		expected bool
	}{
		{scheme: "", expected: true},
		{scheme: "file", expected: true},
		{scheme: "git", expected: true},
		{scheme: "ssh", expected: true},
		{scheme: "git::https", expected: true},
		{scheme: "git::ssh", expected: true},
		{scheme: "tfr", expected: false},
		{scheme: "s3", expected: false},
		{scheme: "s3::https", expected: false},
		{scheme: "gcs::https", expected: false},
		{scheme: "http", expected: false},
		{scheme: "https", expected: false},
		{scheme: "oci", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.scheme, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, scaffold.IsGitShapedScheme(tc.scheme))
		})
	}
}
