package run

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrepareInitCommandRunCfg verifies that prepareInitCommandRunCfg inserts
// the correct CLI args for various remote_state configurations.
//
// Bootstrap skip behavior (DisableInit=true preventing bootstrap even when
// BackendBootstrap=true) is tested by:
//   - TestNeedsBootstrapDisableInit in internal/remotestate/remote_state_test.go
//   - TestAwsDisableInitS3Backend in test/integration_aws_test.go
func TestPrepareInitCommandRunCfg(t *testing.T) {
	t.Parallel()

	s3Config := backend.Config{
		"bucket": "test-bucket",
		"key":    "test.tfstate",
		"region": "us-east-1",
	}

	testCases := []struct {
		remoteStateCfg    *remotestate.Config
		name              string
		backendBootstrap  bool
		expectBackendArgs bool
	}{
		{
			name:              "nil remote state config - no args inserted",
			remoteStateCfg:    nil,
			backendBootstrap:  false,
			expectBackendArgs: false,
		},
		{
			name: "disable_init=false, bootstrap=false - backend-config args inserted",
			remoteStateCfg: &remotestate.Config{
				BackendName:   "s3",
				DisableInit:   false,
				BackendConfig: s3Config,
			},
			backendBootstrap:  false,
			expectBackendArgs: true,
		},
		{
			name: "disable_init=true, bootstrap=false - backend-config args inserted",
			remoteStateCfg: &remotestate.Config{
				BackendName:   "s3",
				DisableInit:   true,
				BackendConfig: s3Config,
			},
			backendBootstrap:  false,
			expectBackendArgs: true,
		},
		{
			name: "disable_init=true, bootstrap=true - backend-config args inserted",
			remoteStateCfg: &remotestate.Config{
				BackendName:   "s3",
				DisableInit:   true,
				BackendConfig: s3Config,
			},
			backendBootstrap:  true,
			expectBackendArgs: true,
		},
		{
			// When generate is set, backend config goes into the generated .tf file,
			// not via -backend-config= CLI args.
			name: "disable_init=true, generate=true - no backend-config args",
			remoteStateCfg: &remotestate.Config{
				BackendName:   "s3",
				DisableInit:   true,
				Generate:      &remotestate.ConfigGenerate{Path: "backend.tf", IfExists: "overwrite"},
				BackendConfig: s3Config,
			},
			backendBootstrap:  false,
			expectBackendArgs: false,
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

			require.NoError(t, err)

			allArgs := opts.TerraformCliArgs.Slice()
			if tc.expectBackendArgs {
				assert.NotContains(t, allArgs, "-backend=false", "disable_init should not pass -backend=false to terraform")

				hasBackendConfig := false

				for _, f := range allArgs {
					if strings.HasPrefix(f, "-backend-config=") {
						hasBackendConfig = true

						break
					}
				}

				assert.True(t, hasBackendConfig, "expected -backend-config= flag in CLI args, got: %v", allArgs)
			} else {
				assert.Empty(t, allArgs, "expected no CLI args, got: %v", allArgs)
			}
		})
	}
}
