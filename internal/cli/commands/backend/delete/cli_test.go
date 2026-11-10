package delete_test

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend/delete"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// localBackendHCL configures a backend that reports no versioning and deletes without a cloud account.
const localBackendHCL = `
remote_state {
  backend = "local"
  config = {
    path = "terraform.tfstate"
  }
}
`

// invalidHCL fails config parsing before any backend is contacted.
const invalidHCL = "this is ) not ( valid hcl"

var memRoot = venvtest.Root("/delete")

// TestNewCommandExposesTheDeleteFlags pins the flags users pass to `backend delete`.
func TestNewCommandExposesTheDeleteFlags(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv()
	cmd := delete.NewCommand(logger.CreateLogger(), newOpts(t, v, ""), v, new(rewrite.Store))

	assert.Equal(t, delete.CommandName, cmd.Name)

	for _, name := range []string{
		delete.BucketFlagName,
		delete.ForceBackendDeleteFlagName,
		shared.ConfigFlagName,
		shared.DownloadDirFlagName,
		shared.AllFlagName,
		shared.FailFastFlagName,
	} {
		assert.NotNil(t, cmd.Flags.Get(name), name)
	}
}

// TestNewCommandDeletesTheUnitState pins when deleting one unit's state is refused and when it proceeds.
func TestNewCommandDeletesTheUnitState(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		hcl       string
		args      []string
		wantErr   bool
		wantDiags bool
	}{
		{name: "config without remote state is a no-op"},
		{name: "unversioned backend is refused without force", hcl: localBackendHCL, wantErr: true},
		{name: "force deletes an unversioned backend", hcl: localBackendHCL, args: []string{"--force"}},
		{
			name:    "deleting the bucket is not supported",
			hcl:     localBackendHCL,
			args:    []string{"--force", "--bucket"},
			wantErr: true,
		},
		{name: "invalid config fails", hcl: invalidHCL, wantErr: true, wantDiags: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv().WithFS(venvtest.NewFS(t, memRoot, map[string]string{
				"unit/terragrunt.hcl": tc.hcl,
			}))

			opts := newOpts(t, v, filepath.Join(memRoot, "unit", "terragrunt.hcl"))

			err := runCommand(t, delete.NewCommand(logger.CreateLogger(), opts, v, new(rewrite.Store)), tc.args...)
			if !tc.wantErr {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)

			var diags hcl.Diagnostics

			assert.Equal(t, tc.wantDiags, errors.As(err, &diags), "only an invalid config fails to parse")
		})
	}
}

// TestNewCommandAllDeletesEveryUnit pins how --all reports units that fail with and without --fail-fast.
func TestNewCommandAllDeletesEveryUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		files      map[string]string
		args       []string
		wantJoined int
		wantErr    bool
	}{
		{
			name: "every unit is deleted",
			files: map[string]string{
				"a/terragrunt.hcl": localBackendHCL,
				"b/terragrunt.hcl": localBackendHCL,
			},
			args: []string{"--all", "--force"},
		},
		{
			name: "failures are joined",
			files: map[string]string{
				"a/terragrunt.hcl": localBackendHCL,
				"b/terragrunt.hcl": invalidHCL,
				"c/terragrunt.hcl": invalidHCL,
			},
			args:       []string{"--all", "--force"},
			wantErr:    true,
			wantJoined: 2,
		},
		{
			name: "fail fast stops at the first failure",
			files: map[string]string{
				"a/terragrunt.hcl": invalidHCL,
				"b/terragrunt.hcl": invalidHCL,
			},
			args:    []string{"--all", "--force", "--fail-fast"},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv().WithFS(venvtest.NewFS(t, memRoot, tc.files))

			err := runCommand(t, delete.NewCommand(logger.CreateLogger(), newOpts(t, v, ""), v, new(rewrite.Store)), tc.args...)
			if !tc.wantErr {
				require.NoError(t, err)

				return
			}

			var diags hcl.Diagnostics

			require.ErrorAs(t, err, &diags)

			var joined interface{ Unwrap() []error }

			if tc.wantJoined == 0 {
				assert.NotErrorAs(t, err, &joined, "fail fast returns the first error unjoined")

				return
			}

			require.ErrorAs(t, err, &joined)
			assert.Len(t, joined.Unwrap(), tc.wantJoined, "one error per failed unit")
		})
	}
}

// TestNewCommandAllFailsOnAMissingWorkingDir pins that --all surfaces the discovery failure.
func TestNewCommandAllFailsOnAMissingWorkingDir(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv()
	opts := newOpts(t, v, "")
	opts.WorkingDir = filepath.Join(memRoot, "missing")

	err := runCommand(t, delete.NewCommand(logger.CreateLogger(), opts, v, new(rewrite.Store)), "--all")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

// TestRunUsesTheConfiguredConfigFilename pins that --all reads each unit's config by the configured filename.
func TestRunUsesTheConfiguredConfigFilename(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv().WithFS(venvtest.NewFS(t, memRoot, map[string]string{
		"a/terragrunt.hcl": "",
		"a/custom.hcl":     invalidHCL,
	}))

	opts := newOpts(t, v, filepath.Join(memRoot, "custom.hcl"))
	opts.WorkingDir = memRoot
	opts.RunAll = true

	err := delete.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, new(rewrite.Store), opts)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags, "the unit's custom.hcl is the config that fails to parse")
}

// newOpts returns options rooted at the in-memory fixture, reading configPath when set.
func newOpts(t *testing.T, v *venv.Venv, configPath string) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsWithConfigPath(v.Exec, configPath)
	require.NoError(t, err)

	if configPath == "" {
		opts.WorkingDir = memRoot
	}

	opts.RootWorkingDir = memRoot
	opts.NonInteractive = true

	return opts
}

// runCommand runs cmd with args the way the app does and returns its error.
func runCommand(t *testing.T, cmd *clihelper.Command, args ...string) error {
	t.Helper()

	app := clihelper.NewApp(map[string]string{})
	app.Writer = new(bytes.Buffer)
	app.ExitErrHandler = func(_ *clihelper.Context, err error) error { return err }

	return cmd.Run(config.WithCaches(t.Context()), clihelper.NewAppContext(app, args), args)
}
