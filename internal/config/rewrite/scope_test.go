package config_test

import (
	"maps"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
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
			name: "exclude condition",
			body: `
exclude {
  if      = dependency.vpc.outputs.id == "vpc-123"
  actions = ["plan"]
}
`,
		},
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
			v := venvtest.New().WithFS(memFS(root, files)(t)).WithHandler(stub.handler())
			cfgPath := filepath.Join(root, "app", pkgconfig.DefaultTerragruntConfigPath)
			ctx, pctx := newTestParsingContext(t, cfgPath)
			l := logger.CreateLogger()

			parsed, parseErr := config.ParseConfigFile(
				ctx,
				l,
				v,
				&hclparse.Store{},
				config.NewParseContext(pctx),
				cfgPath,
			)
			require.Error(t, parseErr)
			assert.Contains(t, parseErr.Error(), "Unknown variable")
			assert.Contains(t, parseErr.Error(), pkgconfig.MetadataDependency)
			require.NotNil(t, parsed)

			assert.Zero(t, stub.parseExecs.Load(), "the parse ran tofu")

			_, toV1Err := parsed.ToV1(ctx, l, v)
			require.Error(t, toV1Err)
			assert.Contains(t, toV1Err.Error(), "Unknown variable")
		})
	}
}
