package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateStacksCyclicSource_FailsAtMaxLevel pins the cycle detection in
// GenerateStacks: a stack whose source resolves to a stack that sources itself
// nests one generation level per iteration, and the run must stop with a cycle
// error once the configured level cap is reached.
func TestGenerateStacksCyclicSource_FailsAtMaxLevel(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	liveDir := filepath.Join(tmpDir, "live")
	stackDir := filepath.Join(tmpDir, "stack")

	// Both stack files source the same absolute stack directory, so every
	// generated level discovers one more copy of the self-sourcing stack.
	stackConfig := fmt.Sprintf(`stack "stack" {
  source = %q
  path   = "stack"
}
`, stackDir)

	require.NoError(t, os.MkdirAll(liveDir, 0o755))
	require.NoError(t, os.MkdirAll(stackDir, 0o755))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(liveDir, "terragrunt.stack.hcl"), []byte(stackConfig), 0o644),
	)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(stackConfig), 0o644),
	)

	l := logger.CreateLogger()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = liveDir
	opts.RootWorkingDir = liveDir
	opts.Parallelism = 1
	opts.NoCAS = true

	const maxLevel = 5

	err := NewGenerator().
		WithMaxLevel(maxLevel).
		GenerateStacks(t.Context(), l, venvtest.NewOSWithEmptyEnv(), opts, nil)

	require.ErrorContains(
		t,
		err,
		fmt.Sprintf("cycle detected: maximum level (%d) exceeded", maxLevel),
		"self-sourcing stacks must trip the nesting cap instead of recursing until the filesystem gives up",
	)
}

// TestGeneratorDefaultMaxLevel pins the default nesting cap so it stays
// generous enough that real stack trees never trip cycle detection.
func TestGeneratorDefaultMaxLevel(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		1024,
		DefaultMaxLevel,
		"default level cap must stay generous so only genuine cycles reach it",
	)
}

// TestStackDiscoveryFilters pins how a generation run composes its stack filters. A user
// filter that already narrows to stacks must stand on its own, because unioning a blanket
// type=stack alongside it would re-select the stacks it excludes.
func TestStackDiscoveryFilters(t *testing.T) {
	t.Parallel()

	stacks := component.Components{
		component.NewStack("./live/land-mine"),
		component.NewStack("./live/normal"),
	}

	for _, c := range stacks {
		c.SetDiscoveryContext(&component.DiscoveryContext{WorkingDir: "."})
	}

	tests := []struct {
		name     string
		filters  []string
		expected []string
	}{
		{
			name:     "no filters selects every stack",
			filters:  nil,
			expected: []string{"./live/land-mine", "./live/normal"},
		},
		{
			name:     "stack filters that exclude keep excluding",
			filters:  []string{"!./live/land-mine | type=stack"},
			expected: []string{"./live/normal"},
		},
		{
			name:     "filters that do not narrow to stacks fall back to every stack",
			filters:  []string{"[HEAD~1...HEAD]"},
			expected: []string{"./live/land-mine", "./live/normal"},
		},
		{
			name:     "a path filter alone still leaves generation permissive",
			filters:  []string{"./apps/*"},
			expected: []string{"./live/land-mine", "./live/normal"},
		},
		{
			name:     "another filter's stacks survive alongside a stack filter",
			filters:  []string{"type=stack | name=normal", "name=land-mine"},
			expected: []string{"./live/normal", "./live/land-mine"},
		},
		{
			name:     "a git expression never narrows the stacks a stack filter selects",
			filters:  []string{"[HEAD~1...HEAD]", "!./live/land-mine | type=stack"},
			expected: []string{"./live/normal"},
		},
		{
			name:     "a filter that only excludes keeps subtracting",
			filters:  []string{"!./live/land-mine | type=stack", "!name=normal"},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()

			parsed, err := filter.ParseFilterQueries(l, tt.filters)
			require.NoError(t, err)

			selected, err := StackDiscoveryFilters(parsed).Evaluate(l, filter.EvaluationContext{}, stacks)
			require.NoError(t, err)

			selectedPaths := make([]string, 0, len(selected))
			for _, c := range selected {
				selectedPaths = append(selectedPaths, c.Path())
			}

			assert.ElementsMatch(t, tt.expected, selectedPaths)
		})
	}
}

// TestCanonicalStackFilePathKeepsSymlinkDirectory reproduces the regression where a symlinked
// terragrunt.stack.hcl had its source directory rewritten to the symlink target's directory,
// so read_terragrunt_config and other relative reads looked in the wrong folder. The directory
// portion must be canonicalised, but the stack file must stay anchored to the directory it
// lives in rather than resolving through the symlink to its target's directory.
func TestCanonicalStackFilePathKeepsSymlinkDirectory(t *testing.T) {
	t.Parallel()

	// test/helpers itself is unreachable from this package: importing it would cycle back
	// through internal/cli to internal/stacks/generate, so the platform check uses runtime.
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires elevated privileges on Windows")
	}

	// Resolve the temp root up front so the assertion is not confused by e.g. /var -> /private/var.
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	// The real stack file lives in a "source" directory that is NOT where terragrunt is invoked.
	sourceDir := filepath.Join(base, "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, config.DefaultStackFile), []byte("# stack\n"), 0644))

	// The "live" directory holds a symlink to the real stack file plus the sibling file the stack
	// would read relatively. read_terragrunt_config must resolve against liveDir, not sourceDir.
	liveDir := filepath.Join(base, "live")
	require.NoError(t, os.MkdirAll(liveDir, 0755))

	symlink := filepath.Join(liveDir, config.DefaultStackFile)
	require.NoError(t, os.Symlink(filepath.Join(sourceDir, config.DefaultStackFile), symlink))

	got, err := canonicalStackFilePath(vfs.NewOSFS(), liveDir, base)
	require.NoError(t, err)

	// The generated source dir is filepath.Dir of the returned stack-file path. It must be the
	// directory the symlink lives in, not the symlink target's directory.
	assert.Equal(t, liveDir, filepath.Dir(got),
		"symlinked stack file should keep its own directory as the source dir")
	assert.NotEqual(t, sourceDir, filepath.Dir(got),
		"symlinked stack file must not be rewritten to the symlink target's directory")
}

// TestCanonicalStackFilePathResolvesDirectory confirms the directory portion is still
// symlink-resolved, so topology keys stay consistent with the canonicalised working dir.
func TestCanonicalStackFilePathResolvesDirectory(t *testing.T) {
	t.Parallel()

	// test/helpers itself is unreachable from this package: importing it would cycle back
	// through internal/cli to internal/stacks/generate, so the platform check uses runtime.
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires elevated privileges on Windows")
	}

	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	realDir := filepath.Join(base, "real")
	require.NoError(t, os.MkdirAll(realDir, 0755))

	// A symlinked directory should resolve to its target for a consistent, deduplicable key.
	linkDir := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(realDir, linkDir))

	got, err := canonicalStackFilePath(vfs.NewOSFS(), linkDir, base)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(realDir, config.DefaultStackFile), got)
	// Sanity check against the raw util helper on a non-symlinked directory.
	direct, err := util.CanonicalResolvedPath(vfs.NewOSFS(), realDir, base)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(direct, config.DefaultStackFile), got)
}
