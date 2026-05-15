package tflint_test

import (
	"context"
	"io"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tflint"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputsToTflintVar(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		inputs   map[string]any
		name     string
		expected []string
	}{
		{
			name:     "strings",
			inputs:   map[string]any{"region": "eu-central-1", "instance_count": 3},
			expected: []string{"--var=region=eu-central-1", "--var=instance_count=3"},
		},
		{
			name:     "strings and arrays",
			inputs:   map[string]any{"cidr_blocks": []string{"10.0.0.0/16"}},
			expected: []string{"--var=cidr_blocks=[\"10.0.0.0/16\"]"},
		},
		{
			name:     "boolean",
			inputs:   map[string]any{"create_resource": true},
			expected: []string{"--var=create_resource=true"},
		},
		{
			name: "with white spaces",
			// With white spaces, the string is still validated by tflint.
			inputs:   map[string]any{"region": " eu-central-1 "},
			expected: []string{"--var=region= eu-central-1 "},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual, err := tflint.InputsToTflintVar(tc.inputs)
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.expected, actual)
		})
	}
}

func TestTFArgumentsToVar(t *testing.T) {
	t.Parallel()

	const (
		optionalPresent = "/work/present.tfvars"
		optionalAbsent  = "/work/absent.tfvars"
	)

	testCases := []struct {
		name     string
		expected []string
		hook     runcfg.Hook
		cfg      runcfg.TerraformConfig
	}{
		{
			name: "command mismatch is skipped",
			hook: runcfg.Hook{Commands: []string{"plan"}},
			cfg: runcfg.TerraformConfig{ExtraArgs: []runcfg.TerraformExtraArguments{
				{
					Commands:  []string{"apply"},
					Arguments: []string{"-var=foo=bar"},
				},
			}},
			expected: nil,
		},
		{
			name: "TF_VAR_ env extracts; non-TF env ignored",
			hook: runcfg.Hook{Commands: []string{"plan"}},
			cfg: runcfg.TerraformConfig{ExtraArgs: []runcfg.TerraformExtraArguments{
				{
					Commands: []string{"plan"},
					EnvVars: map[string]string{
						"TF_VAR_region": "us-east-1",
						"PATH":          "/ignored",
					},
				},
			}},
			expected: []string{"--var='region=us-east-1'"},
		},
		{
			name: "-var= and -var-file= arguments split correctly",
			hook: runcfg.Hook{Commands: []string{"plan"}},
			cfg: runcfg.TerraformConfig{ExtraArgs: []runcfg.TerraformExtraArguments{
				{
					Commands: []string{"plan"},
					Arguments: []string{
						"-var=foo=bar",
						"-var-file=common.tfvars",
						"--unrelated", // no prefix → ignored by tflint translation
					},
				},
			}},
			expected: []string{"--var='foo=bar'", "--var-file=common.tfvars"},
		},
		{
			name: "required var files pass through verbatim",
			hook: runcfg.Hook{Commands: []string{"plan"}},
			cfg: runcfg.TerraformConfig{ExtraArgs: []runcfg.TerraformExtraArguments{
				{
					Commands:         []string{"plan"},
					RequiredVarFiles: []string{"/missing.tfvars"},
				},
			}},
			expected: []string{"--var-file=/missing.tfvars"},
		},
		{
			name: "optional var files filter on existence",
			hook: runcfg.Hook{Commands: []string{"plan"}},
			cfg: runcfg.TerraformConfig{ExtraArgs: []runcfg.TerraformExtraArguments{
				{
					Commands:         []string{"plan"},
					OptionalVarFiles: []string{optionalPresent, optionalAbsent},
				},
			}},
			expected: []string{"--var-file=" + optionalPresent},
		},
		{
			name: "optional var files: duplicates collapse to last occurrence",
			hook: runcfg.Hook{Commands: []string{"plan"}},
			cfg: runcfg.TerraformConfig{ExtraArgs: []runcfg.TerraformExtraArguments{
				{
					Commands:         []string{"plan"},
					OptionalVarFiles: []string{optionalPresent, optionalPresent},
				},
			}},
			expected: []string{"--var-file=" + optionalPresent},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := vfs.NewMemMapFS()
			require.NoError(t, vfs.WriteFile(fs, optionalPresent, []byte("x = 1"), 0o644))

			l := logger.CreateLogger()
			actual, err := tflint.TFArgumentsToVar(l, fs, &tc.hook, &tc.cfg)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestConfigFilePath_ShortCircuitsOnConfigFlag(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	l := logger.CreateLogger()

	got, err := tflint.ConfigFilePath(l, fs, &tflint.TFLintOptions{
		WorkingDir:        "/work/unit",
		RootWorkingDir:    "/work",
		MaxFoldersToCheck: 5,
	}, []string{"tflint", "--config", "/explicit/.tflint.hcl"})

	require.NoError(t, err)
	assert.Equal(t, "/explicit/.tflint.hcl", got)
}

func TestConfigFilePath_FallsBackToProjectWalk(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/work/.tflint.hcl", []byte("config {}"), 0o644))

	l := logger.CreateLogger()

	got, err := tflint.ConfigFilePath(l, fs, &tflint.TFLintOptions{
		WorkingDir:        "/work/unit",
		RootWorkingDir:    "/work",
		MaxFoldersToCheck: 5,
	}, []string{"tflint"})

	require.NoError(t, err)
	assert.Equal(t, "/work/.tflint.hcl", got)
}

func TestFindConfigInProject(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErrType error
		name        string
		wantPath    string
		seedFiles   []string
		opts        tflint.TFLintOptions
	}{
		{
			name:      "config in WorkingDir itself",
			seedFiles: []string{"/work/unit/.tflint.hcl"},
			opts: tflint.TFLintOptions{
				WorkingDir:        "/work/unit",
				RootWorkingDir:    "/work",
				MaxFoldersToCheck: 3,
			},
			wantPath: "/work/unit/.tflint.hcl",
		},
		{
			name:      "config two parents up",
			seedFiles: []string{"/work/.tflint.hcl"},
			opts: tflint.TFLintOptions{
				WorkingDir:        "/work/a/b",
				RootWorkingDir:    "/work",
				MaxFoldersToCheck: 5,
			},
			wantPath: "/work/.tflint.hcl",
		},
		{
			name:      "TerragruntConfigPath overrides WorkingDir as start",
			seedFiles: []string{"/source/.tflint.hcl"},
			opts: tflint.TFLintOptions{
				WorkingDir:           "/cache/abc",
				TerragruntConfigPath: "/source/unit/terragrunt.hcl",
				RootWorkingDir:       "/source",
				MaxFoldersToCheck:    5,
			},
			wantPath: "/source/.tflint.hcl",
		},
		{
			name:      "walk exceeds MaxFoldersToCheck before reaching config",
			seedFiles: []string{"/.tflint.hcl"},
			opts: tflint.TFLintOptions{
				WorkingDir:        "/a/b/c/d/e/f",
				RootWorkingDir:    "/",
				MaxFoldersToCheck: 2,
			},
			wantErrType: tflint.ConfigNotFound{},
		},
		{
			name:      "no config anywhere - walks to root",
			seedFiles: nil,
			opts: tflint.TFLintOptions{
				WorkingDir:        "/work/a/b",
				RootWorkingDir:    "/work",
				MaxFoldersToCheck: 50,
			},
			wantErrType: tflint.ConfigNotFound{},
		},
		{
			name:      "MaxFoldersToCheck of zero returns ConfigNotFound immediately",
			seedFiles: []string{"/work/unit/.tflint.hcl"},
			opts: tflint.TFLintOptions{
				WorkingDir:        "/work/unit",
				RootWorkingDir:    "/work",
				MaxFoldersToCheck: 0,
			},
			wantErrType: tflint.ConfigNotFound{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := vfs.NewMemMapFS()
			for _, p := range tc.seedFiles {
				require.NoError(t, vfs.WriteFile(fs, p, []byte("config {}"), 0o644))
			}

			l := logger.CreateLogger()

			got, err := tflint.FindConfigInProject(l, fs, &tc.opts)
			if tc.wantErrType != nil {
				require.Error(t, err)

				var notFound tflint.ConfigNotFound

				assert.True(t, errors.As(err, &notFound), "expected ConfigNotFound, got %T", err)
				assert.Empty(t, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantPath, got)
		})
	}
}

func TestRunTflintWithOpts_HappyPath(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/work/unit/.tflint.hcl", []byte("config {}"), 0o644))

	var calls []vexec.Invocation

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		calls = append(calls, cloneInvocation(&inv))

		return vexec.Result{}
	})

	runErr := runWithOpts(t, fs, exec, &runcfg.Hook{
		Name:     "tflint",
		Commands: []string{"plan"},
		Execute:  []string{"tflint"},
	}, &runcfg.RunConfig{
		Inputs: map[string]any{"region": "us-east-1"},
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{
				{
					Commands:  []string{"plan"},
					Arguments: []string{"-var=foo=bar"},
				},
			},
		},
	})
	require.NoError(t, runErr)

	require.Len(t, calls, 2, "expected exactly init + lint subprocess calls")

	// Both invocations name the tflint binary and are dispatched from RootWorkingDir.
	assert.Equal(t, "tflint", calls[0].Name)
	assert.Equal(t, "tflint", calls[1].Name)

	// init call carries --init plus the resolved relative paths.
	assert.Equal(t, []string{"--init", "--config", "./.tflint.hcl", "--chdir", "./unit"}, calls[0].Args)

	// lint call: the fixed prefix is deterministic; --var/--var-file order is
	// map-iteration dependent so check membership separately.
	require.GreaterOrEqual(t, len(calls[1].Args), 5)
	assert.Equal(t, []string{"--config", "./.tflint.hcl", "--chdir", "./unit"}, calls[1].Args[:4])
	assert.Contains(t, calls[1].Args, "--var=region=us-east-1")
	assert.Contains(t, calls[1].Args, "--var='foo=bar'")
}

func TestRunTflintWithOpts_StripsExternalTflintFlag(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/work/unit/.tflint.hcl", []byte("config {}"), 0o644))

	var calls []vexec.Invocation

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		calls = append(calls, cloneInvocation(&inv))

		return vexec.Result{}
	})

	require.NoError(t, runWithOpts(t, fs, exec, &runcfg.Hook{
		Name:     "tflint",
		Commands: []string{"plan"},
		Execute:  []string{"tflint", "--terragrunt-external-tflint", "--minimum-failure-severity=warning"},
	}, &runcfg.RunConfig{}))

	require.Len(t, calls, 2)

	for _, inv := range calls {
		assert.NotContains(t, inv.Args, "--terragrunt-external-tflint",
			"flag should not be forwarded to the tflint binary")
	}

	assert.Contains(t, calls[1].Args, "--minimum-failure-severity=warning")
}

func TestRunTflintWithOpts_InitFailureWraps(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/work/unit/.tflint.hcl", []byte("config {}"), 0o644))

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if slices.Contains(inv.Args, "--init") {
			return vexec.Result{ExitCode: 1, Stderr: []byte("init failed\n")}
		}

		return vexec.Result{}
	})

	err := runWithOpts(t, fs, exec, &runcfg.Hook{
		Name:     "tflint",
		Commands: []string{"plan"},
		Execute:  []string{"tflint"},
	}, &runcfg.RunConfig{})

	require.Error(t, err)

	var wrapped tflint.ErrorRunningTflint

	require.True(t, errors.As(err, &wrapped), "expected ErrorRunningTflint, got %T", err)
	assert.Contains(t, wrapped.Args, "--init")
}

func TestRunTflintWithOpts_LintFailureWraps(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/work/unit/.tflint.hcl", []byte("config {}"), 0o644))

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if slices.Contains(inv.Args, "--init") {
			return vexec.Result{}
		}

		return vexec.Result{ExitCode: 2, Stderr: []byte("3 issue(s) found\n")}
	})

	err := runWithOpts(t, fs, exec, &runcfg.Hook{
		Name:     "tflint",
		Commands: []string{"plan"},
		Execute:  []string{"tflint"},
	}, &runcfg.RunConfig{})

	require.Error(t, err)

	var wrapped tflint.ErrorRunningTflint

	require.True(t, errors.As(err, &wrapped), "expected ErrorRunningTflint, got %T", err)
	assert.NotContains(t, wrapped.Args, "--init")
}

func TestRunTflintWithOpts_MissingConfigSurfacesNotFound(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		t.Fatal("subprocess invoked despite missing config")
		return vexec.Result{}
	})

	err := runWithOpts(t, fs, exec, &runcfg.Hook{
		Name:     "tflint",
		Commands: []string{"plan"},
		Execute:  []string{"tflint"},
	}, &runcfg.RunConfig{})

	require.Error(t, err)

	var notFound tflint.ConfigNotFound

	assert.True(t, errors.As(err, &notFound), "expected ConfigNotFound, got %T", err)
}

// runWithOpts wires the standard test fixture so each test does not have to
// rebuild the same TFLintOptions and Venv.
func runWithOpts(
	t *testing.T,
	fs vfs.FS,
	exec vexec.Exec,
	hook *runcfg.Hook,
	cfg *runcfg.RunConfig,
) error {
	t.Helper()

	opts := &tflint.TFLintOptions{
		ShellOptions:      shell.NewShellOptions(),
		WorkingDir:        "/work/unit",
		RootWorkingDir:    "/work",
		MaxFoldersToCheck: 5,
	}

	venv := &tflint.Venv{
		Exec:    exec,
		FS:      fs,
		Env:     map[string]string{},
		Writers: &writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}

	l := logger.CreateLogger()

	return tflint.RunTflintWithOpts(t.Context(), l, venv, opts, cfg, hook)
}

func cloneInvocation(inv *vexec.Invocation) vexec.Invocation {
	return vexec.Invocation{
		Name: inv.Name,
		Dir:  inv.Dir,
		Args: slices.Clone(inv.Args),
		Env:  slices.Clone(inv.Env),
	}
}
