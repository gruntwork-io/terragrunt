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

	tmpDir := t.TempDir()
	valuesFilePath := setupTestFiles(t, tmpDir)

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
		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+tmpDir)
		require.NoError(t, err)
		require.FileExists(t, valuesFilePath)

		content := readValuesFile()
		generationContents = append(generationContents, content)

		t.Logf("Generation %d content:\n%s\n", iteration+1, content)
	}

	// Extract only the complex verification logic to reduce cyclomatic complexity
	verifyDeterministicSortedOutput(t, generationContents)
}

// setupTestFiles creates the test environment and returns the values file path.
func setupTestFiles(t *testing.T, tmpDir string) string {
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
	stackFilePath := filepath.Join(tmpDir, config.DefaultStackFile)
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
	unitConfigPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
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

	return filepath.Join(tmpDir, ".terragrunt-stack", "test_unit", "terragrunt.values.hcl")
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

func TestStackGenerationWithNestedTopologyWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	setupNestedStackFixture(t, tmpDir)

	liveDir := filepath.Join(tmpDir, "live")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+liveDir)
	require.NoError(t, err)

	stackDir := filepath.Join(liveDir, ".terragrunt-stack")
	require.DirExists(t, stackDir)

	foundFiles := findStackFiles(t, liveDir)
	require.NotEmpty(t, foundFiles, "Expected to find generated stack files")

	l := logger.CreateLogger()
	topology := config.BuildStackTopology(l, foundFiles, liveDir)
	require.NotEmpty(t, topology, "Expected non-empty topology")

	levelCounts := make(map[int]int)
	for _, node := range topology {
		levelCounts[node.Level]++
	}

	t.Logf("Topology levels found: %v", levelCounts)

	assert.Len(t, levelCounts, 3, "Expected levels in nested topology")

	assert.Equal(t, 1, levelCounts[0], "Level 0 should have exactly 1 stack file")
	assert.Equal(t, 3, levelCounts[1], "Level 1 should have exactly 3 stack files")
	assert.Equal(t, 9, levelCounts[2], "Level 2 should have exactly 9 stack files")

	verifyGeneratedUnits(t, stackDir)

	// Run one more time just to be sure things don't break when running in a dirty directory
	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+liveDir)
	require.NoError(t, err)
}

// setupNestedStackFixture creates a test fixture similar to testing-nested-stacks
func setupNestedStackFixture(t *testing.T, tmpDir string) {
	t.Helper()

	liveDir := filepath.Join(tmpDir, "live")
	stacksDir := filepath.Join(tmpDir, "stacks")
	unitsDir := filepath.Join(tmpDir, "units")

	require.NoError(t, os.MkdirAll(liveDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(stacksDir, "foo"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(stacksDir, "final"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(unitsDir, "final"), 0755))

	liveStackConfig := `stack "foo" {
  source = "../stacks/foo"
  path   = "foo"
}

stack "foo2" {
  source = "../stacks/foo"
  path   = "foo2"
}

stack "foo3" {
  source = "../stacks/foo"
  path   = "foo3"
}
`
	liveStackPath := filepath.Join(liveDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(liveStackPath, []byte(liveStackConfig), 0644))

	fooStackConfig := `locals {
  final_stack = find_in_parent_folders("stacks/final")
}

stack "final" {
  source = local.final_stack
  path   = "final"
}

stack "final2" {
  source = local.final_stack
  path   = "final2"
}

stack "final3" {
  source = local.final_stack
  path   = "final3"
}
`
	fooStackPath := filepath.Join(stacksDir, "foo", config.DefaultStackFile)
	require.NoError(t, os.WriteFile(fooStackPath, []byte(fooStackConfig), 0644))

	finalStackConfig := `locals {
  final_unit = find_in_parent_folders("units/final")
}

unit "final" {
  source = local.final_unit
  path   = "final"
}
`
	finalStackPath := filepath.Join(stacksDir, "final", config.DefaultStackFile)
	require.NoError(t, os.WriteFile(finalStackPath, []byte(finalStackConfig), 0644))

	finalUnitPath := filepath.Join(unitsDir, "final", config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(finalUnitPath, []byte(``), 0644))

	finalMainTfPath := filepath.Join(unitsDir, "final", "main.tf")
	require.NoError(t, os.WriteFile(finalMainTfPath, []byte(``), 0644))
}

// verifyGeneratedUnits checks that some units were generated correctly
func verifyGeneratedUnits(t *testing.T, stackDir string) {
	t.Helper()

	var (
		unitDirs  []string
		stackDirs []string
	)

	err := filepath.WalkDir(stackDir, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.Name() == "terragrunt.hcl" {
			unitDir := filepath.Dir(path)
			unitDirs = append(unitDirs, unitDir)
		}

		if !info.IsDir() && info.Name() == "terragrunt.stack.hcl" {
			stackDir := filepath.Dir(path)
			stackDirs = append(stackDirs, stackDir)
		}

		return nil
	})
	require.NoError(t, err)

	require.Len(t, unitDirs, 9, "Expected exactly 9 generated units")
	require.Len(t, stackDirs, 12, "Expected exactly 12 generated stacks")
}

// findStackFiles recursively finds all terragrunt.stack.hcl files in a directory
func findStackFiles(t *testing.T, dir string) []string {
	t.Helper()

	var stackFiles []string
	err := filepath.WalkDir(dir, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if strings.HasSuffix(path, "terragrunt.stack.hcl") {
			stackFiles = append(stackFiles, path)
		}

		return nil
	})

	require.NoError(t, err)
	return stackFiles
}
