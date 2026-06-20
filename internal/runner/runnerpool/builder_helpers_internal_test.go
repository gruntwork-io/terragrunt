package runnerpool

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnitFiltersForStackDirs verifies matched stack dirs become path filters selecting every unit beneath them.
func TestUnitFiltersForStackDirs(t *testing.T) {
	t.Parallel()

	workingDir := filepath.FromSlash("/work")

	testCases := []struct {
		name      string
		stackDirs []string
		want      []string
	}{
		{
			name:      "stack dir equals working dir",
			stackDirs: []string{workingDir},
			want:      []string{"**"},
		},
		{
			name:      "nested stack dir under working dir",
			stackDirs: []string{filepath.Join(workingDir, "stacks", "first")},
			want:      []string{"stacks/first/**"},
		},
		{
			name: "multiple stack dirs",
			stackDirs: []string{
				filepath.Join(workingDir, "stacks", "first"),
				filepath.Join(workingDir, "stacks", "second"),
			},
			want: []string{"stacks/first/**", "stacks/second/**"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			filters, err := unitFiltersForStackDirs(workingDir, tc.stackDirs)
			require.NoError(t, err)

			got := make([]string, 0, len(filters))
			for _, f := range filters {
				got = append(got, f.String())
			}

			assert.Equal(t, tc.want, got)
		})
	}
}

// TestNegatedFilters verifies only the user's negated filters are carried into the stack-to-unit expansion.
func TestNegatedFilters(t *testing.T) {
	t.Parallel()

	parse := func(query string) *filter.Filter {
		f, err := filter.Parse(query)
		require.NoError(t, err)

		return f
	}

	filters := filter.Filters{
		parse("./x | type=stack"),
		parse("!./x/.terragrunt-stack/sub/module_2"),
		parse("!type=stack"),
	}

	got := negatedFilters(filters)

	gotStrings := make([]string, 0, len(got))
	for _, f := range got {
		gotStrings = append(gotStrings, f.String())
	}

	assert.Equal(t, []string{"!./x/.terragrunt-stack/sub/module_2", "!type=stack"}, gotStrings)
}
