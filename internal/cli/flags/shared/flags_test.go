package shared_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSharedFlagsSetOptions pins the option field each shared flag writes, by name, alias, and current or deprecated env var.
func TestSharedFlagsSetOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		want     any
		newFlags func(opts *options.TerragruntOptions) clihelper.Flags
		got      func(opts *options.TerragruntOptions) any
		env      map[string]string
		name     string
		args     clihelper.Args
	}{
		{
			name:     "all by name",
			newFlags: allFlags,
			args:     clihelper.Args{"--all"},
			got:      func(opts *options.TerragruntOptions) any { return opts.RunAll },
			want:     true,
		},
		{
			name:     "all by alias",
			newFlags: allFlags,
			args:     clihelper.Args{"-a"},
			got:      func(opts *options.TerragruntOptions) any { return opts.RunAll },
			want:     true,
		},
		{
			name:     "all by env var",
			newFlags: allFlags,
			env:      map[string]string{"TG_ALL": "true"},
			got:      func(opts *options.TerragruntOptions) any { return opts.RunAll },
			want:     true,
		},
		{
			name:     "graph by name",
			newFlags: graphFlags,
			args:     clihelper.Args{"--graph"},
			got:      func(opts *options.TerragruntOptions) any { return opts.Graph },
			want:     true,
		},
		{
			name:     "auth provider cmd by name",
			newFlags: authProviderCmdFlags,
			args:     clihelper.Args{"--auth-provider-cmd", "auth.sh"},
			got:      func(opts *options.TerragruntOptions) any { return opts.AuthProviderCmd },
			want:     "auth.sh",
		},
		{
			name:     "auth provider cmd by deprecated env var",
			newFlags: authProviderCmdFlags,
			env:      map[string]string{"TERRAGRUNT_AUTH_PROVIDER_CMD": "auth.sh"},
			got:      func(opts *options.TerragruntOptions) any { return opts.AuthProviderCmd },
			want:     "auth.sh",
		},
		{
			name:     "no discovery auth provider cmd disables discovery auth",
			newFlags: noDiscoveryAuthProviderCmdFlags,
			args:     clihelper.Args{"--no-discovery-auth-provider-cmd"},
			got:      func(opts *options.TerragruntOptions) any { return opts.DiscoveryAuthProviderCmd },
			want:     false,
		},
		{
			name:     "no auto init disables auto init",
			newFlags: noAutoInitFlags,
			args:     clihelper.Args{"--no-auto-init"},
			got:      func(opts *options.TerragruntOptions) any { return opts.AutoInit },
			want:     false,
		},
		{
			name:     "deprecated auto init env var false disables auto init",
			newFlags: noAutoInitFlags,
			env:      map[string]string{"TERRAGRUNT_AUTO_INIT": "false"},
			got:      func(opts *options.TerragruntOptions) any { return opts.AutoInit },
			want:     false,
		},
		{
			name:     "backend bootstrap by name",
			newFlags: backendFlags,
			args:     clihelper.Args{"--backend-bootstrap"},
			got:      func(opts *options.TerragruntOptions) any { return opts.BackendBootstrap },
			want:     true,
		},
		{
			name:     "backend require bootstrap by deprecated env var",
			newFlags: backendFlags,
			env:      map[string]string{"TERRAGRUNT_FAIL_ON_STATE_BUCKET_CREATION": "true"},
			got:      func(opts *options.TerragruntOptions) any { return opts.FailIfBucketCreationRequired },
			want:     true,
		},
		{
			name:     "disable bucket update by name",
			newFlags: backendFlags,
			args:     clihelper.Args{"--disable-bucket-update"},
			got:      func(opts *options.TerragruntOptions) any { return opts.DisableBucketUpdate },
			want:     true,
		},
		{
			name:     "config by name",
			newFlags: configFlags,
			args:     clihelper.Args{"--config", "custom.hcl"},
			got:      func(opts *options.TerragruntOptions) any { return opts.TerragruntConfigPath },
			want:     "custom.hcl",
		},
		{
			name:     "config by deprecated env var",
			newFlags: configFlags,
			env:      map[string]string{"TERRAGRUNT_CONFIG": "custom.hcl"},
			got:      func(opts *options.TerragruntOptions) any { return opts.TerragruntConfigPath },
			want:     "custom.hcl",
		},
		{
			name:     "download dir by name",
			newFlags: downloadDirFlags,
			args:     clihelper.Args{"--download-dir", "/cache"},
			got:      func(opts *options.TerragruntOptions) any { return opts.DownloadDir },
			want:     "/cache",
		},
		{
			name:     "download dir by deprecated download env var",
			newFlags: downloadDirFlags,
			env:      map[string]string{"TERRAGRUNT_DOWNLOAD": "/cache"},
			got:      func(opts *options.TerragruntOptions) any { return opts.DownloadDir },
			want:     "/cache",
		},
		{
			name:     "fail fast by env var",
			newFlags: failFastFlags,
			env:      map[string]string{"TG_FAIL_FAST": "true"},
			got:      func(opts *options.TerragruntOptions) any { return opts.FailFast },
			want:     true,
		},
		{
			name:     "feature flags merge repeated values",
			newFlags: featureFlags,
			args:     clihelper.Args{"--feature", "a=1", "--feature", "b=2"},
			got:      func(opts *options.TerragruntOptions) any { return opts.FeatureFlags },
			want:     map[string]string{"a": "1", "b": "2"},
		},
		{
			name:     "feature flags by env var",
			newFlags: featureFlags,
			env:      map[string]string{"TG_FEATURE": "a=1,b=2"},
			got:      func(opts *options.TerragruntOptions) any { return opts.FeatureFlags },
			want:     map[string]string{"a": "1", "b": "2"},
		},
		{
			name:     "iam assume role by name",
			newFlags: iamFlags,
			args:     clihelper.Args{"--iam-assume-role", "arn:aws:iam::123:role/r"},
			got:      func(opts *options.TerragruntOptions) any { return opts.IAMRoleOptions.RoleARN },
			want:     "arn:aws:iam::123:role/r",
		},
		{
			name:     "iam assume role by deprecated env var",
			newFlags: iamFlags,
			env:      map[string]string{"TERRAGRUNT_IAM_ROLE": "arn:aws:iam::123:role/r"},
			got:      func(opts *options.TerragruntOptions) any { return opts.IAMRoleOptions.RoleARN },
			want:     "arn:aws:iam::123:role/r",
		},
		{
			name:     "iam assume role duration by name",
			newFlags: iamFlags,
			args:     clihelper.Args{"--iam-assume-role-duration", "900"},
			got:      func(opts *options.TerragruntOptions) any { return opts.IAMRoleOptions.AssumeRoleDuration },
			want:     int64(900),
		},
		{
			name:     "iam assume role session name by name",
			newFlags: iamFlags,
			args:     clihelper.Args{"--iam-assume-role-session-name", "session"},
			got:      func(opts *options.TerragruntOptions) any { return opts.IAMRoleOptions.AssumeRoleSessionName },
			want:     "session",
		},
		{
			name:     "iam web identity token by deprecated env var",
			newFlags: iamFlags,
			env:      map[string]string{"TERRAGRUNT_IAM_WEB_IDENTITY_TOKEN": "token"},
			got:      func(opts *options.TerragruntOptions) any { return opts.IAMRoleOptions.WebIdentityToken },
			want:     "token",
		},
		{
			name:     "inputs debug by deprecated env var",
			newFlags: inputsDebugFlags,
			env:      map[string]string{"TERRAGRUNT_DEBUG": "true"},
			got:      func(opts *options.TerragruntOptions) any { return opts.Debug },
			want:     true,
		},
		{
			name:     "source by name",
			newFlags: sourceFlags,
			args:     clihelper.Args{"--source", "git::https://example.com/repo.git"},
			got:      func(opts *options.TerragruntOptions) any { return opts.Source },
			want:     "git::https://example.com/repo.git",
		},
		{
			name:     "source by deprecated env var",
			newFlags: sourceFlags,
			env:      map[string]string{"TERRAGRUNT_SOURCE": "/local/modules"},
			got:      func(opts *options.TerragruntOptions) any { return opts.Source },
			want:     "/local/modules",
		},
		{
			name:     "source map by name",
			newFlags: sourceMapFlags,
			args:     clihelper.Args{"--source-map", "git::https://example.com/repo.git=/local/repo"},
			got:      func(opts *options.TerragruntOptions) any { return opts.SourceMap },
			want:     map[string]string{"git::https://example.com/repo.git": "/local/repo"},
		},
		{
			name:     "tf path by name marks it explicit",
			newFlags: tfPathFlags,
			args:     clihelper.Args{"--tf-path", "/usr/local/bin/tofu"},
			got: func(opts *options.TerragruntOptions) any {
				return []any{opts.TFPath, opts.TFPathExplicitlySet}
			},
			want: []any{"/usr/local/bin/tofu", true},
		},
		{
			name:     "tf path by deprecated env var",
			newFlags: tfPathFlags,
			env:      map[string]string{"TERRAGRUNT_TFPATH": "/usr/local/bin/terraform"},
			got:      func(opts *options.TerragruntOptions) any { return opts.TFPath },
			want:     "/usr/local/bin/terraform",
		},
		{
			name:     "no include root by name",
			newFlags: scaffoldingFlags,
			args:     clihelper.Args{"--no-include-root"},
			got:      func(opts *options.TerragruntOptions) any { return opts.ScaffoldNoIncludeRoot },
			want:     true,
		},
		{
			name:     "no shell and no hooks by name",
			newFlags: scaffoldingFlags,
			args:     clihelper.Args{"--no-shell", "--no-hooks"},
			got:      func(opts *options.TerragruntOptions) any { return []bool{opts.NoShell, opts.NoHooks} },
			want:     []bool{true, true},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()

			env := tc.env
			if env == nil {
				env = map[string]string{}
			}

			require.NoError(t, parseAndRunActions(t, tc.newFlags(opts), tc.args, env))
			assert.Equal(t, tc.want, tc.got(opts), "flag should write the parsed value to its option field")
		})
	}
}

// TestAllAndGraphFlagsConflict pins that --all and --graph reject each other whichever flag runs its action first.
func TestAllAndGraphFlagsConflict(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		newFlags func(opts *options.TerragruntOptions) clihelper.Flags
		name     string
	}{
		{
			name: "all action sees graph",
			newFlags: func(opts *options.TerragruntOptions) clihelper.Flags {
				return clihelper.Flags{shared.NewAllFlag(opts, nil), shared.NewGraphFlag(opts, nil)}
			},
		},
		{
			name: "graph action sees all",
			newFlags: func(opts *options.TerragruntOptions) clihelper.Flags {
				return clihelper.Flags{shared.NewGraphFlag(opts, nil), shared.NewAllFlag(opts, nil)}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()

			err := parseAndRunActions(t, tc.newFlags(opts), clihelper.Args{"--all", "--graph"}, map[string]string{})

			var conflictErr *shared.AllGraphFlagsError
			require.ErrorAs(t, err, &conflictErr)
			assert.Contains(t, conflictErr.Error(), "--"+shared.AllFlagName, "error should name the --all flag")
			assert.Contains(t, conflictErr.Error(), "--"+shared.GraphFlagName, "error should name the --graph flag")
		})
	}
}

// TestSharedFlagsUsePrefixedEnvVars pins that a command prefix is inserted after TG_ in the env var name.
func TestSharedFlagsUsePrefixedEnvVars(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	allFlag := shared.NewAllFlag(opts, flags.Prefix{"stack"})

	require.NoError(t, parseAndRunActions(t, clihelper.Flags{allFlag}, nil, map[string]string{"TG_STACK_ALL": "true"}))
	assert.True(t, opts.RunAll, "TG_STACK_ALL should set --all for a stack-prefixed flag")
}

func allFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{shared.NewAllFlag(opts, nil)}
}

func graphFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{shared.NewGraphFlag(opts, nil)}
}

func authProviderCmdFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{shared.NewAuthProviderCmdFlag(opts, nil)}
}

func noDiscoveryAuthProviderCmdFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{shared.NewNoDiscoveryAuthProviderCmdFlag(opts, nil)}
}

func noAutoInitFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{shared.NewNoAutoInitFlag(opts, nil)}
}

func backendFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return shared.NewBackendFlags(opts, nil)
}

func configFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{shared.NewConfigFlag(opts, nil)}
}

func downloadDirFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{shared.NewDownloadDirFlag(opts, nil)}
}

func failFastFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{shared.NewFailFastFlag(opts)}
}

func featureFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return shared.NewFeatureFlags(opts, nil)
}

func iamFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return shared.NewIAMAssumeRoleFlags(opts, nil)
}

func inputsDebugFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{shared.NewInputsDebugFlag(opts, nil)}
}

func sourceFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{shared.NewSourceFlag(opts, nil)}
}

func sourceMapFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{shared.NewSourceMapFlag(opts, nil)}
}

func tfPathFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{shared.NewTFPathFlag(opts)}
}

func scaffoldingFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return shared.NewScaffoldingFlags(opts, nil)
}

// parseAndRunActions parses args and env into fs and runs the actions of every flag that was set.
func parseAndRunActions(t *testing.T, fs clihelper.Flags, args clihelper.Args, env map[string]string) error {
	t.Helper()

	if err := fs.Parse(args, env); err != nil {
		return err
	}

	return fs.RunActions(log.ContextWithLogger(t.Context(), logger.CreateLogger()), &clihelper.Context{})
}
