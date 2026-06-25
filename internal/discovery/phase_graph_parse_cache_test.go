package discovery_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// syncBuffer is a bytes.Buffer safe for concurrent writes from logger goroutines.
type syncBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// TestGraphPhase_DependentFiltersReuseParsedConfigs verifies that passing multiple
// dependents filters (...target) does not re-parse every discovered config once per
// target. The dependency graph is parsed once up front, and the per-target upstream
// walks must reuse those cached configs instead of parsing fresh component copies.
//
// Regression test for https://github.com/gruntwork-io/terragrunt/issues/6323
// (runtime of `terragrunt find` grew linearly with the number of --filter flags).
func TestGraphPhase_DependentFiltersReuseParsedConfigs(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Initialize a git repository so the graph phase performs the upstream dependent walk.
	cmd := exec.CommandContext(t.Context(), "git", "init")
	cmd.Dir = tmpDir
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Run())

	// Two independent dependency chains plus standalone units that depend on nothing.
	// Standalone units are the interesting case: they are not dependents of any target,
	// so every per-target upstream walk inspects (and used to re-parse) them.
	testFiles := map[string]string{
		filepath.Join(tmpDir, "vpc", "terragrunt.hcl"): ``,
		filepath.Join(tmpDir, "db", "terragrunt.hcl"): `
dependency "vpc" {
	config_path = "../vpc"
}
`,
		filepath.Join(tmpDir, "cache", "terragrunt.hcl"): ``,
		filepath.Join(tmpDir, "web", "terragrunt.hcl"): `
dependency "cache" {
	config_path = "../cache"
}
`,
		filepath.Join(tmpDir, "standalone-a", "terragrunt.hcl"): ``,
		filepath.Join(tmpDir, "standalone-b", "terragrunt.hcl"): ``,
	}

	for path, content := range testFiles {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	}

	out := &syncBuffer{}
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)
	l := log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter), log.WithOutput(out))

	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	filters, err := filter.ParseFilterQueries(l, []string{"...vpc", "...cache"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	paths := components.Paths()
	assert.Contains(t, paths, filepath.Join(tmpDir, "vpc"))
	assert.Contains(t, paths, filepath.Join(tmpDir, "db"))
	assert.Contains(t, paths, filepath.Join(tmpDir, "cache"))
	assert.Contains(t, paths, filepath.Join(tmpDir, "web"))

	// Each config must be parsed at most once for the whole discovery, regardless
	// of how many dependents filters were passed.
	parseCounts := make(map[string]int)

	for line := range strings.SplitSeq(out.String(), "\n") {
		_, after, found := strings.Cut(line, "Discovery: parsing ")
		if !found {
			continue
		}

		path, _, _ := strings.Cut(after, " (")
		parseCounts[path]++
	}

	require.NotEmpty(t, parseCounts, "expected at least one parse to be logged")

	for path, count := range parseCounts {
		assert.LessOrEqual(
			t, count, 1,
			"config %s was parsed %d times; parsed configs must be reused across filter targets",
			path, count,
		)
	}
}

// TestGraphPhase_UpstreamWalkConcurrentTargets runs discovery from a subdirectory of a
// git repository with many dependents filters, so concurrent per-target upstream walks
// encounter the same configs above the working directory. Those configs are not parsed
// by the upfront dependency-graph build, so this exercises concurrent ensureParsed calls
// on shared canonical components. Run with -race to validate synchronization.
func TestGraphPhase_UpstreamWalkConcurrentTargets(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	cmd := exec.CommandContext(t.Context(), "git", "init")
	cmd.Dir = tmpDir
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Run())

	// Working directory is a subdirectory of the git repo.
	workDir := filepath.Join(tmpDir, "live")

	targetNames := []string{"t0", "t1", "t2", "t3", "t4", "t5", "t6", "t7"}
	filterStrings := make([]string, 0, len(targetNames))

	for _, name := range targetNames {
		dir := filepath.Join(workDir, name)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(``), 0644))

		filterStrings = append(filterStrings, "..."+name)
	}

	// Units above the working directory: walked (and parsed) only during the
	// per-target upstream walks, concurrently across targets. One of them depends
	// on a target so the upstream walk has a real dependent to find.
	outside := map[string]string{
		filepath.Join(tmpDir, "shared-a", "terragrunt.hcl"): ``,
		filepath.Join(tmpDir, "shared-b", "terragrunt.hcl"): ``,
		filepath.Join(tmpDir, "shared-c", "terragrunt.hcl"): ``,
		filepath.Join(tmpDir, "consumer", "terragrunt.hcl"): `
dependency "t0" {
	config_path = "../live/t0"
}
`,
	}

	for path, content := range outside {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     workDir,
		RootWorkingDir: workDir,
	}

	filters, err := filter.ParseFilterQueries(l, filterStrings)
	require.NoError(t, err)

	d := discovery.NewDiscovery(workDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workDir}).
		WithFilters(filters)

	components, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	paths := components.Paths()
	for _, name := range targetNames {
		assert.Contains(t, paths, filepath.Join(workDir, name))
	}

	assert.Contains(
		t, paths, filepath.Join(tmpDir, "consumer"),
		"dependent above the working directory must be discovered",
	)
}
