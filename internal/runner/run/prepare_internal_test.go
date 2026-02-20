package run

import (
	"context"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareInitCommandRunCfg(t *testing.T) {
	t.Parallel()

	s3Config := backend.Config{
		"bucket": "test-bucket",
		"key":    "test.tfstate",
		"region": "us-east-1",
	}

	testCases := []struct {
		remoteStateCfg       *remotestate.Config
		name                 string
		backendBootstrap     bool
		expectBackendArgs    bool
		expectBootstrapCalls int
	}{
		{
			name:                 "nil remote state config - no args inserted",
			remoteStateCfg:       nil,
			backendBootstrap:     false,
			expectBackendArgs:    false,
			expectBootstrapCalls: 0,
		},
		{
			name: "disable_init=false, bootstrap=false - backend-config args inserted, no bootstrap",
			remoteStateCfg: &remotestate.Config{
				BackendName:   "s3",
				DisableInit:   false,
				BackendConfig: s3Config,
			},
			backendBootstrap:     false,
			expectBackendArgs:    true,
			expectBootstrapCalls: 0,
		},
		{
			name: "disable_init=true, bootstrap=false - backend-config args inserted, no bootstrap",
			remoteStateCfg: &remotestate.Config{
				BackendName:   "s3",
				DisableInit:   true,
				BackendConfig: s3Config,
			},
			backendBootstrap:     false,
			expectBackendArgs:    true,
			expectBootstrapCalls: 0,
		},
		{
			// Key regression test for #1422: even with BackendBootstrap=true, Bootstrap must be
			// skipped when DisableInit=true. The spy directly proves this: if Bootstrap were
			// invoked, expectBootstrapCalls=0 would fail — no inference from error behavior needed.
			name: "disable_init=true, bootstrap=true - backend-config args inserted, bootstrap SKIPPED",
			remoteStateCfg: &remotestate.Config{
				BackendName:   "s3",
				DisableInit:   true,
				BackendConfig: s3Config,
			},
			backendBootstrap:     true,
			expectBackendArgs:    true,
			expectBootstrapCalls: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest("mock.hcl")
			require.NoError(t, err)

			opts.BackendBootstrap = tc.backendBootstrap

			var cfg runcfg.RunConfig
			if tc.remoteStateCfg != nil {
				cfg.RemoteState = *remotestate.New(tc.remoteStateCfg)
			}

			bootstrapCalled := 0
			spy := func(_ context.Context, _ log.Logger, _ *options.TerragruntOptions) error {
				bootstrapCalled++

				return nil
			}

			err = prepareInitCommandRunCfg(t.Context(), logger.CreateLogger(), opts, &cfg, spy)

			require.NoError(t, err)
			assert.Equal(t, tc.expectBootstrapCalls, bootstrapCalled, "unexpected bootstrap call count")

			flags := opts.TerraformCliArgs.Flags
			if tc.expectBackendArgs {
				assert.NotContains(t, flags, "-backend=false", "disable_init should not pass -backend=false to terraform")

				hasBackendConfig := false

				for _, f := range flags {
					if strings.HasPrefix(f, "-backend-config=") {
						hasBackendConfig = true

						break
					}
				}

				assert.True(t, hasBackendConfig, "expected -backend-config= flag in CLI args, got: %v", flags)
			} else {
				assert.Empty(t, flags, "expected no CLI args for nil config, got: %v", flags)
			}
		})
	}
}
