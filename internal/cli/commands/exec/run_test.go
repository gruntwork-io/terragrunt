package exec_test

import (
	"bytes"
	"context"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/exec"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inputsHCL passes one input, which exec exposes to the target command as a TF_VAR_ variable.
const inputsHCL = `
inputs = {
  greeting = "hello"
}
`

// targetCommand is the command every test asks exec to run.
const targetCommand = "list-files"

var memRoot = venvtest.Root("/exec")

// TestNewCommandShowsHelpWithoutATargetCommand pins that bare `exec` prints its help instead of running anything.
func TestNewCommandShowsHelpWithoutATargetCommand(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": ""})

	cmd := exec.NewCommand(logger.CreateLogger(), newOpts(t, v), v, new(rewrite.Store))
	cmd.CustomHelpTemplate = "exec help"

	out, err := runCommand(t, cmd)
	require.NoError(t, err)
	assert.Equal(t, "exec help", out)
	assert.Empty(t, rec.invocations(), "help runs no command")
}

// TestNewCommandRunsTheTargetCommand pins where the target command runs and the inputs it receives.
func TestNewCommandRunsTheTargetCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		args         []string
		wantCacheDir bool
	}{
		{name: "after the separator", args: []string{"--", targetCommand, "main.tf"}},
		{name: "without the separator", args: []string{targetCommand, "main.tf"}},
		{
			name:         "in the download dir",
			args:         []string{"--in-download-dir", "--", targetCommand, "main.tf"},
			wantCacheDir: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": inputsHCL, "unit/main.tf": ""})

			_, err := runCommand(t, exec.NewCommand(logger.CreateLogger(), newOpts(t, v), v, new(rewrite.Store)), tc.args...)
			require.NoError(t, err)

			inv := rec.find(targetCommand)
			require.NotNil(t, inv, "the target command ran")
			assert.Equal(t, []string{"main.tf"}, inv.Args)
			assert.Contains(t, inv.Env, "TF_VAR_greeting=hello", "inputs reach the target command")

			unitDir := filepath.Join(memRoot, "unit")
			if !tc.wantCacheDir {
				assert.Equal(t, unitDir, inv.Dir)

				return
			}

			assert.True(
				t,
				strings.HasPrefix(inv.Dir, filepath.Join(unitDir, ".terragrunt-cache")+string(filepath.Separator)),
				"runs in the download dir: %s", inv.Dir,
			)
		})
	}
}

// TestNewCommandReportsPreparationFailures pins the typed errors exec returns before the target command runs.
func TestNewCommandReportsPreparationFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files map[string]string
		check func(t *testing.T, err error)
		name  string
		args  []string
	}{
		{
			name:  "invalid config",
			files: map[string]string{"unit/terragrunt.hcl": "this is ) not ( valid hcl", "unit/main.tf": ""},
			check: func(t *testing.T, err error) {
				t.Helper()

				var diags hcl.Diagnostics

				require.ErrorAs(t, err, &diags)
			},
		},
		{
			name: "source that fails to download",
			files: map[string]string{
				"unit/terragrunt.hcl": "terraform {\n  source = \"../modules/missing\"\n}\n",
			},
			check: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, fs.ErrNotExist)
			},
		},
		{
			name: "generate block that refuses to overwrite",
			files: map[string]string{
				"unit/terragrunt.hcl": "generate \"main\" {\n  path      = \"main.tf\"\n" +
					"  if_exists = \"error\"\n  contents  = \"\"\n}\n",
				"unit/main.tf": "",
			},
			check: func(t *testing.T, err error) {
				t.Helper()

				var existsErr codegen.GenerateFileExistsError

				require.ErrorAs(t, err, &existsErr)
			},
		},
		{
			name:  "unit without OpenTofu/Terraform code",
			files: map[string]string{"unit/terragrunt.hcl": ""},
			check: func(t *testing.T, err error) {
				t.Helper()

				var noFiles run.NoTerraformFilesFound

				require.ErrorAs(t, err, &noFiles)
			},
		},
		{
			name:  "unit without OpenTofu/Terraform code in the download dir",
			files: map[string]string{"unit/terragrunt.hcl": ""},
			args:  []string{"--in-download-dir"},
			check: func(t *testing.T, err error) {
				t.Helper()

				var noFiles run.NoTerraformFilesFound

				require.ErrorAs(t, err, &noFiles)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			v := rec.venv(t, tc.files)

			args := append(slices.Clone(tc.args), "--", targetCommand)

			_, err := runCommand(t, exec.NewCommand(logger.CreateLogger(), newOpts(t, v), v, new(rewrite.Store)), args...)
			tc.check(t, err)
			assert.Nil(t, rec.find(targetCommand), "the target command never ran")
		})
	}
}

// TestRunReportsATargetCommandFailure pins that a failing target command surfaces as a process error.
func TestRunReportsATargetCommandFailure(t *testing.T) {
	t.Parallel()

	rec := &recorder{exitCode: 3}
	v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": ""})

	err := exec.Run(
		config.WithCaches(t.Context()),
		logger.CreateLogger(),
		v,
		new(rewrite.Store),
		newOpts(t, v),
		exec.NewOptions(),
		clihelper.Args{targetCommand},
	)

	var procErr *util.ProcessExecutionError

	require.ErrorAs(t, err, &procErr)
	assert.Equal(t, 3, vexec.ExitCode(err), "the target command's exit code reaches the caller")
}

// recorder answers OpenTofu version probes and records every other command a venv runs.
type recorder struct {
	recorded []vexec.Invocation
	exitCode int
	mu       sync.Mutex
}

// venv returns an in-memory venv holding files under memRoot whose commands rec answers.
func (rec *recorder) venv(t *testing.T, files map[string]string) *venv.Venv {
	t.Helper()

	return venv.MemVenv().
		WithFS(venvtest.NewFS(t, memRoot, files)).
		WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
			if slices.Contains(inv.Args, "-version") || slices.Contains(inv.Args, "version") {
				return vexec.Result{Stdout: []byte("OpenTofu v1.9.0\n")}
			}

			rec.mu.Lock()
			defer rec.mu.Unlock()

			rec.recorded = append(rec.recorded, inv)

			if inv.Name == targetCommand {
				return vexec.Result{ExitCode: rec.exitCode}
			}

			return vexec.Result{}
		})
}

// invocations returns every recorded command.
func (rec *recorder) invocations() []vexec.Invocation {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	return slices.Clone(rec.recorded)
}

// find returns the recorded invocation of name, or nil when it never ran.
func (rec *recorder) find(name string) *vexec.Invocation {
	for _, inv := range rec.invocations() {
		if inv.Name == name {
			return &inv
		}
	}

	return nil
}

// newOpts returns options for the unit under memRoot.
func newOpts(t *testing.T, v *venv.Venv) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsWithConfigPath(v.Exec, filepath.Join(memRoot, "unit", "terragrunt.hcl"))
	require.NoError(t, err)

	opts.NonInteractive = true

	return opts
}

// runCommand runs cmd with args the way the app does and returns what it printed.
func runCommand(t *testing.T, cmd *clihelper.Command, args ...string) (string, error) {
	t.Helper()

	out := new(bytes.Buffer)

	app := clihelper.NewApp(map[string]string{})
	app.Writer = out
	app.ExitErrHandler = func(_ *clihelper.Context, err error) error { return err }

	err := cmd.Run(config.WithCaches(t.Context()), clihelper.NewAppContext(app, args), args)

	return out.String(), err
}
