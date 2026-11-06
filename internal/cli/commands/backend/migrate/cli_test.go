package migrate_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend/migrate"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
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

// localBackendHCL configures a backend that reports no versioning and migrates without a cloud account.
const localBackendHCL = `
remote_state {
  backend = "local"
  config = {
    path = "terraform.tfstate"
  }
}
`

// envBackendHCL reads its state path from an environment variable the tests never set.
const envBackendHCL = `
remote_state {
  backend = "local"
  config = {
    path = get_env("TG_TEST_MIGRATE_STATE_PATH")
  }
}
`

var memRoot = venvtest.Root("/migrate")

// TestNewCommandExposesTheMigrateFlags pins the flags users pass to `backend migrate`.
func TestNewCommandExposesTheMigrateFlags(t *testing.T) {
	t.Parallel()

	v := venvtest.New()
	cmd := migrate.NewCommand(logger.CreateLogger(), newOpts(t, v), v)

	assert.Equal(t, migrate.CommandName, cmd.Name)

	for _, name := range []string{
		migrate.ForceBackendMigrateFlagName,
		shared.ConfigFlagName,
		shared.DownloadDirFlagName,
	} {
		assert.NotNil(t, cmd.Flags.Get(name), name)
	}
}

// TestNewCommandMigratesBetweenUnits pins when a migration between two units is refused and when it proceeds.
func TestNewCommandMigratesBetweenUnits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		files     map[string]string
		args      []string
		wantErr   bool
		wantDiags bool
	}{
		{
			name:  "force migrates an unversioned backend",
			files: map[string]string{"src/terragrunt.hcl": localBackendHCL, "dst/terragrunt.hcl": localBackendHCL},
			args:  []string{"--force", "src", "dst"},
		},
		{
			name:    "unversioned source is refused without force",
			files:   map[string]string{"src/terragrunt.hcl": localBackendHCL, "dst/terragrunt.hcl": localBackendHCL},
			args:    []string{"src", "dst"},
			wantErr: true,
		},
		{
			name:    "source without remote state is refused",
			files:   map[string]string{"src/terragrunt.hcl": "", "dst/terragrunt.hcl": localBackendHCL},
			args:    []string{"--force", "src", "dst"},
			wantErr: true,
		},
		{
			name:    "destination without remote state is refused",
			files:   map[string]string{"src/terragrunt.hcl": localBackendHCL, "dst/terragrunt.hcl": ""},
			args:    []string{"--force", "src", "dst"},
			wantErr: true,
		},
		{
			name:      "source remote state that fails to parse is refused",
			files:     map[string]string{"src/terragrunt.hcl": envBackendHCL, "dst/terragrunt.hcl": localBackendHCL},
			args:      []string{"--force", "src", "dst"},
			wantErr:   true,
			wantDiags: true,
		},
		{
			name:      "destination remote state that fails to parse is refused",
			files:     map[string]string{"src/terragrunt.hcl": localBackendHCL, "dst/terragrunt.hcl": envBackendHCL},
			args:      []string{"--force", "src", "dst"},
			wantErr:   true,
			wantDiags: true,
		},
		{
			name:    "unknown source unit is refused",
			files:   map[string]string{"dst/terragrunt.hcl": localBackendHCL},
			args:    []string{"--force", "src", "dst"},
			wantErr: true,
		},
		{
			name:    "unknown destination unit is refused",
			files:   map[string]string{"src/terragrunt.hcl": localBackendHCL},
			args:    []string{"--force", "src", "dst"},
			wantErr: true,
		},
		{
			name:    "missing destination argument is refused",
			files:   map[string]string{"src/terragrunt.hcl": localBackendHCL},
			args:    []string{"src"},
			wantErr: true,
		},
		{
			name:    "missing source argument is refused",
			files:   map[string]string{"src/terragrunt.hcl": localBackendHCL},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := newVenv().WithFS(venvtest.NewFS(t, memRoot, tc.files))

			err := runCommand(t, migrate.NewCommand(logger.CreateLogger(), newOpts(t, v), v), tc.args...)
			if tc.wantErr {
				require.Error(t, err)

				var diags hcl.Diagnostics

				assert.Equal(t, tc.wantDiags, errors.As(err, &diags), "only the unset variable fails to parse")

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestRunFailsOnAMissingWorkingDir pins that the discovery failure behind the stack reaches the caller.
func TestRunFailsOnAMissingWorkingDir(t *testing.T) {
	t.Parallel()

	v := newVenv()
	opts := newOpts(t, v)
	opts.WorkingDir = filepath.Join(memRoot, "missing")

	err := migrate.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, "src", "dst", opts)

	require.ErrorIs(t, err, fs.ErrNotExist)
}

// newVenv returns an in-memory venv whose binary reports an OpenTofu version, so the stack builds.
func newVenv() *venv.Venv {
	return venvtest.New().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("OpenTofu v1.9.0\n")}
	})
}

// newOpts returns options whose working directory is the in-memory fixture root.
func newOpts(t *testing.T, v *venv.Venv) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsWithConfigPath(v.Exec, filepath.Join(memRoot, "terragrunt.hcl"))
	require.NoError(t, err)

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
