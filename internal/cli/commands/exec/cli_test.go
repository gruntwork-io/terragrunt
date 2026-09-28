package exec_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/exec"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewFlagsRegistersSourceAndAutoInitFlags pins the flags issue 4200 reported missing on `exec`.
func TestNewFlagsRegistersSourceAndAutoInitFlags(t *testing.T) {
	t.Parallel()

	flags := newExecFlags(t)

	for _, name := range []string{shared.SourceFlagName, shared.SourceMapFlagName, shared.NoAutoInitFlagName} {
		assert.NotNil(t, flags.Get(name), name)
	}
}

// TestNewFlagsParsesSourceAndAutoInitFlags covers the three flags across CLI values, env vars and deprecated env vars.
func TestNewFlagsParsesSourceAndAutoInitFlags(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		expectedSource    string
		env               map[string]string
		expectedSourceMap map[string]string
		args              clihelper.Args
		expectedAutoInit  bool
	}{
		{
			name:              "defaults leave auto-init on and the source map empty",
			expectedSourceMap: map[string]string{},
			expectedAutoInit:  true,
		},
		{
			name:              "source flag",
			args:              clihelper.Args{"--source", "/local/vpc"},
			expectedSource:    "/local/vpc",
			expectedSourceMap: map[string]string{},
			expectedAutoInit:  true,
		},
		{
			name:              "source env var",
			env:               map[string]string{"TG_SOURCE": "/local/vpc"},
			expectedSource:    "/local/vpc",
			expectedSourceMap: map[string]string{},
			expectedAutoInit:  true,
		},
		{
			name:              "deprecated source env var",
			env:               map[string]string{"TERRAGRUNT_SOURCE": "/local/vpc"},
			expectedSource:    "/local/vpc",
			expectedSourceMap: map[string]string{},
			expectedAutoInit:  true,
		},
		{
			name: "source map flag",
			args: clihelper.Args{
				"--source-map",
				"git::ssh://git@github.com/acme/vpc.git=/local/vpc",
			},
			expectedSourceMap: map[string]string{
				"git::ssh://git@github.com/acme/vpc.git": "/local/vpc",
			},
			expectedAutoInit: true,
		},
		{
			name: "repeated source map flags accumulate",
			args: clihelper.Args{
				"--source-map", "git::ssh://git@github.com/acme/vpc.git=/local/vpc",
				"--source-map", "git::ssh://git@github.com/acme/app.git=/local/app",
			},
			expectedSourceMap: map[string]string{
				"git::ssh://git@github.com/acme/vpc.git": "/local/vpc",
				"git::ssh://git@github.com/acme/app.git": "/local/app",
			},
			expectedAutoInit: true,
		},
		{
			name: "source map value keeps its ref query parameter",
			args: clihelper.Args{
				"--source-map",
				"git::ssh://git@github.com/acme/vpc.git=git::ssh://git@github.com/fork/vpc.git?ref=FEATURE",
			},
			expectedSourceMap: map[string]string{
				"git::ssh://git@github.com/acme/vpc.git": "git::ssh://git@github.com/fork/vpc.git?ref=FEATURE",
			},
			expectedAutoInit: true,
		},
		{
			name: "source map env var",
			env: map[string]string{
				"TG_SOURCE_MAP": "git::ssh://git@github.com/acme/vpc.git=/local/vpc",
			},
			expectedSourceMap: map[string]string{
				"git::ssh://git@github.com/acme/vpc.git": "/local/vpc",
			},
			expectedAutoInit: true,
		},
		{
			name: "deprecated source map env var",
			env: map[string]string{
				"TERRAGRUNT_SOURCE_MAP": "git::ssh://git@github.com/acme/vpc.git=/local/vpc",
			},
			expectedSourceMap: map[string]string{
				"git::ssh://git@github.com/acme/vpc.git": "/local/vpc",
			},
			expectedAutoInit: true,
		},
		{
			name: "source map env var splits multiple entries without breaking ref queries",
			env: map[string]string{
				"TG_SOURCE_MAP": "git::ssh://git@github.com/acme/vpc.git=/vpc?ref=main," +
					"git::ssh://git@github.com/acme/app.git=/app",
			},
			expectedSourceMap: map[string]string{
				"git::ssh://git@github.com/acme/vpc.git": "/vpc?ref=main",
				"git::ssh://git@github.com/acme/app.git": "/app",
			},
			expectedAutoInit: true,
		},
		{
			name:              "no auto init flag",
			args:              clihelper.Args{"--no-auto-init"},
			expectedSourceMap: map[string]string{},
		},
		{
			name:              "no auto init env var",
			env:               map[string]string{"TG_NO_AUTO_INIT": "true"},
			expectedSourceMap: map[string]string{},
		},
		{
			name:              "deprecated auto init env var is inverted",
			env:               map[string]string{"TERRAGRUNT_AUTO_INIT": "false"},
			expectedSourceMap: map[string]string{},
		},
		{
			name:              "deprecated auto init env var left on",
			env:               map[string]string{"TERRAGRUNT_AUTO_INIT": "true"},
			expectedSourceMap: map[string]string{},
			expectedAutoInit:  true,
		},
		{
			name: "all flags together",
			args: clihelper.Args{
				"--source=/local/root",
				"--source-map=git::ssh://git@github.com/acme/vpc.git=/local/vpc",
				"--no-auto-init",
			},
			expectedSource: "/local/root",
			expectedSourceMap: map[string]string{
				"git::ssh://git@github.com/acme/vpc.git": "/local/vpc",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			flags := exec.NewFlags(logger.CreateLogger(), opts, exec.NewOptions(), nil)

			env := tc.env
			if env == nil {
				env = map[string]string{}
			}

			require.NoError(t, flags.Parse(tc.args, env))
			require.NoError(t, flags.RunActions(t.Context(), &clihelper.Context{}))
			assert.Equal(t, tc.expectedSource, opts.Source)
			assert.Equal(t, tc.expectedSourceMap, opts.SourceMap)
			assert.Equal(t, tc.expectedAutoInit, opts.AutoInit)
		})
	}
}

// newExecFlags builds the exec flag set against throwaway options.
func newExecFlags(t *testing.T) clihelper.Flags {
	t.Helper()

	return exec.NewFlags(
		logger.CreateLogger(),
		options.NewTerragruntOptions(vexec.NewOSExec()),
		exec.NewOptions(),
		nil,
	)
}
