package render_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/render"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderJSON_Basic(t *testing.T) {
	t.Parallel()

	opts, _ := setupTest(t)

	var outputBuffer bytes.Buffer

	opts.Writer = &outputBuffer
	opts.Format = render.FormatJSON
	opts.DisableDependentModules = true
	opts.RenderMetadata = false
	opts.Write = false

	err := render.Run(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	var result map[string]any

	err = json.Unmarshal(outputBuffer.Bytes(), &result)
	require.NoError(t, err)
	assert.NotNil(t, result)

	validateRenderedJSON(t, result, false)
}

func TestRenderJSON_WithMetadata(t *testing.T) {
	t.Parallel()

	opts, _ := setupTest(t)

	var outputBuffer bytes.Buffer

	opts.Writer = &outputBuffer
	opts.Format = render.FormatJSON
	opts.DisableDependentModules = true
	opts.RenderMetadata = true
	opts.Write = false

	err := render.Run(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	var result map[string]any

	err = json.Unmarshal(outputBuffer.Bytes(), &result)
	require.NoError(t, err)
	assert.NotNil(t, result)

	validateRenderedJSON(t, result, true)
}

func TestRenderJSON_WriteToFile(t *testing.T) {
	t.Parallel()

	opts, _ := setupTest(t)
	outputPath := filepath.Join(t.TempDir(), "output.json")
	opts.Format = render.FormatJSON
	opts.DisableDependentModules = true
	opts.RenderMetadata = false
	opts.Write = true
	opts.OutputPath = outputPath

	err := render.Run(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	// Verify the file was created and contains valid JSON
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var result map[string]any

	err = json.Unmarshal(content, &result)
	require.NoError(t, err)
	assert.NotNil(t, result)

	validateRenderedJSON(t, result, false)
}

func TestRenderJSON_InvalidFormat(t *testing.T) {
	t.Parallel()

	opts, _ := setupTest(t)
	opts.Format = "invalid"

	err := render.Run(t.Context(), logger.CreateLogger(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
}

func TestRenderJSON_HCLFormat(t *testing.T) {
	t.Parallel()

	opts, _ := setupTest(t)
	opts.Format = render.FormatHCL

	var renderedBuffer bytes.Buffer

	opts.Writer = &renderedBuffer

	err := render.Run(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	assert.Equal(t, testTerragruntConfigFixture, renderedBuffer.String())
}

// setupTest creates a temporary directory with a terragrunt config file and returns the necessary test setup
func setupTest(t *testing.T) (*render.Options, string) {
	t.Helper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")
	err := os.WriteFile(configPath, []byte(testTerragruntConfigFixture), 0644)
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest(configPath)
	require.NoError(t, err)

	return render.NewOptions(tgOptions), configPath
}

// validateRenderedJSON validates the common JSON structure and values
func validateRenderedJSON(t *testing.T, result map[string]any, withMetadata bool) {
	t.Helper()

	inputs, ok := result["inputs"].(map[string]any)
	require.True(t, ok)

	stringInput := inputs["string_input"]

	if withMetadata {
		data, ok := stringInput.(map[string]any)
		require.True(t, ok)
		assert.NotNil(t, data)

		metadata, ok := data["metadata"].(map[string]any)
		require.True(t, ok)
		assert.NotNil(t, metadata)

		value, ok := data["value"].(string)
		require.True(t, ok)
		assert.Equal(t, "test", value)
	} else {
		assert.Equal(t, "test", stringInput)
	}

	numberInput := inputs["number_input"]

	if withMetadata {
		data, ok := numberInput.(map[string]any)
		require.True(t, ok)
		assert.NotNil(t, data)
	} else {
		assert.InEpsilon(t, float64(42), numberInput, 0.1)
	}

	boolInput := inputs["bool_input"]

	if withMetadata {
		data, ok := boolInput.(map[string]any)
		require.True(t, ok)
		assert.NotNil(t, data)
	} else {
		assert.Equal(t, true, boolInput)
	}

	listInput := inputs["list_input"]

	if withMetadata {
		data, ok := listInput.(map[string]any)
		require.True(t, ok)
		assert.NotNil(t, data)
	} else {
		assert.Equal(t, []any{"item1", "item2"}, listInput)
	}

	mapInput := inputs["map_input"]

	if withMetadata {
		data, ok := mapInput.(map[string]any)
		require.True(t, ok)
		assert.NotNil(t, data)
	} else {
		assert.Equal(t, map[string]any{"key": "value"}, mapInput)
	}
}

const testTerragruntConfigFixture = `terraform {
  source = "test"
}
inputs = {
  bool_input = true
  list_input = ["item1", "item2"]
  map_input = {
    key = "value"
  }
  number_input = 42
  string_input = "test"
}
`
