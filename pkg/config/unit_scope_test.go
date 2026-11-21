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

// TestQueuePartsCannotReadDependency pins that a queue part which reads `dependency` fails in the parse, and that
// ToV1 reports the same failure, since the parse decodes those parts once with no `dependency` in scope.
func TestQueuePartsCannotReadDependency(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		body string
	}{
		{
			name: "errors blocks",
			body: `
errors {
  retry "transient" {
    retryable_errors   = [dependency.vpc.outputs.id]
    max_attempts       = 2
    sleep_interval_sec = 1
  }
}
`,
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

			stub := &tofuStub{}
			v := venv.MemVenv().WithFS(memFS(root, files)(t)).WithHandler(stub.handler())
			cfgPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
			ctx, pctx := newTestUnitParsingContext(t, cfgPath)
			l := logger.CreateLogger()

			pc := pctx

			s := new(config.Store)

			parsed, parseErr := s.ResolveUnitConfig(ctx, l, v, pc, cfgPath)
			require.Error(t, parseErr)
			assert.Contains(t, parseErr.Error(), "Unknown variable")
			assert.Contains(t, parseErr.Error(), config.MetadataDependency)
			require.NotNil(t, parsed)

			assert.Zero(t, stub.parseExecs.Load(), "the parse ran tofu")

			_, toV1Err := parsed.ToV1(ctx, l, v, new(config.Store), pc)
			require.Error(t, toV1Err)
			assert.Contains(t, toV1Err.Error(), "Unknown variable")
		})
	}
}
