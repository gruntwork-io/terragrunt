package validate_test

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunValidateCheckDependencies(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")

	const dependsOnDeleted = "dependency \"deleted\" {\n  config_path = \"../deleted\"\n}\n"

	tcs := []struct {
		files       map[string]string
		name        string
		wantMissing []string
		check       bool
	}{
		{
			name:  "missing dependency passes without the flag",
			files: map[string]string{"app/terragrunt.hcl": dependsOnDeleted},
		},
		{
			name:        "missing dependency fails with the flag",
			files:       map[string]string{"app/terragrunt.hcl": dependsOnDeleted},
			check:       true,
			wantMissing: []string{missingEntry(root, "app", "deleted")},
		},
		{
			name: "every missing dependency is reported",
			files: map[string]string{
				"app/terragrunt.hcl": dependsOnDeleted,
				"web/terragrunt.hcl": "dependency \"gone\" {\n  config_path = \"../gone\"\n}\n" +
					"dependency \"deleted\" {\n  config_path = \"../deleted\"\n}\n",
				"db/terragrunt.hcl": "",
			},
			check: true,
			wantMissing: []string{
				missingEntry(root, "app", "deleted"),
				missingEntry(root, "web", "gone"),
				missingEntry(root, "web", "deleted"),
			},
		},
		{
			name: "existing dependency passes with the flag",
			files: map[string]string{
				"app/terragrunt.hcl": "dependency \"db\" {\n  config_path = \"../db\"\n}\n",
				"db/terragrunt.hcl":  "",
			},
			check: true,
		},
		{
			name: "disabled dependency passes with the flag",
			files: map[string]string{
				"app/terragrunt.hcl": "dependency \"deleted\" {\n  config_path = \"../deleted\"\n" +
					"  enabled = false\n}\n",
			},
			check: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New().WithFS(venvtest.NewFS(t, root, tc.files))

			opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, config.DefaultTerragruntConfigPath))
			require.NoError(t, err)

			opts.HCLValidateCheckDependencies = tc.check

			err = validate.RunValidate(t.Context(), logger.CreateLogger(), v, opts)
			if len(tc.wantMissing) == 0 {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.ElementsMatch(t, tc.wantMissing, missingDependencies(err))
		})
	}
}

// TestRunValidateCheckDependenciesWithParseError pins that a parse error alongside a config does not hide missing dependencies.
func TestRunValidateCheckDependenciesWithParseError(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	v := venvtest.New().WithFS(venvtest.NewFS(t, root, map[string]string{
		"app/terragrunt.hcl": "dependency \"foo\" {\n  config_path = \"../foo\"\n}\n" +
			"dependencies {\n  paths = [\"../bar\"]\n}\n",
	}))

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, config.DefaultTerragruntConfigPath))
	require.NoError(t, err)

	opts.HCLValidateCheckDependencies = true

	err = validate.RunValidate(t.Context(), logger.CreateLogger(), v, opts)
	require.Error(t, err)

	_, ok := errors.AsType[config.DependencyDirNotFoundError](err)
	assert.True(t, ok, "the parser error for the missing dependencies path must be kept: %v", err)
	assert.Equal(t, []string{missingEntry(root, "app", "foo")}, missingDependencies(err))
}

// TestRunValidateCheckDependenciesStatFailure pins that a dependency that cannot be checked is reported with its cause.
func TestRunValidateCheckDependenciesStatFailure(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	fsys := &statFailFS{
		FS: venvtest.NewFS(t, root, map[string]string{
			"app/terragrunt.hcl": "dependency \"db\" {\n  config_path = \"../db\"\n}\n",
			"db/terragrunt.hcl":  "",
		}),
		failPath: filepath.Join(root, "db", config.DefaultTerragruntConfigPath),
	}

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, "app", config.DefaultTerragruntConfigPath))
	require.NoError(t, err)

	opts.HCLValidateCheckDependencies = true

	err = validate.RunValidate(t.Context(), logger.CreateLogger(), venvtest.New().WithFS(fsys), opts)
	require.ErrorIs(t, err, fs.ErrPermission)
	assert.Empty(t, missingDependencies(err))

	checkErr, ok := errors.AsType[config.DependencyConfigCheckError](err)
	require.True(t, ok, "unexpected error %v", err)
	assert.Equal(t, filepath.Join(root, "db"), checkErr.DependencyPath)
}

func TestRunValidateInputsCheckDependencies(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")

	tcs := []struct {
		name        string
		wantMissing []string
		check       bool
	}{
		{
			name: "without the flag",
		},
		{
			name:        "with the flag",
			check:       true,
			wantMissing: []string{missingEntry(root, "app", "deleted")},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New().WithFS(venvtest.NewFS(t, root, map[string]string{
				"app/terragrunt.hcl": "dependency \"deleted\" {\n  config_path = \"../deleted\"\n}\n" +
					"inputs = {\n  region = \"eu-west-1\"\n}\n",
				"app/main.tf": "variable \"region\" {}\n",
			}))

			opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, config.DefaultTerragruntConfigPath))
			require.NoError(t, err)

			opts.HCLValidateCheckDependencies = tc.check

			err = validate.RunValidateInputs(t.Context(), logger.CreateLogger(), v, opts)
			if len(tc.wantMissing) == 0 {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Equal(t, tc.wantMissing, missingDependencies(err))
		})
	}
}

func TestRunCheckDependenciesFlagCombinations(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name    string
		wantErr string
		json    bool
		show    bool
	}{
		{
			name:    "json output",
			json:    true,
			wantErr: "specifying both -json and -check-dependencies is invalid",
		},
		{
			name:    "show config path",
			show:    true,
			wantErr: "specifying both -show-config-path and -check-dependencies is invalid",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/repo")
			v := venvtest.New().WithFS(venvtest.NewFS(t, root, map[string]string{"app/terragrunt.hcl": ""}))

			opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, config.DefaultTerragruntConfigPath))
			require.NoError(t, err)

			opts.HCLValidateCheckDependencies = true
			opts.HCLValidateJSONOutput = tc.json
			opts.HCLValidateShowConfigPath = tc.show

			err = validate.Run(t.Context(), logger.CreateLogger(), v, opts)
			require.EqualError(t, err, tc.wantErr)
		})
	}
}

// TestRunCheckDependenciesDisablesTip pins that the flag silences the tip that points users at this very check.
func TestRunCheckDependenciesDisablesTip(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name      string
		check     bool
		wantShown bool
	}{
		{
			name:      "without the flag",
			wantShown: true,
		},
		{
			name:  "with the flag",
			check: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/repo")
			v := venvtest.New().WithFS(venvtest.NewFS(t, root, map[string]string{"app/terragrunt.hcl": ""}))

			opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, config.DefaultTerragruntConfigPath))
			require.NoError(t, err)

			opts.Tips = tips.NewTips()
			opts.HCLValidateCheckDependencies = tc.check

			require.NoError(t, validate.Run(t.Context(), logger.CreateLogger(), v, opts))

			l, output := newTipLogger()
			tips.GiveMissingDependencyConfigTip(l, config.MissingDependencyConfigError{}, opts.Tips)

			if tc.wantShown {
				assert.Contains(t, output.String(), tips.MissingDependencyConfig)

				return
			}

			assert.Empty(t, output.String())
		})
	}
}

// missingEntry renders a unit and its missing dependency the way missingDependencies reports them.
func missingEntry(root, unit, dep string) string {
	return filepath.Join(root, unit) + " -> " + filepath.Join(root, dep)
}

// missingDependencies lists every missing dependency reported in err, keeping duplicates.
func missingDependencies(err error) []string {
	var missing []string

	var walk func(error)

	walk = func(err error) {
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			for _, inner := range joined.Unwrap() {
				walk(inner)
			}

			return
		}

		if depErr, ok := errors.AsType[config.MissingDependencyConfigError](err); ok {
			missing = append(missing, depErr.UnitPath+" -> "+depErr.DependencyPath)
		}
	}

	walk(err)

	return missing
}

// newTipLogger returns a logger that writes bare messages to the returned buffer.
func newTipLogger() (log.Logger, *bytes.Buffer) {
	output := new(bytes.Buffer)

	return log.New(
		log.WithOutput(output),
		log.WithLevel(log.InfoLevel),
		log.WithFormatter(format.NewFormatter(placeholders.Placeholders{placeholders.Message()})),
	), output
}

// statFailFS fails Stat for failPath with a permission error.
type statFailFS struct {
	vfs.FS
	failPath string
}

func (f *statFailFS) Stat(name string) (fs.FileInfo, error) {
	if name == f.failPath {
		return nil, fs.ErrPermission
	}

	return f.FS.Stat(name)
}
