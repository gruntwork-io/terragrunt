package discovery_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// countingFS tallies how often each path is opened beneath it. Discovery reads
// a unit's configuration by opening its config file and lists a directory by
// opening the directory, so the tally is what makes redundant parses and
// redundant directory listings countable without asserting on log output.
type countingFS struct {
	vfs.FS
	opens map[string]int
	mu    sync.Mutex
}

// newCountingFS wraps fsys so every subsequent open is counted.
func newCountingFS(fsys vfs.FS) *countingFS {
	return &countingFS{FS: fsys, opens: map[string]int{}}
}

// Open counts the open and delegates.
func (fsys *countingFS) Open(name string) (afero.File, error) {
	fsys.count(name)

	return fsys.FS.Open(name)
}

// OpenFile counts the open and delegates.
func (fsys *countingFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	fsys.count(name)

	return fsys.FS.OpenFile(name, flag, perm)
}

// count records one open of name.
func (fsys *countingFS) count(name string) {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()

	fsys.opens[filepath.Clean(name)]++
}

// opensOf returns how many times name was opened.
func (fsys *countingFS) opensOf(name string) int {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()

	return fsys.opens[name]
}

// directoryOpens returns how many opens landed on a directory. Discovery lists
// a directory by opening it, and every config file in these fixtures carries an
// .hcl extension, so the paths without one are the directories.
func (fsys *countingFS) directoryOpens() int {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()

	total := 0

	for path, n := range fsys.opens {
		if filepath.Ext(path) != ".hcl" {
			total += n
		}
	}

	return total
}

// dependentsReadFixture lays out a repository whose dependents all sit above
// the working directory, so the whole selection comes from the upstream
// dependent walk. vpc is the target; a and b depend on it, top depends on both,
// and the noise units are unrelated work the walk still has to read.
func dependentsReadFixture() map[string]string {
	files := map[string]string{
		repoPath("vpc", "terragrunt.hcl"): "",
		repoPath("a", "terragrunt.hcl"):   unitHCL("../vpc"),
		repoPath("b", "terragrunt.hcl"):   unitHCL("../vpc"),
		repoPath("top", "terragrunt.hcl"): unitHCL("../a", "../b"),
	}

	for _, name := range []string{"one", "two", "three", "four"} {
		files[repoPath("noise", name, "terragrunt.hcl")] = ""
	}

	return files
}

// discoverWithCountingFS runs a dependents query over the fixture from
// workingDir and returns the selected unit paths alongside the open tally.
func discoverWithCountingFS(
	t *testing.T,
	files map[string]string,
	workingDir string,
) ([]string, *countingFS) {
	t.Helper()

	v := memRepoRootVenv(t, corpusRepoRoot)

	writeFixture(t, v, files)

	counting := newCountingFS(v.FS)
	v = v.WithFS(counting)

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = workingDir
	opts.RootWorkingDir = workingDir

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{"...{" + repoPath("vpc") + "}"})
	require.NoError(t, err)

	components, err := discovery.NewDiscovery(workingDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir}).
		WithGitRoot(corpusRepoRoot).
		WithFilters(filters).
		Discover(t.Context(), l, v, opts)
	require.NoError(t, err)

	return components.Filter(component.UnitKind).Paths(), counting
}

// TestGraphPhase_DependentsParseEachUnitReadCount caps how often a dependents
// query reads each unit's configuration, from both placements of the working
// directory that matter: the repository root, where the pre-graph dependency
// pass has already read every unit, and the target's own directory, where the
// upstream walk sweeps the boundary tree again for every dependent it finds.
//
// The walk mints a throwaway component for each config file it passes, so
// parsing that component rather than the one the shared set already holds is
// what used to read the same file once per unit per target.
//
// From the target's own directory the cap is two rather than one. The walk keeps
// a candidate out of the shared set until it has parsed it and resolved its
// dependencies, so that a candidate it abandons leaves no half-classified
// component behind. A unit first reached by the walk and by another unit's
// dependency resolution at the same moment is therefore minted twice, and
// whichever component loses the race to be published is read for nothing.
func TestGraphPhase_DependentsParseEachUnitReadCount(t *testing.T) {
	t.Parallel()

	files := dependentsReadFixture()

	units := []string{
		repoPath("vpc"),
		repoPath("a"),
		repoPath("b"),
		repoPath("top"),
		repoPath("noise", "one"),
		repoPath("noise", "two"),
		repoPath("noise", "three"),
		repoPath("noise", "four"),
	}

	for _, tc := range []struct {
		name       string
		workingDir string
		maxReads   int
	}{
		{name: "repository root", workingDir: corpusRepoRoot, maxReads: 1},
		{name: "target directory", workingDir: repoPath("vpc"), maxReads: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			selected, counting := discoverWithCountingFS(t, files, tc.workingDir)

			assert.ElementsMatch(t, []string{
				repoPath("vpc"),
				repoPath("a"),
				repoPath("b"),
				repoPath("top"),
			}, selected)

			for _, unit := range units {
				reads := counting.opensOf(filepath.Join(unit, "terragrunt.hcl"))

				assert.Positivef(t, reads, "unit %s was never read", unit)
				assert.LessOrEqualf(t, reads, tc.maxReads, "unit %s was read %d times", unit, reads)
			}
		})
	}
}

// TestGraphPhase_DependentsDirectoryListings caps how many directory listings a
// dependents query costs. The upstream walk restarts from a fresh visited set
// for every dependent it finds, so the boundary tree is swept once per
// dependent; the cap is the measured cost of that today, and holds a future
// change to the walk to it.
func TestGraphPhase_DependentsDirectoryListings(t *testing.T) {
	t.Parallel()

	files := dependentsReadFixture()

	for _, tc := range []struct {
		name       string
		workingDir string
		maxOpens   int
	}{
		{name: "repository root", workingDir: corpusRepoRoot, maxOpens: 20},
		{name: "target directory", workingDir: repoPath("vpc"), maxOpens: 41},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, counting := discoverWithCountingFS(t, files, tc.workingDir)

			assert.LessOrEqual(t, counting.directoryOpens(), tc.maxOpens)
		})
	}
}
