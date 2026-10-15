package providercache_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/providercache"
	pcoptions "github.com/gruntwork-io/terragrunt/internal/providercache/options"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerraformCommandHookReportsProviderCaching pins that only a run with a reporter logs the done line.
func TestTerraformCommandHookReportsProviderCaching(t *testing.T) {
	t.Parallel()

	const (
		workDir = "/virtual/work"
		started = "Caching providers for " + workDir
		working = started + "..."
		done    = "Cached providers for " + workDir
	)

	tc := []struct {
		name     string
		reported bool
	}{
		{name: "run with a reporter", reported: true},
		{name: "run without a reporter", reported: false},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var invocations []vexec.Invocation

			v := venvtest.New().
				WithGOOS("linux").
				WithUserHomeDir(func() (string, error) { return "/virtual/home", nil }).
				WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
					invocations = append(invocations, inv)
					return vexec.Result{}
				})

			require.NoError(t, v.FS.MkdirAll(workDir, 0o755))

			pc := providercache.NewProviderCache()
			require.NoError(t, pc.Init(
				logger.CreateLogger(),
				v,
				tfimpl.OpenTofu,
				&pcoptions.ProviderCacheOptions{
					Dir:           "/virtual/provider-cache",
					Token:         "11111111-2222-3333-4444-555555555555",
					RegistryNames: pcoptions.DefaultRegistryNames,
				},
				workDir,
			))

			tfOpts := &tf.TFOptions{
				TerraformCliArgs:   iacargs.New(),
				ShellOptions:       shell.NewShellOptions(map[string]string{}).WithTFPath("tofu").WithWorkingDir(workDir),
				TofuImplementation: tfimpl.OpenTofu,
			}

			ctx := t.Context()
			if tt.reported {
				ctx = spinner.ContextWithReporter(ctx, spinner.New(spinner.Options{}))
			}

			l, logs := newBufferLogger()

			output, err := pc.TerraformCommandHook(ctx, l, v, tfOpts, clihelper.Args{"init"})
			require.NoError(t, err)
			require.NotNil(t, output)
			require.Len(t, invocations, 2, "the warm-up command and the target command each run once")

			assert.Equal(t, 1, strings.Count(logs.String(), started), "the start must be announced exactly once")
			assert.NotContains(t, logs.String(), working)

			if !tt.reported {
				assert.NotContains(t, logs.String(), done)

				return
			}

			assert.Contains(t, logs.String(), done)
		})
	}
}
