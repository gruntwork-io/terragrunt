package find_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup         func(t *testing.T) string
		validate      func(t *testing.T, output string, expectedPaths []string)
		name          string
		format        string
		mode          string
		expectedPaths []string
		hidden        bool
		dependencies  bool
		external      bool
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
			mode:          "normal",
			dependencies:  false,
			external:      false,
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Split output into lines and trim whitespace
				lines := strings.Split(strings.TrimSpace(output), "\n")

				// Verify we have the expected number of lines
				assert.Len(t, lines, len(expectedPaths))

				// Convert expected paths to use OS-specific path separators
				var osExpectedPaths []string
				for _, path := range expectedPaths {
					osExpectedPaths = append(osExpectedPaths, filepath.FromSlash(path))
				}

				// Convert actual paths to use OS-specific path separators
				var osPaths []string
				for _, line := range lines {
					osPaths = append(osPaths, filepath.FromSlash(strings.TrimSpace(line)))
				}

				// Verify all expected paths are present
				assert.ElementsMatch(t, osExpectedPaths, osPaths)
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
			mode:          "normal",
			dependencies:  false,
			external:      false,
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Verify the output is valid JSON
				var configs find.FoundConfigs
				err := json.Unmarshal([]byte(output), &configs)
				require.NoError(t, err)

				// Verify we have the expected number of configs
				assert.Len(t, configs, len(expectedPaths))

				// Convert expected paths to use OS-specific path separators
				var osExpectedPaths []string
				for _, path := range expectedPaths {
					osExpectedPaths = append(osExpectedPaths, filepath.FromSlash(path))
				}

				// Extract paths and convert to OS-specific separators
				var paths []string
				for _, config := range configs {
					paths = append(paths, filepath.FromSlash(config.Path))
				}

				// Verify all expected paths are present
				assert.ElementsMatch(t, osExpectedPaths, paths)

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
			mode:          "normal",
			hidden:        true,
			dependencies:  false,
			external:      false,
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Split output into lines and trim whitespace
				lines := strings.Split(strings.TrimSpace(output), "\n")

				// Verify we have the expected number of lines
				assert.Len(t, lines, len(expectedPaths))

				// Convert expected paths to use OS-specific path separators
				var osExpectedPaths []string
				for _, path := range expectedPaths {
					osExpectedPaths = append(osExpectedPaths, filepath.FromSlash(path))
				}

				// Convert actual paths to use OS-specific path separators
				var osPaths []string
				for _, line := range lines {
					osPaths = append(osPaths, filepath.FromSlash(strings.TrimSpace(line)))
				}

				// Verify all expected paths are present
				assert.ElementsMatch(t, osExpectedPaths, osPaths)
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
			mode:          "dag",
			dependencies:  true,
			external:      false,
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Split output into lines and trim whitespace
				lines := strings.Split(strings.TrimSpace(output), "\n")

				// Verify we have the expected number of lines
				assert.Len(t, lines, len(expectedPaths))

				// Convert paths to use OS-specific separators
				var osPaths []string
				for _, line := range lines {
					osPaths = append(osPaths, filepath.FromSlash(strings.TrimSpace(line)))
				}

				// Convert expected paths to use OS-specific separators
				var osExpectedPaths []string
				for _, path := range expectedPaths {
					osExpectedPaths = append(osExpectedPaths, filepath.FromSlash(path))
				}

				// For DAG sorting, order matters - verify exact order
				assert.Equal(t, osExpectedPaths, osPaths)
			},
		},
		{
			name: "dag sorting - json output with dependencies",
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
			mode:          "dag",
			dependencies:  true,
			external:      false,
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
					paths = append(paths, filepath.FromSlash(config.Path))
				}

				// Convert expected paths to use OS-specific separators
				var osExpectedPaths []string
				for _, path := range expectedPaths {
					osExpectedPaths = append(osExpectedPaths, filepath.FromSlash(path))
				}

				assert.Equal(t, osExpectedPaths, paths)

				// Verify dependencies are correctly represented in JSON
				assert.Empty(t, configs[0].Dependencies, "A should have no dependencies")
				assert.Equal(t, []string{"A"}, configs[1].Dependencies, "B should depend on A")
				assert.Equal(t, []string{"B"}, configs[2].Dependencies, "C should depend on B")
			},
		},
		{
			name: "invalid format",
			setup: func(t *testing.T) string {
				t.Helper()

				return t.TempDir()
			},
			format: "invalid",
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name: "invalid sort",
			setup: func(t *testing.T) string {
				t.Helper()

				return t.TempDir()
			},
			mode: "invalid",
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()
				assert.Empty(t, output)
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

			l := logger.CreateLogger()
			l.Formatter().SetDisabledColors(true)

			// Create options
			opts := find.NewOptions(tgOpts)
			opts.Format = tt.format
			opts.Hidden = tt.hidden
			opts.Mode = tt.mode
			opts.Dependencies = tt.dependencies
			opts.External = tt.external

			// Create a pipe to capture output
			r, w, err := os.Pipe()
			require.NoError(t, err)

			// Set the writer in options
			opts.Writer = w

			err = find.Run(t.Context(), l, opts)
			if tt.format == "invalid" || tt.mode == "invalid" {
				require.Error(t, err)
				return
			}
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

	colorizer := find.NewColorizer(true)

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
