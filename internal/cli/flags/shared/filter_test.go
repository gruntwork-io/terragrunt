package shared_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryBoundaryFlagSetsBoundary(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	flags := shared.NewFilterFlags(logger.CreateLogger(), opts, venv.MemVenv())

	require.NoError(t, flags.Parse(clihelper.Args{"--discovery-boundary", "."}, map[string]string{}))
	require.NoError(t, flags.RunActions(t.Context(), &clihelper.Context{}))
	assert.Equal(t, ".", opts.DiscoveryBoundary)
}

// TestFilterFlagsSetOptions pins the option fields and filters the --filter family writes.
func TestFilterFlagsSetOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		want any
		got  func(opts *options.TerragruntOptions) any
		name string
		args clihelper.Args
	}{
		{
			name: "filter queries are parsed and turn on run all",
			args: clihelper.Args{"--filter", "./a", "--filter", "./b"},
			got: func(opts *options.TerragruntOptions) any {
				return []any{filterQueries(opts), opts.RunAll}
			},
			want: []any{[]string{"./a", "./b"}, true},
		},
		{
			name: "filter allow destroy",
			args: clihelper.Args{"--filter-allow-destroy"},
			got:  func(opts *options.TerragruntOptions) any { return opts.FilterAllowDestroy },
			want: true,
		},
		{
			name: "filters file",
			args: clihelper.Args{"--filters-file", "filters.txt"},
			got:  func(opts *options.TerragruntOptions) any { return opts.FiltersFile },
			want: "filters.txt",
		},
		{
			name: "no filters file",
			args: clihelper.Args{"--no-filters-file"},
			got:  func(opts *options.TerragruntOptions) any { return opts.NoFiltersFile },
			want: true,
		},
		{
			name: "filter affected set false adds nothing",
			args: clihelper.Args{"--filter-affected=false"},
			got:  func(opts *options.TerragruntOptions) any { return len(opts.Filters) },
			want: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()
			filterFlags := shared.NewFilterFlags(logger.CreateLogger(), opts, venv.MemVenv())

			require.NoError(t, parseAndRunActions(t, filterFlags, tc.args, map[string]string{}))
			assert.Equal(t, tc.want, tc.got(opts), "filter flag should write the parsed value")
		})
	}
}

// TestFilterFlagRejectsInvalidQuery pins that a malformed --filter query fails the run and adds nothing.
func TestFilterFlagRejectsInvalidQuery(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	filterFlags := shared.NewFilterFlags(logger.CreateLogger(), opts, venv.MemVenv())

	require.Error(t, parseAndRunActions(t, filterFlags, clihelper.Args{"--filter", "[main..."}, map[string]string{}))
	assert.Empty(t, opts.Filters, "a rejected query must not add a filter")
	assert.False(t, opts.RunAll, "a rejected query must not turn on run all")
}

// TestFilterAffectedFlag pins that --filter-affected compares the default branch with HEAD from the resolved working dir.
func TestFilterAffectedFlag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		workingDir     string
		rootWorkingDir string
		status         string
		wantDir        string
	}{
		{
			name:       "working dir with uncommitted changes",
			workingDir: "/repo/unit",
			status:     " M main.tf\n",
			wantDir:    "/repo/unit",
		},
		{name: "root working dir fallback", rootWorkingDir: "/repo", wantDir: "/repo"},
		{name: "current dir fallback", wantDir: "."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gitDirs []string

			v := venv.MemVenv().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
				gitDirs = append(gitDirs, inv.Dir)

				switch inv.Args[0] {
				case "status":
					return vexec.Result{Stdout: []byte(tc.status)}
				case "rev-parse":
					return vexec.Result{Stdout: []byte("origin/develop\n")}
				}

				return vexec.Result{ExitCode: 1}
			})

			opts := options.NewTerragruntOptions()
			opts.WorkingDir = tc.workingDir
			opts.RootWorkingDir = tc.rootWorkingDir

			filterFlags := shared.NewFilterFlags(logger.CreateLogger(), opts, v)

			require.NoError(t, parseAndRunActions(t, filterFlags, clihelper.Args{"--filter-affected"}, map[string]string{}))
			assert.Equal(t, []string{"[develop...HEAD]"}, filterQueries(opts), "filter should span default branch to HEAD")
			require.NotEmpty(t, gitDirs, "git should be queried")

			for _, dir := range gitDirs {
				assert.Equal(t, tc.wantDir, dir, "git should run in the resolved working dir")
			}
		})
	}
}

// TestFilterAffectedFlagWithoutGit pins that --filter-affected fails with a general exit error when git is not installed.
func TestFilterAffectedFlagWithoutGit(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv().WithExec(&vexec.NoLookPathExec{Exec: vexec.NewNoSpawnExec()})

	opts := options.NewTerragruntOptions()
	filterFlags := shared.NewFilterFlags(logger.CreateLogger(), opts, v)

	err := parseAndRunActions(t, filterFlags, clihelper.Args{"--filter-affected"}, map[string]string{})

	var exitErr clihelper.ExitCoder
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, int(clihelper.ExitCodeGeneralError), exitErr.ExitCode())
	require.ErrorIs(t, err, git.ErrGitCommand)
	assert.Empty(t, opts.Filters, "no filter should be added without git")
}
