package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCandidacyClassifier_Analyze(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		filterStrings          []string
		expectHasPositive      bool
		expectHasParseRequired bool
		expectHasGraphFilters  bool
		expectGraphExprCount   int
	}{
		{
			name:              "empty filters",
			filterStrings:     []string{},
			expectHasPositive: false,
		},
		{
			name:              "simple path filter",
			filterStrings:     []string{"./foo"},
			expectHasPositive: true,
		},
		{
			name:              "negated path filter only",
			filterStrings:     []string{"!./foo"},
			expectHasPositive: false,
		},
		{
			name:              "path filter with negation",
			filterStrings:     []string{"./foo", "!./bar"},
			expectHasPositive: true,
		},
		{
			name:                   "reading attribute filter",
			filterStrings:          []string{"reading=config/*"},
			expectHasPositive:      true,
			expectHasParseRequired: true,
		},
		{
			name:                  "dependency graph filter",
			filterStrings:         []string{"./foo..."},
			expectHasPositive:     true,
			expectHasGraphFilters: true,
			expectGraphExprCount:  1,
		},
		{
			name:                  "dependent graph filter",
			filterStrings:         []string{"..../foo"},
			expectHasPositive:     true,
			expectHasGraphFilters: true,
			expectGraphExprCount:  1,
		},
		{
			name:                  "exclude target graph filter",
			filterStrings:         []string{"^{./foo}..."},
			expectHasPositive:     true,
			expectHasGraphFilters: true,
			expectGraphExprCount:  1,
		},
		{
			name:                  "multiple graph filters",
			filterStrings:         []string{"./foo...", "..../bar"},
			expectHasPositive:     true,
			expectHasGraphFilters: true,
			expectGraphExprCount:  2,
		},
		{
			name:              "name attribute filter",
			filterStrings:     []string{"name=my-app"},
			expectHasPositive: true,
		},
		{
			name:              "type attribute filter",
			filterStrings:     []string{"type=unit"},
			expectHasPositive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			filters, err := filter.ParseFilterQueries(l, tt.filterStrings)
			require.NoError(t, err)

			classifier := filter.NewClassifier(filters)

			assert.Equal(
				t,
				tt.expectHasPositive,
				classifier.HasPositiveFilters(),
				"HasPositiveFilters mismatch",
			)
			assert.Equal(
				t,
				tt.expectHasParseRequired,
				classifier.HasParseRequiredFilters(),
				"HasParseRequiredFilters mismatch",
			)
			assert.Equal(
				t,
				tt.expectHasGraphFilters,
				classifier.HasGraphFilters(),
				"HasGraphFilters mismatch",
			)

			if tt.expectGraphExprCount > 0 {
				assert.Len(
					t,
					classifier.GraphExpressions(),
					tt.expectGraphExprCount,
					"GraphExpressions count mismatch",
				)
			}
		})
	}
}

func TestCandidacyClassifier_ClassifyComponent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		componentPath string
		workingDir    string
		filterStrings []string
		expectStatus  filter.ClassificationStatus
		expectReason  filter.CandidacyReason
		expectIndex   int
	}{
		{
			name:          "no filters - include by default",
			filterStrings: []string{},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusReadyForFilter,
			expectReason:  filter.CandidacyReasonNone,
			expectIndex:   -1,
		},
		{
			name:          "matching path filter",
			filterStrings: []string{"./foo"},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusReadyForFilter,
			expectReason:  filter.CandidacyReasonNone,
			expectIndex:   -1,
		},
		{
			name:          "non-matching path filter - exclude by default",
			filterStrings: []string{"./bar"},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusExcluded,
			expectReason:  filter.CandidacyReasonNone,
			expectIndex:   -1,
		},
		{
			name:          "negated filter only - exclude component",
			filterStrings: []string{"!./foo"},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusExcluded,
			expectReason:  filter.CandidacyReasonNone,
			expectIndex:   -1,
		},
		{
			name:          "negated filter only - include other",
			filterStrings: []string{"!./foo"},
			componentPath: "/project/bar",
			workingDir:    "/project",
			expectStatus:  filter.StatusReadyForFilter,
			expectReason:  filter.CandidacyReasonNone,
			expectIndex:   -1,
		},
		{
			name:          "graph expression target - candidate",
			filterStrings: []string{"./foo..."},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusCandidate,
			expectReason:  filter.CandidacyReasonGraphTarget,
			expectIndex:   0,
		},
		{
			name:          "parse required filter - candidate",
			filterStrings: []string{"reading=config/*"},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusCandidate,
			expectReason:  filter.CandidacyReasonRequiresParse,
			expectIndex:   -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			filters, err := filter.ParseFilterQueries(l, tt.filterStrings)
			require.NoError(t, err)

			classifier := filter.NewClassifier(filters)

			// Create a test component
			c := component.NewUnit(tt.componentPath)
			c.SetDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: tt.workingDir,
			})

			ctx := filter.ClassificationContext{}
			status, reason, index := classifier.Classify(c, ctx)

			assert.Equal(t, tt.expectStatus, status, "status mismatch")
			assert.Equal(t, tt.expectReason, reason, "reason mismatch")
			assert.Equal(t, tt.expectIndex, index, "index mismatch")
		})
	}
}

func TestDiscovery_SimpleFilesystem(t *testing.T) {
	t.Parallel()

	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Create some terragrunt.hcl files
	dirs := []string{"foo", "bar", "baz"}
	for _, dir := range dirs {
		dirPath := filepath.Join(tmpDir, dir)
		require.NoError(t, os.MkdirAll(dirPath, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dirPath, "terragrunt.hcl"),
			[]byte("# Test config\n"),
			0644,
		))
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Test: discover all components
	d := discovery.NewDiscovery(tmpDir).WithDiscoveryContext(&component.DiscoveryContext{
		WorkingDir: tmpDir,
	})

	components, err := d.Discover(ctx, l, venv.OSVenv(), opts)
	require.NoError(t, err)
	assert.Len(t, components, 3, "should discover 3 components")
}

func TestDiscovery_WithPathFilter(t *testing.T) {
	t.Parallel()

	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Create some terragrunt.hcl files
	dirs := []string{"apps/foo", "apps/bar", "infra/baz"}
	for _, dir := range dirs {
		dirPath := filepath.Join(tmpDir, dir)
		require.NoError(t, os.MkdirAll(dirPath, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dirPath, "terragrunt.hcl"),
			[]byte("# Test config\n"),
			0644,
		))
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Test: filter to apps/* only
	filters, err := filter.ParseFilterQueries(l, []string{"./apps/*"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
		}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, venv.OSVenv(), opts)
	require.NoError(t, err)
	assert.Len(t, components, 2, "should discover 2 components in apps/")
}

func TestDiscovery_WithNegatedFilter(t *testing.T) {
	t.Parallel()

	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Create some terragrunt.hcl files
	dirs := []string{"foo", "bar", "baz"}
	for _, dir := range dirs {
		dirPath := filepath.Join(tmpDir, dir)
		require.NoError(t, os.MkdirAll(dirPath, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dirPath, "terragrunt.hcl"),
			[]byte("# Test config\n"),
			0644,
		))
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Test: exclude ./bar
	filters, err := filter.ParseFilterQueries(l, []string{"!./bar"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
		}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, venv.OSVenv(), opts)
	require.NoError(t, err)
	assert.Len(t, components, 2, "should discover 2 components (excluding bar)")

	// Verify bar is not in results
	for _, c := range components {
		assert.NotContains(t, c.Path(), "bar", "bar should be excluded")
	}
}

func TestDiscovery_CombinedFilters(t *testing.T) {
	t.Parallel()

	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Create some terragrunt.hcl files
	dirs := []string{"apps/foo", "apps/bar", "apps/baz", "infra/db"}
	for _, dir := range dirs {
		dirPath := filepath.Join(tmpDir, dir)
		require.NoError(t, os.MkdirAll(dirPath, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dirPath, "terragrunt.hcl"),
			[]byte("# Test config\n"),
			0644,
		))
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Test: ./apps/* but not ./apps/baz
	filters, err := filter.ParseFilterQueries(l, []string{"./apps/*", "!./apps/baz"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
		}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, venv.OSVenv(), opts)
	require.NoError(t, err)
	assert.Len(t, components, 2, "should discover 2 components (apps/* minus baz)")

	// Verify baz is not in results
	for _, c := range components {
		assert.NotContains(t, c.Path(), "baz", "baz should be excluded")
	}
}

func TestPhaseKind_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected string
		kind     discovery.PhaseKind
	}{
		{expected: "filesystem", kind: discovery.PhaseFilesystem},
		{expected: "worktree", kind: discovery.PhaseWorktree},
		{expected: "parse", kind: discovery.PhaseParse},
		{expected: "graph", kind: discovery.PhaseGraph},
		{expected: "relationship", kind: discovery.PhaseRelationship},
		{expected: "final", kind: discovery.PhaseFinal},
		{expected: "unknown", kind: discovery.PhaseKind(999)},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.kind.String())
		})
	}
}

func TestDiscoveryStatus_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected string
		status   filter.ClassificationStatus
	}{
		{expected: "discovered", status: filter.StatusReadyForFilter},
		{expected: "candidate", status: filter.StatusCandidate},
		{expected: "excluded", status: filter.StatusExcluded},
		{expected: "unknown", status: filter.ClassificationStatus(999)},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestCandidacyReason_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected string
		reason   filter.CandidacyReason
	}{
		{expected: "none", reason: filter.CandidacyReasonNone},
		{expected: "graph-target", reason: filter.CandidacyReasonGraphTarget},
		{expected: "requires-parse", reason: filter.CandidacyReasonRequiresParse},
		{expected: "unknown", reason: filter.CandidacyReason(999)},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.reason.String())
		})
	}
}

// TestDiscovery_PopulatesReadingField verifies that the Reading field is populated
// with files read during parsing via read_terragrunt_config() and read_tfvars_file().
func TestDiscovery_PopulatesReadingField(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	appDir := filepath.Join(tmpDir, "app")
	require.NoError(t, os.MkdirAll(appDir, 0755))

	// Create shared files that will be read
	sharedHCL := filepath.Join(tmpDir, "shared.hcl")
	sharedTFVars := filepath.Join(tmpDir, "shared.tfvars")

	require.NoError(t, os.WriteFile(sharedHCL, []byte(`
		locals {
			common_value = "test"
		}
	`), 0644))

	require.NoError(t, os.WriteFile(sharedTFVars, []byte(`
		test_var = "value"
	`), 0644))

	// Create terragrunt config that reads both files
	terragruntConfig := filepath.Join(appDir, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(terragruntConfig, []byte(`
		locals {
			shared_config = read_terragrunt_config("../shared.hcl")
			tfvars = read_tfvars_file("../shared.tfvars")
		}
	`), 0644))

	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	l := logger.CreateLogger()
	ctx := t.Context()

	// Discover components with ReadFiles enabled to populate Reading field
	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithReadFiles()

	components, err := d.Discover(ctx, l, venv.OSVenv(), opts)
	require.NoError(t, err)

	// Find the app component
	var appComponent *component.Unit

	for _, c := range components {
		if c.Path() == appDir {
			if unit, ok := c.(*component.Unit); ok {
				appComponent = unit
			}

			break
		}
	}

	require.NotNil(t, appComponent, "app component should be discovered")
	require.NotNil(t, appComponent.Reading(), "Reading field should be initialized")

	// Verify Reading field contains the files that were read
	require.NotEmpty(t, appComponent.Reading(), "should have read files")
	assert.Contains(t, appComponent.Reading(), sharedHCL, "should contain shared.hcl")
	assert.Contains(t, appComponent.Reading(), sharedTFVars, "should contain shared.tfvars")
}

func TestDiscovery_BothHclAndStackFileInSameDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "app")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	require.NoError(t, os.WriteFile(
		filepath.Join(subDir, "terragrunt.hcl"),
		[]byte("# empty unit config\n"),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(subDir, "terragrunt.stack.hcl"),
		[]byte("# empty stack config\n"),
		0644,
	))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir})

	_, err := d.Discover(t.Context(), l, venv.OSVenv(), opts)
	require.Error(t, err)

	var coexistErr discovery.CoexistenceError
	require.ErrorAs(t, err, &coexistErr)
	assert.Equal(t, subDir, coexistErr.ComponentPath)
}

// TestDiscovery_SingleUnitNoDuplicateError verifies that a directory with only
// a single config file does not trigger a coexistence error.
func TestDiscovery_SingleUnitNoDuplicateError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "app")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	require.NoError(t, os.WriteFile(
		filepath.Join(subDir, "terragrunt.hcl"),
		[]byte("# config\n"),
		0644,
	))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir})

	components, err := d.Discover(t.Context(), l, venv.OSVenv(), opts)
	require.NoError(t, err)
	assert.Len(t, components, 1)
	assert.Equal(t, component.UnitKind, components[0].Kind())
}

// TestDiscovery_ParsesStackConfigs verifies that WithParseStackConfigs stores
// parsed stack configs on discovered stack components, and that parse failures
// are skipped without failing discovery.
func TestDiscovery_ParsesStackConfigs(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	stackDir := filepath.Join(tmpDir, "stack")
	brokenDir := filepath.Join(tmpDir, "broken")

	require.NoError(t, os.MkdirAll(stackDir, 0755))
	require.NoError(t, os.MkdirAll(brokenDir, 0755))

	require.NoError(t, os.WriteFile(
		filepath.Join(stackDir, "terragrunt.stack.hcl"),
		[]byte(`
unit "app" {
	source = "../units/app"
	path   = "app"
}
`),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(brokenDir, "terragrunt.stack.hcl"),
		[]byte("unit {\n"),
		0644,
	))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithParseStackConfigs()

	components, err := d.Discover(t.Context(), l, venv.OSVenv(), opts)
	require.NoError(t, err)

	stacks := make(map[string]*component.Stack)

	for _, c := range components {
		if s, ok := c.(*component.Stack); ok {
			stacks[s.Path()] = s
		}
	}

	parsed := stacks[stackDir]
	require.NotNil(t, parsed, "stack component should be discovered")
	require.NotNil(t, parsed.Config(), "stack config should be parsed and stored")
	require.Len(t, parsed.Config().Units, 1)
	assert.Equal(t, "app", parsed.Config().Units[0].Name)

	broken := stacks[brokenDir]
	require.NotNil(t, broken, "broken stack component should still be discovered")
	assert.Nil(t, broken.Config(), "unparsable stack config should be skipped")
}

// TestDiscovery_BasicWithHiddenDirectories tests discovery with and without hidden directories.
func TestDiscovery_BasicWithHiddenDirectories(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create test directory structure
	unit1Dir := filepath.Join(tmpDir, "unit1")
	unit2Dir := filepath.Join(tmpDir, "unit2")
	stack1Dir := filepath.Join(tmpDir, "stack1")
	hiddenUnitDir := filepath.Join(tmpDir, ".hidden", "hidden-unit")
	nestedUnit4Dir := filepath.Join(tmpDir, "nested", "unit4")

	v := memGitTopLevelVenv(t, tmpDir)

	writeUnits(t, v.FS, map[string]string{
		unit1Dir:       "",
		unit2Dir:       "",
		hiddenUnitDir:  "",
		nestedUnit4Dir: "",
	})

	require.NoError(t, vfs.WriteFile(
		v.FS,
		filepath.Join(stack1Dir, "terragrunt.stack.hcl"),
		[]byte(""),
		0o644,
	))

	tests := []struct {
		name       string
		wantUnits  []string
		wantStacks []string
		noHidden   bool
	}{
		{
			name:       "discovery without hidden",
			noHidden:   true,
			wantUnits:  []string{unit1Dir, unit2Dir, nestedUnit4Dir},
			wantStacks: []string{stack1Dir},
		},
		{
			name:       "discovery with hidden",
			noHidden:   false,
			wantUnits:  []string{unit1Dir, unit2Dir, hiddenUnitDir, nestedUnit4Dir},
			wantStacks: []string{stack1Dir},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			opts := &options.TerragruntOptions{
				WorkingDir: tmpDir,
			}

			ctx := t.Context()

			d := discovery.NewDiscovery(tmpDir).WithDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: tmpDir,
			})

			if tt.noHidden {
				d = d.WithNoHidden()
			}

			components, err := d.Discover(ctx, l, v, opts)
			require.NoError(t, err)

			units := components.Filter(component.UnitKind).Paths()
			stacks := components.Filter(component.StackKind).Paths()

			assert.ElementsMatch(t, tt.wantUnits, units)
			assert.ElementsMatch(t, tt.wantStacks, stacks)
		})
	}
}

// TestDiscovery_StackHiddenDiscovered tests that .terragrunt-stack directories are discovered by default.
func TestDiscovery_StackHiddenDiscovered(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	stackHiddenDir := filepath.Join(tmpDir, ".terragrunt-stack", "u")

	v := memGitTopLevelVenv(t, tmpDir)

	writeUnits(t, v.FS, map[string]string{
		stackHiddenDir: "",
	})

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	// By default, .terragrunt-stack contents should be discovered
	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir})

	components, err := d.Discover(ctx, l, v, opts)
	require.NoError(t, err)
	assert.Contains(t, components.Filter(component.UnitKind).Paths(), stackHiddenDir)
}

// TestDiscovery_WithDependencies tests dependency discovery and relationship building.
func TestDiscovery_WithDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	internalDir := filepath.Join(tmpDir, "internal")
	appDir := filepath.Join(internalDir, "app")
	dbDir := filepath.Join(internalDir, "db")
	vpcDir := filepath.Join(internalDir, "vpc")

	externalDir := filepath.Join(tmpDir, "external")
	externalAppDir := filepath.Join(externalDir, "app")

	v := memGitTopLevelVenv(t, tmpDir)

	writeUnits(t, v.FS, map[string]string{
		appDir: `
		dependency "db" {
			config_path = "../db"
		}

		dependency "external" {
			config_path = "../../external/app"
		}
		`,
		dbDir: `
		dependency "vpc" {
			config_path = "../vpc"
		}
		`,
		vpcDir:         ``,
		externalAppDir: ``,
	})

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     internalDir,
		RootWorkingDir: internalDir,
	}

	ctx := t.Context()

	t.Run("discovery with relationships", func(t *testing.T) {
		t.Parallel()

		d := discovery.NewDiscovery(internalDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: internalDir}).
			WithRelationships()

		components, err := d.Discover(ctx, l, v, opts)
		require.NoError(t, err)

		// Should discover all internal components
		paths := components.Paths()
		assert.Contains(t, paths, appDir)
		assert.Contains(t, paths, dbDir)
		assert.Contains(t, paths, vpcDir)

		// Find app component and verify dependencies
		var appComponent component.Component

		for _, c := range components {
			if c.Path() == appDir {
				appComponent = c
				break
			}
		}

		require.NotNil(t, appComponent, "app component should be discovered")
		depPaths := appComponent.Dependencies().Paths()
		assert.Contains(t, depPaths, dbDir, "app should depend on db")
		assert.Contains(t, depPaths, externalAppDir, "app should depend on external app")

		// Verify db's dependencies
		var dbComponent component.Component

		for _, c := range components {
			if c.Path() == dbDir {
				dbComponent = c
				break
			}
		}

		require.NotNil(t, dbComponent)
		assert.Contains(t, dbComponent.Dependencies().Paths(), vpcDir, "db should depend on vpc")
	})

	t.Run("discovery with dependency graph filter", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
		require.NoError(t, err)

		d := discovery.NewDiscovery(internalDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: internalDir}).
			WithFilters(filters)

		components, err := d.Discover(ctx, l, v, opts)
		require.NoError(t, err)

		// Should discover all components including external dependency
		paths := components.Paths()
		assert.Contains(t, paths, appDir)
		assert.Contains(t, paths, dbDir)
		assert.Contains(t, paths, vpcDir)
		assert.Contains(t, paths, externalAppDir)

		// Find external app and verify it's marked as external
		for _, c := range components {
			if c.Path() == externalAppDir {
				assert.True(t, c.External(), "external app should be marked as external")
				break
			}
		}
	})
}

// TestDiscovery_CycleDetection tests that cycles in dependency graphs are detected.
func TestDiscovery_CycleDetection(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	fooDir := filepath.Join(tmpDir, "foo")
	barDir := filepath.Join(tmpDir, "bar")

	v := memGitTopLevelVenv(t, tmpDir)

	// Create terragrunt.hcl files with mutual dependencies (cycle)
	writeUnits(t, v.FS, map[string]string{
		fooDir: `
dependency "bar" {
	config_path = "../bar"
}
`,
		barDir: `
dependency "foo" {
	config_path = "../foo"
}
`,
	})

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, v, opts)
	require.NoError(t, err, "Discovery should complete even with cycles")

	// Verify that a cycle is detected
	cycleComponent, cycleErr := components.CycleCheck()
	require.Error(t, cycleErr, "Cycle check should detect a cycle between foo and bar")
	assert.Contains(t, cycleErr.Error(), "cycle detected", "Error message should mention cycle")
	assert.NotNil(
		t,
		cycleComponent,
		"Cycle check should return the component that is part of the cycle",
	)

	// Verify both foo and bar are in the discovered components
	componentPaths := components.Paths()
	assert.Contains(t, componentPaths, fooDir, "Foo should be discovered")
	assert.Contains(t, componentPaths, barDir, "Bar should be discovered")
}

// TestDiscovery_CycleDetectionWithDisabledDependency tests that disabled dependencies don't create cycles.
func TestDiscovery_CycleDetectionWithDisabledDependency(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	fooDir := filepath.Join(tmpDir, "foo")
	barDir := filepath.Join(tmpDir, "bar")

	v := memGitTopLevelVenv(t, tmpDir)

	// Create terragrunt.hcl files where one dependency is disabled
	writeUnits(t, v.FS, map[string]string{
		fooDir: `
dependency "bar" {
	config_path = "../bar"
	enabled = false
}
`,
		barDir: `
dependency "foo" {
	config_path = "../foo"
}
`,
	})

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, v, opts)
	require.NoError(t, err, "Discovery should complete")

	// Verify that a cycle is NOT detected because one dependency is disabled
	_, cycleErr := components.CycleCheck()
	require.NoError(
		t,
		cycleErr,
		"Cycle check should not detect a cycle when dependency is disabled",
	)

	// Verify both foo and bar are in the discovered components
	componentPaths := components.Paths()
	assert.Contains(t, componentPaths, fooDir, "Foo should be discovered")
	assert.Contains(t, componentPaths, barDir, "Bar should be discovered")
}

// TestDiscovery_WithParseExclude tests that WithParseExclude enables parsing of exclude blocks
// and that the exclude configurations are accessible on the discovered units.
func TestDiscovery_WithParseExclude(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	v := memGitTopLevelVenv(t, tmpDir)

	// Create test files with exclude configurations
	writeUnits(t, v.FS, map[string]string{
		filepath.Join(tmpDir, "unit1"): `
exclude {
  if      = true
  actions = ["plan"]
}`,
		filepath.Join(tmpDir, "unit2"): `
exclude {
  if      = true
  actions = ["apply"]
}`,
		filepath.Join(tmpDir, "unit3"): "",
	})

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	// WithParseExclude sets requiresParse=true which triggers the parse phase,
	// allowing exclude blocks to be parsed and accessible on the units.
	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithParseExclude()

	components, err := d.Discover(ctx, l, v, opts)
	require.NoError(t, err)

	// Verify we found all configurations
	assert.Len(t, components, 3)

	// Helper to find unit by path
	findUnit := func(path string) *component.Unit {
		for _, c := range components {
			if filepath.Base(c.Path()) == path {
				if unit, ok := c.(*component.Unit); ok {
					return unit
				}
			}
		}

		return nil
	}

	// Verify exclude configurations were parsed correctly
	unit1 := findUnit("unit1")
	require.NotNil(t, unit1)
	require.NotNil(t, unit1.Config(), "unit1 should have a parsed config")
	require.NotNil(t, unit1.Config().Exclude, "unit1 should have an exclude block")
	assert.Contains(
		t,
		unit1.Config().Exclude.Actions,
		"plan",
		"unit1 exclude should contain 'plan' action",
	)

	unit2 := findUnit("unit2")
	require.NotNil(t, unit2)
	require.NotNil(t, unit2.Config(), "unit2 should have a parsed config")
	require.NotNil(t, unit2.Config().Exclude, "unit2 should have an exclude block")
	assert.Contains(
		t,
		unit2.Config().Exclude.Actions,
		"apply",
		"unit2 exclude should contain 'apply' action",
	)

	unit3 := findUnit("unit3")
	require.NotNil(t, unit3)
	// unit3 has an empty config, so Config() may be nil or Exclude may be nil
	if unit3.Config() != nil {
		assert.Nil(t, unit3.Config().Exclude, "unit3 should not have an exclude block")
	}
}

// TestDiscovery_WithCustomConfigFilenames tests discovery with custom config filenames.
func TestDiscovery_WithCustomConfigFilenames(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create units with custom config filenames
	unit1Dir := filepath.Join(tmpDir, "unit1")
	unit2Dir := filepath.Join(tmpDir, "unit2")

	v := memGitTopLevelVenv(t, tmpDir)

	// Standard terragrunt.hcl in unit1
	writeUnits(t, v.FS, map[string]string{
		unit1Dir: "",
	})

	// Custom config in unit2
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(unit2Dir, "custom.hcl"), []byte(""), 0o644))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	t.Run("discover only custom config filename", func(t *testing.T) {
		t.Parallel()

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithConfigFilenames([]string{"custom.hcl"})

		components, err := d.Discover(ctx, l, v, opts)
		require.NoError(t, err)

		units := components.Filter(component.UnitKind).Paths()
		assert.Len(t, units, 1)
		assert.ElementsMatch(t, []string{unit2Dir}, units)
	})

	t.Run("discover both standard and custom config filenames", func(t *testing.T) {
		t.Parallel()

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithConfigFilenames([]string{"terragrunt.hcl", "custom.hcl"})

		components, err := d.Discover(ctx, l, v, opts)
		require.NoError(t, err)

		units := components.Filter(component.UnitKind).Paths()
		assert.Len(t, units, 2)
		assert.ElementsMatch(t, []string{unit1Dir, unit2Dir}, units)
	})
}

// TestDiscovery_WithStackConfigParsing tests that stack files are discovered but not parsed as unit configs.
func TestDiscovery_WithStackConfigParsing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	stackDir := filepath.Join(tmpDir, "stack")
	unitDir := filepath.Join(tmpDir, "unit")

	v := memGitTopLevelVenv(t, tmpDir)

	// Create a stack file with unit blocks
	stackContent := `
unit "unit_a" {
  source = "${get_repo_root()}/unit_a"
  path   = "unit_a"
}

unit "unit_b" {
  source = "${get_repo_root()}/unit_b"
  path   = "unit_b"
}
`

	// Create a unit file with valid unit configuration
	unitContent := `
terraform {
  source = "."
}

inputs = {
  test = "value"
}
`

	writeUnits(t, v.FS, map[string]string{
		unitDir: unitContent,
	})

	require.NoError(t, vfs.WriteFile(
		v.FS,
		filepath.Join(stackDir, "terragrunt.stack.hcl"),
		[]byte(stackContent),
		0o644,
	))

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, v, opts)
	require.NoError(t, err)

	// Verify that both stack and unit configurations are discovered
	units := components.Filter(component.UnitKind)
	stacks := components.Filter(component.StackKind)

	assert.Len(t, units, 1)
	assert.Len(t, stacks, 1)

	// Verify that stack configuration is not parsed (Config should be nil)
	stackComp := stacks[0]
	stack, ok := stackComp.(*component.Stack)
	require.True(t, ok, "should be a Stack")
	assert.Nil(t, stack.Config(), "Stack configuration should not be parsed")

	// Verify that unit configuration is parsed (Config should not be nil)
	unitComp := units[0]
	unit, ok := unitComp.(*component.Unit)
	require.True(t, ok, "should be a Unit")
	assert.NotNil(t, unit.Config(), "Unit configuration should be parsed")
}

// TestDiscovery_IncludeExcludeFilterSemantics tests include/exclude filter behavior.
func TestDiscovery_IncludeExcludeFilterSemantics(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	unit1Dir := filepath.Join(tmpDir, "unit1")
	unit2Dir := filepath.Join(tmpDir, "unit2")
	unit3Dir := filepath.Join(tmpDir, "unit3")

	v := memGitTopLevelVenv(t, tmpDir)

	writeUnits(t, v.FS, map[string]string{
		unit1Dir: "",
		unit2Dir: "",
		unit3Dir: "",
	})

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	tests := []struct {
		name    string
		filters []string
		want    []string
	}{
		{
			name:    "include by default (no filters)",
			filters: []string{},
			want:    []string{unit1Dir, unit2Dir, unit3Dir},
		},
		{
			name:    "exclude by default when positive filter",
			filters: []string{"unit1"},
			want:    []string{unit1Dir},
		},
		{
			name:    "include by default with only negative filter",
			filters: []string{"!unit2"},
			want:    []string{unit1Dir, unit3Dir},
		},
		{
			name:    "exclude by default with positive and negative filters",
			filters: []string{"unit1", "!unit2"},
			want:    []string{unit1Dir},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(l, tt.filters)
			require.NoError(t, err)

			d := discovery.NewDiscovery(tmpDir).
				WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
				WithFilters(filters)

			components, err := d.Discover(ctx, l, v, opts)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, components.Filter(component.UnitKind).Paths())
		})
	}
}

// TestDiscovery_HiddenIncludedByIncludeDirs tests hidden directories are included when explicitly filtered.
func TestDiscovery_HiddenIncludedByIncludeDirs(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	hiddenUnitDir := filepath.Join(tmpDir, ".hidden", "hunit")

	v := memGitTopLevelVenv(t, tmpDir)

	writeUnits(t, v.FS, map[string]string{
		hiddenUnitDir: "",
	})

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	filters, err := filter.ParseFilterQueries(l, []string{"./.hidden/**"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, v, opts)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{hiddenUnitDir}, components.Filter(component.UnitKind).Paths())
}

// TestDiscovery_ExternalDependencies tests that external dependencies are correctly identified.
func TestDiscovery_ExternalDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	internalDir := filepath.Join(tmpDir, "internal")
	externalDir := filepath.Join(tmpDir, "external")
	appDir := filepath.Join(internalDir, "app")
	dbDir := filepath.Join(internalDir, "db")
	vpcDir := filepath.Join(internalDir, "vpc")
	extApp := filepath.Join(externalDir, "app")

	v := memGitTopLevelVenv(t, tmpDir)

	writeUnits(t, v.FS, map[string]string{
		appDir: `
	dependency "db" { config_path = "../db" }
	dependency "external" { config_path = "../../external/app" }
	`,
		dbDir: `
	dependency "vpc" { config_path = "../vpc" }
	`,
		vpcDir: "",
		extApp: "",
	})

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     internalDir,
		RootWorkingDir: internalDir,
	}

	ctx := t.Context()

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(internalDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: internalDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, v, opts)
	require.NoError(t, err)

	// Find app config and assert it has external dependency
	var appCfg *component.Unit

	for _, c := range components {
		if c.Path() == appDir {
			if unit, ok := c.(*component.Unit); ok {
				appCfg = unit
			}

			break
		}
	}

	require.NotNil(t, appCfg)
	depPaths := appCfg.Dependencies().Paths()
	assert.Contains(t, depPaths, dbDir)
	assert.Contains(t, depPaths, extApp)

	// Verify external dependency is marked as external
	for _, dep := range appCfg.Dependencies() {
		if dep.Path() == extApp {
			assert.True(t, dep.External(), "external app should be marked as external")
		}
	}
}

// TestDiscovery_BreakCycles tests that WithBreakCycles removes cyclic components.
func TestDiscovery_BreakCycles(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	fooDir := filepath.Join(tmpDir, "foo")
	barDir := filepath.Join(tmpDir, "bar")

	v := memGitTopLevelVenv(t, tmpDir)

	// Create terragrunt.hcl files with mutual dependencies (cycle)
	writeUnits(t, v.FS, map[string]string{
		fooDir: `
dependency "bar" {
	config_path = "../bar"
}
`,
		barDir: `
dependency "foo" {
	config_path = "../foo"
}
`,
	})

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters).
		WithBreakCycles()

	components, err := d.Discover(ctx, l, v, opts)
	require.NoError(t, err, "Discovery should complete with break cycles enabled")

	// With break cycles enabled, the cycle should be resolved (one component removed)
	_, cycleErr := components.CycleCheck()
	require.NoError(t, cycleErr, "Cycle check should not detect a cycle after breaking")
}

// TestDiscovery_WithNumWorkers tests that the worker count can be configured.
func TestDiscovery_WithNumWorkers(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	v := memGitTopLevelVenv(t, tmpDir)

	// Create a few test units
	units := map[string]string{}
	for i := range 5 {
		units[filepath.Join(tmpDir, "unit"+string(rune('a'+i)))] = ""
	}

	writeUnits(t, v.FS, units)

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithNumWorkers(2)

	components, err := d.Discover(ctx, l, v, opts)
	require.NoError(t, err)
	assert.Len(t, components, 5)
}

// TestDiscovery_WithMaxDependencyDepth tests dependency depth limiting.
func TestDiscovery_WithMaxDependencyDepth(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create chain: a -> b -> c -> d
	aDir := filepath.Join(tmpDir, "a")
	bDir := filepath.Join(tmpDir, "b")
	cDir := filepath.Join(tmpDir, "c")
	dDir := filepath.Join(tmpDir, "d")

	v := memGitTopLevelVenv(t, tmpDir)

	writeUnits(t, v.FS, map[string]string{
		aDir: `
dependency "b" {
	config_path = "../b"
}
`,
		bDir: `
dependency "c" {
	config_path = "../c"
}
`,
		cDir: `
dependency "d" {
	config_path = "../d"
}
`,
		dDir: ``,
	})

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	t.Run("full depth discovers all", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(l, []string{"a..."})
		require.NoError(t, err)

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithFilters(filters).
			WithMaxDependencyDepth(100)

		components, err := d.Discover(ctx, l, v, opts)
		require.NoError(t, err)

		paths := components.Paths()
		assert.Contains(t, paths, aDir)
		assert.Contains(t, paths, bDir)
		assert.Contains(t, paths, cDir)
		assert.Contains(t, paths, dDir)
	})

	t.Run("limited depth", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(l, []string{"a..."})
		require.NoError(t, err)

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithFilters(filters).
			WithMaxDependencyDepth(1)

		components, err := d.Discover(ctx, l, v, opts)
		require.NoError(t, err)

		paths := components.Paths()
		assert.Contains(t, paths, aDir, "a should always be included")
		// With depth 1, we should get at least a and b
		assert.Contains(t, paths, bDir, "b should be included with depth 1")
	})
}

// TestDiscovery_SuppressParseErrors tests that parse errors can be suppressed.
func TestDiscovery_SuppressParseErrors(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	validDir := filepath.Join(tmpDir, "valid")
	invalidDir := filepath.Join(tmpDir, "invalid")

	v := memGitTopLevelVenv(t, tmpDir)

	writeUnits(t, v.FS, map[string]string{
		// Valid config
		validDir: "",
		// Invalid config (should cause parse error)
		invalidDir: `
terraform {
  source = undefined_function()
}
`,
	})

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithParseExclude().
		WithSuppressParseErrors()

	components, err := d.Discover(ctx, l, v, opts)
	require.NoError(t, err, "Discovery should succeed with suppressed parse errors")

	// Valid config should be discovered
	paths := components.Paths()
	assert.Contains(t, paths, validDir)
}

// TestDiscovery_ExcludeDependencies tests that ExcludeDependencies only takes effect
// when the dependent unit's exclude condition (If) is true.
func TestDiscovery_ExcludeDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		excludeIf          string
		dependentExcluded  bool
		dependencyExcluded bool
	}{
		{
			name:               "exclude_dependencies with if=false",
			excludeIf:          "false",
			dependentExcluded:  false,
			dependencyExcluded: false,
		},
		{
			name:               "exclude_dependencies with if=true",
			excludeIf:          "true",
			dependentExcluded:  true,
			dependencyExcluded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

			dependentDir := filepath.Join(tmpDir, "dependent")
			dependencyDir := filepath.Join(tmpDir, "dependency")

			v := memGitTopLevelVenv(t, tmpDir)

			dependentHCL := `
exclude {
  if                   = ` + tt.excludeIf + `
  actions              = ["all"]
  exclude_dependencies = true
}

dependency "dependency" {
  config_path = "../dependency"
}
`
			writeUnits(t, v.FS, map[string]string{
				dependentDir:  dependentHCL,
				dependencyDir: "",
			})

			l := logger.CreateLogger()
			opts := &options.TerragruntOptions{
				WorkingDir:       tmpDir,
				RootWorkingDir:   tmpDir,
				TerraformCommand: "plan",
			}

			ctx := t.Context()

			d := discovery.NewDiscovery(tmpDir).
				WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
				WithParseExclude().
				WithRelationships()

			components, err := d.Discover(ctx, l, v, opts)
			require.NoError(t, err)

			var dependentUnit, dependencyUnit *component.Unit

			for _, c := range components {
				unit, ok := c.(*component.Unit)
				if !ok {
					continue
				}

				switch c.Path() {
				case dependentDir:
					dependentUnit = unit
				case dependencyDir:
					dependencyUnit = unit
				}
			}

			require.NotNil(t, dependentUnit, "dependent unit should be discovered")
			require.NotNil(t, dependencyUnit, "dependency unit should be discovered")

			assert.Equal(
				t,
				tt.dependentExcluded,
				dependentUnit.Excluded(),
				"dependent excluded state",
			)
			assert.Equal(
				t,
				tt.dependencyExcluded,
				dependencyUnit.Excluded(),
				"dependency excluded state",
			)
		})
	}
}

// TestDiscovery_OriginalTerragruntConfigPath tests that get_original_terragrunt_dir() returns the
// correct directory during parsing. This verifies that phase_parse.go correctly sets
// OriginalTerragruntConfigPath when parsing units.
func TestDiscovery_OriginalTerragruntConfigPath(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	appDir := filepath.Join(tmpDir, "app")
	dbDir := filepath.Join(tmpDir, "db")

	v := memGitTopLevelVenv(t, tmpDir)

	// Create a config that uses get_original_terragrunt_dir() in the terraform source
	// This function relies on OriginalTerragruntConfigPath being set correctly
	writeUnits(t, v.FS, map[string]string{
		appDir: `
terraform {
  source = "${get_original_terragrunt_dir()}/module"
}

dependency "db" {
  config_path = "../db"
}
`,
		dbDir: "",
	})

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
		// Start with a different config path to simulate the scenario where opts is cloned
		TerragruntConfigPath:         tmpDir,
		OriginalTerragruntConfigPath: tmpDir,
	}

	ctx := t.Context()

	// Use a dependency traversal filter (app...) to trigger parsing
	filters, err := filter.ParseFilterQueries(l, []string{"app..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, v, opts)
	require.NoError(t, err)

	// Find the app component
	var appComponent *component.Unit

	for _, c := range components {
		if c.Path() == appDir {
			if unit, ok := c.(*component.Unit); ok {
				appComponent = unit
			}

			break
		}
	}

	require.NotNil(t, appComponent, "app component should be discovered")
	require.NotNil(t, appComponent.Config(), "app config should be parsed")
	require.NotNil(t, appComponent.Config().Terraform, "terraform block should be parsed")
	require.NotNil(t, appComponent.Config().Terraform.Source, "terraform source should be parsed")

	// The key test: verify that get_original_terragrunt_dir() returned the correct directory
	// It should resolve to the app unit's directory, not the initial opts value (tmpDir)
	expectedSource := filepath.Join(appDir, "module")
	assert.Equal(t, expectedSource, *appComponent.Config().Terraform.Source,
		"terraform source should use the correct unit directory from get_original_terragrunt_dir()")
}

// TestDiscovery_WithReadFiles tests that reading field is populated when using reading filters.
// The implementation requires a filter that triggers parsing to populate the reading field.
//
// This test stays on the OS filesystem because read_terragrunt_config and
// read_tfvars_file resolve their targets through direct os calls rather than
// the venv filesystem, so files written to an in-memory vfs are invisible to
// them.
func TestDiscovery_WithReadFiles(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	appDir := filepath.Join(tmpDir, "app")
	require.NoError(t, os.MkdirAll(appDir, 0755))

	// Create shared files that will be read
	sharedHCL := filepath.Join(tmpDir, "shared.hcl")
	sharedTFVars := filepath.Join(tmpDir, "shared.tfvars")

	require.NoError(t, os.WriteFile(sharedHCL, []byte(`
		locals {
			common_value = "test"
		}
	`), 0644))

	require.NoError(t, os.WriteFile(sharedTFVars, []byte(`
		test_var = "value"
	`), 0644))

	// Create terragrunt config that reads both files
	terragruntConfig := filepath.Join(appDir, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(terragruntConfig, []byte(`
		locals {
			shared_config = read_terragrunt_config("../shared.hcl")
			tfvars = read_tfvars_file("../shared.tfvars")
		}
	`), 0644))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Use a reading filter to trigger parsing and populate the reading field
	filters, err := filter.ParseFilterQueries(l, []string{"reading=shared.hcl"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters).
		WithReadFiles()

	components, err := d.Discover(ctx, l, venv.OSVenv(), opts)
	require.NoError(t, err)

	// Find the app component
	var appComponent *component.Unit

	for _, c := range components {
		if c.Path() == appDir {
			if unit, ok := c.(*component.Unit); ok {
				appComponent = unit
			}

			break
		}
	}

	require.NotNil(t, appComponent, "app component should be discovered")
	require.NotNil(t, appComponent.Reading(), "Reading field should be initialized")

	// Verify Reading field contains the files that were read
	require.NotEmpty(t, appComponent.Reading(), "should have read files")
	assert.Contains(t, appComponent.Reading(), sharedHCL, "should contain shared.hcl")
}
