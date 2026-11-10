package clean_test

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/stacks/clean"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const workDir = "/work"

func TestCleanStacks(t *testing.T) {
	t.Parallel()

	stackFiles := []string{
		"/work/a/.terragrunt-stack/vpc/terragrunt.hcl",
		"/work/b/c/.terragrunt-stack/db/terragrunt.hcl",
	}
	keptFile := "/work/a/terragrunt.stack.hcl"

	testCases := []struct {
		name        string
		command     string
		args        []string
		wantRemoved bool
	}{
		{
			name:        "plan removes every generated stack directory",
			command:     "plan",
			wantRemoved: true,
		},
		{
			name:    "destroy keeps generated stack directories",
			command: "destroy",
		},
		{
			name:    "apply -destroy keeps generated stack directories",
			command: "apply",
			args:    []string{"-destroy"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			for _, path := range append([]string{keptFile}, stackFiles...) {
				require.NoError(t, vfs.WriteFile(fsys, path, []byte(""), 0o644))
			}

			opts := newOptions(t, tc.command, tc.args...)

			require.NoError(t, clean.CleanStacks(logger.CreateLogger(), fsys, opts))

			for _, path := range stackFiles {
				exists, err := vfs.FileExists(fsys, path)
				require.NoError(t, err)
				assert.Equal(t, !tc.wantRemoved, exists, "unexpected state for %s", path)
			}

			exists, err := vfs.FileExists(fsys, keptFile)
			require.NoError(t, err)
			assert.True(t, exists, "files outside generated stack directories should be kept")
		})
	}
}

func TestCleanStacks_MissingWorkingDirFails(t *testing.T) {
	t.Parallel()

	err := clean.CleanStacks(logger.CreateLogger(), vfs.NewMemMapFS(), newOptions(t, "plan"))

	require.ErrorIs(t, err, fs.ErrNotExist, "a missing working directory should be reported")
}

func TestCleanStacks_RemoveFailureIsReturned(t *testing.T) {
	t.Parallel()

	base := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(base, "/work/a/.terragrunt-stack/vpc/terragrunt.hcl", []byte(""), 0o644))

	err := clean.CleanStacks(logger.CreateLogger(), removeRefusingFS{FS: base}, newOptions(t, "plan"))

	require.ErrorIs(t, err, errRemove, "a failed delete should be reported")
}

// errRemove is the error removeRefusingFS reports.
var errRemove = errors.New("remove failed")

// removeRefusingFS is a filesystem whose RemoveAll always fails.
type removeRefusingFS struct {
	vfs.FS
}

func (removeRefusingFS) RemoveAll(string) error {
	return errRemove
}

func newOptions(t *testing.T, command string, args ...string) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.WorkingDir = workDir
	opts.TofuCommand = command
	opts.TerraformCliArgs = iacargs.New(args...)

	return opts
}
