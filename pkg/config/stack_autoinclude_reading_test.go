package config_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoIncludeMarkAsReadTrackedOnGeneratedUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
	}{
		{name: "absolute path", path: `"${get_terragrunt_dir()}/tpl/msg.tftpl"`},
		{name: "path relative to the stack file", path: `"tpl/msg.tftpl"`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tplPath := filepath.Join(generationParityLiveDir, "tpl", "msg.tftpl")
			contentPath := filepath.Join(generationParityLiveDir, "content.txt")

			v, err := generateLiveStack(t, config.DefaultStackFile, map[string]string{
				filepath.Join(generationParityLiveDir, config.DefaultStackFile): `
unit "src" {
  source = "` + generationParityUnitSource + `"
  path   = "src"
  values = { content = file("content.txt") }
}

unit "render" {
  source = "` + generationParityUnitSource + `"
  path   = "render"

  autoinclude {
    dependency "src" {
      config_path  = unit.src.path
      mock_outputs = { content = "mock" }
    }

    inputs = {
      content = templatefile(mark_as_read(` + tc.path + `), { v = dependency.src.outputs.content })
    }
  }
}
`,
				tplPath:     "hello ${v}\n",
				contentPath: "world",
			})
			require.NoError(t, err)

			l := logger.CreateLogger()

			unitConfig := filepath.Join(
				generationParityLiveDir,
				config.StackDir,
				"render",
				config.DefaultTerragruntConfigPath,
			)
			ctx, pctx := newTestParsingContext(t, unitConfig)
			pctx = pctx.WithFileReadTracking()

			_, err = config.PartialParseConfigFile(ctx, l, v, pctx, unitConfig, nil)
			require.NoError(t, err)
			assert.Contains(t, pctx.FilesRead.Paths(), tplPath)

			stackConfig := filepath.Join(generationParityLiveDir, config.DefaultStackFile)
			stackCtx, stackPctx := newTestParsingContext(t, stackConfig)
			stackPctx = stackPctx.WithFileReadTracking()

			_, err = config.ReadStackConfigFile(stackCtx, l, v, stackPctx, stackConfig, nil)
			require.NoError(t, err)
			require.Contains(t, stackPctx.FilesRead.Paths(), contentPath)
			assert.NotContains(t, stackPctx.FilesRead.Paths(), tplPath)
		})
	}
}
