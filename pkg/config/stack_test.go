package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctyjson "github.com/zclconf/go-cty/cty/json"
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
	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	terragruntStackConfig, err := config.ReadStackConfigString(
		ctx,
		logger.CreateLogger(),
		v,
		new(config.Store),
		pctx,
		config.DefaultStackFile,
		cfg,
	)
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
	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	terragruntStackConfig, err := config.ReadStackConfigString(
		ctx,
		logger.CreateLogger(),
		v,
		new(config.Store),
		pctx,
		config.DefaultStackFile,
		cfg,
	)
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
	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	_, err := config.ReadStackConfigString(
		ctx,
		logger.CreateLogger(),
		v,
		new(config.Store),
		pctx,
		config.DefaultStackFile,
		invalidCfg,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid multi-line string")
}

func TestWriteValuesSortsKeys(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
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
		_, _, err := helpers.RunTerragruntCommandWithOutput(
			t,
			"terragrunt stack generate --working-dir "+tmpDir,
		)
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

		assert.True(
			t,
			allIdentical,
			"Stack generation should be deterministic - all runs should produce identical values files",
		)

		return
	}

	t.Logf("All generations produced identical output - checking if it's sorted...")

	// Now test the actual content and ordering using the first generation
	contentStr := generationContents[0]

	// Check if the keys appear in alphabetical order
	keys := []string{
		"aaa_first",
		"alpha",
		"beta",
		"charlie",
		"delta",
		"gamma",
		"mmm_middle",
		"omega",
		"zebra",
		"zzz_last",
	}

	positions := make([]int, len(keys))
	for i, key := range keys {
		positions[i] = strings.Index(contentStr, key)
		require.NotEqual(t, -1, positions[i], "Key %s not found in generated content", key)
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
		assert.True(
			t,
			keysInOrder,
			"Keys should appear in alphabetical order for deterministic output",
		)
	} else {
		t.Logf("Keys are in alphabetical order - sorting implementation is working!")
	}
}

func TestWriteValuesSkipsWhenNilOrNull(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create two units: one without values, one with explicit null values
	stackConfig := `
unit "u1" {
  source = "./unit1"
  path   = "u1"
}

unit "u2" {
  source = "./unit2"
  path   = "u2"
  values = null
}
`

	stackFilePath := filepath.Join(tmpDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(stackFilePath, []byte(stackConfig), 0644))

	// Unit 1
	unit1Dir := filepath.Join(tmpDir, "unit1")
	require.NoError(t, os.MkdirAll(unit1Dir, 0755))

	unit1Config := `
terraform {
  source = "."
}
`
	unit1ConfigPath := filepath.Join(unit1Dir, config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(unit1ConfigPath, []byte(unit1Config), 0644))

	unit1MainTf := ""
	require.NoError(t, os.WriteFile(filepath.Join(unit1Dir, "main.tf"), []byte(unit1MainTf), 0644))

	// Unit 2
	unit2Dir := filepath.Join(tmpDir, "unit2")
	require.NoError(t, os.MkdirAll(unit2Dir, 0755))

	unit2Config := unit1Config
	unit2ConfigPath := filepath.Join(unit2Dir, config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(unit2ConfigPath, []byte(unit2Config), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(unit2Dir, "main.tf"), []byte(unit1MainTf), 0644))

	// Generate the stack
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+tmpDir,
	)
	require.NoError(t, err)

	// Ensure values files are not created for both units
	valuesU1 := filepath.Join(tmpDir, ".terragrunt-stack", "u1", "terragrunt.values.hcl")
	valuesU2 := filepath.Join(tmpDir, ".terragrunt-stack", "u2", "terragrunt.values.hcl")

	assert.NoFileExists(t, valuesU1)
	assert.NoFileExists(t, valuesU2)
}

func TestWriteValuesRejectsNonObjectValues(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	stackConfig := `
unit "bad" {
  source = "./unit"
  path   = "bad"
  values = 666
}
`

	stackFilePath := filepath.Join(tmpDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(stackFilePath, []byte(stackConfig), 0644))

	unitDir := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0755))

	unitConfig := `
terraform {
  source = "."
}
`
	unitConfigPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(unitConfigPath, []byte(unitConfig), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "main.tf"), []byte(""), 0644))

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+tmpDir,
	)
	require.Error(
		t,
		err,
		"expected error when values is non-object, got none. stdout=%s stderr=%s",
		stdout,
		stderr,
	)

	combined := stdout + "\n" + stderr + "\n" + err.Error()
	assert.Contains(t, combined, "expected object or map")
}

func TestWriteValuesWithKeywordKeysReadsBack(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	stackConfig := `
unit "app" {
  source = "./unit"
  path   = "app"
  values = {
    "for" = "top"
    "a-b" = "dash"
    m     = { "for" = "x", zone = "y" }
    list  = [{ "for" = "x" }, [{ "for" = { "for" = "y" } }]]
    keys  = { "a b" = "space", "a-b" = "dash", "if" = "keyword" }
  }
}
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, config.DefaultStackFile), []byte(stackConfig), 0644))

	unitDir := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, config.DefaultTerragruntConfigPath), []byte(""), 0644))

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+tmpDir)
	require.NoError(t, err)

	l := logger.CreateLogger()
	v := venvtest.NewWithOSFS()
	pctx := config.NewParsingContext()

	values, err := config.ReadValues(
		config.WithCaches(t.Context()),
		l,
		v,
		new(config.Store),
		pctx,
		emptyUnitConfig(t, l, v, pctx, filepath.Join(tmpDir, ".terragrunt-stack", "app")),
		config.ComponentConfig,
	)
	require.NoError(t, err)
	require.NotNil(t, values)

	encoded, err := ctyjson.Marshal(*values, values.Type())
	require.NoError(t, err)

	assert.JSONEq(t, `{
  "for": "top",
  "a-b": "dash",
  "m": {"for": "x", "zone": "y"},
  "list": [{"for": "x"}, [{"for": {"for": "y"}}]],
  "keys": {"a b": "space", "a-b": "dash", "if": "keyword"}
}`, string(encoded))
}

func TestStackGenerateRejectsValuesKeysThatAreNotIdentifiers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		values     string
		block      string
		sourceFile string
		line       int
	}{
		{
			name:       "literal key",
			block:      "unit",
			sourceFile: config.DefaultTerragruntConfigPath,
			values:     "{\n    \"a b\" = \"space\"\n  }",
			line:       6,
		},
		{
			name:       "computed key",
			block:      "unit",
			sourceFile: config.DefaultTerragruntConfigPath,
			values:     `merge({ "a b" = "space" }, {})`,
			line:       5,
		},
		{
			name:       "stack block",
			block:      "stack",
			sourceFile: config.DefaultStackFile,
			values:     "{\n    \"a b\" = \"space\"\n  }",
			line:       6,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

			stackConfig := fmt.Sprintf(`
%s "app" {
  source = "./app"
  path   = "app"
  values = %s
}
`, tc.block, tc.values)
			stackFile := filepath.Join(tmpDir, config.DefaultStackFile)
			require.NoError(t, os.WriteFile(stackFile, []byte(stackConfig), 0644))

			appDir := filepath.Join(tmpDir, "app")
			require.NoError(t, os.MkdirAll(appDir, 0755))
			require.NoError(t, os.WriteFile(filepath.Join(appDir, tc.sourceFile), []byte(""), 0644))

			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+tmpDir)

			var diags hcl.Diagnostics

			require.ErrorAs(t, err, &diags)
			require.Len(t, diags, 1)
			assert.Equal(t, "Invalid values key", diags[0].Summary)
			require.NotNil(t, diags[0].Subject)
			assert.Equal(t, stackFile, diags[0].Subject.Filename)
			assert.Equal(t, tc.line, diags[0].Subject.Start.Line)
			assert.NoDirExists(t, filepath.Join(tmpDir, ".terragrunt-stack", "app"))
		})
	}
}
