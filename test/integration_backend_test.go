package test_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/require"
)

const testFixtureBackendUnappliedDependency = "fixtures/backend-unapplied-dependency"

func TestBackendBootstrapWithUnappliedDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureBackendUnappliedDependency)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureBackendUnappliedDependency)
	unitPath := filepath.Join(tmpEnvPath, testFixtureBackendUnappliedDependency, "foo")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt backend bootstrap --non-interactive --working-dir "+unitPath,
	)
	require.NoError(t, err)
}

func TestBackendAllReadsFilterFiles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		command  string
		file     string
		contents string
	}{
		{
			name:     "bootstrap with a filters file",
			command:  "backend bootstrap --all",
			file:     ".terragrunt-filters",
			contents: "a\n",
		},
		{
			name:     "bootstrap with an excludes file",
			command:  "backend bootstrap --all",
			file:     ".terragrunt-excludes",
			contents: "b\n",
		},
		{
			name:     "delete with a filters file",
			command:  "backend delete --all --force",
			file:     ".terragrunt-filters",
			contents: "a\n",
		},
		{
			name:     "delete with an excludes file",
			command:  "backend delete --all --force",
			file:     ".terragrunt-excludes",
			contents: "b\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

			files := map[string]string{
				filepath.Join("a", "terragrunt.hcl"): `remote_state {
  backend = "local"
  config = {
    path = "terraform.tfstate"
  }
}
`,
				filepath.Join("b", "terragrunt.hcl"): "this is ) not ( valid hcl",
				tc.file:                              tc.contents,
			}

			for name, contents := range files {
				path := filepath.Join(tmpDir, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
			}

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt "+tc.command+" --non-interactive --working-dir "+tmpDir,
			)
			require.NoError(t, err, "b fails to parse if the command reaches it\nstdout: %s\nstderr: %s", stdout, stderr)
		})
	}
}
