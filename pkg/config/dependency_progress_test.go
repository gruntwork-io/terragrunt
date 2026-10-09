package config_test

import (
	"bytes"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDependencyOutputFetchProgress pins that only a real fetch with a reporter on the context is reported.
func TestDependencyOutputFetchProgress(t *testing.T) {
	t.Parallel()

	target := filepath.Join("..", "producer", config.DefaultTerragruntConfigPath)
	working := "Fetching outputs of dependency " + target + "..."
	done := "Fetched outputs of dependency " + target

	testCases := []struct {
		reporter    func(term io.Writer) *spinner.Reporter
		name        string
		wantResult  string
		wantFetches int
		wantWorking int
		wantDone    int
		cached      bool
		wantDrawn   bool
	}{
		{
			name:        "reporter that writes log lines",
			reporter:    logProgressReporter,
			wantResult:  "from-direct",
			wantFetches: 1,
			wantWorking: 1,
			wantDone:    1,
		},
		{
			name:        "reporter that draws on a terminal",
			reporter:    terminalProgressReporter,
			wantResult:  "from-direct",
			wantFetches: 1,
			wantDone:    1,
			wantDrawn:   true,
		},
		{
			name:       "reporter and cached outputs",
			reporter:   logProgressReporter,
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

			if tc.reporter != nil {
				ctx = spinner.ContextWithReporter(ctx, tc.reporter(term))
			}

			if tc.cached {
				cache.ContextCache[[]byte](ctx, config.JSONOutputCacheContextKey).Put(
					ctx,
					venvtest.Root("/repo/producer/terragrunt.hcl"),
					terraformOutput("from-cache"),
				)
			}

			logs := new(bytes.Buffer)
			l := logger.CreateLogger()
			l.SetOptions(log.WithOutput(util.NewSyncWriter(logs)), log.WithLevel(log.InfoLevel))

			cfg, err := config.ParseConfigFile(ctx, l, v, pctx, configPath, nil)
			require.NoError(t, err)

			assert.Equal(t, tc.wantResult, cfg.Inputs["result"])
			assert.Len(t, recorder.requestPaths(), tc.wantFetches)
			assert.Equal(t, tc.wantWorking, strings.Count(logs.String(), working), logs)
			assert.Equal(t, tc.wantDone, strings.Count(logs.String(), done), logs)
			assert.Equal(t, tc.wantDrawn, strings.Contains(term.String(), working), term)
		})
	}
}

// logProgressReporter returns a reporter that reports at once and writes log lines.
func logProgressReporter(io.Writer) *spinner.Reporter {
	return spinner.New(spinner.Options{})
}

// terminalProgressReporter returns a reporter that reports at once and draws its progress line on term.
func terminalProgressReporter(term io.Writer) *spinner.Reporter {
	return spinner.New(spinner.Options{
		Out:           term,
		Width:         func() int { return 120 },
		Env:           map[string]string{"TERM": "xterm-256color"},
		GOOS:          "linux",
		OutIsTTY:      true,
		LogsForHumans: true,
	})
}
