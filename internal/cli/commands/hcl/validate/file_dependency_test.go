package validate_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fileDependency returns the configuration of a unit whose dependency "db" has the given config_path.
func fileDependency(configPath string) string {
	return "dependency \"db\" {\n  config_path = \"" + configPath + "\"\n}\n"
}

// TestRunValidateAcceptsFileDependency pins that a `dependency` whose `config_path` names an existing
// configuration file validates, with and without a dependency graph filter.
//
// This is deprecated functionality: a file `config_path` is undocumented and deprecated behind the
// dependency-config-path-file strict control. These cases keep it working for backwards compatibility
// through Terragrunt 1.x.
func TestRunValidateAcceptsFileDependency(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		files   map[string]string
		name    string
		queries []string
	}{
		{
			name: "configuration with another name",
			files: map[string]string{
				"app/terragrunt.hcl": fileDependency("../db/custom.hcl"),
				"db/custom.hcl":      "",
			},
		},
		{
			name: "configuration with the default name",
			files: map[string]string{
				"app/terragrunt.hcl": fileDependency("../db/terragrunt.hcl"),
				"db/terragrunt.hcl":  "",
			},
		},
		{
			name: "configuration with another name under a graph filter",
			files: map[string]string{
				"app/terragrunt.hcl": fileDependency("../db/custom.hcl"),
				"db/custom.hcl":      "",
			},
			queries: []string{"{./**}..."},
		},
		{
			name: "configuration with the default name under a graph filter",
			files: map[string]string{
				"app/terragrunt.hcl": fileDependency("../db/terragrunt.hcl"),
				"db/terragrunt.hcl":  "",
			},
			queries: []string{"{./**}..."},
		},
		{
			name: "dependent left out of the validated set",
			files: map[string]string{
				"app/terragrunt.hcl": fileDependency("../db/custom.hcl"),
				"db/custom.hcl":      "",
			},
			queries: []string{"^{./app}..."},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, validateRun{files: tc.files, queries: tc.queries})

			require.NoError(t, result.err)
			assert.Empty(t, result.diags)
		})
	}
}

// TestRunValidateReportsMissingFileDependency pins that a `config_path` naming a configuration file that
// is not there is reported at the expression, as a missing file.
//
// This is deprecated functionality: a file `config_path` is deprecated behind the
// dependency-config-path-file strict control. Its missing target is still reported while the form stays
// working for backwards compatibility.
func TestRunValidateReportsMissingFileDependency(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name    string
		queries []string
	}{
		{name: "no filter"},
		{name: "graph filter", queries: []string{"{./**}..."}},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, validateRun{
				files: map[string]string{
					"app/terragrunt.hcl": fileDependency("../db/custom.hcl"),
					"db/terragrunt.hcl":  "",
				},
				queries: tc.queries,
			})

			requireMissingDependencyAt(
				t, result, "app/terragrunt.hcl", 2, `"../db/custom.hcl"`,
				"This file does not exist.",
			)
		})
	}
}

// TestRunValidateRejectsFileDependencyWhenStrict pins that the dependency-config-path-file strict
// control fails validation of a `config_path` that names a file, and leaves a directory alone.
func TestRunValidateRejectsFileDependencyWhenStrict(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name       string
		configPath string
		wantErr    bool
	}{
		{name: "configuration with another name", configPath: "../db/custom.hcl", wantErr: true},
		{
			name:       "configuration with the default name",
			configPath: "../db/terragrunt.hcl",
			wantErr:    true,
		},
		{name: "unit directory", configPath: "../db"},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, validateRun{
				files: map[string]string{
					"app/terragrunt.hcl": fileDependency(tc.configPath),
					"db/terragrunt.hcl":  "",
					"db/custom.hcl":      "",
				},
				strictControls: []string{controls.DependencyConfigPathFile},
			})

			if !tc.wantErr {
				require.NoError(t, result.err)

				return
			}

			var typed config.DependencyConfigPathFileError
			require.ErrorAs(t, result.err, &typed)
			assert.Equal(t, "db", typed.Address)
			assert.Equal(t, tc.configPath, typed.DependencyPath)
		})
	}
}

// TestRunValidateFileDependencyDeclaredInInclude pins that a file `config_path` declared in an included
// file is resolved against the unit that includes it, both when it is accepted and when the
// dependency-config-path-file strict control rejects it.
//
// This is deprecated functionality: a file `config_path` is deprecated behind that strict control. The
// accepted case keeps it working for backwards compatibility through Terragrunt 1.x.
func TestRunValidateFileDependencyDeclaredInInclude(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"common.hcl":         fileDependency("../db/custom.hcl"),
		"app/terragrunt.hcl": "include \"common\" {\n  path = \"../common.hcl\"\n}\n",
		"db/custom.hcl":      "",
	}

	t.Run("accepted by default", func(t *testing.T) {
		t.Parallel()

		result := runValidate(t, validateRun{files: files})

		require.NoError(t, result.err)
		assert.Empty(t, result.diags)
	})

	t.Run("rejected when strict", func(t *testing.T) {
		t.Parallel()

		result := runValidate(t, validateRun{
			files:          files,
			strictControls: []string{controls.DependencyConfigPathFile},
		})

		var typed config.DependencyConfigPathFileError
		require.ErrorAs(t, result.err, &typed)
		assert.Equal(t, "db", typed.Address)
		assert.Equal(t, "../db/custom.hcl", typed.DependencyPath)
	})
}
