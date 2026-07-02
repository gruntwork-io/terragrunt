package generate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
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

	err := generate.NewGenerator().
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
		generate.DefaultMaxLevel,
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

			selected, err := generate.StackDiscoveryFilters(parsed).Evaluate(l, filter.EvaluationContext{}, stacks)
			require.NoError(t, err)

			selectedPaths := make([]string, 0, len(selected))
			for _, c := range selected {
				selectedPaths = append(selectedPaths, c.Path())
			}

			assert.ElementsMatch(t, tt.expected, selectedPaths)
		})
	}
}

// stackFileFixtureOpts returns the options a ListStackFilesWithExcludes call needs to discover
// stack files under workingDir.
func stackFileFixtureOpts(workingDir string) *options.TerragruntOptions {
	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = workingDir
	opts.RootWorkingDir = workingDir
	opts.Parallelism = 1
	opts.NoCAS = true

	return opts
}

// TestListStackFilesKeepsSymlinkedStackFileDirectory reproduces the regression where a
// symlinked terragrunt.stack.hcl had its directory rewritten to the symlink target's, so
// read_terragrunt_config and other reads relative to get_terragrunt_dir() looked in the wrong
// folder. Discovery canonicalises the stack file's directory, but the file must stay anchored
// to the directory it lives in rather than resolving through the symlink to its target's.
func TestListStackFilesKeepsSymlinkedStackFileDirectory(t *testing.T) {
	t.Parallel()

	// Resolve the temp root up front so assertions are not confused by e.g. /var -> /private/var.
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	// The real stack file lives in a "source" directory that is NOT where terragrunt is invoked.
	sourceDir := filepath.Join(base, "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(sourceDir, "terragrunt.stack.hcl"), []byte("# stack\n"), 0o644),
	)

	// "live" holds only a symlink to the real stack file. Terragrunt runs here, so the
	// discovered path must stay under liveDir for relative reads to resolve next to it.
	liveDir := filepath.Join(base, "live")
	require.NoError(t, os.MkdirAll(liveDir, 0o755))

	if err := os.Symlink(
		filepath.Join(sourceDir, "terragrunt.stack.hcl"),
		filepath.Join(liveDir, "terragrunt.stack.hcl"),
	); err != nil {
		if helpers.SymlinkUnsupported(err) {
			t.Skipf("host does not support creating symlinks: %v", err)
		}

		require.NoError(t, err)
	}

	found, _, err := generate.ListStackFilesWithExcludes(
		t.Context(),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv(),
		stackFileFixtureOpts(liveDir),
		nil,
	)
	require.NoError(t, err)

	assert.Equal(t, []string{filepath.Join(liveDir, "terragrunt.stack.hcl")}, found,
		"a symlinked stack file must keep the directory it lives in, not its target's")
}

// TestListStackFilesDeduplicatesSymlinkedDirectory pins that a stack file reachable both
// directly and through a symlinked directory yields one entry spelled as the real path. Keeping
// the stack file anchored to its own directory must not weaken the directory canonicalisation
// that makes two spellings of one location collapse into a single topology key.
func TestListStackFilesDeduplicatesSymlinkedDirectory(t *testing.T) {
	t.Parallel()

	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	realDir := filepath.Join(base, "real")
	require.NoError(t, os.MkdirAll(realDir, 0o755))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(realDir, "terragrunt.stack.hcl"), []byte("# stack\n"), 0o644),
	)

	// A sibling symlink offering a second spelling of realDir. Terragrunt runs at base, so the
	// walk sees both "real" and "link" but must not report the stack file twice.
	if err := os.Symlink(realDir, filepath.Join(base, "link")); err != nil {
		if helpers.SymlinkUnsupported(err) {
			t.Skipf("host does not support creating symlinks: %v", err)
		}

		require.NoError(t, err)
	}

	found, _, err := generate.ListStackFilesWithExcludes(
		t.Context(),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv(),
		stackFileFixtureOpts(base),
		nil,
	)
	require.NoError(t, err)

	assert.Equal(t, []string{filepath.Join(realDir, "terragrunt.stack.hcl")}, found,
		"a symlinked directory must not add a second spelling of the same stack file")
}
