package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

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

	// Drive the HCL function via a locals block so we exercise the registered cty wrapper.
	// Brace alternation covers files at the current depth AND deeper, since gobwas's
	// "**" does not collapse the surrounding separators.
	hcl := `locals { matched = mark_glob_as_read("{*.tf,**/*.tf}") }`

	out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	require.NotNil(t, pctx.FilesRead)
	// mark_glob_as_read resolves through glob.Expand, which returns forward-slash paths, so
	// compare against forward-slash expectations.
	read := pctx.FilesRead.Paths()
	assert.Contains(t, read, filepath.ToSlash(filepath.Join(dir, "a.tf")))
	assert.Contains(t, read, filepath.ToSlash(filepath.Join(dir, "b.tf")))
	assert.Contains(t, read, filepath.ToSlash(filepath.Join(dir, "nested", "c.tf")))
	assert.NotContains(t, read, filepath.ToSlash(filepath.Join(dir, "README.md")))
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

	// The HCL string literal '"a\\*b.tf"' decodes to 'a\*b.tf', which the glob
	// engine reads as a literal 'a*b.tf'.
	hcl := `locals { matched = mark_glob_as_read("a\\*b.tf") }`

	out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	require.NotNil(t, pctx.FilesRead)
	read := pctx.FilesRead.Paths()
	assert.Contains(t, read, filepath.Join(dir, "a*b.tf"))
	assert.NotContains(t, read, filepath.Join(dir, "acb.tf"))
}

// TestMarkManyAsReadMarksModuleSourceFilesByDefault pins the default behavior:
// a full parse of a config with a local terraform source marks the module's
// configuration files as read, with no experiment flag required.
func TestMarkManyAsReadMarksModuleSourceFilesByDefault(t *testing.T) {
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

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, configPath)
	pctx.WorkingDir = unitDir

	out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, pctx.FilesRead)

	read := pctx.FilesRead.Paths()
	assert.Contains(t, read, filepath.Join(moduleDir, "main.tf"))
	assert.Contains(t, read, filepath.Join(moduleDir, "variables.tf.json"))
	assert.Contains(t, read, filepath.Join(moduleDir, "helpers.hcl"))
	assert.Contains(t, read, filepath.Join(moduleDir, "sub", "nested.tf"))
	assert.Contains(t, read, filepath.Join(moduleDir, ".terraform.lock.hcl"))
	assert.NotContains(t, read, filepath.Join(moduleDir, "README.md"))
}

// TestMarkManyAsReadRelativeConfigPathAnchorsToWorkingDir pins that a relative
// config path resolves against the parsing context's working directory before
// the module walk. The file detector roots relative paths at "/", so without
// anchoring, a relative config path would send the walk to the filesystem root.
func TestMarkManyAsReadRelativeConfigPathAnchorsToWorkingDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	moduleDir := filepath.Join(root, "modules", "foo")
	unitDir := filepath.Join(root, "units", "bar")

	writeFile(t, filepath.Join(moduleDir, "main.tf"), "")

	configPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	writeFile(t, configPath, "")

	hcl := `terraform { source = "../../modules/foo" }`

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, configPath)
	pctx.WorkingDir = unitDir

	out, err := config.ParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, pctx.FilesRead)

	assert.Contains(t, pctx.FilesRead.Paths(), filepath.Join(moduleDir, "main.tf"))
}

func TestMarkManyAsReadPartialParseSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	moduleDir := filepath.Join(root, "modules", "foo")
	unitDir := filepath.Join(root, "units", "bar")

	writeFile(t, filepath.Join(moduleDir, "main.tf"), "")
	writeFile(t, filepath.Join(moduleDir, "variables.tf.json"), "{}")
	writeFile(t, filepath.Join(moduleDir, "README.md"), "")

	configPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	hcl := `terraform { source = "../../modules/foo" }`
	writeFile(t, configPath, hcl)

	for _, decode := range []config.PartialDecodeSectionType{config.TerraformSource, config.TerraformBlock} {
		t.Run(fmt.Sprintf("decode_%d", decode), func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			ctx, pctx := newTestParsingContext(t, configPath)
			pctx.WorkingDir = unitDir
			pctx = pctx.WithDecodeList(decode)

			out, err := config.PartialParseConfigString(ctx, pctx, l, configPath, hcl, nil)
			require.NoError(t, err)
			require.NotNil(t, out)
			require.NotNil(t, pctx.FilesRead)

			read := pctx.FilesRead.Paths()
			assert.Contains(t, read, filepath.Join(moduleDir, "main.tf"))
			assert.Contains(t, read, filepath.Join(moduleDir, "variables.tf.json"))
			assert.NotContains(t, read, filepath.Join(moduleDir, "README.md"))
		})
	}
}

// TestMarkManyAsReadPartialParseIncludedSource pins that the partial-parse hook
// still fires when the terraform source comes from an included parent rather
// than the leaf unit itself. The check runs after handleInclude, so
// output.Terraform is populated from the merged parent by the time the hook runs.
func TestMarkManyAsReadPartialParseIncludedSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	moduleDir := filepath.Join(root, "modules", "foo")
	unitDir := filepath.Join(root, "units", "bar")

	writeFile(t, filepath.Join(moduleDir, "main.tf"), "")
	writeFile(t, filepath.Join(moduleDir, "variables.tf.json"), "{}")
	writeFile(t, filepath.Join(moduleDir, "README.md"), "")

	// Absolute source in the parent sidesteps relative-path resolution, which
	// markLocalModuleSourceAsRead anchors to the leaf unit's directory.
	parentPath := filepath.Join(root, "root.hcl")
	writeFile(t, parentPath, fmt.Sprintf(`terraform { source = %q }`, moduleDir))

	childPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	writeFile(t, childPath, fmt.Sprintf(`include "root" { path = %q }`, parentPath))

	for _, decode := range []config.PartialDecodeSectionType{config.TerraformSource, config.TerraformBlock} {
		t.Run(fmt.Sprintf("decode_%d", decode), func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			ctx, pctx := newTestParsingContext(t, childPath)
			pctx.WorkingDir = unitDir
			pctx = pctx.WithDecodeList(decode)

			out, err := config.PartialParseConfigFile(ctx, pctx, l, childPath, nil)
			require.NoError(t, err)
			require.NotNil(t, out)
			require.NotNil(t, pctx.FilesRead)

			read := pctx.FilesRead.Paths()
			assert.Contains(t, read, filepath.Join(moduleDir, "main.tf"))
			assert.Contains(t, read, filepath.Join(moduleDir, "variables.tf.json"))
			assert.NotContains(t, read, filepath.Join(moduleDir, "README.md"))
		})
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}
