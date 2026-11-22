package config_test

import (
	"maps"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestErrorsBlocksReadDependencyInTheRun pins that the parse leaves out an errors block that reads `dependency`,
// running no tofu, and that ResolveRunConfig decodes it with the outputs.
func TestErrorsBlocksReadDependencyInTheRun(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		check func(t *testing.T, errs *config.ErrorsConfig)
		name  string
		body  string
	}{
		{
			name: "retry block",
			body: `
errors {
  retry "transient" {
    retryable_errors   = [dependency.vpc.outputs.id]
    max_attempts       = 2
    sleep_interval_sec = 1
  }
}
`,
			check: func(t *testing.T, errs *config.ErrorsConfig) {
				t.Helper()

				require.Len(t, errs.Retry, 1)
				assert.Equal(t, []string{"vpc-123"}, errs.Retry[0].RetryableErrors)
			},
		},
		{
			name: "ignore block",
			body: `
errors {
  ignore "known" {
    ignorable_errors = [dependency.vpc.outputs.id]
  }
}
`,
			check: func(t *testing.T, errs *config.ErrorsConfig) {
				t.Helper()

				require.Len(t, errs.Ignore, 1)
				assert.Equal(t, []string{"vpc-123"}, errs.Ignore[0].IgnorableErrors)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/live")

			files := maps.Clone(targets)
			files["app/terragrunt.hcl"] = `
dependency "vpc" {
  config_path = "../vpc"
}
` + tc.body

			outputs := map[string]string{filepath.Join(root, "vpc"): targetOutputs["vpc"]}
			stub := &tofuStub{outputs: outputs}
			v := venv.MemVenv().WithFS(memFS(root, files)(t)).WithHandler(stub.handler())
			cfgPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
			ctx, pc := newTestUnitParsingContext(t, cfgPath)
			l := logger.CreateLogger()

			s := new(config.Store)

			parsed, parseErr := s.ResolveUnitConfig(ctx, l, v, pc, cfgPath)
			require.NoError(t, parseErr)
			require.NotNil(t, parsed)
			assert.Nil(t, parsed.Queue.Errors)

			assert.Zero(t, stub.parseExecs.Load(), "the parse ran tofu")

			cfg, runConfigErr := parsed.ResolveRunConfig(ctx, l, v, new(config.Store), pc)
			require.NoError(t, runConfigErr)
			require.NotNil(t, cfg.Errors)
			tc.check(t, cfg.Errors)
		})
	}
}
