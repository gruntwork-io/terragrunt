package cli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boundaryUnits holds a dependency that crosses out of prod: app depends on
// prod/db and on shared/dns, so a boundary drawn at prod has one edge to cut
// and one to keep.
var boundaryUnits = map[string]string{
	"prod/app/terragrunt.hcl": `
dependency "db" {
  config_path = "../db"
  mock_outputs = {
    db_id = "db-mocked"
  }
}

dependency "dns" {
  config_path = "../../shared/dns"
  mock_outputs = {
    dns_id = "dns-mocked"
  }
}

inputs = {
  db_id  = dependency.db.outputs.db_id
  dns_id = dependency.dns.outputs.dns_id
}
`,
	"prod/db/terragrunt.hcl":    "",
	"shared/dns/terragrunt.hcl": "",
}

// runBounded runs a discovery command under the bounded-discovery experiment,
// from workDir inside the boundary fixture.
func runBounded(t *testing.T, workDir string, args ...string) (string, error) {
	t.Helper()

	v := venvtest.New().WithFS(venvtest.NewFS(t, discoveryRoot, boundaryUnits))

	base := []string{
		"--experiment", "bounded-discovery",
		"--no-color",
		"--working-dir", filepath.Join(discoveryRoot, workDir),
	}

	return runCLI(t, v, append(args, base...)...)
}

func TestDiscoveryBoundaryBoundsGraphTraversal(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "dependency traversal leaves the working directory when unbounded",
			args: []string{"--filter", "{./app}..."},
			want: []string{"app", "db", "../shared/dns"},
		},
		{
			name: "dependency traversal stops at the boundary",
			args: []string{"--filter", "{./app}...", "--discovery-boundary", "."},
			want: []string{"app", "db"},
		},
		{
			name: "dependent traversal keeps what the boundary encloses",
			args: []string{"--filter", "...{./db}", "--discovery-boundary", "."},
			want: []string{"app", "db"},
		},
		{
			name: "inline operand reaches wider than the boundary it overrides",
			args: []string{"--filter", "{./app}...(..)", "--discovery-boundary", "."},
			want: []string{"app", "db", "../shared/dns"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := runBounded(t, "prod", append([]string{"find"}, tc.args...)...)
			require.NoError(t, err)

			assert.ElementsMatch(t, tc.want, discoveredPaths(out))
		})
	}
}

// TestDiscoveryBoundaryKeepsEdgesToWithheldDependencies pins the edge to a
// dependency the boundary withholds. The unit that stays still has to be
// ordered against that dependency and to read its outputs, so the edge has to
// survive the component being dropped.
func TestDiscoveryBoundaryKeepsEdgesToWithheldDependencies(t *testing.T) {
	t.Parallel()

	out, err := runBounded(t, "prod", "dag", "graph", "--discovery-boundary", ".")
	require.NoError(t, err)

	assert.Contains(t, out, `"app" -> "../shared/dns"`)
	assert.Contains(t, out, `"app" -> "db"`)

	// DOT gives every component it reports a `"path" ;` line of its own.
	// shared/dns has none, so the boundary dropped it.
	assert.NotContains(t, out, `"../shared/dns" ;`)
}

// TestDiscoveryBoundaryRejectsDependentTraversalOutsideIt pins the refusal of
// a boundary that excludes the working directory. Filters that traverse
// dependents search upward from the working directory, so such a boundary
// could never take effect.
func TestDiscoveryBoundaryRejectsDependentTraversalOutsideIt(t *testing.T) {
	t.Parallel()

	_, err := runBounded(t, ".", "find", "--filter", "...{./prod/db}", "--discovery-boundary", "./prod")

	var scopeErr discovery.DiscoveryBoundaryScopeError

	require.ErrorAs(t, err, &scopeErr)
	assert.Equal(t, filepath.Join(discoveryRoot, "prod"), scopeErr.Boundary)
	assert.Equal(t, discoveryRoot, scopeErr.WorkingDir)
}

// regionUnits puts each unit in its own region directory, with the dependency
// crossing between them, so a run rooted at one region has to reach outside it
// to draw the edge.
//
// Neither include resolves, since there is no parent config for
// find_in_parent_folders to find. Discovery suppresses the failure and reads
// the dependency block anyway.
var regionUnits = map[string]string{
	"region-1/unit-a/terragrunt.hcl": `
include {
  path = find_in_parent_folders()
}

dependency "b" {
  config_path = "../../region-2/unit-b/"
}
`,
	"region-2/unit-b/terragrunt.hcl": `
include {
  path = find_in_parent_folders()
}
`,
}

// TestIncludeExternalInDagGraph pins that both DOT renderers draw a unit
// outside the working directory. An edge pointing at nothing would misreport
// the order the queue runs in.
func TestIncludeExternalInDagGraph(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
	}{
		{name: "dag graph", args: []string{"dag", "graph"}},
		{name: "list", args: []string{"list", "--format=dot", "--dependencies"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New().WithFS(venvtest.NewFS(t, discoveryRoot, regionUnits))
			workDir := filepath.Join(discoveryRoot, "region-1")

			out, err := runCLI(t, v, append(tc.args, "--no-color", "--working-dir", workDir)...)
			require.NoError(t, err)

			assert.Contains(t, out, `unit-a" ->`)
		})
	}
}

func discoveredPaths(out string) []string {
	var paths []string

	for line := range strings.SplitSeq(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}

	return paths
}
