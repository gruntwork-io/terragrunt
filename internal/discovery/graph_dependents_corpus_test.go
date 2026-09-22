package discovery_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// corpusRepoRoot is the git root every corpus fixture is laid out under. The
// fixtures live on an in-memory filesystem, so the path never touches disk.
var corpusRepoRoot = venvtest.Root("/repo")

// dependentsCase is one entry in the dependents-discovery corpus: a fixture
// tree, the queries run over it, and the components the run has to select.
//
// The corpus is a differential harness, not a set of one-off tests. Every entry
// records the selection the upstream dependent walk produces today, so a change
// to that walk is checked by running the whole corpus and comparing selections
// rather than by reasoning about which shapes the change could disturb.
type dependentsCase struct {
	// files maps an absolute config file path to its HCL contents.
	files map[string]string
	// name identifies the fixture in test output.
	name string
	// workingDir is the directory discovery runs from.
	workingDir string
	// queries are the filter queries applied to the discovered set.
	queries []string
	// configNames overrides the config filenames discovery looks for, for
	// fixtures that exercise a non-default config name.
	configNames []string
	// expected is the sorted set of component paths the run has to select.
	expected []string
	// noHidden excludes hidden directories from the filesystem walk.
	noHidden bool
}

// run executes discovery over the fixture and returns the selected component
// paths, sorted. Stacks count alongside units, since a dependents query can
// select either. Discovery hands its results back in the order its phases
// happened to finish, so the order carries no contract and the comparison is
// over sets.
func (tc *dependentsCase) run(t *testing.T) []string {
	t.Helper()

	v := memRepoRootVenv(t, corpusRepoRoot)

	writeFixture(t, v, tc.files)

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = tc.workingDir
	opts.RootWorkingDir = tc.workingDir

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, tc.queries)
	require.NoError(t, err)

	d := discovery.NewDiscovery(tc.workingDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tc.workingDir}).
		WithGitRoot(corpusRepoRoot).
		WithFilters(filters)

	if len(tc.configNames) > 0 {
		d = d.WithConfigFilenames(tc.configNames)
	}

	if tc.noHidden {
		d = d.WithNoHidden()
	}

	components, err := d.Discover(t.Context(), l, v, opts)
	require.NoError(t, err)

	paths := components.Paths()
	slices.Sort(paths)

	return paths
}

// writeFixture materializes a fixture tree on the venv's filesystem.
func writeFixture(t *testing.T, v *venv.Venv, files map[string]string) {
	t.Helper()

	for path, contents := range files {
		require.NoError(t, vfs.WriteFile(v.FS, path, []byte(contents), 0o644))
	}
}

// repoPath joins path segments onto the corpus repo root.
func repoPath(segments ...string) string {
	return filepath.Join(append([]string{corpusRepoRoot}, segments...)...)
}

// unitHCL renders a unit whose dependencies are the given relative paths.
func unitHCL(deps ...string) string {
	var hcl strings.Builder

	for i, dep := range deps {
		fmt.Fprintf(&hcl, "dependency \"d%d\" {\n  config_path = %q\n}\n", i, dep)
	}

	return hcl.String()
}

// dependentsCorpus is the fixture corpus the differential test runs over. The
// shapes are the ones the upstream dependent walk is most likely to get wrong:
// a diamond, a dependent above another dependent, a unit reached through two
// directories, a target nobody depends on, a boundary the walk must not cross,
// a unit above the working directory whose config file is not the default one,
// a directory above the working directory holding two config files, a unit above
// the working directory sharing its directory with a stack, a
// dependent hidden from the filesystem walk, and a dependent reachable only
// through a unit a negated query excludes.
func dependentsCorpus() []dependentsCase {
	return []dependentsCase{
		{
			name: "diamond from the repo root",
			files: map[string]string{
				repoPath("vpc", "terragrunt.hcl"): "",
				repoPath("a", "terragrunt.hcl"):   unitHCL("../vpc"),
				repoPath("b", "terragrunt.hcl"):   unitHCL("../vpc"),
				repoPath("top", "terragrunt.hcl"): unitHCL("../a", "../b"),
			},
			workingDir: corpusRepoRoot,
			queries:    []string{"...{" + repoPath("vpc") + "}"},
			expected: []string{
				repoPath("a"),
				repoPath("b"),
				repoPath("top"),
				repoPath("vpc"),
			},
		},
		{
			name: "diamond from the target's own directory",
			files: map[string]string{
				repoPath("vpc", "terragrunt.hcl"): "",
				repoPath("a", "terragrunt.hcl"):   unitHCL("../vpc"),
				repoPath("b", "terragrunt.hcl"):   unitHCL("../vpc"),
				repoPath("top", "terragrunt.hcl"): unitHCL("../a", "../b"),
			},
			workingDir: repoPath("vpc"),
			queries:    []string{"...{" + repoPath("vpc") + "}"},
			expected: []string{
				repoPath("a"),
				repoPath("b"),
				repoPath("top"),
				repoPath("vpc"),
			},
		},
		{
			name: "dependent above another dependent",
			files: map[string]string{
				repoPath("svc", "net", "vpc", "terragrunt.hcl"):    "",
				repoPath("svc", "net", "nearby", "terragrunt.hcl"): unitHCL("../vpc"),
				repoPath("shared", "terragrunt.hcl"):               unitHCL("../svc/net/nearby"),
				repoPath("noise", "one", "terragrunt.hcl"):         "",
				repoPath("noise", "two", "terragrunt.hcl"):         "",
			},
			workingDir: repoPath("svc", "net"),
			queries:    []string{"...{" + repoPath("svc", "net", "vpc") + "}"},
			expected: []string{
				repoPath("shared"),
				repoPath("svc", "net", "nearby"),
				repoPath("svc", "net", "vpc"),
			},
		},
		{
			name: "dependent reached through two directories",
			files: map[string]string{
				repoPath("vpc", "terragrunt.hcl"):      "",
				repoPath("x", "a", "terragrunt.hcl"):   unitHCL("../../vpc"),
				repoPath("y", "b", "terragrunt.hcl"):   unitHCL("../../x/a"),
				repoPath("y", "c", "terragrunt.hcl"):   unitHCL("../../x/a"),
				repoPath("z", "far", "terragrunt.hcl"): "",
			},
			workingDir: repoPath("vpc"),
			queries:    []string{"...{" + repoPath("vpc") + "}"},
			expected: []string{
				repoPath("vpc"),
				repoPath("x", "a"),
				repoPath("y", "b"),
				repoPath("y", "c"),
			},
		},
		{
			name: "target with no dependents",
			files: map[string]string{
				repoPath("vpc", "terragrunt.hcl"): "",
				repoPath("a", "terragrunt.hcl"):   "",
				repoPath("b", "terragrunt.hcl"):   unitHCL("../a"),
			},
			workingDir: corpusRepoRoot,
			queries:    []string{"...{" + repoPath("vpc") + "}"},
			expected:   []string{repoPath("vpc")},
		},
		{
			name: "dependents crossing the git root",
			files: map[string]string{
				repoPath("environments", "staging", "vpc", "terragrunt.hcl"): "",
				repoPath("environments", "staging", "app", "terragrunt.hcl"): unitHCL("../vpc"),
				repoPath("environments", "production", "consumer", "terragrunt.hcl"): unitHCL(
					"../../staging/vpc",
				),
			},
			workingDir: repoPath("environments", "staging"),
			queries: []string{
				"...{" + repoPath("environments", "staging", "vpc") + "}",
			},
			expected: []string{
				repoPath("environments", "production", "consumer"),
				repoPath("environments", "staging", "app"),
				repoPath("environments", "staging", "vpc"),
			},
		},
		{
			name: "dependents confined by an inline boundary",
			files: map[string]string{
				repoPath("environments", "staging", "vpc", "terragrunt.hcl"): "",
				repoPath("environments", "staging", "app", "terragrunt.hcl"): unitHCL("../vpc"),
				repoPath("environments", "production", "consumer", "terragrunt.hcl"): unitHCL(
					"../../staging/vpc",
				),
			},
			workingDir: repoPath("environments", "staging"),
			queries: []string{
				"(" + repoPath("environments", "staging") + ")...{" +
					repoPath("environments", "staging", "vpc") + "}",
			},
			expected: []string{
				repoPath("environments", "staging", "app"),
				repoPath("environments", "staging", "vpc"),
			},
		},
		{
			// The unit above the working directory is reached as a dependency of a
			// nearer unit first, so the shared component set already holds it under
			// the default config filename by the time the walk reaches its own
			// directory. Parsing that component instead of the walked one reads a
			// file that does not exist.
			name: "unit above the working directory with a non-default config name",
			files: map[string]string{
				repoPath("svc", "net", "vpc", "terragrunt.hcl"):    "",
				repoPath("svc", "net", "nearby", "terragrunt.hcl"): unitHCL("../vpc"),
				repoPath("svc", "mid", "terragrunt.hcl"):           unitHCL("../net/vpc", "../../shared"),
				repoPath("shared", "custom.hcl"):                   unitHCL("../svc/net/vpc"),
			},
			workingDir:  repoPath("svc", "net"),
			configNames: []string{"terragrunt.hcl", "custom.hcl"},
			queries:     []string{"...{" + repoPath("svc", "net", "vpc") + "}"},
			expected: []string{
				repoPath("shared"),
				repoPath("svc", "mid"),
				repoPath("svc", "net", "nearby"),
				repoPath("svc", "net", "vpc"),
			},
		},
		{
			// A directory above the working directory holds two config files, so
			// the walk mints two candidates reporting the same path, and only the
			// one the query never names declares the dependency on the target.
			name: "two config files in one directory above the working directory",
			files: map[string]string{
				repoPath("svc", "net", "vpc", "terragrunt.hcl"):    "",
				repoPath("svc", "net", "nearby", "terragrunt.hcl"): unitHCL("../vpc"),
				repoPath("dual", "terragrunt.hcl"):                 "",
				repoPath("dual", "custom.hcl"):                     unitHCL("../svc/net/vpc"),
			},
			workingDir:  repoPath("svc", "net"),
			configNames: []string{"terragrunt.hcl", "custom.hcl"},
			queries:     []string{"...{" + repoPath("svc", "net", "vpc") + "}"},
			expected: []string{
				repoPath("dual"),
				repoPath("svc", "net", "nearby"),
				repoPath("svc", "net", "vpc"),
			},
		},
		{
			// Coexistence validation only covers what the filesystem walk reached,
			// so a directory above the working directory can hold both a unit and a
			// stack config. The nearer unit's dependency mints the stack into the
			// shared set first, leaving a stack published at the unit candidate's
			// own path.
			name: "unit sharing a directory with a stack above the working directory",
			files: map[string]string{
				repoPath("svc", "net", "vpc", "terragrunt.hcl"):    "",
				repoPath("svc", "net", "nearby", "terragrunt.hcl"): unitHCL("../vpc"),
				repoPath("svc", "mid", "terragrunt.hcl"):           unitHCL("../net/vpc", "../../mixed"),
				repoPath("mixed", "terragrunt.stack.hcl"):          "",
				repoPath("mixed", "terragrunt.hcl"):                unitHCL("../svc/net/vpc"),
				repoPath("other", "terragrunt.hcl"):                unitHCL("../mixed"),
			},
			workingDir: repoPath("svc", "net"),
			queries:    []string{"...{" + repoPath("svc", "net", "vpc") + "}"},
			expected: []string{
				repoPath("mixed"),
				repoPath("other"),
				repoPath("svc", "mid"),
				repoPath("svc", "net", "nearby"),
				repoPath("svc", "net", "vpc"),
			},
		},
		{
			// The filesystem walk skips hidden directories under noHidden; the
			// upstream dependent walk does not, so the hidden dependent is selected.
			name: "dependent inside a hidden directory",
			files: map[string]string{
				repoPath("vpc", "terragrunt.hcl"):               "",
				repoPath("plain", "terragrunt.hcl"):             unitHCL("../vpc"),
				repoPath(".hidden", "sneaky", "terragrunt.hcl"): unitHCL("../../vpc"),
			},
			workingDir: corpusRepoRoot,
			noHidden:   true,
			queries:    []string{"...{" + repoPath("vpc") + "}"},
			expected: []string{
				repoPath(".hidden", "sneaky"),
				repoPath("plain"),
				repoPath("vpc"),
			},
		},
		{
			// A negated query excludes the intermediate unit from the discovered
			// set, but its own dependent still reaches the target through it.
			name: "dependent behind a negated intermediate",
			files: map[string]string{
				repoPath("vpc", "terragrunt.hcl"):            "",
				repoPath("apps", "a", "terragrunt.hcl"):      unitHCL("../../vpc"),
				repoPath("apps", "b", "terragrunt.hcl"):      unitHCL("../a"),
				repoPath("apps", "spare", "terragrunt.hcl"):  "",
				repoPath("infra", "other", "terragrunt.hcl"): "",
			},
			workingDir: corpusRepoRoot,
			queries: []string{
				"...{" + repoPath("vpc") + "}",
				"!./apps/a",
			},
			expected: []string{
				repoPath("apps", "b"),
				repoPath("vpc"),
			},
		},
	}
}

// TestGraphPhase_DependentsCorpus pins the selection every corpus fixture
// produces. Discovery decides which units a command runs against, so a change
// to the upstream dependent walk that drops a unit narrows a plan or an apply
// silently; running the whole corpus is what catches that.
func TestGraphPhase_DependentsCorpus(t *testing.T) {
	t.Parallel()

	for _, tc := range dependentsCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, tc.run(t))
		})
	}
}
