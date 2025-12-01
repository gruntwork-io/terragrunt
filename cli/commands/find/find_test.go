package find_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/internal/component"
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
		reading       bool
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
				var configs find.FoundComponents
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
					assert.True(t, config.Type == component.UnitKind || config.Type == component.StackKind)
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
				var configs []find.FoundComponent
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
		{
			name: "reading flag with json output",
			setup: func(t *testing.T) string {
				t.Helper()

				tmpDir := t.TempDir()
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

				return tmpDir
			},
			expectedPaths: []string{"app"},
			format:        "json",
			mode:          "normal",
			reading:       true,
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				// Verify the output is valid JSON
				var configs find.FoundComponents
				err := json.Unmarshal([]byte(output), &configs)
				require.NoError(t, err)

				// Verify we have one config
				require.Len(t, configs, 1)

				// Verify the component has the Reading field populated
				appConfig := configs[0]
				require.NotNil(t, appConfig.Reading, "Reading field should be populated")
				require.NotEmpty(t, appConfig.Reading, "Reading field should contain files")

				// Verify Reading field contains the shared files
				readingPaths := appConfig.Reading
				assert.Len(t, readingPaths, 2, "should have read 2 files")

				// Convert to map for easier checking
				readingMap := make(map[string]bool)
				for _, path := range readingPaths {
					readingMap[filepath.FromSlash(path)] = true
				}

				// Check that shared files are in the reading list
				assert.True(t, readingMap["shared.hcl"], "should contain shared.hcl")
				assert.True(t, readingMap["shared.tfvars"], "should contain shared.tfvars")
			},
		},
		{
			name: "external flag implies dependencies",
			setup: func(t *testing.T) string {
				t.Helper()

				tmpDir := t.TempDir()

				internalDir := filepath.Join(tmpDir, "internal")
				require.NoError(t, os.MkdirAll(internalDir, 0755))

				unitADir := filepath.Join(internalDir, "unitA")
				require.NoError(t, os.MkdirAll(unitADir, 0755))

				externalDir := filepath.Join(tmpDir, "external")
				require.NoError(t, os.MkdirAll(externalDir, 0755))

				unitBDir := filepath.Join(externalDir, "unitB")
				require.NoError(t, os.MkdirAll(unitBDir, 0755))

				require.NoError(t, os.WriteFile(filepath.Join(unitBDir, "terragrunt.hcl"), []byte(""), 0644))

				require.NoError(t, os.WriteFile(filepath.Join(unitADir, "terragrunt.hcl"), []byte(`
dependency "unitB" {
  config_path = "../../external/unitB"
}
`), 0644))

				return internalDir
			},
			expectedPaths: []string{"unitA", "../external/unitB"},
			format:        "json",
			mode:          "normal",
			external:      true,
			validate: func(t *testing.T, output string, expectedPaths []string) {
				t.Helper()

				var configs find.FoundComponents
				err := json.Unmarshal([]byte(output), &configs)
				require.NoError(t, err)

				assert.Len(t, configs, 2, "should include both internal and external units")

				var (
					internalUnit *find.FoundComponent
					externalUnit *find.FoundComponent
				)

				for _, cfg := range configs {
					if cfg.Path == "unitA" {
						internalUnit = cfg
					} else if strings.Contains(cfg.Path, "external") {
						externalUnit = cfg
					}
				}

				require.NotNil(t, internalUnit, "should find internal unit")
				require.NotNil(t, externalUnit, "should find external unit")
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
			opts.Reading = tt.reading

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
		config *find.FoundComponent
		// We can't test exact ANSI codes as they might vary by environment,
		// so we'll test that different types result in different outputs
		shouldBeDifferent []component.Kind
	}{
		{
			name: "unit config",
			config: &find.FoundComponent{
				Type: component.UnitKind,
				Path: "path/to/unit",
			},
			shouldBeDifferent: []component.Kind{component.StackKind},
		},
		{
			name: "stack config",
			config: &find.FoundComponent{
				Type: component.StackKind,
				Path: "path/to/stack",
			},
			shouldBeDifferent: []component.Kind{component.UnitKind},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := colorizer.Colorize(tt.config)
			assert.NotEmpty(t, result)

			// Test that different types produce different colorized outputs
			for _, diffType := range tt.shouldBeDifferent {
				diffConfig := &find.FoundComponent{
					Type: diffType,
					Path: tt.config.Path,
				}
				diffResult := colorizer.Colorize(diffConfig)
				assert.NotEqual(t, result, diffResult)
			}
		})
	}
}
