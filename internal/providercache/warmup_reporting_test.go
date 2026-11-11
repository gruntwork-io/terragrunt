package providercache_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerraformCommandHookProgress checks that provider caching is drawn and logs one done line only with a reporter.
func TestTerraformCommandHookProgress(t *testing.T) {
	t.Parallel()

	const workDir = "/virtual/work"

	tc := []struct {
		name         string
		wantLogLines int
		reported     bool
	}{
		{name: "run with a reporter", reported: true, wantLogLines: 2},
		{name: "run without a reporter", wantLogLines: 1},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var invocations []vexec.Invocation

			v := venv.MemVenv().
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

			term := new(bytes.Buffer)

			ctx := t.Context()
			if tt.reported {
				ctx = spinner.ContextWithReporter(ctx, drawingReporter(term))
			}

			l, logs := newBufferLogger()

			output, err := pc.TerraformCommandHook(ctx, l, v, tfOpts, clihelper.Args{"init"})
			require.NoError(t, err)
			require.NotNil(t, output)
			require.Len(t, invocations, 2, "the warm-up command and the target command each run once")

			assert.Equal(t, tt.reported, term.Len() > 0)
			assert.Equal(t, tt.wantLogLines, strings.Count(logs.String(), "\n"), logs)
		})
	}
}

// drawingReporter returns a reporter that draws its progress line on term from the start.
func drawingReporter(term io.Writer) *spinner.Reporter {
	v := venv.MemVenv().WithErrWriter(term).WithEnv(map[string]string{"TERM": "xterm"}).WithGOOS("linux")
	v.Terminal.StderrIsTTY = func() bool { return true }
	v.Terminal.StderrHandlesEscapes = func() bool { return true }
	v.Terminal.ErrWidth = func() int { return 120 }

	return spinner.New(v, spinner.Options{LogsForHumans: true})
}
