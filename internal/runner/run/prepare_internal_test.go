package run

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/writer"
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

			opts := &Options{
				BackendBootstrap: tc.backendBootstrap,
				TerraformCliArgs: iacargs.New(),
			}

			var cfg runcfg.RunConfig
			if tc.remoteStateCfg != nil {
				cfg.RemoteState = *remotestate.New(tc.remoteStateCfg)
			}

			err := prepareInitCommandRunCfg(t.Context(), logger.CreateLogger(), OSVenv(), opts, &cfg)

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

// TestRemoteStateOptsPropagatesVenv pins that remoteStateOpts carries the
// caller's venv writers and env onto the constructed backend.Options. The
// S3 backend's CreateS3BucketIfNecessary writes the create-bucket prompt
// to opts.Writers.ErrWriter; if that field is zero-valued the call site
// panics inside shell.PromptUserForInput.
func TestRemoteStateOptsPropagatesVenv(t *testing.T) {
	t.Parallel()

	var (
		out bytes.Buffer
		err bytes.Buffer
	)

	v := Venv{
		Env:     map[string]string{"TF_VAR_x": "1"},
		Writers: writer.Writers{Writer: &out, ErrWriter: &err},
	}

	opts := &Options{
		TerraformCliArgs: iacargs.New(),
	}

	got := opts.remoteStateOpts(v)

	require.NotNil(t, got)
	assert.Same(t, &out, got.Writers.Writer, "stdout writer must come from the venv")
	assert.Same(t, &err, got.Writers.ErrWriter, "stderr writer must come from the venv; a nil ErrWriter panics in shell.PromptUserForInput")
	assert.Equal(t, "1", got.Env["TF_VAR_x"], "env must come from the venv")
}
