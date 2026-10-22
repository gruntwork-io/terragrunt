package config_test

import (
	"path/filepath"
	"testing"

	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateStackRejectsAutoIncludeInJSONStackFile pins that a JSON stack file declaring an
// autoinclude fails generation.
func TestGenerateStackRejectsAutoIncludeInJSONStackFile(t *testing.T) {
	t.Parallel()

	const stackFile = config.DefaultStackFile + ".json"

	_, err := generateLiveStack(t, stackFile, map[string]string{
		filepath.Join(generationParityLiveDir, stackFile): `{
  "unit": {
    "app": {
      "source": "` + generationParityUnitSource + `",
      "path": "app",
      "autoinclude": {
        "inputs": { "name": "app" }
      }
    }
  }
}`,
	})

	var stageErr config.AutoIncludeParserStageError

	require.ErrorAs(t, err, &stageErr)
	assert.Equal(t, filepath.Join(generationParityLiveDir, stackFile), stageErr.File)
}

// TestParseConfigRejectsValuesInGeneratedAutoInclude pins that a values attribute in a unit
// autoinclude generates, then fails when the unit is parsed.
func TestParseConfigRejectsValuesInGeneratedAutoInclude(t *testing.T) {
	t.Parallel()

	v, err := generateLiveStack(t, config.DefaultStackFile, map[string]string{
		filepath.Join(generationParityLiveDir, config.DefaultStackFile): `
unit "app" {
  source = "` + generationParityUnitSource + `"
  path   = "app"

  autoinclude {
    values = {
      name = "app"
    }
  }
}
`,
	})
	require.NoError(t, err)

	unitDir := filepath.Join(generationParityLiveDir, config.StackDir, "app")
	unitConfig := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, unitConfig)

	_, err = config.ParseConfigFile(ctx, l, v, pctx, unitConfig, nil)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	require.NotEmpty(t, diags)
	require.NotNil(t, diags[0].Subject)
	assert.Equal(t, filepath.Join(unitDir, inthclparse.AutoIncludeFile), diags[0].Subject.Filename)
}

// TestGenerateStackRejectsUnresolvableInjectedBlock pins the two references a block injected by a
// stack-level autoinclude cannot make, because the injecting stack file evaluates it.
func TestGenerateStackRejectsUnresolvableInjectedBlock(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		injected string
	}{
		{
			name: "autoinclude references a unit of the nested stack",
			injected: `
    unit "extra" {
      source = "` + generationParityUnitSource + `"
      path   = "extra"

      autoinclude {
        dependency "lead" {
          config_path = unit.lead.path
        }
      }
    }
`,
		},
		{
			name: "block declares its own expansion",
			injected: `
    unit "extra" {
      expansion {
        for_each = toset(["a", "b"])
      }

      source = "` + generationParityUnitSource + `"
      path   = "extra-${each.key}"
    }
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, err := generateLiveStack(t, config.DefaultStackFile, map[string]string{
				filepath.Join(generationParityLiveDir, config.DefaultStackFile): `
stack "team" {
  source = "` + generationParityStackSource + `"
  path   = "team"

  autoinclude {` + tc.injected + `  }
}
`,
				filepath.Join(generationParityStackSource, config.DefaultStackFile): `
unit "lead" {
  source = "` + generationParityUnitSource + `"
  path   = "lead"
}
`,
			})

			var diags hcl.Diagnostics

			require.ErrorAs(t, err, &diags)
			require.NotEmpty(t, diags)
			require.NotNil(t, diags[0].Subject)
			assert.Equal(t, config.DefaultStackFile, diags[0].Subject.Filename)

			assert.False(t, vfs.Exists(v.FS, filepath.Join(
				generationParityLiveDir,
				config.StackDir,
				"team",
				config.DefaultAutoIncludeStackFile,
			)))
		})
	}
}

// generateLiveStack writes files into an in-memory filesystem beside a unit source at
// generationParityUnitSource, generates the named stack file in generationParityLiveDir, and
// returns the generation error.
func generateLiveStack(t *testing.T, stackFile string, files map[string]string) (*venv.Venv, error) {
	t.Helper()

	v := venvtest.New()

	for _, name := range []string{"main.tf", config.DefaultTerragruntConfigPath} {
		require.NoError(t, vfs.WriteFile(
			v.FS,
			filepath.Join(generationParityUnitSource, name),
			nil,
			0o644,
		))
	}

	for path, body := range files {
		require.NoError(t, vfs.WriteFile(v.FS, path, []byte(body), 0o644))
	}

	stackPath := filepath.Join(generationParityLiveDir, stackFile)

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, stackPath)

	pctx.TerragruntStackConfigPath = stackPath
	pctx.NoCAS = true

	pool := worker.NewWorkerPool(1)
	pool.Start()

	defer pool.Stop()

	if err := config.GenerateStackFile(ctx, l, v, pctx, pool, stackPath); err != nil {
		return v, err
	}

	return v, pool.Wait()
}
