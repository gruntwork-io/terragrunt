package scaffold_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
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
}

func (r *commandRecorder) venv() *venv.Venv {
	return venvtest.New().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		r.names = append(r.names, inv.Name)

		return vexec.Result{ExitCode: 1}
	})
}

// registryStub answers the registry protocol for one module and records the
// hosts it was asked about, so a test can pin which registry a source with no
// host of its own resolves against.
type registryStub struct {
	modulePath string
	versions   []string
	seen       []string
}

func (s *registryStub) venv() *venv.Venv {
	return venvtest.New().WithHTTP(vhttp.NewMemClient(s.handle))
}

func (s *registryStub) handle(_ context.Context, req *http.Request) (*http.Response, error) {
	s.seen = append(s.seen, req.URL.Host)

	json := http.Header{"Content-Type": []string{"application/json"}}

	if req.URL.Path == "/.well-known/terraform.json" {
		return vhttp.Respond(http.StatusOK, []byte(`{"modules.v1":"/v1/modules/"}`), json), nil
	}

	if req.URL.Path == "/v1/modules/"+s.modulePath+"/versions" {
		quoted := make([]string, 0, len(s.versions))
		for _, v := range s.versions {
			quoted = append(quoted, `{"version":"`+v+`"}`)
		}

		body := `{"modules":[{"versions":[` + strings.Join(quoted, ",") + `]}]}`

		return vhttp.Respond(http.StatusOK, []byte(body), json), nil
	}

	return vhttp.Respond(http.StatusNotFound, nil, nil), nil
}

// TestParseModuleURLPinsUnpinnedRegistrySourceToLatestStable covers the
// registry a source that names no host of its own resolves against. Only the
// auto provider cache dir setup detects the wrapped binary for scaffold, so a
// run without it must still reach tofu's registry rather than Terraform's.
func TestParseModuleURLPinsUnpinnedRegistrySourceToLatestStable(t *testing.T) {
	t.Parallel()

	registry := &registryStub{
		modulePath: "acme/vpc/aws",
		versions:   []string{"0.0.1", "1.2.0", "1.3.0-rc1"},
	}

	opts := options.NewTerragruntOptions()
	opts.TofuImplementation = tfimpl.Unknown

	resolved, err := scaffold.ParseModuleURL(
		t.Context(),
		logger.CreateLogger(),
		registry.venv(),
		opts,
		map[string]any{},
		"tfr:///acme/vpc/aws",
	)
	require.NoError(t, err)
	assert.Equal(t, "tfr:///acme/vpc/aws?version=1.2.0", resolved)
	assert.Equal(t, []string{"registry.opentofu.org", "registry.opentofu.org"}, registry.seen)
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
			assert.Empty(t, recorder.names)
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
	assert.Contains(t, recorder.names, "git")
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
