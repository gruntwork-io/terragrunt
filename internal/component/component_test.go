package component_test

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentsSort(t *testing.T) {
	t.Parallel()

	// Setup
	configs := component.Components{
		component.NewUnit("c"),
		component.NewUnit("a"),
		component.NewStack("b"),
	}

	// Act
	sorted := configs.Sort()

	// Assert
	require.Len(t, sorted, 3)
	assert.Equal(t, "a", sorted[0].Path())
	assert.Equal(t, "b", sorted[1].Path())
	assert.Equal(t, "c", sorted[2].Path())
}

func TestComponentsFilter(t *testing.T) {
	t.Parallel()

	// Setup
	configs := component.Components{
		component.NewUnit("unit1"),
		component.NewStack("stack1"),
		component.NewUnit("unit2"),
	}

	// Test unit filtering
	t.Run("filter units", func(t *testing.T) {
		t.Parallel()

		units := configs.Filter(component.UnitKind)
		require.Len(t, units, 2)
		assert.Equal(t, component.UnitKind, units[0].Kind())
		assert.Equal(t, component.UnitKind, units[1].Kind())
		assert.ElementsMatch(t, []string{"unit1", "unit2"}, units.Paths())
	})

	// Test stack filtering
	t.Run("filter stacks", func(t *testing.T) {
		t.Parallel()

		stacks := configs.Filter(component.StackKind)
		require.Len(t, stacks, 1)
		assert.Equal(t, component.StackKind, stacks[0].Kind())
		assert.Equal(t, "stack1", stacks[0].Path())
	})
}

func TestComponentsCycleCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupFunc     func() component.Components
		name          string
		errorExpected bool
	}{
		{
			name: "no cycles",
			setupFunc: func() component.Components {
				a := component.NewUnit("a")
				b := component.NewUnit("b")
				a.AddDependency(b)

				return component.Components{a, b}
			},
			errorExpected: false,
		},
		{
			name: "direct cycle",
			setupFunc: func() component.Components {
				a := component.NewUnit("a")
				b := component.NewUnit("b")
				a.AddDependency(b)
				b.AddDependency(a)

				return component.Components{a, b}
			},
			errorExpected: true,
		},
		{
			name: "indirect cycle",
			setupFunc: func() component.Components {
				a := component.NewUnit("a")
				b := component.NewUnit("b")
				c := component.NewUnit("c")

				a.AddDependency(b)
				b.AddDependency(c)
				c.AddDependency(a)

				return component.Components{a, b, c}
			},
			errorExpected: true,
		},
		{
			name: "diamond dependency - no cycle",
			setupFunc: func() component.Components {
				a := component.NewUnit("a")
				b := component.NewUnit("b")
				c := component.NewUnit("c")
				d := component.NewUnit("d")

				a.AddDependency(b)
				a.AddDependency(c)
				b.AddDependency(d)
				c.AddDependency(d)

				return component.Components{a, b, c, d}
			},
			errorExpected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			configs := tt.setupFunc()

			cfg, err := configs.CycleCheck()
			if tt.errorExpected {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cycle detected")
				assert.NotNil(t, cfg)
			} else {
				require.NoError(t, err)
				assert.Nil(t, cfg)
			}
		})
	}
}

func TestUnitStringConcurrent(t *testing.T) {
	t.Parallel()

	unit := component.NewUnit("/test/path")
	dep := component.NewUnit("/test/dep")
	unit.AddDependency(dep)

	var wg sync.WaitGroup

	const goroutines = 10

	for range goroutines {
		wg.Go(func() {
			for range 100 {
				s := unit.String()
				assert.Contains(t, s, "/test/path")
			}
		})
	}

	wg.Wait()
}

func TestThreadSafeComponentsEnsureNoDuplicates(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	tsc := component.NewThreadSafeComponents(fsys, component.Components{})

	// Add same path twice - should not duplicate
	unit1 := component.NewUnit("/test/path")
	unit2 := component.NewUnit("/test/path")

	added1, wasAdded1 := tsc.EnsureComponent(fsys, unit1)
	added2, wasAdded2 := tsc.EnsureComponent(fsys, unit2)

	assert.True(t, wasAdded1, "first component should be added")
	assert.False(t, wasAdded2, "second component should not be added (duplicate)")
	assert.Same(t, added1, added2, "should return same component instance")
	assert.Equal(t, 1, tsc.Len(), "should have exactly one component")
}

func TestThreadSafeComponentsFindByPath(t *testing.T) {
	t.Parallel()

	unit := component.NewUnit("/test/path")
	fsys := vfs.NewMemMapFS()
	tsc := component.NewThreadSafeComponents(fsys, component.Components{unit})

	// Find by exact path
	found := tsc.FindByPath(fsys, "/test/path")
	assert.NotNil(t, found, "should find component by exact path")
	assert.Equal(t, "/test/path", found.Path())

	// Find non-existent path
	notFound := tsc.FindByPath(fsys, "/nonexistent")
	assert.Nil(t, notFound, "should not find non-existent path")
}

func TestThreadSafeComponentsConcurrentAccess(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	tsc := component.NewThreadSafeComponents(fsys, component.Components{})

	var wg sync.WaitGroup

	const goroutines = 10

	// Concurrent writes tests
	for range goroutines {
		wg.Go(func() {
			unit := component.NewUnit("/test/path")
			tsc.EnsureComponent(fsys, unit)
		})
	}

	// Concurrent reads
	for range goroutines {
		wg.Go(func() {
			for range 100 {
				_ = tsc.FindByPath(fsys, "/test/path")
				_ = tsc.Len()
				_ = tsc.ToComponents()
			}
		})
	}

	wg.Wait()

	// Should have exactly one component despite concurrent adds
	assert.Equal(t, 1, tsc.Len(), "should have exactly one component after concurrent adds")
}

// TestUnitGuardConfigParseWithRacing verifies the parse runs exactly once when
// many goroutines race to parse the same unit.
func TestUnitGuardConfigParseWithRacing(t *testing.T) {
	t.Parallel()

	unit := component.NewUnit("/test/unit")

	var parses atomic.Int32

	var wg sync.WaitGroup

	const goroutines = 32

	for range goroutines {
		wg.Go(func() {
			err := unit.GuardConfigParse(func() error {
				parses.Add(1)
				unit.StoreConfig(&config.TerragruntConfig{})

				return nil
			})
			assert.NoError(t, err)
		})
	}

	wg.Wait()

	assert.Equal(t, int32(1), parses.Load(), "config should be parsed exactly once")
	assert.NotNil(t, unit.Config(), "config should be populated after parsing")
}

// TestUnitOutputFile checks that the same unit is mirrored under the output folder at the
// same path whether it was found on the filesystem or in a Git worktree, and that a unit
// found outside the working directory is still mirrored inside the output folder.
func TestUnitOutputFile(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	workingDir := filepath.Join(base, "repo", "live")
	worktreeRoot := filepath.Join(base, "worktree")
	outputFolder := filepath.Join(base, "plans")

	testCases := []struct {
		discoveryContext *component.DiscoveryContext
		name             string
		unitPath         string
		expected         string
	}{
		{
			name:     "filesystem discovery",
			unitPath: filepath.Join(workingDir, "unit2"),
			discoveryContext: &component.DiscoveryContext{
				WorkingDir: workingDir,
			},
			expected: filepath.Join(outputFolder, "unit2", "tfplan.tfplan"),
		},
		{
			name:     "worktree discovery",
			unitPath: filepath.Join(worktreeRoot, "live", "unit2"),
			discoveryContext: &component.DiscoveryContext{
				WorkingDir:    worktreeRoot,
				OutputKeyBase: filepath.Join(worktreeRoot, "live"),
			},
			expected: filepath.Join(outputFolder, "unit2", "tfplan.tfplan"),
		},
		{
			name:     "worktree discovery outside the working directory",
			unitPath: filepath.Join(worktreeRoot, "other", "unit3"),
			discoveryContext: &component.DiscoveryContext{
				WorkingDir:    worktreeRoot,
				OutputKeyBase: filepath.Join(worktreeRoot, "live"),
			},
			expected: filepath.Join(outputFolder, "other", "unit3", "tfplan.tfplan"),
		},
		{
			name:     "worktree discovery without a mapped working directory",
			unitPath: filepath.Join(worktreeRoot, "live", "unit2"),
			discoveryContext: &component.DiscoveryContext{
				WorkingDir: worktreeRoot,
			},
			expected: filepath.Join(outputFolder, "live", "unit2", "tfplan.tfplan"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			unit := component.NewUnit(tc.unitPath).WithDiscoveryContext(tc.discoveryContext)

			assert.Equal(t, tc.expected, unit.OutputFile(workingDir, outputFolder))
			assert.Equal(
				t,
				filepath.Join(filepath.Dir(tc.expected), "tfplan.json"),
				unit.OutputJSONFile(workingDir, outputFolder),
			)
		})
	}
}
