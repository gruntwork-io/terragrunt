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

func TestCandidacyClassifier_ClassifyComponent(t *testing.T) {
	t.Parallel()

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
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
	d := discovery.New(tmpDir).WithDiscoveryContext(&component.DiscoveryContext{
		WorkingDir: tmpDir,
	})

	components, err := d.Discover(ctx, l, opts)
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

	d := discovery.New(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
		}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
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

	d := discovery.New(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
		}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
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

	d := discovery.New(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
		}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
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
		{expected: "discovered", status: filter.StatusDiscovered},
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
	d := discovery.New(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithReadFiles()

	components, err := d.Discover(ctx, l, opts)
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
