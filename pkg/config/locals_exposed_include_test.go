package config_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPartialParseEvaluatesAnExposedIncludeOnce pins that a partial parse evaluates an exposed include once, whether
// or not the locals of the including config read `include`.
func TestPartialParseEvaluatesAnExposedIncludeOnce(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		locals string
	}{
		{name: "locals that read include", locals: `env = include.root.locals.env`},
		{name: "locals that do not read include", locals: `env = "dev"`},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root(fmt.Sprintf("/exposed-include-locals-%d", i))
			cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)

			var evaluations atomic.Int64

			exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
				evaluations.Add(1)

				return vexec.Result{Stdout: []byte("dev")}
			}, vexec.WithLookPath(func(file string) (string, error) { return "/usr/bin/" + file, nil }))

			v := venv.MemVenv().WithExec(exec).WithFS(venvtest.NewFS(t, root, map[string]string{
				"root.hcl": `
locals {
  env = run_cmd("--terragrunt-quiet", "--terragrunt-no-cache", "mark")
}
`,
				filepath.Join("unit", config.DefaultTerragruntConfigPath): `
include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

locals {
  ` + tc.locals + `
}
`,
			}))

			ctx, pctx := newTestParsingContext(t, cfgPath)
			pctx = pctx.WithDecodeList(config.TerragruntVersionConstraints, config.FeatureFlagsBlock)

			s := new(config.Store)

			_, err := s.ResolvePartialConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)
			require.NoError(t, err)
			assert.Equal(t, int64(1), evaluations.Load())
		})
	}
}
