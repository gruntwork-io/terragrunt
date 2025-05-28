package list_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/list"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicDiscovery(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create test directory structure
	testDirs := []string{
		"unit1",
		"unit2",
		"stack1",
		".hidden/unit3",
		"nested/unit4",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files
	testFiles := map[string]string{
		"unit1/terragrunt.hcl":         "",
		"unit2/terragrunt.hcl":         "",
		"stack1/terragrunt.stack.hcl":  "",
		".hidden/unit3/terragrunt.hcl": "",
		"nested/unit4/terragrunt.hcl":  "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	expectedPaths := []string{"unit1", "unit2", "nested/unit4", "stack1"}

	tgOpts := options.NewTerragruntOptions()
	tgOpts.WorkingDir = tmpDir

	// Create options
	opts := list.NewOptions(tgOpts)
	opts.Format = "text" //nolint: goconst
	opts.Mode = "normal"
	opts.Dependencies = false
	opts.External = false

	// Create a pipe to capture output
	r, w, err := os.Pipe()
	require.NoError(t, err)

	// Set the writer in options
	opts.Writer = w

	l := logger.CreateLogger()

	l.Formatter().SetDisabledColors(true)

	err = list.Run(t.Context(), l, opts)
	require.NoError(t, err)

	// Close the write end of the pipe
	w.Close()

	// Read all output
	output, err := io.ReadAll(r)
	require.NoError(t, err)

	// Split output into fields and trim whitespace
	fields := strings.Fields(string(output))

	// Normalize path separators in the output fields
	for i, field := range fields {
		fields[i] = filepath.ToSlash(field)
	}

	// Verify we have the expected number of lines
	assert.Len(t, fields, len(expectedPaths))

	// Verify each line is a clean path without any formatting
	for _, field := range fields {
		assert.NotEmpty(t, field)
	}

	// Verify all expected paths are present
	assert.ElementsMatch(t, expectedPaths, fields)
}

func TestHiddenDiscovery(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create test directory structure
	testDirs := []string{
		"unit1",
		"unit2",
		"stack1",
		".hidden/unit3",
		"nested/unit4",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files
	testFiles := map[string]string{
		"unit1/terragrunt.hcl":         "",
		"unit2/terragrunt.hcl":         "",
		"stack1/terragrunt.stack.hcl":  "",
		".hidden/unit3/terragrunt.hcl": "",
		"nested/unit4/terragrunt.hcl":  "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	expectedPaths := []string{"unit1", "unit2", "nested/unit4", "stack1", ".hidden/unit3"}

	tgOpts := options.NewTerragruntOptions()
	tgOpts.WorkingDir = tmpDir

	l := logger.CreateLogger()
	l.Formatter().SetDisabledColors(true)

	// Create options
	opts := list.NewOptions(tgOpts)
	opts.Format = "text"
	opts.Hidden = true
	opts.Dependencies = false
	opts.External = false

	// Create a pipe to capture output
	r, w, err := os.Pipe()
	require.NoError(t, err)

	// Set the writer in options
	opts.Writer = w

	err = list.Run(t.Context(), l, opts)
	require.NoError(t, err)

	// Close the write end of the pipe
	w.Close()

	// Read all output
	output, err := io.ReadAll(r)
	require.NoError(t, err)

	// Split output into fields and trim whitespace
	fields := strings.Fields(string(output))

	// Normalize path separators in the output fields
	for i, field := range fields {
		fields[i] = filepath.ToSlash(field)
	}

	// Verify we have the expected number of lines
	assert.Len(t, fields, len(expectedPaths))

	// Verify all expected paths are present
	assert.ElementsMatch(t, expectedPaths, fields)
}

func TestDAGSortingSimpleDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create test directory structure with dependencies:
	// unit2 -> unit1
	// unit3 -> unit2
	testDirs := []string{
		"unit1",
		"unit2",
		"unit3",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files with dependencies
	testFiles := map[string]string{
		"unit1/terragrunt.hcl": "",
		"unit2/terragrunt.hcl": `
dependency "unit1" {
  config_path = "../unit1"
}`,
		"unit3/terragrunt.hcl": `
dependency "unit2" {
  config_path = "../unit2"
}`,
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	expectedPaths := []string{"unit1", "unit2", "unit3"}

	tgOpts := options.NewTerragruntOptions()
	tgOpts.WorkingDir = tmpDir

	l := logger.CreateLogger()
	l.Formatter().SetDisabledColors(true)

	// Create options
	opts := list.NewOptions(tgOpts)
	opts.Format = "text"
	opts.Mode = "dag" //nolint: goconst
	opts.Dependencies = true
	opts.External = false

	// Create a pipe to capture output
	r, w, err := os.Pipe()
	require.NoError(t, err)

	// Set the writer in options
	opts.Writer = w

	err = list.Run(t.Context(), l, opts)
	require.NoError(t, err)

	// Close the write end of the pipe
	w.Close()

	// Read all output
	output, err := io.ReadAll(r)
	require.NoError(t, err)

	// Split output into fields and trim whitespace
	fields := strings.Fields(string(output))

	// Verify we have the expected number of lines
	assert.Len(t, fields, len(expectedPaths))

	// For DAG sorting, order matters - verify exact order
	assert.Equal(t, expectedPaths, fields)
}

func TestDAGSortingReversedDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create test directory structure with dependencies:
	// unit3 -> unit2
	// unit2 -> unit1
	testDirs := []string{
		"unit1",
		"unit2",
		"unit3",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files with dependencies
	testFiles := map[string]string{
		"unit1/terragrunt.hcl": `
dependency "unit2" {
  config_path = "../unit2"
}`,
		"unit2/terragrunt.hcl": `
dependency "unit3" {
  config_path = "../unit3"
}`,
		"unit3/terragrunt.hcl": "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	expectedPaths := []string{"unit3", "unit2", "unit1"}

	tgOpts := options.NewTerragruntOptions()
	tgOpts.WorkingDir = tmpDir

	l := logger.CreateLogger()
	l.Formatter().SetDisabledColors(true)

	// Create options
	opts := list.NewOptions(tgOpts)
	opts.Format = "text"
	opts.Mode = "dag" //nolint: goconst
	opts.Dependencies = true
	opts.External = false

	// Create a pipe to capture output
	r, w, err := os.Pipe()
	require.NoError(t, err)

	// Set the writer in options
	opts.Writer = w

	err = list.Run(t.Context(), l, opts)
	require.NoError(t, err)

	// Close the write end of the pipe
	w.Close()

	// Read all output
	output, err := io.ReadAll(r)
	require.NoError(t, err)

	// Split output into fields and trim whitespace
	fields := strings.Fields(string(output))

	// Verify we have the expected number of lines
	assert.Len(t, fields, len(expectedPaths))

	// For DAG sorting, order matters - verify exact order
	assert.Equal(t, expectedPaths, fields)

	// Helper to find index of a path
	findIndex := func(path string) int {
		for i, field := range fields {
			if field == path {
				return i
			}
		}
		return -1
	}

	// Verify dependency ordering
	unit1Index := findIndex("unit1")
	unit2Index := findIndex("unit2")
	unit3Index := findIndex("unit3")

	assert.Less(t, unit3Index, unit2Index, "unit3 (no deps) should come before unit2 (depends on unit3)")
	assert.Less(t, unit2Index, unit1Index, "unit2 should come before unit1 (depends on unit2)")
}

func TestDAGSortingComplexDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create test directory structure with complex dependencies:
	// A (no deps)
	// B (no deps)
	// C -> A
	// D -> A,B
	// E -> C
	// F -> C
	testDirs := []string{
		"A", "B", "C", "D", "E", "F",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files with dependencies
	testFiles := map[string]string{
		"A/terragrunt.hcl": "",
		"B/terragrunt.hcl": "",
		"C/terragrunt.hcl": `
dependency "A" {
  config_path = "../A"
}`,
		"D/terragrunt.hcl": `
dependency "A" {
  config_path = "../A"
}
dependency "B" {
  config_path = "../B"
}`,
		"E/terragrunt.hcl": `
dependency "C" {
  config_path = "../C"
}`,
		"F/terragrunt.hcl": `
dependency "C" {
  config_path = "../C"
}`,
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	expectedPaths := []string{"A", "B", "C", "D", "E", "F"}

	tgOpts := options.NewTerragruntOptions()
	tgOpts.WorkingDir = tmpDir

	l := logger.CreateLogger()

	l.Formatter().SetDisabledColors(true)

	// Create options
	opts := list.NewOptions(tgOpts)
	opts.Format = "text"
	opts.Mode = "dag" //nolint: goconst
	opts.Dependencies = true
	opts.External = false

	// Create a pipe to capture output
	r, w, err := os.Pipe()
	require.NoError(t, err)

	// Set the writer in options
	opts.Writer = w

	err = list.Run(t.Context(), l, opts)
	require.NoError(t, err)

	// Close the write end of the pipe
	w.Close()

	// Read all output
	output, err := io.ReadAll(r)
	require.NoError(t, err)

	// Split output into fields and trim whitespace
	fields := strings.Fields(string(output))

	// Verify we have the expected number of lines
	assert.Len(t, fields, len(expectedPaths))

	// For DAG sorting, order matters - verify exact order
	// and also verify relative ordering constraints
	assert.Equal(t, expectedPaths, fields)

	// Helper to find index of a path
	findIndex := func(path string) int {
		for i, line := range fields {
			if line == path {
				return i
			}
		}
		return -1
	}

	// Verify dependency ordering
	aIndex := findIndex("A")
	bIndex := findIndex("B")
	cIndex := findIndex("C")
	dIndex := findIndex("D")
	eIndex := findIndex("E")
	fIndex := findIndex("F")

	// Level 0 items should be before their dependents
	assert.Less(t, aIndex, cIndex, "A should come before C")
	assert.Less(t, aIndex, dIndex, "A should come before D")
	assert.Less(t, bIndex, dIndex, "B should come before D")

	// Level 1 items should be before their dependents
	assert.Less(t, cIndex, eIndex, "C should come before E")
	assert.Less(t, cIndex, fIndex, "C should come before F")
}

func TestColorizer(t *testing.T) {
	t.Parallel()

	colorizer := list.NewColorizer(true)

	tests := []struct {
		name   string
		config *list.ListedConfig
		// We can't test exact ANSI codes as they might vary by environment,
		// so we'll test that different types result in different outputs
		shouldBeDifferent []discovery.ConfigType
	}{
		{
			name: "unit config",
			config: &list.ListedConfig{
				Type: discovery.ConfigTypeUnit,
				Path: "path/to/unit",
			},
			shouldBeDifferent: []discovery.ConfigType{discovery.ConfigTypeStack},
		},
		{
			name: "stack config",
			config: &list.ListedConfig{
				Type: discovery.ConfigTypeStack,
				Path: "path/to/stack",
			},
			shouldBeDifferent: []discovery.ConfigType{discovery.ConfigTypeUnit},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := colorizer.Colorize(tt.config)
			assert.NotEmpty(t, result)

			// Test that different types produce different colorized outputs
			for _, diffType := range tt.shouldBeDifferent {
				diffConfig := &list.ListedConfig{
					Type: diffType,
					Path: tt.config.Path,
				}
				diffResult := colorizer.Colorize(diffConfig)
				assert.NotEqual(t, result, diffResult)
			}
		})
	}
}
