package run_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestRunWritesVarFilesToCacheDir(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	files := &varFilesRecorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(files.handler())).WithFS(vfs.NewOSFS())

	cfg := &runcfg.RunConfig{
		Inputs: map[string]any{"from_env": "env"},
		TFVarsInputs: map[string]any{
			"name":     "value",
			"template": "${not_a_reference}",
			"tags":     map[string]any{"team": "platform"},
			"unset":    nil,
		},
		JSONInputs: map[string]any{
			"count":    json.Number("2"),
			"template": "${not_a_reference}",
		},
	}

	require.NoError(
		t,
		run.Run(t.Context(), logger.CreateLogger(), v, newRunE2EOpts(t, s, "plan"), report.NewReport(), cfg, creds.NewGetter()),
	)

	workingDir := cacheWorkingDir(t, s)
	tfvars := filepath.Join(workingDir, run.TFVarsFile)
	jsonVars := filepath.Join(workingDir, run.JSONVarsFile)

	assert.Equal(t, `name = "value"
tags = {
  "team" = "platform"
}
template = "$${not_a_reference}"
unset    = null
`, files.contents[tfvars])
	assert.JSONEq(t, `{"count": 2, "template": "${not_a_reference}"}`, files.contents[jsonVars])

	assert.Contains(t, files.env, "TF_VAR_from_env=env")
	assert.NotContains(t, files.env, `TF_VAR_name="value"`)

	assert.FileExists(t, tfvars, "the tfvars file must stay in the cache directory after the run")
	assert.FileExists(t, jsonVars, "the tfvars.json file must stay in the cache directory after the run")
}

func TestRunWritesTFVarsThatParse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value any
		name  string
	}{
		{name: "object with a keyword key", value: map[string]any{"for": "team"}},
		{name: "keyword key in a list", value: []any{map[string]any{"for": "team"}}},
		{name: "nested keyword keys", value: map[string]any{"for": map[string]any{"for": "team", "if": "x"}}},
		{name: "empty object", value: map[string]any{}},
		{name: "empty list", value: []any{}},
		{name: "null", value: nil},
		{name: "nested null", value: map[string]any{"unset": nil}},
		{name: "template sequences", value: "${var.x} %{if}"},
		{name: "multiple lines", value: "line1\nline2\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := setupRunE2EScaffold(t)
			files := &varFilesRecorder{}
			v := venvtest.New().WithExec(vexec.NewMemExec(files.handler())).WithFS(vfs.NewOSFS())

			cfg := &runcfg.RunConfig{TFVarsInputs: map[string]any{"value": tc.value}}

			require.NoError(
				t,
				run.Run(t.Context(), logger.CreateLogger(), v, newRunE2EOpts(t, s, "plan"), report.NewReport(), cfg, creds.NewGetter()),
			)

			contents := files.contents[filepath.Join(cacheWorkingDir(t, s), run.TFVarsFile)]

			parsed, diags := hclsyntax.ParseConfig([]byte(contents), run.TFVarsFile, hcl.Pos{Line: 1, Column: 1})
			require.False(t, diags.HasErrors(), "%s", diags)

			attrs, diags := parsed.Body.JustAttributes()
			require.False(t, diags.HasErrors(), "%s", diags)

			value, diags := attrs["value"].Expr.Value(nil)
			require.False(t, diags.HasErrors(), "%s", diags)

			decoded, err := ctyhelper.ParseCtyValueToMap(cty.ObjectVal(map[string]cty.Value{"value": value}))
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"value": tc.value}, decoded)
		})
	}
}

func TestRunWritesVarFilesForOwnerOnly(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Windows does not have POSIX file modes")
	}

	s := setupRunE2EScaffold(t)
	files := &varFilesRecorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(files.handler())).WithFS(vfs.NewOSFS())

	cfg := &runcfg.RunConfig{
		TFVarsInputs: map[string]any{"name": "value"},
		JSONInputs:   map[string]any{"other": "value"},
	}

	require.NoError(
		t,
		run.Run(t.Context(), logger.CreateLogger(), v, newRunE2EOpts(t, s, "plan"), report.NewReport(), cfg, creds.NewGetter()),
	)

	workingDir := cacheWorkingDir(t, s)

	assert.Equal(t, map[string]os.FileMode{
		filepath.Join(workingDir, run.TFVarsFile):   0o600,
		filepath.Join(workingDir, run.JSONVarsFile): 0o600,
	}, files.modes)
}

func TestRunDebugFileIncludesVarFileInputs(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	require.NoError(t, os.WriteFile(filepath.Join(s.dir, "main.tf"), []byte(`
variable "from_env" {}
variable "from_tfvar" {}
variable "from_json" {}
`), 0o644))

	rec := &recorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

	opts := newRunE2EOpts(t, s, "plan")
	opts.Debug = true

	cfg := &runcfg.RunConfig{
		Inputs:       map[string]any{"from_env": "env"},
		TFVarsInputs: map[string]any{"from_tfvar": "tfvar", "undeclared": "tfvar"},
		JSONInputs:   map[string]any{"from_json": "json"},
	}

	require.NoError(t, run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), cfg, creds.NewGetter()))

	contents, err := os.ReadFile(filepath.Join(s.dir, run.TerragruntTFVarsFile))
	require.NoError(t, err)
	assert.JSONEq(t, `{"from_env": "env", "from_tfvar": "tfvar", "from_json": "json"}`, string(contents))
}

func TestRunWritesVarFilesForInputsSetInEnv(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	files := &varFilesRecorder{}
	v := venvtest.New().
		WithExec(vexec.NewMemExec(files.handler())).
		WithFS(vfs.NewOSFS()).
		WithEnv(map[string]string{"TF_VAR_name": `"from-env"`, "TF_VAR_other": `"from-env"`})

	cfg := &runcfg.RunConfig{
		TFVarsInputs: map[string]any{"name": "from-config"},
		JSONInputs:   map[string]any{"other": "from-config"},
	}

	require.NoError(
		t,
		run.Run(t.Context(), logger.CreateLogger(), v, newRunE2EOpts(t, s, "plan"), report.NewReport(), cfg, creds.NewGetter()),
	)

	workingDir := cacheWorkingDir(t, s)

	assert.Equal(t, "name = \"from-config\"\n", files.contents[filepath.Join(workingDir, run.TFVarsFile)])
	assert.JSONEq(t, `{"other": "from-config"}`, files.contents[filepath.Join(workingDir, run.JSONVarsFile)])
}

func TestRunRemovesStaleVarFilesFromCacheDir(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	rec := &recorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())
	l := logger.CreateLogger()
	opts := newRunE2EOpts(t, s, "plan")
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.VariablesBlock))

	withFiles := &runcfg.RunConfig{
		TFVarsInputs: map[string]any{"name": "value"},
		JSONInputs:   map[string]any{"name": "value"},
	}
	require.NoError(t, run.Run(t.Context(), l, v, opts, report.NewReport(), withFiles, creds.NewGetter()))

	workingDir := cacheWorkingDir(t, s)
	require.FileExists(t, filepath.Join(workingDir, run.TFVarsFile))
	require.FileExists(t, filepath.Join(workingDir, run.JSONVarsFile))

	envOnly := &runcfg.RunConfig{Inputs: map[string]any{"name": "value"}}
	require.NoError(t, run.Run(t.Context(), l, v, opts, report.NewReport(), envOnly, creds.NewGetter()))

	assert.NoFileExists(t, filepath.Join(workingDir, run.TFVarsFile))
	assert.NoFileExists(t, filepath.Join(workingDir, run.JSONVarsFile))
}

func TestRunReservesVarFileNamesOnlyWithTheExperiment(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		experiment bool
	}{
		{name: "experiment off", experiment: false},
		{name: "experiment on", experiment: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := setupRunE2EScaffold(t)
			require.NoError(t, os.WriteFile(filepath.Join(s.dir, run.TFVarsFile), []byte(`name = "mine"`), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(s.dir, run.JSONVarsFile), []byte(`{"other": "mine"}`), 0o644))

			rec := &recorder{}
			v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

			opts := newRunE2EOpts(t, s, "plan")
			if tc.experiment {
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.VariablesBlock))
			}

			require.NoError(
				t,
				run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), &runcfg.RunConfig{}, creds.NewGetter()),
			)

			workingDir := cacheWorkingDir(t, s)

			if tc.experiment {
				assert.NoFileExists(t, filepath.Join(workingDir, run.TFVarsFile))
				assert.NoFileExists(t, filepath.Join(workingDir, run.JSONVarsFile))

				return
			}

			assert.FileExists(t, filepath.Join(workingDir, run.TFVarsFile))
			assert.FileExists(t, filepath.Join(workingDir, run.JSONVarsFile))
		})
	}
}

func TestRunNoDotTerragruntCacheVarFilesAreWrittenForTheRun(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	files := &varFilesRecorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(files.handler())).WithFS(vfs.NewOSFS())

	cfg := &runcfg.RunConfig{
		Terraform:    runcfg.TerraformConfig{NoDotTerragruntCache: true},
		TFVarsInputs: map[string]any{"name": "value"},
		JSONInputs:   map[string]any{"other": "value"},
	}

	require.NoError(
		t,
		run.Run(
			t.Context(), logger.CreateLogger(), v, newNoDotTerragruntCacheOpts(t, s, "plan"),
			report.NewReport(), cfg, creds.NewGetter(),
		),
	)

	for _, name := range []string{run.TFVarsFile, run.JSONVarsFile} {
		path := filepath.Join(s.dir, name)

		assert.Contains(t, files.contents, path, "%s must exist in the unit directory while tofu runs", name)
		assert.NoFileExists(t, path, "%s must be removed from the unit directory after the run", name)
	}
}

func TestRunNoDotTerragruntCacheVarFilesKeepExistingFile(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	tfvars := filepath.Join(s.dir, run.TFVarsFile)
	require.NoError(t, os.WriteFile(tfvars, []byte(`name = "mine"`), 0o644))

	rec := &recorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

	cfg := &runcfg.RunConfig{
		Terraform:    runcfg.TerraformConfig{NoDotTerragruntCache: true},
		TFVarsInputs: map[string]any{"name": "value"},
	}

	err := run.Run(
		t.Context(), logger.CreateLogger(), v, newNoDotTerragruntCacheOpts(t, s, "plan"),
		report.NewReport(), cfg, creds.NewGetter(),
	)

	var exists run.VarsFileExists

	require.ErrorAs(t, err, &exists)
	assert.Equal(t, tfvars, exists.Path)
	assert.Empty(t, rec.snapshot(), "tofu must not run")

	contents, err := os.ReadFile(tfvars)
	require.NoError(t, err)
	assert.Equal(t, `name = "mine"`, string(contents))
}

func TestRunNoDotTerragruntCacheRemovesWrittenFileWhenALaterWriteFails(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	existing := filepath.Join(s.dir, run.JSONVarsFile)
	require.NoError(t, os.WriteFile(existing, []byte(`{"name": "mine"}`), 0o644))

	rec := &recorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

	cfg := &runcfg.RunConfig{
		Terraform:    runcfg.TerraformConfig{NoDotTerragruntCache: true},
		TFVarsInputs: map[string]any{"name": "value"},
		JSONInputs:   map[string]any{"other": "value"},
	}

	err := run.Run(
		t.Context(), logger.CreateLogger(), v, newNoDotTerragruntCacheOpts(t, s, "plan"),
		report.NewReport(), cfg, creds.NewGetter(),
	)

	var exists run.VarsFileExists

	require.ErrorAs(t, err, &exists)
	assert.Equal(t, existing, exists.Path)
	assert.Empty(t, rec.snapshot(), "tofu must not run")
	assert.NoFileExists(t, filepath.Join(s.dir, run.TFVarsFile))

	contents, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name": "mine"}`, string(contents))
}

func TestRunNoDotTerragruntCacheKeepsFileOfUnusedKind(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	jsonVars := filepath.Join(s.dir, run.JSONVarsFile)
	require.NoError(t, os.WriteFile(jsonVars, []byte(`{"name": "mine"}`), 0o644))

	rec := &recorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

	cfg := &runcfg.RunConfig{
		Terraform:    runcfg.TerraformConfig{NoDotTerragruntCache: true},
		TFVarsInputs: map[string]any{"name": "value"},
	}

	opts := newNoDotTerragruntCacheOpts(t, s, "plan")
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.VariablesBlock))

	require.NoError(t, run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), cfg, creds.NewGetter()))

	contents, err := os.ReadFile(jsonVars)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name": "mine"}`, string(contents))
}

func TestRunRejectsTFVarsInputWithInvalidName(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	rec := &recorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

	cfg := &runcfg.RunConfig{TFVarsInputs: map[string]any{"not-valid.name": "value"}}

	err := run.Run(
		t.Context(), logger.CreateLogger(), v, newRunE2EOpts(t, s, "plan"), report.NewReport(), cfg, creds.NewGetter(),
	)

	var nameErr run.InvalidVariableNameError

	require.ErrorAs(t, err, &nameErr)
	assert.Equal(t, "not-valid.name", nameErr.Name)
	assert.Empty(t, rec.snapshot(), "tofu must not run")
}

func TestRunValidatesDeclaredInputs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		module     string
		undeclared []string
	}{
		{
			name:   "every validated input is declared",
			module: `variable "declared" {}`,
		},
		{
			name:       "validated inputs are not declared",
			module:     `variable "other" {}`,
			undeclared: []string{"also_missing", "declared"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := setupRunE2EScaffold(t)
			require.NoError(t, os.WriteFile(filepath.Join(s.dir, "main.tf"), []byte(tc.module), 0o644))

			rec := &recorder{}
			v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

			validated := map[string]struct{}{"declared": {}}
			if tc.undeclared != nil {
				validated["also_missing"] = struct{}{}
			}

			cfg := &runcfg.RunConfig{
				TFVarsInputs:    map[string]any{"declared": "value", "also_missing": "value"},
				ValidatedInputs: validated,
			}

			err := run.Run(
				t.Context(), logger.CreateLogger(), v, newRunE2EOpts(t, s, "plan"),
				report.NewReport(), cfg, creds.NewGetter(),
			)

			if tc.undeclared == nil {
				require.NoError(t, err)
				assert.Len(t, rec.snapshot(), 1)

				return
			}

			var undeclaredErr run.UndeclaredInputsError

			require.ErrorAs(t, err, &undeclaredErr)
			assert.Equal(t, tc.undeclared, undeclaredErr.Names)
			assert.Empty(t, rec.snapshot(), "tofu must not run")
		})
	}
}

func TestRunValidatesDeclaredInputsReportsInvalidModule(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	require.NoError(t, os.WriteFile(filepath.Join(s.dir, "main.tf"), []byte(`variable {{{`), 0o644))

	rec := &recorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

	cfg := &runcfg.RunConfig{ValidatedInputs: map[string]struct{}{"name": {}}}

	err := run.Run(
		t.Context(), logger.CreateLogger(), v, newRunE2EOpts(t, s, "plan"), report.NewReport(), cfg, creds.NewGetter(),
	)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	assert.Empty(t, rec.snapshot(), "tofu must not run")
}

func TestRunValidatesDeclaredInputsBeforeBackendBootstrap(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	require.NoError(t, os.WriteFile(filepath.Join(s.dir, "main.tf"), []byte(`
terraform {
  backend "s3" {}
}
`), 0o644))

	rec := &recorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

	opts := newRunE2EOpts(t, s, "init")
	opts.BackendBootstrap = true

	cfg := &runcfg.RunConfig{
		RemoteState: *remotestate.New(&remotestate.Config{
			BackendName:   "s3",
			BackendConfig: backend.Config{"bucket": "state", "key": "tofu.tfstate", "region": "us-east-1"},
		}),
		ValidatedInputs: map[string]struct{}{"undeclared": {}},
	}

	err := run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), cfg, creds.NewGetter())

	var undeclaredErr run.UndeclaredInputsError

	require.ErrorAs(t, err, &undeclaredErr)
	assert.Empty(t, rec.snapshot(), "tofu must not run")
}

// varFilesRecorder records, when tofu runs, the variable files in its working directory and its environment.
type varFilesRecorder struct {
	contents map[string]string
	modes    map[string]os.FileMode
	env      []string
	mu       sync.Mutex
}

func (r *varFilesRecorder) handler() vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		r.mu.Lock()
		defer r.mu.Unlock()

		r.contents = map[string]string{}
		r.modes = map[string]os.FileMode{}
		r.env = inv.Env

		for _, name := range []string{run.TFVarsFile, run.JSONVarsFile} {
			path := filepath.Join(inv.Dir, name)

			contents, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			r.contents[path] = string(contents)

			info, err := os.Stat(path)
			if err != nil {
				continue
			}

			r.modes[path] = info.Mode().Perm()
		}

		return vexec.Result{}
	}
}

// cacheWorkingDir returns the single cache working directory a run of s copied the unit into.
func cacheWorkingDir(t *testing.T, s runE2EScaffold) string {
	t.Helper()

	mainFiles, err := filepath.Glob(filepath.Join(s.dir, ".terragrunt-cache", "*", "*", "main.tf"))
	require.NoError(t, err)
	require.Len(t, mainFiles, 1, "expected exactly one cache working directory")

	return filepath.Dir(mainFiles[0])
}
