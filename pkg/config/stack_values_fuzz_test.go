package config_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

var valuesFuzzStackDir = venvtest.Root("/virtual/values-fuzz")

// FuzzStackGenerateValuesKeys pins that stack generate writes a top-level values key that is an HCL identifier to
// terragrunt.values.hcl and reads it back unchanged, and rejects any other key with an "Invalid values key"
// diagnostic before writing anything.
func FuzzStackGenerateValuesKeys(f *testing.F) {
	for _, seed := range []string{
		"for", "if", "true", "a b", "a-b", "_x", "1x", "", "é", "é", "${x}", "%{x}", "a\nb", `"`, `\`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, key string) {
		// An HCL file holds only UTF-8, so no values key can be anything else.
		if !utf8.ValidString(key) {
			t.Skip()
		}

		quoted := string(hclwrite.TokensForValue(cty.StringVal(key)).Bytes())
		normalized := cty.StringVal(key).AsString()

		forms := map[string]string{
			"literal unit key": fmt.Sprintf(`
unit "app" {
  source = "./units/app"
  path   = "app"
  values = { %s = "v" }
}
`, quoted),
			"computed unit key": fmt.Sprintf(`
locals {
  k = %s
}

unit "app" {
  source = "./units/app"
  path   = "app"
  values = { (local.k) = "v" }
}
`, quoted),
			"literal stack key": fmt.Sprintf(`
stack "app" {
  source = "./stacks/team"
  path   = "app"
  values = { %s = "v" }
}
`, quoted),
		}

		for name, stackHCL := range forms {
			v := venv.MemVenv()
			valuesPath := filepath.Join(valuesFuzzStackDir, config.StackDir, "app", "terragrunt.values.hcl")

			err := generateValuesFuzzStack(t, v, stackHCL)

			if !hclsyntax.ValidIdentifier(normalized) {
				var diags hcl.Diagnostics

				require.ErrorAs(t, err, &diags, name)
				require.Len(t, diags, 1, name)
				assert.Equal(t, "Invalid values key", diags[0].Summary, name)
				assert.False(t, vfs.IsFile(v.FS, valuesPath), name)

				continue
			}

			require.NoError(t, err, name)

			ctx, pctx := newTestParsingContext(t, valuesPath)

			l := logger.CreateLogger()
			unit := emptyUnitConfig(t, l, v, pctx, filepath.Dir(valuesPath))

			values, err := config.ReadValues(ctx, l, v, pctx, unit, config.ComponentConfig)
			require.NoError(t, err, name)
			require.NotNil(t, values, name)
			assert.Equal(t, map[string]cty.Value{normalized: cty.StringVal("v")}, values.AsValueMap(), name)
		}
	})
}

// generateValuesFuzzStack writes stackHCL with a local unit and stack source into v, and returns the error from
// generating it.
func generateValuesFuzzStack(t *testing.T, v *venv.Venv, stackHCL string) error {
	t.Helper()

	files := map[string]string{
		filepath.Join("units", "app", "main.tf"):                          "",
		filepath.Join("units", "app", config.DefaultTerragruntConfigPath): "",
		filepath.Join("stacks", "team", config.DefaultStackFile):          "",
		config.DefaultStackFile:                                           stackHCL,
	}

	for path, body := range files {
		require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(valuesFuzzStackDir, path), []byte(body), 0o644))
	}

	stackPath := filepath.Join(valuesFuzzStackDir, config.DefaultStackFile)

	ctx, pctx := newTestParsingContext(t, stackPath)
	pctx.TerragruntStackConfigPath = stackPath
	pctx.NoCAS = true

	pool := worker.NewWorkerPool(1)
	pool.Start()

	defer pool.Stop()

	genErr := config.GenerateStackFile(ctx, logger.CreateLogger(), v, pctx, pool, stackPath)

	return errors.Join(genErr, pool.Wait())
}
