package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkGlobAsRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "a.tf"), "")
	writeFile(t, filepath.Join(dir, "b.tf"), "")
	writeFile(t, filepath.Join(dir, "nested", "c.tf"), "")
	writeFile(t, filepath.Join(dir, "README.md"), "")

	l := logger.CreateLogger()
	configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, configPath)
	pctx.WorkingDir = dir
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.MarkManyAsRead))

	// Drive the HCL function via a locals block so we exercise the registered cty wrapper.
	// Brace alternation covers files at the current depth AND deeper, since gobwas's
	// "**" does not collapse the surrounding separators.
	hcl := `locals { matched = mark_glob_as_read("{*.tf,**/*.tf}") }`

	out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	require.NotNil(t, pctx.FilesRead)
	read := *pctx.FilesRead
	assert.Contains(t, read, filepath.Join(dir, "a.tf"))
	assert.Contains(t, read, filepath.Join(dir, "b.tf"))
	assert.Contains(t, read, filepath.Join(dir, "nested", "c.tf"))
	assert.NotContains(t, read, filepath.Join(dir, "README.md"))
}

// TestMarkGlobAsReadEscapesMetacharacter verifies that a backslash-escaped
// metacharacter in the pattern is treated as a literal, not a wildcard. Windows
// cannot create files whose names contain '*', so the test is skipped there.
func TestMarkGlobAsReadEscapesMetacharacter(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("'*' is not a valid character in Windows filenames")
	}

	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "a*b.tf"), "")
	writeFile(t, filepath.Join(dir, "acb.tf"), "")

	l := logger.CreateLogger()
	configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, configPath)
	pctx.WorkingDir = dir
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.MarkManyAsRead))

	// The HCL string literal '"a\\*b.tf"' decodes to 'a\*b.tf', which the glob
	// engine reads as a literal 'a*b.tf'.
	hcl := `locals { matched = mark_glob_as_read("a\\*b.tf") }`

	out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	require.NotNil(t, pctx.FilesRead)
	read := *pctx.FilesRead
	assert.Contains(t, read, filepath.Join(dir, "a*b.tf"))
	assert.NotContains(t, read, filepath.Join(dir, "acb.tf"))
}

func TestMarkGlobAsReadRequiresExperiment(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.tf"), "")

	l := logger.CreateLogger()
	configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, configPath)
	pctx.WorkingDir = dir

	hcl := `locals { matched = mark_glob_as_read("**/*.tf") }`

	_, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mark-many-as-read")

	if pctx.FilesRead != nil {
		assert.NotContains(t, *pctx.FilesRead, filepath.Join(dir, "a.tf"))
	}
}

func TestMarkManyAsReadExperiment(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	moduleDir := filepath.Join(root, "modules", "foo")
	unitDir := filepath.Join(root, "units", "bar")

	writeFile(t, filepath.Join(moduleDir, "main.tf"), "")
	writeFile(t, filepath.Join(moduleDir, "variables.tf.json"), "{}")
	writeFile(t, filepath.Join(moduleDir, "helpers.hcl"), "")
	writeFile(t, filepath.Join(moduleDir, "sub", "nested.tf"), "")
	writeFile(t, filepath.Join(moduleDir, "README.md"), "")
	writeFile(t, filepath.Join(moduleDir, ".terraform.lock.hcl"), "")

	configPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	writeFile(t, configPath, "")

	hcl := `terraform { source = "../../modules/foo" }`

	t.Run("off by default", func(t *testing.T) {
		t.Parallel()

		l := logger.CreateLogger()
		ctx, pctx := newTestParsingContext(t, configPath)
		pctx.WorkingDir = unitDir

		out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
		require.NoError(t, err)
		require.NotNil(t, out)

		if pctx.FilesRead != nil {
			for _, f := range *pctx.FilesRead {
				assert.NotContains(t, f, moduleDir, "module files should not be marked when experiment is off")
			}
		}
	})

	t.Run("on marks source files", func(t *testing.T) {
		t.Parallel()

		l := logger.CreateLogger()
		ctx, pctx := newTestParsingContext(t, configPath)
		pctx.WorkingDir = unitDir
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.MarkManyAsRead))

		out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
		require.NoError(t, err)
		require.NotNil(t, out)
		require.NotNil(t, pctx.FilesRead)

		read := *pctx.FilesRead
		assert.Contains(t, read, filepath.Join(moduleDir, "main.tf"))
		assert.Contains(t, read, filepath.Join(moduleDir, "variables.tf.json"))
		assert.Contains(t, read, filepath.Join(moduleDir, "helpers.hcl"))
		assert.Contains(t, read, filepath.Join(moduleDir, "sub", "nested.tf"))
		assert.Contains(t, read, filepath.Join(moduleDir, ".terraform.lock.hcl"))
		assert.NotContains(t, read, filepath.Join(moduleDir, "README.md"))
	})
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}
