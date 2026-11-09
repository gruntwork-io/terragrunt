package config_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDependencyOutputFetchProgress checks that only an uncached dependency fetch with a reporter is drawn.
func TestDependencyOutputFetchProgress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		wantResult  string
		wantFetches int
		reporter    bool
		cached      bool
	}{
		{
			name:        "reporter on the context",
			reporter:    true,
			wantResult:  "from-direct",
			wantFetches: 1,
		},
		{
			name:       "reporter and cached outputs",
			reporter:   true,
			cached:     true,
			wantResult: "from-cache",
		},
		{
			name:        "plain context",
			wantResult:  "from-direct",
			wantFetches: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := newDependencyStateRecorder(t, http.StatusOK, terraformState("from-direct"))

			ctx, v, pctx, configPath := prepareDependencyStateFixture(
				t,
				recorder,
				"s3",
				`
        bucket              = "state-bucket"
        key                 = "service.tfstate"
        region              = "us-east-1"
        endpoint            = "https://s3.example.com"
        force_path_style    = true
        skip_credentials_validation = true`,
				map[string]string{
					"AWS_ACCESS_KEY_ID":     "test-access-key",
					"AWS_SECRET_ACCESS_KEY": "test-secret-key",
				},
				"",
			)

			term := new(bytes.Buffer)

			if tc.reporter {
				ctx = spinner.ContextWithReporter(ctx, drawingReporter(term))
			}

			if tc.cached {
				cache.ContextCache[[]byte](ctx, config.JSONOutputCacheContextKey).Put(
					ctx,
					venvtest.Root("/repo/producer/terragrunt.hcl"),
					terraformOutput("from-cache"),
				)
			}

			cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, configPath, nil)
			require.NoError(t, err)

			assert.Equal(t, tc.wantResult, cfg.Inputs["result"])
			assert.Len(t, recorder.requestPaths(), tc.wantFetches)
			assert.Equal(t, tc.wantFetches > 0 && tc.reporter, term.Len() > 0)
		})
	}
}

// TestDependencyOutputParseFailureProgress checks that a fetch whose output is not JSON logs no done line.
func TestDependencyOutputParseFailureProgress(t *testing.T) {
	t.Parallel()

	recorder := newDependencyStateRecorder(t, http.StatusForbidden, []byte(s3AccessDeniedBody))

	ctx, v, pctx, configPath := prepareDependencyStateFixture(
		t,
		recorder,
		"s3",
		`
        bucket              = "state-bucket"
        key                 = "service.tfstate"
        region              = "us-east-1"
        endpoint            = "https://s3.example.com"
        force_path_style    = true
        skip_credentials_validation = true`,
		map[string]string{
			"AWS_ACCESS_KEY_ID":     "test-access-key",
			"AWS_SECRET_ACCESS_KEY": "test-secret-key",
		},
		"",
	)

	v = v.WithExec(vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if slices.Contains(inv.Args, "output") {
			return vexec.Result{Stdout: []byte(`{"result":`)}
		}

		return vexec.Result{}
	}))

	term := new(bytes.Buffer)
	ctx = spinner.ContextWithReporter(ctx, drawingReporter(term))

	logs := new(bytes.Buffer)
	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(util.NewSyncWriter(logs)), log.WithLevel(log.InfoLevel))

	_, err := config.ParseConfigFile(ctx, l, v, pctx, configPath, nil)
	require.ErrorAs(t, err, new(config.TerragruntOutputParsingError))

	assert.NotEmpty(t, term.String())
	assert.NotContains(t, logs.String(), "level=info", logs)
}

// drawingReporter returns a reporter that draws its progress line on term from the start.
func drawingReporter(term io.Writer) *spinner.Reporter {
	v := venv.MemVenv().WithErrWriter(term).WithEnv(map[string]string{"TERM": "xterm"}).WithGOOS("linux")
	v.Terminal.StderrIsTTY = func() bool { return true }
	v.Terminal.StderrHandlesEscapes = func() bool { return true }
	v.Terminal.ErrWidth = func() int { return 120 }

	return spinner.New(v, spinner.Options{LogsForHumans: true})
}
