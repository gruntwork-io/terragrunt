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
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Verify the output is valid JSON
				var configs []discovery.DiscoveredConfig
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
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup test directory
			tmpDir := tt.setup(t)

			tgOpts := options.NewTerragruntOptions()
			tgOpts.WorkingDir = tmpDir

			// Create options
			opts := find.NewOptions(tgOpts)
			opts.Format = tt.format

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
		config *discovery.DiscoveredConfig
		// We can't test exact ANSI codes as they might vary by environment,
		// so we'll test that different types result in different outputs
		shouldBeDifferent []discovery.ConfigType
	}{
		{
			name: "unit config",
			config: &discovery.DiscoveredConfig{
				Type: discovery.ConfigTypeUnit,
				Path: "path/to/unit",
			},
			shouldBeDifferent: []discovery.ConfigType{discovery.ConfigTypeStack},
		},
		{
			name: "stack config",
			config: &discovery.DiscoveredConfig{
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
				diffConfig := &discovery.DiscoveredConfig{
					Type: diffType,
					Path: tt.config.Path,
				}
				diffResult := colorizer.Colorize(diffConfig)
				assert.NotEqual(t, result, diffResult)
			}
		})
	}
}
