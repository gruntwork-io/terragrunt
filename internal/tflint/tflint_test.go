package tflint_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tflint"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

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
			expected: []string{"--var=instance_count=3", "--var=region=eu-central-1"},
		},
		{
			name: "keys are emitted in sorted order",
			inputs: map[string]any{
				"zeta":  1,
				"alpha": 2,
				"mu":    3,
				"beta":  4,
			},
			expected: []string{
				"--var=alpha=2",
				"--var=beta=4",
				"--var=mu=3",
				"--var=zeta=1",
			},
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
			assert.Equal(t, tc.expected, actual)
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
			name: "TF_VAR_ env vars are emitted in sorted order",
			hook: runcfg.Hook{Commands: []string{"plan"}},
			cfg: runcfg.TerraformConfig{ExtraArgs: []runcfg.TerraformExtraArguments{
				{
					Commands: []string{"plan"},
					EnvVars: map[string]string{
						"TF_VAR_zeta":  "1",
						"TF_VAR_alpha": "2",
						"TF_VAR_mu":    "3",
						"TF_VAR_beta":  "4",
					},
				},
			}},
			expected: []string{
				"--var='alpha=2'",
				"--var='beta=4'",
				"--var='mu=3'",
				"--var='zeta=1'",
			},
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

			fsys := vfs.NewMemMapFS()
			require.NoError(t, vfs.WriteFile(fsys, optionalPresent, []byte("x = 1"), 0o644))

			l := logger.CreateLogger()
			actual, err := tflint.TFArgumentsToVar(l, fsys, &tc.hook, &tc.cfg)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestConfigFilePath_ShortCircuitsOnConfigFlag(t *testing.T) {
	t.Parallel()

	const explicit = "/explicit/.tflint.hcl"

	testCases := []struct {
		name      string
		arguments []string
	}{
		{
			name:      "long flag, space separated",
			arguments: []string{"tflint", "--config", explicit},
		},
		{
			name:      "long flag, equals separated",
			arguments: []string{"tflint", "--config=" + explicit},
		},
		{
			name:      "short flag, space separated",
			arguments: []string{"tflint", "-c", explicit},
		},
		{
			name:      "short flag, equals separated",
			arguments: []string{"tflint", "-c=" + explicit},
		},
		{
			name:      "flag follows unrelated arguments",
			arguments: []string{"tflint", "--minimum-failure-severity=error", "--config=" + explicit},
		},
		{
			name:      "earliest of several config flags is used",
			arguments: []string{"tflint", "--config=" + explicit, "--config=/other/.tflint.hcl"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// An empty FS makes the parent walk impossible, so a successful
			// return can only have come from the arguments.
			fsys := vfs.NewMemMapFS()
			l := logger.CreateLogger()

			got, err := tflint.ConfigFilePath(l, fsys, &tflint.TFLintOptions{
				WorkingDir:        "/work/unit",
				RootWorkingDir:    "/work",
				MaxFoldersToCheck: 5,
			}, tc.arguments)

			require.NoError(t, err)
			assert.Equal(t, explicit, got)
		})
	}
}

func TestConfigFilePath_IgnoresNonConfigArguments(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		arguments []string
	}{
		{
			name:      "no config flag at all",
			arguments: []string{"tflint"},
		},
		{
			name:      "flag names sharing the config prefix",
			arguments: []string{"tflint", "--config-file=/other/.tflint.hcl", "--chdir=/other"},
		},
		{
			name:      "long flag with no value to consume",
			arguments: []string{"tflint", "--config"},
		},
		{
			name:      "short flag with no value to consume",
			arguments: []string{"tflint", "-c"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			require.NoError(t, vfs.WriteFile(fsys, "/work/.tflint.hcl", []byte("config {}"), 0o644))

			l := logger.CreateLogger()

			got, err := tflint.ConfigFilePath(l, fsys, &tflint.TFLintOptions{
				WorkingDir:        "/work/unit",
				RootWorkingDir:    "/work",
				MaxFoldersToCheck: 5,
			}, tc.arguments)

			require.NoError(t, err)
			assert.Equal(t, filepath.FromSlash("/work/.tflint.hcl"), got)
		})
	}
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

			fsys := vfs.NewMemMapFS()
			for _, p := range tc.seedFiles {
				require.NoError(t, vfs.WriteFile(fsys, p, []byte("config {}"), 0o644))
			}

			l := logger.CreateLogger()

			got, err := tflint.FindConfigInProject(l, fsys, &tc.opts)
			if tc.wantErrType != nil {
				require.Error(t, err)

				var notFound tflint.ConfigNotFound

				require.ErrorAs(t, err, &notFound, "expected ConfigNotFound, got %T", err)
				assert.Empty(t, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, filepath.FromSlash(tc.wantPath), got)
		})
	}
}

func TestRunTflintWithOpts_HappyPath(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/work/unit/.tflint.hcl", []byte("config {}"), 0o644))

	var calls []vexec.Invocation

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		calls = append(calls, cloneInvocation(&inv))

		return vexec.Result{}
	})

	runErr := runWithOpts(t, fsys, exec, &runcfg.Hook{
		Name:     "tflint",
		Commands: []string{"plan"},
		Execute:  []string{"tflint"},
	}, &runcfg.RunConfig{
		Inputs: map[string]any{"region": "us-east-1", "instance_count": 3},
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{
				{
					Commands:  []string{"plan"},
					Arguments: []string{"-var=foo=bar"},
					EnvVars:   map[string]string{"TF_VAR_zone": "a", "TF_VAR_ami": "b"},
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
	assert.Equal(
		t,
		[]string{
			"--init",
			"--config",
			filepath.FromSlash("./.tflint.hcl"),
			"--chdir",
			filepath.FromSlash("./unit"),
		},
		calls[0].Args,
	)

	assert.Equal(t, []string{
		"--config", filepath.FromSlash("./.tflint.hcl"),
		"--chdir", filepath.FromSlash("./unit"),
		"--var=instance_count=3",
		"--var=region=us-east-1",
		"--var='ami=b'",
		"--var='zone=a'",
		"--var='foo=bar'",
	}, calls[1].Args)
}

func TestRunTflintWithOpts_HonorsConfigFlagInHook(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	// Deliberately not seeded anywhere the parent walk would reach, so the
	// run can only succeed by reading the path out of the hook arguments.
	require.NoError(t, vfs.WriteFile(fsys, "/elsewhere/custom.tflint.hcl", []byte("config {}"), 0o644))

	var calls []vexec.Invocation

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		calls = append(calls, cloneInvocation(&inv))

		return vexec.Result{}
	})

	require.NoError(t, runWithOpts(t, fsys, exec, &runcfg.Hook{
		Name:     "tflint",
		Commands: []string{"plan"},
		Execute:  []string{"tflint", "--config=/elsewhere/custom.tflint.hcl"},
	}, &runcfg.RunConfig{}))

	require.Len(t, calls, 2)
	assert.Equal(t, []string{
		"--init",
		"--config", filepath.FromSlash("../../elsewhere/custom.tflint.hcl"),
		"--chdir", filepath.FromSlash("./unit"),
	}, calls[0].Args)
}

func TestRunTflintWithOpts_StripsExternalTflintFlag(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/work/unit/.tflint.hcl", []byte("config {}"), 0o644))

	var calls []vexec.Invocation

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		calls = append(calls, cloneInvocation(&inv))

		return vexec.Result{}
	})

	require.NoError(t, runWithOpts(t, fsys, exec, &runcfg.Hook{
		Name:     "tflint",
		Commands: []string{"plan"},
		Execute: []string{
			"tflint",
			"--terragrunt-external-tflint",
			"--minimum-failure-severity=warning",
		},
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

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/work/unit/.tflint.hcl", []byte("config {}"), 0o644))

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if slices.Contains(inv.Args, "--init") {
			return vexec.Result{ExitCode: 1, Stderr: []byte("init failed\n")}
		}

		return vexec.Result{}
	})

	err := runWithOpts(t, fsys, exec, &runcfg.Hook{
		Name:     "tflint",
		Commands: []string{"plan"},
		Execute:  []string{"tflint"},
	}, &runcfg.RunConfig{})

	require.Error(t, err)

	var wrapped tflint.ErrorRunningTflint

	require.ErrorAs(t, err, &wrapped, "expected ErrorRunningTflint, got %T", err)
	assert.Contains(t, wrapped.Args, "--init")
}

func TestRunTflintWithOpts_LintFailureWraps(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/work/unit/.tflint.hcl", []byte("config {}"), 0o644))

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if slices.Contains(inv.Args, "--init") {
			return vexec.Result{}
		}

		return vexec.Result{ExitCode: 2, Stderr: []byte("3 issue(s) found\n")}
	})

	err := runWithOpts(t, fsys, exec, &runcfg.Hook{
		Name:     "tflint",
		Commands: []string{"plan"},
		Execute:  []string{"tflint"},
	}, &runcfg.RunConfig{})

	require.Error(t, err)

	var wrapped tflint.ErrorRunningTflint

	require.ErrorAs(t, err, &wrapped, "expected ErrorRunningTflint, got %T", err)
	assert.NotContains(t, wrapped.Args, "--init")
}

func TestRunTflintWithOpts_MissingConfigSurfacesNotFound(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		require.FailNow(t, "subprocess invoked despite missing config")
		return vexec.Result{}
	})

	err := runWithOpts(t, fsys, exec, &runcfg.Hook{
		Name:     "tflint",
		Commands: []string{"plan"},
		Execute:  []string{"tflint"},
	}, &runcfg.RunConfig{})

	require.Error(t, err)

	var notFound tflint.ConfigNotFound

	assert.ErrorAs(t, err, &notFound, "expected ConfigNotFound, got %T", err)
}

func TestRunTflintWithOpts_TooDeeplyNestedInputsFailBeforeRunning(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/work/unit/.tflint.hcl", []byte("config {}"), 0o644))

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		require.FailNow(t, "subprocess invoked despite unencodable inputs")
		return vexec.Result{}
	})

	err := runWithOpts(t, fsys, exec, &runcfg.Hook{
		Name:     "tflint",
		Commands: []string{"plan"},
		Execute:  []string{"tflint"},
	}, &runcfg.RunConfig{Inputs: map[string]any{"deep": nestedInput(tooDeepInput)}})

	require.Error(t, err, "inputs nested past the encoding limit should fail the run")
}

func TestInputsToTflintVar_RejectsTooDeeplyNestedInputs(t *testing.T) {
	t.Parallel()

	_, err := tflint.InputsToTflintVar(map[string]any{"deep": nestedInput(tooDeepInput)})
	require.Error(t, err, "inputs nested past the encoding limit should not be encoded")

	vars, err := tflint.InputsToTflintVar(map[string]any{"deep": nestedInput(tooDeepInput - 1)})
	require.NoError(t, err)
	assert.Len(t, vars, 1, "the deepest accepted nesting should still be encoded")
}

func TestTFArgumentsToVar_SkipsOptionalVarFileThatCannotBeRead(t *testing.T) {
	t.Parallel()

	const unreadable = "/work/unreadable.tfvars"

	fsys := statFailingFS{FS: vfs.NewMemMapFS(), path: unreadable}

	vars, err := tflint.TFArgumentsToVar(
		logger.CreateLogger(),
		fsys,
		&runcfg.Hook{Commands: []string{"plan"}},
		&runcfg.TerraformConfig{ExtraArgs: []runcfg.TerraformExtraArguments{
			{
				Commands:         []string{"plan"},
				RequiredVarFiles: []string{"/work/required.tfvars"},
				OptionalVarFiles: []string{unreadable},
			},
		}},
	)

	require.NoError(t, err, "an unreadable optional var file should not fail the conversion")
	assert.Equal(t, []string{"--var-file=/work/required.tfvars"}, vars,
		"an unreadable optional var file should be left out")
}

func TestFindConfigInProject_SurfacesStatErrors(t *testing.T) {
	t.Parallel()

	fsys := statFailingFS{FS: vfs.NewMemMapFS(), path: filepath.FromSlash("/work/unit/.tflint.hcl")}

	_, err := tflint.FindConfigInProject(logger.CreateLogger(), fsys, &tflint.TFLintOptions{
		WorkingDir:        "/work/unit",
		RootWorkingDir:    "/work",
		MaxFoldersToCheck: 3,
	})

	require.ErrorIs(t, err, errStat, "a failing stat should not read as a missing config")
}

func TestErrorTypes_Error(t *testing.T) {
	t.Parallel()

	errCause := errors.New("exit status 2")
	args := []string{"tflint", "--chdir", "./unit"}

	testCases := []struct {
		err       error
		cause     error
		name      string
		mentioned []string
	}{
		{
			name:      "run error with a cause",
			err:       tflint.ErrorRunningTflint{Args: args, Err: errCause},
			cause:     errCause,
			mentioned: []string{"--chdir", "./unit", errCause.Error()},
		},
		{
			name:      "run error without a cause",
			err:       tflint.ErrorRunningTflint{Args: args},
			mentioned: []string{"--chdir", "./unit"},
		},
		{
			name: "issues found",
			err:  tflint.IssuesFound{},
		},
		{
			name: "unknown status code",
			err:  tflint.UnknownError{},
		},
		{
			name: "config not found",
			err:  tflint.ConfigNotFound{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			require.NotEmpty(t, msg, "every error should describe itself")

			for _, want := range tc.mentioned {
				assert.Contains(t, msg, want, "the message should name the failing invocation and its cause")
			}

			assert.Equal(t, tc.cause, errors.Unwrap(tc.err), "only a run error with a cause should unwrap")
		})
	}
}

// runWithOpts wires the standard test fixture so each test does not have to
// rebuild the same TFLintOptions and Venv.
func runWithOpts(
	t *testing.T,
	fsys vfs.FS,
	exec vexec.Exec,
	hook *runcfg.Hook,
	cfg *runcfg.RunConfig,
) error {
	t.Helper()

	opts := &tflint.TFLintOptions{
		ShellOptions:      shell.NewShellOptions(map[string]string{}),
		WorkingDir:        "/work/unit",
		RootWorkingDir:    "/work",
		MaxFoldersToCheck: 5,
	}

	v := venvtest.New().WithExec(exec).WithFS(fsys)

	l := logger.CreateLogger()

	return tflint.RunTflintWithOpts(t.Context(), l, v, opts, cfg, hook)
}

func cloneInvocation(inv *vexec.Invocation) vexec.Invocation {
	return vexec.Invocation{
		Name: inv.Name,
		Dir:  inv.Dir,
		Args: slices.Clone(inv.Args),
		Env:  slices.Clone(inv.Env),
	}
}

// tooDeepInput is one level past the deepest nesting the input encoder accepts.
const tooDeepInput = 101

// errStat is the error statFailingFS reports.
var errStat = errors.New("stat failed")

// statFailingFS fails Stat for one path and serves everything else from FS.
type statFailingFS struct {
	vfs.FS
	path string
}

func (fsys statFailingFS) Stat(name string) (os.FileInfo, error) {
	if name == fsys.path {
		return nil, errStat
	}

	return fsys.FS.Stat(name)
}

// nestedInput returns a map nested depth levels deep.
func nestedInput(depth int) any {
	var value any = "leaf"

	for range depth {
		value = map[string]any{"next": value}
	}

	return value
}
