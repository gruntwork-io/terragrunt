package render_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/render"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderJSON_Basic(t *testing.T) {
	t.Parallel()

	opts, _ := setupTest(t, testTerragruntConfigFixture)

	var outputBuffer bytes.Buffer

	opts.Format = render.FormatJSON
	opts.RenderMetadata = false
	opts.Write = false

	err := render.Run(
		config.WithCaches(t.Context()),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv().WithWriter(&outputBuffer), new(rewrite.Store), opts,
	)
	require.NoError(t, err)

	var result map[string]any

	err = json.Unmarshal(outputBuffer.Bytes(), &result)
	require.NoError(t, err)
	assert.NotNil(t, result)

	validateRenderedJSON(t, result, false)
}

func TestRenderJSON_WithMetadata(t *testing.T) {
	t.Parallel()

	opts, _ := setupTest(t, testTerragruntConfigFixture)

	var outputBuffer bytes.Buffer

	opts.Format = render.FormatJSON
	opts.RenderMetadata = true
	opts.Write = false

	err := render.Run(
		config.WithCaches(t.Context()),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv().WithWriter(&outputBuffer), new(rewrite.Store), opts,
	)
	require.NoError(t, err)

	var result map[string]any

	err = json.Unmarshal(outputBuffer.Bytes(), &result)
	require.NoError(t, err)
	assert.NotNil(t, result)

	validateRenderedJSON(t, result, true)
}

func TestRenderJSON_WriteToFile(t *testing.T) {
	t.Parallel()

	opts, _ := setupTest(t, testTerragruntConfigFixture)
	outputPath := filepath.Join(helpers.TmpDirWOSymlinks(t), "output.json")
	opts.Format = render.FormatJSON
	opts.RenderMetadata = false
	opts.Write = true
	opts.OutputPath = outputPath

	err := render.Run(
		config.WithCaches(t.Context()),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv().WithWriter(io.Discard), new(rewrite.Store), opts,
	)
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

func TestRenderWriteWithoutOutputPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		check    func(t *testing.T, content []byte)
		name     string
		format   string
		filename string
	}{
		{
			name:     "hcl",
			format:   render.FormatHCL,
			filename: "terragrunt.rendered.hcl",
			check: func(t *testing.T, content []byte) {
				t.Helper()

				assert.Equal(t, testTerragruntConfigFixture, string(content))
			},
		},
		{
			name:     "json",
			format:   render.FormatJSON,
			filename: "terragrunt.rendered.json",
			check: func(t *testing.T, content []byte) {
				t.Helper()

				var result map[string]any

				require.NoError(t, json.Unmarshal(content, &result))
				validateRenderedJSON(t, result, false)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, configPath := setupTest(t, testTerragruntConfigFixture)
			opts.Format = tc.format
			opts.Write = true

			err := render.Run(
				config.WithCaches(t.Context()),
				logger.CreateLogger(),
				venvtest.NewOSWithEmptyEnv().WithWriter(io.Discard), new(rewrite.Store), opts,
			)
			require.NoError(t, err)

			content, err := os.ReadFile(filepath.Join(filepath.Dir(configPath), tc.filename))
			require.NoError(t, err)

			tc.check(t, content)
		})
	}
}

func TestRenderWriteFilePermissions(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Windows does not enforce POSIX file permissions")
	}

	testCases := []struct {
		name   string
		format string
	}{
		{
			name:   "hcl",
			format: render.FormatHCL,
		},
		{
			name:   "json",
			format: render.FormatJSON,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			outputPath := filepath.Join(helpers.TmpDirWOSymlinks(t), "rendered")

			assert.Equal(t, os.FileMode(0600), renderToFilePerm(t, tc.format, outputPath))
		})
	}
}

func TestRenderWriteReplacesLooserPermissions(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Windows does not enforce POSIX file permissions")
	}

	outputPath := filepath.Join(helpers.TmpDirWOSymlinks(t), "rendered")
	require.NoError(t, os.WriteFile(outputPath, []byte("stale"), 0644))

	assert.Equal(t, os.FileMode(0600), renderToFilePerm(t, render.FormatJSON, outputPath))

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var result map[string]any

	require.NoError(t, json.Unmarshal(content, &result))
	validateRenderedJSON(t, result, false)
}

// renderToFilePerm renders a config and returns the permissions of the file it writes.
func renderToFilePerm(t *testing.T, format, outputPath string) os.FileMode {
	t.Helper()

	opts, _ := setupTest(t, testTerragruntConfigFixture)
	opts.Format = format
	opts.Write = true
	opts.OutputPath = outputPath

	err := render.Run(
		config.WithCaches(t.Context()),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv().WithWriter(io.Discard), new(rewrite.Store), opts,
	)
	require.NoError(t, err)

	info, err := os.Stat(outputPath)
	require.NoError(t, err)

	return info.Mode().Perm()
}

func TestRenderJSON_InvalidFormat(t *testing.T) {
	t.Parallel()

	opts, _ := setupTest(t, testTerragruntConfigFixture)
	opts.Format = "invalid"

	err := render.Run(
		config.WithCaches(t.Context()),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv().WithWriter(io.Discard), new(rewrite.Store), opts,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
}

func TestRenderJSON_HCLFormat(t *testing.T) {
	t.Parallel()

	opts, _ := setupTest(t, testTerragruntConfigFixture)
	opts.Format = render.FormatHCL

	var renderedBuffer bytes.Buffer

	err := render.Run(
		config.WithCaches(t.Context()),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv().WithWriter(&renderedBuffer), new(rewrite.Store), opts,
	)
	require.NoError(t, err)

	assert.Equal(t, testTerragruntConfigFixture, renderedBuffer.String())
}

func TestRenderJSON_NumberOutOfRange(t *testing.T) {
	t.Parallel()

	opts, configPath := setupTest(t, testExtremeExponentConfigFixture)
	depDir := filepath.Join(filepath.Dir(configPath), "dep")
	require.NoError(t, os.MkdirAll(depDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(depDir, "terragrunt.hcl"), []byte("inputs = {}\n"), 0644))

	opts.Format = render.FormatJSON
	opts.Write = false

	err := render.Run(
		config.WithCaches(t.Context()),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv().WithWriter(io.Discard), new(rewrite.Store), opts,
	)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	require.Len(t, diags, 1)
	assert.Equal(t, "Number out of range", diags[0].Summary)
	assert.Equal(t, 6, diags[0].Subject.Start.Line)
}

func TestRenderAllFilter(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	for _, name := range []string{"a", "b"} {
		unitDir := filepath.Join(tmpDir, name)
		require.NoError(t, os.MkdirAll(unitDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(unitDir, "terragrunt.hcl"),
			[]byte(`inputs = { name = "`+name+`" }`+"\n"),
			0644,
		))
	}

	tgOptions, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	l := logger.CreateLogger()

	tgOptions.WorkingDir = tmpDir
	tgOptions.RunAll = true
	tgOptions.Filters, err = filter.ParseFilterQueries(l, []string{"a"})
	require.NoError(t, err)

	opts := render.NewOptions(tgOptions)
	opts.Format = render.FormatHCL

	var outputBuffer bytes.Buffer

	err = render.Run(
		config.WithCaches(t.Context()),
		l,
		venvtest.NewOSWithEmptyEnv().WithWriter(&outputBuffer), new(rewrite.Store), opts,
	)
	require.NoError(t, err)

	assert.Equal(t, "inputs = {\n  name = \"a\"\n}\n", outputBuffer.String())
}

func TestRenderAllDiscoveryBoundaryMustExist(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	tgOptions, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	tgOptions.WorkingDir = tmpDir
	tgOptions.RunAll = true
	tgOptions.DiscoveryBoundary = filepath.Join(tmpDir, "missing")

	err = render.Run(
		config.WithCaches(t.Context()),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv(), new(rewrite.Store), render.NewOptions(tgOptions),
	)

	var boundaryErr discovery.DiscoveryBoundaryDirError

	require.ErrorAs(t, err, &boundaryErr)
}

func TestRenderAllGitFilterReadsUnchangedIncludes(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	runner := helpers.InitTestGitRunner(t, tmpDir)

	unitDir := filepath.Join(tmpDir, "a")
	unitPath := filepath.Join(unitDir, "terragrunt.hcl")
	unitConfig := func(name string) []byte {
		return []byte(`include "root" {
  path = find_in_parent_folders("root.hcl")
}

inputs = { name = "` + name + `" }
`)
	}

	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "root.hcl"), []byte(`inputs = { env = "prod" }`+"\n"), 0644))
	require.NoError(t, os.WriteFile(unitPath, unitConfig("before"), 0644))
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Add unit"))

	require.NoError(t, os.WriteFile(unitPath, unitConfig("after"), 0644))
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Change unit"))

	l := logger.CreateLogger()

	tgOptions, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	tgOptions.WorkingDir = tmpDir
	tgOptions.RunAll = true
	tgOptions.Filters, err = filter.ParseFilterQueries(l, []string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	opts := render.NewOptions(tgOptions)
	opts.Format = render.FormatJSON

	var outputBuffer bytes.Buffer

	require.NoError(t, render.Run(
		config.WithCaches(t.Context()),
		l,
		venvtest.NewOSWithEmptyEnv().WithWriter(&outputBuffer), new(rewrite.Store), opts,
	))

	var rendered struct {
		Inputs map[string]string `json:"inputs"`
	}

	require.NoError(t, json.Unmarshal(outputBuffer.Bytes(), &rendered))
	assert.Equal(t, map[string]string{"env": "prod", "name": "after"}, rendered.Inputs)

	worktrees, err := os.ReadDir(filepath.Join(tmpDir, ".git", "worktrees"))
	if !errors.Is(err, fs.ErrNotExist) {
		require.NoError(t, err)
		assert.Empty(t, worktrees, "render removes the worktrees it checked out")
	}
}

// setupTest writes config to a terragrunt.hcl in a fresh temporary directory
func setupTest(t *testing.T, config string) (*render.Options, string) {
	t.Helper()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")
	err := os.WriteFile(configPath, []byte(config), 0644)
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

const testExtremeExponentConfigFixture = `dependency "dep" {
  config_path  = "./dep"
  skip_outputs = true

  mock_outputs = {
    count = 9E9999999
  }
}
`
