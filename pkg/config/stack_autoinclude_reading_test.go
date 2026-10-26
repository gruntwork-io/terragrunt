package config_test

import (
	"path/filepath"
	"testing"

	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAutoIncludeReadingAttributeGeneratesMarkAsRead verifies that a reading
// attribute in an autoinclude block produces mark_as_read calls in the
// generated terragrunt.autoinclude.hcl, so discovery attributes the reads to
// the unit instead of the stack.
func TestAutoIncludeReadingAttributeGeneratesMarkAsRead(t *testing.T) {
	t.Parallel()

	tplPath := filepath.Join(generationParityLiveDir, "tpl", "msg.tftpl")

	v, err := generateLiveStack(t, config.DefaultStackFile, map[string]string{
		filepath.Join(generationParityLiveDir, config.DefaultStackFile): `
unit "src" {
  source = "` + generationParityUnitSource + `"
  path   = "src"
  values = { content = "world" }
}

unit "render" {
  source = "` + generationParityUnitSource + `"
  path   = "render"

  autoinclude {
    reading = ["` + filepath.ToSlash(tplPath) + `"]

    dependency "src" {
      config_path  = unit.src.path
      mock_outputs = { content = "mock" }
    }

    inputs = {
      content = dependency.src.outputs.content
    }
  }
}
`,
		tplPath: "hello ${v}\n",
	})
	require.NoError(t, err)

	unitDir := filepath.Join(generationParityLiveDir, config.StackDir, "render")
	autoIncludePath := filepath.Join(unitDir, inthclparse.AutoIncludeFile)

	content, readErr := vfs.ReadFile(v.FS, autoIncludePath)
	require.NoError(t, readErr)

	contentStr := string(content)
	assert.Contains(t, contentStr, "mark_as_read", "generated file must contain mark_as_read")
	assert.Contains(t, contentStr, "_autoinclude_reads", "generated file must contain _autoinclude_reads local")
	assert.NotContains(t, contentStr, "reading =", "reading attribute must not appear in generated file")
}

// TestAutoIncludeReadingAttributeTrackedByDiscoveryParse verifies that the
// mark_as_read calls written into the generated autoinclude file are evaluated
// during partial parse, so discovery records the read on the unit.
func TestAutoIncludeReadingAttributeTrackedByDiscoveryParse(t *testing.T) {
	t.Parallel()

	tplPath := filepath.Join(generationParityLiveDir, "tpl", "msg.tftpl")

	v, err := generateLiveStack(t, config.DefaultStackFile, map[string]string{
		filepath.Join(generationParityLiveDir, config.DefaultStackFile): `
unit "src" {
  source = "` + generationParityUnitSource + `"
  path   = "src"
  values = { content = "world" }
}

unit "render" {
  source = "` + generationParityUnitSource + `"
  path   = "render"

  autoinclude {
    reading = ["` + filepath.ToSlash(tplPath) + `"]

    dependency "src" {
      config_path  = unit.src.path
      mock_outputs = { content = "mock" }
    }

    inputs = {
      content = dependency.src.outputs.content
    }
  }
}
`,
		tplPath: "hello ${v}\n",
	})
	require.NoError(t, err)

	unitDir := filepath.Join(generationParityLiveDir, config.StackDir, "render")
	unitConfig := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, unitConfig)
	pctx.FilesRead = config.NewFilesRead()

	_, parseErr := config.PartialParseConfigFile(
		ctx,
		l,
		v,
		pctx,
		unitConfig,
		nil,
	)
	require.NoError(t, parseErr)

	readPaths := pctx.FilesRead.Paths()
	assert.Contains(t, readPaths, tplPath, "template path must be tracked as read on the unit")
}
