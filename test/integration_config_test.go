// +build integration

package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

// TestIntegration_ForwardCompatibility tests that unknown flags in .terragruntrc.json
// produce warnings instead of errors, supporting forward compatibility where config
// files created for newer Terragrunt versions can work with older versions.
func TestIntegration_ForwardCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name                 string
		fixtureDir           string
		expectedNoError      bool
		expectedKnownFlagSet bool // Whether known flags (like non-interactive) should still work
	}{
		{
			name:                 "unknown flags produce warnings not errors",
			fixtureDir:           "unknown-flags",
			expectedNoError:      true,
			expectedKnownFlagSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: Create temporary test environment
			testDir := setupTestEnvWithFixture(t, tt.fixtureDir)

			// Setup: Create a CLI app with some known flags
			app := createTestCliApp()

			// Setup: Set working directory to test fixture location
			origWd, err := os.Getwd()
			require.NoError(t, err)
			err = os.Chdir(testDir)
			require.NoError(t, err)
			defer func() {
				os.Chdir(origWd)
			}()

			// Execute: Call LoadTerragruntRC
			ctx := context.Background()
			err = config.LoadTerragruntRC(ctx, app)

			// Validate: Should not return error (forward compatibility)
			// This is the key test - unknown flags should NOT cause errors
			if tt.expectedNoError {
				assert.NoError(t, err, "LoadTerragruntRC should not error on unknown flags (forward compatibility)")
			} else {
				assert.Error(t, err)
			}

			// Validate: Known flags should still be applied correctly
			// Even when unknown flags are present, known flags should work
			if tt.expectedKnownFlagSet {
				// Check that non-interactive flag was set from config
				nonInteractiveFlag := findFlagByName(app.Flags, "non-interactive")
				require.NotNil(t, nonInteractiveFlag, "non-interactive flag should exist in app")

				if boolFlag, ok := nonInteractiveFlag.(*cli.BoolFlag); ok {
					require.NotNil(t, boolFlag.Destination, "non-interactive flag destination should be set")
					assert.True(t, *boolFlag.Destination,
						"Known flag 'non-interactive' should be set to true from config, even with unknown flags present")
				} else {
					t.Fatalf("non-interactive flag is not a BoolFlag: %T", nonInteractiveFlag)
				}
			}

			t.Logf("✓ Forward compatibility verified: unknown flags did not cause error")
			t.Logf("✓ Known flags still work correctly with unknown flags present")
		})
	}
}

// setupTestEnvWithFixture creates a temporary test environment with the specified fixture.
// Returns the path to the temporary directory containing the fixture.
func setupTestEnvWithFixture(t *testing.T, fixtureName string) string {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()

	// Get fixture source path
	fixtureSource := filepath.Join("fixtures", "config-files", fixtureName)

	// Copy fixture to temp directory
	err := copyDir(fixtureSource, tmpDir)
	require.NoError(t, err, "Failed to copy fixture to temp directory")

	return tmpDir
}

// createTestCliApp creates a CLI app with test flags that match Terragrunt's actual flags.
func createTestCliApp() *cli.App {
	app := cli.NewApp()
	app.Name = "terragrunt-test"

	// Add common Terragrunt flags for testing
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "non-interactive",
			Usage: "Run in non-interactive mode",
		},
		&cli.StringFlag{
			Name:  "working-dir",
			Usage: "Set working directory",
		},
		&cli.IntFlag{
			Name:  "parallelism",
			Usage: "Set parallelism level",
		},
		&cli.StringSliceFlag{
			Name:  "terragrunt-config",
			Usage: "Path to terragrunt config files",
		},
	}

	return app
}

// findFlagByName finds a flag in the flags slice by name.
func findFlagByName(flags []cli.Flag, name string) cli.Flag {
	for _, flag := range flags {
		names := flag.Names()
		for _, n := range names {
			if n == name {
				return flag
			}
		}
	}
	return nil
}

// copyDir recursively copies a directory from src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Construct destination path
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			// Create directory
			return os.MkdirAll(dstPath, 0755)
		}

		// Copy file
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstPath, data, info.Mode())
	})
}
