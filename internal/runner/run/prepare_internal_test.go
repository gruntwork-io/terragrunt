package run

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
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
		// remoteStateCfg before name: *Config (8 bytes) then string (16 bytes) → 16 GC pointer bytes.
		remoteStateCfg    *remotestate.Config
		name              string
		backendBootstrap  bool
		expectError       bool
		expectBackendArgs bool
	}{
		{
			name:              "nil remote state config - no args inserted",
			remoteStateCfg:    nil,
			backendBootstrap:  false,
			expectError:       false,
			expectBackendArgs: false,
		},
		{
			name: "disable_init=false, bootstrap=false - backend-config args inserted, no bootstrap",
			remoteStateCfg: &remotestate.Config{
				BackendName:   "s3",
				DisableInit:   false,
				BackendConfig: s3Config,
			},
			backendBootstrap:  false,
			expectError:       false,
			expectBackendArgs: true,
		},
		{
			name: "disable_init=true, bootstrap=false - backend-config args inserted, no bootstrap",
			remoteStateCfg: &remotestate.Config{
				BackendName:   "s3",
				DisableInit:   true,
				BackendConfig: s3Config,
			},
			backendBootstrap:  false,
			expectError:       false,
			expectBackendArgs: true,
		},
		{
			// Key regression test for #1422: even with BackendBootstrap=true, Bootstrap must be
			// skipped when DisableInit=true. If Bootstrap were called with a fake S3 config
			// (no real AWS), it would return an AWS connectivity error — returning nil proves
			// Bootstrap was correctly skipped.
			name: "disable_init=true, bootstrap=true - backend-config args inserted, bootstrap SKIPPED",
			remoteStateCfg: &remotestate.Config{
				BackendName:   "s3",
				DisableInit:   true,
				BackendConfig: s3Config,
			},
			backendBootstrap:  true,
			expectError:       false,
			expectBackendArgs: true,
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

			err = prepareInitCommandRunCfg(t.Context(), logger.CreateLogger(), opts, &cfg)

			if tc.expectError {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			flags := opts.TerraformCliArgs.Flags
			if tc.expectBackendArgs {
				assert.NotContains(t, flags, "-backend=false", "disable_init should not pass -backend=false to terraform")

				hasBackendConfig := false

				for _, f := range flags {
					if len(f) > 16 && f[:16] == "-backend-config=" {
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
