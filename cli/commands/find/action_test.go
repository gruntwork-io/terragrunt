package find

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Parallel()

	// Setup test directory
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

	tgOpts := options.NewTerragruntOptions()
	tgOpts.WorkingDir = tmpDir

	// Create a single test case first
	opts := NewOptions(tgOpts)

	// Create a pipe to capture output
	r, w, err := os.Pipe()
	require.NoError(t, err)

	// Set the writer in options
	opts.Writer = w

	err = Run(context.Background(), opts)
	require.NoError(t, err)

	// Close the write end of the pipe
	w.Close()

	// Read all output
	output, err := io.ReadAll(r)
	require.NoError(t, err)

	// Verify output contains expected paths
	outputStr := string(output)
	expectedPaths := []string{"unit1", "unit2", "nested/unit4", "stack1"}
	for _, path := range expectedPaths {
		assert.Contains(t, outputStr, path)
	}
}

func TestColorizer(t *testing.T) {
	t.Parallel()

	colorizer := NewColorizer()

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
			result := colorizer.colorize(tt.config)
			assert.NotEmpty(t, result)

			// Test that different types produce different colorized outputs
			for _, diffType := range tt.shouldBeDifferent {
				diffConfig := &discovery.DiscoveredConfig{
					Type: diffType,
					Path: tt.config.Path,
				}
				diffResult := colorizer.colorize(diffConfig)
				assert.NotEqual(t, result, diffResult)
			}
		})
	}
}
