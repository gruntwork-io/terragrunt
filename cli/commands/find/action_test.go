package find_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setup         func(t *testing.T) string
		expectedPaths []string
		format        string
		sort          string
		hidden        bool
		validate      func(t *testing.T, output string, expectedPaths []string)
	}{
		{
			name: "basic discovery",
			setup: func(t *testing.T) string {
				t.Helper()

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

				return tmpDir
			},
			expectedPaths: []string{"unit1", "unit2", "nested/unit4", "stack1"},
			format:        "text",
			sort:          "alpha",
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Split output into lines and trim whitespace
				lines := strings.Split(strings.TrimSpace(output), "\n")

				// Verify we have the expected number of lines
				assert.Len(t, lines, len(expectedPaths))

				// Verify each line is a clean path without any formatting
				for _, line := range lines {
					line = strings.TrimSpace(line)
					assert.NotEmpty(t, line)
					assert.NotContains(t, line, "\n")
					assert.NotContains(t, line, "\t")
				}

				// Verify all expected paths are present
				assert.ElementsMatch(t, expectedPaths, lines)
			},
		},
		{
			name: "json output format",
			setup: func(t *testing.T) string {
				t.Helper()

				tmpDir := t.TempDir()

				// Create test directory structure
				testDirs := []string{
					"unit1",
					"unit2",
					"stack1",
				}

				for _, dir := range testDirs {
					err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
					require.NoError(t, err)
				}

				// Create test files
				testFiles := map[string]string{
					"unit1/terragrunt.hcl":        "",
					"unit2/terragrunt.hcl":        "",
					"stack1/terragrunt.stack.hcl": "",
				}

				for path, content := range testFiles {
					err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
					require.NoError(t, err)
				}

				return tmpDir
			},
			expectedPaths: []string{"unit1", "unit2", "stack1"},
			format:        "json",
			sort:          "alpha",
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Verify the output is valid JSON
				var configs find.FoundConfigs
				err := json.Unmarshal([]byte(output), &configs)
				require.NoError(t, err)

				// Verify we have the expected number of configs
				assert.Len(t, configs, len(expectedPaths))

				// Extract paths from configs
				var paths []string
				for _, config := range configs {
					paths = append(paths, config.Path)
				}

				// Verify all expected paths are present
				assert.ElementsMatch(t, expectedPaths, paths)

				// Verify each config has a valid type
				for _, config := range configs {
					assert.NotEmpty(t, config.Type)
					assert.True(t, config.Type == discovery.ConfigTypeUnit || config.Type == discovery.ConfigTypeStack)
				}
			},
		},
		{
			name: "hidden discovery",
			setup: func(t *testing.T) string {
				t.Helper()

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

				return tmpDir
			},
			expectedPaths: []string{"unit1", "unit2", "nested/unit4", "stack1", ".hidden/unit3"},
			format:        "text",
			hidden:        true,
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Split output into lines and trim whitespace
				lines := strings.Split(strings.TrimSpace(output), "\n")

				// Verify we have the expected number of lines
				assert.Len(t, lines, len(expectedPaths))

				// Verify all expected paths are present
				assert.ElementsMatch(t, expectedPaths, lines)
			},
		},
		{
			name: "dag sorting - simple dependencies",
			setup: func(t *testing.T) string {
				t.Helper()

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

				return tmpDir
			},
			expectedPaths: []string{"unit1", "unit2", "unit3"},
			format:        "text",
			sort:          "dag",
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Split output into lines and trim whitespace
				lines := strings.Split(strings.TrimSpace(output), "\n")

				// Verify we have the expected number of lines
				assert.Len(t, lines, len(expectedPaths))

				// For DAG sorting, order matters - verify exact order
				assert.Equal(t, expectedPaths, lines)
			},
		},
		{
			name: "dag sorting - reversed dependencies",
			setup: func(t *testing.T) string {
				t.Helper()

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

				return tmpDir
			},
			expectedPaths: []string{"unit1", "unit2", "unit3"},
			format:        "text",
			sort:          "dag",
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Split output into lines and trim whitespace
				lines := strings.Split(strings.TrimSpace(output), "\n")

				// Verify we have the expected number of lines
				assert.Len(t, lines, len(expectedPaths))

				// For DAG sorting, order matters - verify exact order
				assert.Equal(t, expectedPaths, lines)

				// Helper to find index of a path
				findIndex := func(path string) int {
					for i, line := range lines {
						if line == path {
							return i
						}
					}
					return -1
				}

				// Verify dependency ordering
				unit1Index := findIndex("unit1")
				unit2Index := findIndex("unit2")
				unit3Index := findIndex("unit3")

				assert.Less(t, unit3Index, unit2Index, "unit3 should come before unit2")
				assert.Less(t, unit2Index, unit1Index, "unit2 should come before unit1")
			},
		},
		{
			name: "dag sorting - complex dependencies",
			setup: func(t *testing.T) string {
				t.Helper()

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

				return tmpDir
			},
			expectedPaths: []string{"A", "B", "C", "D", "E", "F"},
			format:        "text",
			sort:          "dag",
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Split output into lines and trim whitespace
				lines := strings.Split(strings.TrimSpace(output), "\n")

				// Verify we have the expected number of lines
				assert.Len(t, lines, len(expectedPaths))

				// For DAG sorting, order matters - verify exact order
				// and also verify relative ordering constraints
				assert.Equal(t, expectedPaths, lines)

				// Helper to find index of a path
				findIndex := func(path string) int {
					for i, line := range lines {
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
			},
		},
		{
			name: "dag sorting - json output",
			setup: func(t *testing.T) string {
				t.Helper()

				tmpDir := t.TempDir()

				// Create test directory structure with dependencies
				testDirs := []string{
					"A", "B", "C",
				}

				for _, dir := range testDirs {
					err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
					require.NoError(t, err)
				}

				// Create test files with dependencies
				testFiles := map[string]string{
					"A/terragrunt.hcl": "",
					"B/terragrunt.hcl": `
dependency "A" {
  config_path = "../A"
}`,
					"C/terragrunt.hcl": `
dependency "B" {
  config_path = "../B"
}`,
				}

				for path, content := range testFiles {
					err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
					require.NoError(t, err)
				}

				return tmpDir
			},
			expectedPaths: []string{"A", "B", "C"},
			format:        "json",
			sort:          "dag",
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Verify the output is valid JSON
				var configs []find.FoundConfig
				err := json.Unmarshal([]byte(output), &configs)
				require.NoError(t, err)

				// Verify we have the expected number of configs
				assert.Len(t, configs, len(expectedPaths))

				// Extract paths and verify order
				var paths []string
				for _, config := range configs {
					paths = append(paths, config.Path)
				}
				assert.Equal(t, expectedPaths, paths)

				// Verify dependencies are correctly represented in JSON
				assert.Empty(t, configs[0].Dependencies, "A should have no dependencies")
				assert.Equal(t, []string{"A"}, configs[1].Dependencies, "B should depend on A")
				assert.Equal(t, []string{"B"}, configs[2].Dependencies, "C should depend on B")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup test directory
			tmpDir := tt.setup(t)

			tgOpts := options.NewTerragruntOptions()
			tgOpts.WorkingDir = tmpDir

			// Create options
			opts := find.NewOptions(tgOpts)
			opts.Format = tt.format
			opts.Hidden = tt.hidden
			opts.Sort = tt.sort

			// Create a pipe to capture output
			r, w, err := os.Pipe()
			require.NoError(t, err)

			// Set the writer in options
			opts.Writer = w

			err = find.Run(context.Background(), opts)
			require.NoError(t, err)

			// Close the write end of the pipe
			w.Close()

			// Read all output
			output, err := io.ReadAll(r)
			require.NoError(t, err)

			// Validate the output
			tt.validate(t, string(output), tt.expectedPaths)
		})
	}
}

func TestColorizer(t *testing.T) {
	t.Parallel()

	colorizer := find.NewColorizer()

	tests := []struct {
		name   string
		config *find.FoundConfig
		// We can't test exact ANSI codes as they might vary by environment,
		// so we'll test that different types result in different outputs
		shouldBeDifferent []discovery.ConfigType
	}{
		{
			name: "unit config",
			config: &find.FoundConfig{
				Type: discovery.ConfigTypeUnit,
				Path: "path/to/unit",
			},
			shouldBeDifferent: []discovery.ConfigType{discovery.ConfigTypeStack},
		},
		{
			name: "stack config",
			config: &find.FoundConfig{
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
				diffConfig := &find.FoundConfig{
					Type: diffType,
					Path: tt.config.Path,
				}
				diffResult := colorizer.Colorize(diffConfig)
				assert.NotEqual(t, result, diffResult)
			}
		})
	}
}
