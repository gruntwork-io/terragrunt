package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTerragruntStackConfig(t *testing.T) {
	t.Parallel()

	cfg := `
locals {
	project = "my-project"
}

unit "unit1" {
	source = "units/app1"
	path   = "unit1"
}

stack "projects" {
	source = "../projects"
	path = "projects"
}

`
	opts := mockOptionsForTest(t)
	ctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), opts)
	terragruntStackConfig, err := config.ReadStackConfigString(ctx, logger.CreateLogger(), opts, config.DefaultStackFile, cfg, nil)
	require.NoError(t, err)

	assert.NotNil(t, terragruntStackConfig)

	assert.NotNil(t, terragruntStackConfig.Locals)
	assert.Len(t, terragruntStackConfig.Locals, 1)
	assert.Equal(t, "my-project", terragruntStackConfig.Locals["project"])

	assert.NotNil(t, terragruntStackConfig.Units)
	assert.Len(t, terragruntStackConfig.Units, 1)

	unit := terragruntStackConfig.Units[0]
	assert.Equal(t, "unit1", unit.Name)
	assert.Equal(t, "units/app1", unit.Source)
	assert.Equal(t, "unit1", unit.Path)
	assert.Nil(t, unit.NoStack)

	assert.NotNil(t, terragruntStackConfig.Stacks)
	assert.Len(t, terragruntStackConfig.Stacks, 1)

	stack := terragruntStackConfig.Stacks[0]
	assert.Equal(t, "projects", stack.Name)
	assert.Equal(t, "../projects", stack.Source)
	assert.Equal(t, "projects", stack.Path)
	assert.Nil(t, stack.NoStack)
}

func TestParseTerragruntStackConfigComplex(t *testing.T) {
	t.Parallel()

	cfg := `
locals {
    project = "my-project"
    env     = "dev"
}

unit "unit1" {
    source = "units/app1"
    path   = "unit1"
    no_dot_terragrunt_stack = true
    values = {
        name = "app1"
        port = 8080
    }
}

unit "unit2" {
    source = "units/app2"
    path   = "unit2"
    no_dot_terragrunt_stack = false
    values = {
        name = "app2"
        port = 9090
    }
}

stack "projects" {
    source = "../projects"
    path = "projects"
    values = {
        region = "us-west-2"
    }
}

stack "network" {
    source = "../network"
    path = "network"
    no_dot_terragrunt_stack = true
}
`
	opts := mockOptionsForTest(t)
	ctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), opts)
	terragruntStackConfig, err := config.ReadStackConfigString(ctx, logger.CreateLogger(), opts, config.DefaultStackFile, cfg, nil)
	require.NoError(t, err)

	// Check that config is not nil
	assert.NotNil(t, terragruntStackConfig)

	assert.NotNil(t, terragruntStackConfig.Locals)
	assert.Len(t, terragruntStackConfig.Locals, 2)
	assert.Equal(t, "my-project", terragruntStackConfig.Locals["project"])
	assert.Equal(t, "dev", terragruntStackConfig.Locals["env"])

	assert.NotNil(t, terragruntStackConfig.Units)
	assert.Len(t, terragruntStackConfig.Units, 2)

	unit1 := terragruntStackConfig.Units[0]
	assert.Equal(t, "unit1", unit1.Name)
	assert.Equal(t, "units/app1", unit1.Source)
	assert.Equal(t, "unit1", unit1.Path)
	assert.NotNil(t, unit1.NoStack)
	assert.True(t, *unit1.NoStack)
	assert.NotNil(t, unit1.Values)

	unit2 := terragruntStackConfig.Units[1]
	assert.Equal(t, "unit2", unit2.Name)
	assert.Equal(t, "units/app2", unit2.Source)
	assert.Equal(t, "unit2", unit2.Path)
	assert.NotNil(t, unit2.NoStack)
	assert.False(t, *unit2.NoStack)
	assert.NotNil(t, unit2.Values)

	assert.NotNil(t, terragruntStackConfig.Stacks)
	assert.Len(t, terragruntStackConfig.Stacks, 2)

	stack1 := terragruntStackConfig.Stacks[0]
	assert.Equal(t, "projects", stack1.Name)
	assert.Equal(t, "../projects", stack1.Source)
	assert.Equal(t, "projects", stack1.Path)
	assert.Nil(t, stack1.NoStack)
	assert.NotNil(t, stack1.Values)

	stack2 := terragruntStackConfig.Stacks[1]
	assert.Equal(t, "network", stack2.Name)
	assert.Equal(t, "../network", stack2.Source)
	assert.Equal(t, "network", stack2.Path)
	assert.NotNil(t, stack2.NoStack)
	assert.True(t, *stack2.NoStack)
}

func TestParseTerragruntStackConfigInvalidSyntax(t *testing.T) {
	t.Parallel()

	invalidCfg := `
locals {
	project = "my-project
}
`
	opts := mockOptionsForTest(t)
	ctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), opts)
	_, err := config.ReadStackConfigString(ctx, logger.CreateLogger(), opts, config.DefaultStackFile, invalidCfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid multi-line string")
}

func TestWriteValuesSortsKeys(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		fileExt     string
		generateCmd string
		jsonFormat  bool
	}{
		{
			name:        "HCL format",
			jsonFormat:  false,
			fileExt:     "hcl",
			generateCmd: "terragrunt stack generate",
		},
		{
			name:        "JSON format",
			jsonFormat:  true,
			fileExt:     "json",
			generateCmd: "terragrunt stack generate --json-values",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			valuesFilePath := setupTestFiles(t, tmpDir, tc.fileExt)

			// Helper function to read and return the values file content
			readValuesFile := func() string {
				content, err := os.ReadFile(valuesFilePath)
				require.NoError(t, err)

				return string(content)
			}

			// Run multiple generations to test for deterministic behavior
			const numIterations = 5
			generationContents := make([]string, 0, numIterations)

			for iteration := range numIterations {
				// Clean up any existing stack directory
				stackDir := filepath.Join(tmpDir, ".terragrunt-stack")
				os.RemoveAll(stackDir)

				// Generate the stack
				_, _, err := helpers.RunTerragruntCommandWithOutput(t, tc.generateCmd+" --working-dir "+tmpDir)
				require.NoError(t, err)
				require.FileExists(t, valuesFilePath)

				content := readValuesFile()
				generationContents = append(generationContents, content)

				t.Logf("Generation %d content:\n%s\n", iteration+1, content)
			}

			// Extract only the complex verification logic to reduce cyclomatic complexity
			verifyDeterministicSortedOutput(t, generationContents)
		})
	}
}

// setupTestFiles creates the test environment and returns the values file path.
func setupTestFiles(t *testing.T, tmpDir string, fileExt string) string {
	t.Helper()

	// Create a test stack configuration with more values in non-alphabetical order
	stackConfig := `
unit "test_unit" {
	source = "./unit"
	path   = "test_unit"
	values = {
		zzz_last    = "should be last"
		aaa_first   = "should be first"
		mmm_middle  = "should be middle"
		beta        = 42
		gamma       = true
		delta       = ["a", "b"]
		zebra       = "animal"
		alpha       = "letter"
		omega       = "end"
		charlie     = "nato"
	}
}
`

	// Create the stack file
	stackFilePath := filepath.Join(tmpDir, "terragrunt.stack.hcl")
	err := os.WriteFile(stackFilePath, []byte(stackConfig), 0644)
	require.NoError(t, err)

	// Create a simple unit directory with minimal terragrunt config
	unitDir := filepath.Join(tmpDir, "unit")
	err = os.MkdirAll(unitDir, 0755)
	require.NoError(t, err)

	unitConfig := `
terraform {
	source = "."
}
`
	unitConfigPath := filepath.Join(unitDir, "terragrunt.hcl")
	err = os.WriteFile(unitConfigPath, []byte(unitConfig), 0644)
	require.NoError(t, err)

	// Create a minimal main.tf in the unit
	mainTf := `
resource "local_file" "test" {
	content  = "test"
	filename = "test.txt"
}
`
	mainTfPath := filepath.Join(unitDir, "main.tf")
	err = os.WriteFile(mainTfPath, []byte(mainTf), 0644)
	require.NoError(t, err)

	return filepath.Join(tmpDir, ".terragrunt-stack", "test_unit", "terragrunt.values."+fileExt)
}

// verifyDeterministicSortedOutput checks that all generations are identical and sorted.
func verifyDeterministicSortedOutput(t *testing.T, generationContents []string) {
	t.Helper()

	// Check if all generations produced identical output
	allIdentical := true
	for i := 1; i < len(generationContents); i++ {
		if generationContents[i] != generationContents[0] {
			allIdentical = false

			break
		}
	}

	if !allIdentical {
		t.Logf("Non-deterministic behavior detected! Generations produced different output:")
		for i, content := range generationContents {
			t.Logf("Generation %d:\n%s\n", i+1, content)
		}
		assert.True(t, allIdentical, "Stack generation should be deterministic - all runs should produce identical values files")

		return
	}

	t.Logf("All generations produced identical output - checking if it's sorted...")

	// Now test the actual content and ordering using the first generation
	contentStr := generationContents[0]

	// Check if the keys appear in alphabetical order
	keys := []string{"aaa_first", "alpha", "beta", "charlie", "delta", "gamma", "mmm_middle", "omega", "zebra", "zzz_last"}

	positions := make([]int, len(keys))
	for i, key := range keys {
		positions[i] = strings.Index(contentStr, key)
		if positions[i] == -1 {
			t.Fatalf("Key %s not found in generated content", key)
		}
	}

	// Check if positions are in ascending order (alphabetical)
	keysInOrder := true
	for i := 1; i < len(positions); i++ {
		if positions[i] < positions[i-1] {
			keysInOrder = false

			break
		}
	}

	t.Logf("Key positions: %v", positions)
	t.Logf("Keys in alphabetical order: %v", keysInOrder)

	if !keysInOrder {
		assert.True(t, keysInOrder, "Keys should appear in alphabetical order for deterministic output")
	} else {
		t.Logf("Keys are in alphabetical order - sorting implementation is working!")
	}
}

func TestReadValuesJSONFile(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTest(t)

	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Create a test JSON values file with comment
	jsonContent := `{
		"_comment": "Auto-generated by the terragrunt.stack.hcl file by Terragrunt. Do not edit manually",
		"project": "test-project",
		"env": "test",
		"count": 3,
		"enabled": true,
		"tags": {
			"Environment": "test",
			"Team": "platform"
		}
	}`

	jsonValuesPath := filepath.Join(tmpDir, "terragrunt.values.json")
	err := os.WriteFile(jsonValuesPath, []byte(jsonContent), 0644)
	require.NoError(t, err)

	// Test reading JSON values
	ctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), opts)
	values, err := config.ReadValues(ctx, logger.CreateLogger(), opts, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, values)

	// Verify the read values
	valueMap := values.AsValueMap()

	projectVal := valueMap["project"]
	assert.Equal(t, "test-project", projectVal.AsString())

	envVal := valueMap["env"]
	assert.Equal(t, "test", envVal.AsString())

	countVal := valueMap["count"]
	countNum, _ := countVal.AsBigFloat().Float64()
	assert.InDelta(t, 3.0, countNum, 0.001)

	enabledVal := valueMap["enabled"]
	assert.True(t, enabledVal.True())
}

func TestReadValuesJSONTakesPrecedenceOverHCL(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTest(t)

	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Create both HCL and JSON values files
	hclContent := `project = "hcl-project"
env = "hcl-env"`

	jsonContent := `{
		"_comment": "Auto-generated by the terragrunt.stack.hcl file by Terragrunt. Do not edit manually",
		"project": "json-project",
		"env": "json-env"
	}`

	hclValuesPath := filepath.Join(tmpDir, "terragrunt.values.hcl")
	err := os.WriteFile(hclValuesPath, []byte(hclContent), 0644)
	require.NoError(t, err)

	jsonValuesPath := filepath.Join(tmpDir, "terragrunt.values.json")
	err = os.WriteFile(jsonValuesPath, []byte(jsonContent), 0644)
	require.NoError(t, err)

	// Test reading values - JSON should take precedence
	ctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), opts)
	values, err := config.ReadValues(ctx, logger.CreateLogger(), opts, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, values)

	// Verify JSON values are used (not HCL)
	valueMap := values.AsValueMap()

	projectVal := valueMap["project"]
	assert.Equal(t, "json-project", projectVal.AsString(), "JSON values should take precedence over HCL")

	envVal := valueMap["env"]
	assert.Equal(t, "json-env", envVal.AsString(), "JSON values should take precedence over HCL")
}

func TestReadJSONValuesWithComment(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTest(t)

	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Create a test JSON values file with comment and verify it's properly handled
	jsonContent := `{
		"_comment": "Auto-generated by the terragrunt.stack.hcl file by Terragrunt. Do not edit manually",
		"project": "test-project",
		"env": "test"
	}`

	jsonValuesPath := filepath.Join(tmpDir, "terragrunt.values.json")
	err := os.WriteFile(jsonValuesPath, []byte(jsonContent), 0644)
	require.NoError(t, err)

	// Test reading JSON values with comment
	ctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), opts)
	values, err := config.ReadValues(ctx, logger.CreateLogger(), opts, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, values)

	// Verify the values are read correctly
	valueMap := values.AsValueMap()

	// The comment field will be present but shouldn't interfere with other values
	commentVal, exists := valueMap["_comment"]
	assert.True(t, exists, "Comment field should be present")
	assert.Equal(t, "Auto-generated by the terragrunt.stack.hcl file by Terragrunt. Do not edit manually", commentVal.AsString())

	// Verify actual values are still correct
	projectVal := valueMap["project"]
	assert.Equal(t, "test-project", projectVal.AsString())

	envVal := valueMap["env"]
	assert.Equal(t, "test", envVal.AsString())
}
