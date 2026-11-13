package shared_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQueueFlagsTranslateToFilters pins the filter queries each legacy queue flag expands into.
func TestQueueFlagsTranslateToFilters(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		env         map[string]string
		name        string
		args        clihelper.Args
		wantFilters []string
	}{
		{
			name:        "exclude dir negates each path",
			args:        clihelper.Args{"--queue-exclude-dir", "a", "--queue-exclude-dir", "b/**"},
			wantFilters: []string{"!a", "!b/**"},
		},
		{
			name:        "exclude dir by deprecated env var",
			env:         map[string]string{"TERRAGRUNT_EXCLUDE_DIR": "a"},
			wantFilters: []string{"!a"},
		},
		{
			name:        "include dir keeps each path",
			args:        clihelper.Args{"--queue-include-dir", "a", "--queue-include-dir", "b"},
			wantFilters: []string{"a", "b"},
		},
		{
			name:        "include external adds all paths with dependencies",
			args:        clihelper.Args{"--queue-include-external"},
			wantFilters: []string{"./**..."},
		},
		{
			name:        "include units reading adds reading attribute",
			args:        clihelper.Args{"--queue-include-units-reading", "common.hcl"},
			wantFilters: []string{"reading=common.hcl"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())

			env := tc.env
			if env == nil {
				env = map[string]string{}
			}

			require.NoError(t, parseAndRunActions(t, queueFlags(opts), tc.args, env))
			assert.Equal(t, tc.wantFilters, filterQueries(opts), "queue flag should expand into these filter queries")
		})
	}
}

// TestQueueFlagsWithoutFilters pins that flags set to their off value add no filters and still write their option field.
func TestQueueFlagsWithoutFilters(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		want any
		got  func(opts *options.TerragruntOptions) any
		env  map[string]string
		name string
		args clihelper.Args
	}{
		{
			name: "ignore errors by name",
			args: clihelper.Args{"--queue-ignore-errors"},
			got:  func(opts *options.TerragruntOptions) any { return opts.IgnoreDependencyErrors },
			want: true,
		},
		{
			name: "ignore dag order by deprecated env var",
			env:  map[string]string{"TERRAGRUNT_IGNORE_DEPENDENCY_ORDER": "true"},
			got:  func(opts *options.TerragruntOptions) any { return opts.IgnoreDependencyOrder },
			want: true,
		},
		{
			name: "excludes file by name",
			args: clihelper.Args{"--queue-excludes-file", "excludes.txt"},
			got:  func(opts *options.TerragruntOptions) any { return opts.ExcludesFile },
			want: "excludes.txt",
		},
		{
			name: "include external set false",
			args: clihelper.Args{"--queue-include-external=false"},
			got:  func(opts *options.TerragruntOptions) any { return len(opts.Filters) },
			want: 0,
		},
		{
			name: "exclude external set false",
			args: clihelper.Args{"--queue-exclude-external=false"},
			got:  func(opts *options.TerragruntOptions) any { return len(opts.Filters) },
			want: 0,
		},
		{
			name: "strict include set false",
			args: clihelper.Args{"--queue-strict-include=false"},
			got:  func(opts *options.TerragruntOptions) any { return len(opts.Filters) },
			want: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())

			env := tc.env
			if env == nil {
				env = map[string]string{}
			}

			require.NoError(t, parseAndRunActions(t, queueFlags(opts), tc.args, env))
			assert.Equal(t, tc.want, tc.got(opts), "queue flag should write the parsed value")
		})
	}
}

// TestQueueFlagsRejectInvalidGlobs pins that a malformed glob in a path-taking queue flag fails the run.
func TestQueueFlagsRejectInvalidGlobs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args clihelper.Args
	}{
		{name: "exclude dir", args: clihelper.Args{"--queue-exclude-dir", "[a"}},
		{name: "include dir", args: clihelper.Args{"--queue-include-dir", "[a"}},
		{name: "include units reading", args: clihelper.Args{"--queue-include-units-reading", "[a"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())

			require.Error(t, parseAndRunActions(t, queueFlags(opts), tc.args, map[string]string{}))
			assert.Empty(t, opts.Filters, "a rejected glob must not add a filter")
		})
	}
}

// TestQueueDeprecatedFlagsEvaluateStrictControls pins that the removed queue flags warn by default and fail once their strict control is enabled.
func TestQueueDeprecatedFlagsEvaluateStrictControls(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		controlName string
		args        clihelper.Args
	}{
		{
			name:        "exclude external",
			controlName: controls.QueueExcludeExternal,
			args:        clihelper.Args{"--queue-exclude-external"},
		},
		{name: "strict include", controlName: controls.QueueStrictInclude, args: clihelper.Args{"--queue-strict-include"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			selectionFlags := shared.NewQueueSelectionFlags(opts, nil)

			require.NoError(t, parseAndRunActions(t, selectionFlags, tc.args, map[string]string{}),
				"a deprecated flag should only warn while its control is off")

			require.NoError(t, opts.StrictControls.EnableControl(tc.controlName))

			ctrl, ok := opts.StrictControls.Find(tc.controlName).(*controls.Control)
			require.True(t, ok, "control %s should be a *controls.Control", tc.controlName)

			err := selectionFlags.RunActions(t.Context(), &clihelper.Context{})
			require.ErrorIs(t, err, ctrl.Error, "an enabled control should reject the deprecated flag")
		})
	}
}

// filterQueries returns the original query of each filter on opts, in order.
func filterQueries(opts *options.TerragruntOptions) []string {
	queries := make([]string, 0, len(opts.Filters))

	for _, f := range opts.Filters {
		queries = append(queries, f.String())
	}

	return queries
}

// queueFlags returns every queue flag: the ones that control how the queue runs and the ones that select its units.
func queueFlags(opts *options.TerragruntOptions) clihelper.Flags {
	return append(shared.NewQueueRunFlags(opts, nil), shared.NewQueueSelectionFlags(opts, nil)...)
}
