package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFilesystemPhase_BasicDiscovery tests the filesystem phase directly.
func TestFilesystemPhase_BasicDiscovery(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create test directory structure
	unit1Dir := filepath.Join(tmpDir, "unit1")
	unit2Dir := filepath.Join(tmpDir, "unit2")
	stackDir := filepath.Join(tmpDir, "stack1")

	testDirs := []string{unit1Dir, unit2Dir, stackDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(unit1Dir, "terragrunt.hcl"):       "",
		filepath.Join(unit2Dir, "terragrunt.hcl"):       "",
		filepath.Join(stackDir, "terragrunt.stack.hcl"): "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Run filesystem phase via full discovery
	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir})

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Verify phase discovered all components
	units := components.Filter(component.UnitKind)
	stacks := components.Filter(component.StackKind)

	assert.Len(t, units, 2)
	assert.Len(t, stacks, 1)
}

// TestFilesystemPhase_SkipsIgnorableDirs tests that .git, .terraform, .terragrunt-cache are skipped.
func TestFilesystemPhase_SkipsIgnorableDirs(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create valid unit
	unitDir := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(""), 0644))

	// Create units in ignorable directories (should be skipped)
	ignorableDirs := []string{".git", ".terraform", ".terragrunt-cache"}
	for _, dir := range ignorableDirs {
		ignorableUnit := filepath.Join(tmpDir, dir, "ignored")
		require.NoError(t, os.MkdirAll(ignorableUnit, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(ignorableUnit, "terragrunt.hcl"), []byte(""), 0644))
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir})

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Should only find the valid unit, not the ones in ignorable directories
	assert.Len(t, components, 1)
	assert.Equal(t, unitDir, components[0].Path())
}

// TestFilesystemPhase_WithNoHidden tests hidden directory filtering.
func TestFilesystemPhase_WithNoHidden(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create visible unit
	visibleDir := filepath.Join(tmpDir, "visible")
	require.NoError(t, os.MkdirAll(visibleDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(visibleDir, "terragrunt.hcl"), []byte(""), 0644))

	// Create hidden unit
	hiddenDir := filepath.Join(tmpDir, ".hidden", "unit")
	require.NoError(t, os.MkdirAll(hiddenDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hiddenDir, "terragrunt.hcl"), []byte(""), 0644))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	t.Run("without noHidden", func(t *testing.T) {
		t.Parallel()

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir})

		components, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)
		assert.Len(t, components, 2, "Should find both visible and hidden")
	})

	t.Run("with noHidden", func(t *testing.T) {
		t.Parallel()

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithNoHidden()

		components, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)
		assert.Len(t, components, 1, "Should find only visible")
		assert.Equal(t, visibleDir, components[0].Path())
	})
}

// TestParsePhase_ParsesConfigsForParseRequiredFilters tests that parse phase handles parse-required filters.
func TestParsePhase_ParsesConfigsForParseRequiredFilters(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create shared file
	sharedFile := filepath.Join(tmpDir, "shared.hcl")
	require.NoError(t, os.WriteFile(sharedFile, []byte(`
locals {
	value = "test"
}
`), 0644))

	// Create unit that reads the shared file
	unitDir := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`
locals {
	shared = read_terragrunt_config("../shared.hcl")
}
`), 0644))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Filter with reading= attribute requires parsing
	filters, err := filter.ParseFilterQueries(l, []string{"reading=shared.hcl"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters).
		WithReadFiles()

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	assert.Len(t, components, 1)
	assert.Equal(t, unitDir, components[0].Path())
}

// TestGraphPhase_DependencyDiscovery tests the graph phase dependency discovery.
func TestGraphPhase_DependencyDiscovery(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create dependency chain: vpc -> db -> app
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")

	testDirs := []string{vpcDir, dbDir, appDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(appDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../db"
}
`,
		filepath.Join(dbDir, "terragrunt.hcl"): `
dependency "vpc" {
	config_path = "../vpc"
}
`,
		filepath.Join(vpcDir, "terragrunt.hcl"): ``,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Use graph filter to trigger graph phase
	filters, err := filter.ParseFilterQueries(l, []string{"app..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Graph phase should discover all dependencies
	paths := components.Paths()
	assert.Contains(t, paths, appDir)
	assert.Contains(t, paths, dbDir)
	assert.Contains(t, paths, vpcDir)

	// Verify dependency relationships are built
	var appComponent component.Component

	for _, c := range components {
		if c.Path() == appDir {
			appComponent = c
			break
		}
	}

	require.NotNil(t, appComponent)
	assert.Contains(t, appComponent.Dependencies().Paths(), dbDir)
}

// TestGraphPhase_DependentDiscoveryRequiresRelationships tests that dependent discovery
// requires relationships to be built first. This is a behavioral test documenting the
// current implementation's requirements for dependent traversal.
func TestGraphPhase_DependentDiscoveryRequiresRelationships(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create dependency chain: vpc -> db -> app
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")

	testDirs := []string{vpcDir, dbDir, appDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(appDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../db"
}
`,
		filepath.Join(dbDir, "terragrunt.hcl"): `
dependency "vpc" {
	config_path = "../vpc"
}
`,
		filepath.Join(vpcDir, "terragrunt.hcl"): ``,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Using dependent filter (...vpc) without pre-built relationships
	// Currently, the implementation requires relationships to be built
	// before dependent traversal can work (unlike dependency traversal which
	// parses configs on-the-fly)
	filters, err := filter.ParseFilterQueries(l, []string{"...vpc"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// The vpc component should always be discovered (it's the target)
	paths := components.Paths()
	assert.Contains(t, paths, vpcDir, "vpc should always be included as the target")
}

// TestRelationshipPhase_BuildsRelationships tests the relationship phase.
func TestRelationshipPhase_BuildsRelationships(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create components with dependencies
	appDir := filepath.Join(tmpDir, "app")
	dbDir := filepath.Join(tmpDir, "db")

	testDirs := []string{appDir, dbDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(appDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../db"
}
`,
		filepath.Join(dbDir, "terragrunt.hcl"): ``,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Use WithRelationships to enable relationship phase
	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithRelationships()

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Verify relationships are built
	var appComponent component.Component

	for _, c := range components {
		if c.Path() == appDir {
			appComponent = c
			break
		}
	}

	require.NotNil(t, appComponent)
	depPaths := appComponent.Dependencies().Paths()
	assert.Contains(t, depPaths, dbDir)
}

// TestCandidacyClassifier_AnalyzesFiltersCorrectly tests the candidacy classifier analysis.
func TestCandidacyClassifier_AnalyzesFiltersCorrectly(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

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
		{
			name:              "external attribute filter",
			filterStrings:     []string{"external=true"},
			expectHasPositive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(l, tt.filterStrings)
			require.NoError(t, err)

			classifier := filter.NewClassifier(l)
			err = classifier.Analyze(filters)
			require.NoError(t, err)

			assert.Equal(t, tt.expectHasPositive, classifier.HasPositiveFilters(), "HasPositiveFilters mismatch")
			assert.Equal(t, tt.expectHasParseRequired, classifier.HasParseRequiredFilters(), "HasParseRequiredFilters mismatch")
			assert.Equal(t, tt.expectHasGraphFilters, classifier.HasGraphFilters(), "HasGraphFilters mismatch")

			if tt.expectGraphExprCount > 0 {
				assert.Len(t, classifier.GraphExpressions(), tt.expectGraphExprCount, "GraphExpressions count mismatch")
			}
		})
	}
}

// TestCandidacyClassifier_ClassifiesComponentsCorrectly tests component classification.
func TestCandidacyClassifier_ClassifiesComponentsCorrectly(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	tests := []struct {
		name          string
		componentPath string
		workingDir    string
		filterStrings []string
		expectStatus  filter.ClassificationStatus
		expectReason  filter.CandidacyReason
	}{
		{
			name:          "no filters - include by default",
			filterStrings: []string{},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusDiscovered,
			expectReason:  filter.CandidacyReasonNone,
		},
		{
			name:          "matching path filter",
			filterStrings: []string{"./foo"},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusDiscovered,
			expectReason:  filter.CandidacyReasonNone,
		},
		{
			name:          "non-matching path filter - exclude by default",
			filterStrings: []string{"./bar"},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusExcluded,
			expectReason:  filter.CandidacyReasonNone,
		},
		{
			name:          "negated filter only - exclude component",
			filterStrings: []string{"!./foo"},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusExcluded,
			expectReason:  filter.CandidacyReasonNone,
		},
		{
			name:          "negated filter only - include other",
			filterStrings: []string{"!./foo"},
			componentPath: "/project/bar",
			workingDir:    "/project",
			expectStatus:  filter.StatusDiscovered,
			expectReason:  filter.CandidacyReasonNone,
		},
		{
			name:          "graph expression target - candidate",
			filterStrings: []string{"./foo..."},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusCandidate,
			expectReason:  filter.CandidacyReasonGraphTarget,
		},
		{
			name:          "parse required filter - candidate",
			filterStrings: []string{"reading=config/*"},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusCandidate,
			expectReason:  filter.CandidacyReasonRequiresParse,
		},
		{
			name:          "wildcard path filter match",
			filterStrings: []string{"./apps/*"},
			componentPath: "/project/apps/frontend",
			workingDir:    "/project",
			expectStatus:  filter.StatusDiscovered,
			expectReason:  filter.CandidacyReasonNone,
		},
		{
			name:          "name filter match",
			filterStrings: []string{"name=foo"},
			componentPath: "/project/foo",
			workingDir:    "/project",
			expectStatus:  filter.StatusDiscovered,
			expectReason:  filter.CandidacyReasonNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(l, tt.filterStrings)
			require.NoError(t, err)

			classifier := filter.NewClassifier(l)
			err = classifier.Analyze(filters)
			require.NoError(t, err)

			// Create a test component
			c := component.NewUnit(tt.componentPath)
			c.SetDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: tt.workingDir,
			})

			ctx := filter.ClassificationContext{}
			status, reason, _ := classifier.Classify(c, ctx)

			assert.Equal(t, tt.expectStatus, status, "status mismatch")
			assert.Equal(t, tt.expectReason, reason, "reason mismatch")
		})
	}
}

// TestClassifier_ParseExpressions tests the ParseExpressions method.
func TestClassifier_ParseExpressions(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{"reading=config/*", "reading=shared.hcl"})
	require.NoError(t, err)

	classifier := filter.NewClassifier(l)
	err = classifier.Analyze(filters)
	require.NoError(t, err)

	parseExprs := classifier.ParseExpressions()
	assert.Len(t, parseExprs, 2, "Should have 2 parse expressions")
}

// TestClassifier_NegatedExpressions tests the NegatedExpressions method.
func TestClassifier_NegatedExpressions(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{"!./foo", "!./bar", "./baz"})
	require.NoError(t, err)

	classifier := filter.NewClassifier(l)
	err = classifier.Analyze(filters)
	require.NoError(t, err)

	negatedExprs := classifier.NegatedExpressions()
	assert.Len(t, negatedExprs, 2, "Should have 2 negated expressions")
}

// TestClassifier_HasDependentFilters tests the HasDependentFilters method.
func TestClassifier_HasDependentFilters(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	tests := []struct {
		name          string
		filterStrings []string
		expectResult  bool
	}{
		{
			name:          "no graph filters",
			filterStrings: []string{"./foo"},
			expectResult:  false,
		},
		{
			name:          "dependency only filter - app...",
			filterStrings: []string{"app..."},
			expectResult:  false,
		},
		{
			name:          "dependent only filter - ...vpc",
			filterStrings: []string{"...vpc"},
			expectResult:  true,
		},
		{
			name:          "bidirectional filter - ...db...",
			filterStrings: []string{"...db..."},
			expectResult:  true,
		},
		{
			name:          "exclude target dependent - ...^vpc",
			filterStrings: []string{"...^vpc"},
			expectResult:  true,
		},
		{
			name:          "multiple filters with dependent",
			filterStrings: []string{"app...", "...vpc"},
			expectResult:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(l, tt.filterStrings)
			require.NoError(t, err)

			classifier := filter.NewClassifier(l)
			err = classifier.Analyze(filters)
			require.NoError(t, err)

			assert.Equal(t, tt.expectResult, classifier.HasDependentFilters(), "HasDependentFilters mismatch")
		})
	}
}

// TestGraphPhase_DependentDiscovery_WithPreBuiltGraph tests that dependent discovery
// works correctly when the dependency graph is pre-built.
func TestGraphPhase_DependentDiscovery_WithPreBuiltGraph(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create dependency chain: vpc -> db -> app
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")

	testDirs := []string{vpcDir, dbDir, appDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(appDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../db"
}
`,
		filepath.Join(dbDir, "terragrunt.hcl"): `
dependency "vpc" {
	config_path = "../vpc"
}
`,
		filepath.Join(vpcDir, "terragrunt.hcl"): ``,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Using dependent filter (...vpc) should now work with pre-built graph
	filters, err := filter.ParseFilterQueries(l, []string{"...vpc"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// With pre-built dependency graph, dependent discovery should now find all dependents
	paths := components.Paths()
	assert.Contains(t, paths, vpcDir, "vpc should be included as the target")
	assert.Contains(t, paths, dbDir, "db should be included as direct dependent of vpc")
	assert.Contains(t, paths, appDir, "app should be included as transitive dependent of vpc")
}
