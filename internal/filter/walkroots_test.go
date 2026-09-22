package filter_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/glob"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifierWalkRoots(t *testing.T) {
	t.Parallel()

	workingDir := venvtest.Root("/repo")
	absolute := filepath.ToSlash(workingDir)
	parent := filepath.ToSlash(filepath.Dir(workingDir))
	everything := []glob.Root{{Dir: ".", Recursive: true}}

	tests := []struct {
		name    string
		queries []string
		roots   []glob.Root
	}{
		{
			name:  "no filter",
			roots: everything,
		},
		{
			name:    "negation only",
			queries: []string{"!./a"},
			roots:   everything,
		},
		{
			name:    "literal path",
			queries: []string{"./path/to/unit"},
			roots:   []glob.Root{{Dir: "path/to/unit"}},
		},
		{
			name:    "the working directory itself",
			queries: []string{"."},
			roots:   []glob.Root{{Dir: "."}},
		},
		{
			name:    "single-segment wildcard",
			queries: []string{"./path/to/*"},
			roots:   []glob.Root{{Dir: "path/to/*"}},
		},
		{
			name:    "globstar below a literal prefix",
			queries: []string{"./path/to/**"},
			roots:   []glob.Root{{Dir: "path/to", Recursive: true}},
		},
		{
			name:    "wildcard before a globstar",
			queries: []string{"./a/*/b/**"},
			roots:   []glob.Root{{Dir: "a/*/b", Recursive: true}},
		},
		{
			name:    "a root below a recursive root collapses into it",
			queries: []string{"./a/b", "./a/**", "./a/c/**"},
			roots:   []glob.Root{{Dir: "a", Recursive: true}},
		},
		{
			name:    "a negation takes no root",
			queries: []string{"./a/**", "!./b"},
			roots:   []glob.Root{{Dir: "a", Recursive: true}},
		},
		{
			name:    "walked and listed roots together",
			queries: []string{"./a/**", "./b/*"},
			roots: []glob.Root{
				{Dir: "a", Recursive: true},
				{Dir: "b/*"},
			},
		},
		{
			name:    "a path above the working directory selects nothing",
			queries: []string{"../other/**"},
		},
		{
			name:    "leading globstar",
			queries: []string{"**/unit"},
			roots:   everything,
		},
		{
			name:    "globstar alone",
			queries: []string{"./**"},
			roots:   everything,
		},
		{
			name:    "name attribute",
			queries: []string{"name=vpc"},
			roots:   everything,
		},
		{
			name:    "type attribute beside a path",
			queries: []string{"./a/**", "type=unit"},
			roots:   everything,
		},
		{
			name:    "intersection with an attribute",
			queries: []string{"./a/** | name=vpc"},
			roots:   everything,
		},
		{
			name:    "reading attribute",
			queries: []string{"reading=common.hcl"},
			roots:   everything,
		},
		{
			name:    "dependents",
			queries: []string{"...vpc"},
			roots:   everything,
		},
		{
			name:    "dependencies of a path",
			queries: []string{"./a..."},
			roots:   everything,
		},
		{
			name:    "negated graph expression",
			queries: []string{"./a/**", "!...vpc"},
			roots:   everything,
		},
		{
			name:    "git expression takes no root",
			queries: []string{"[main...HEAD]"},
		},
		{
			name:    "git expression beside a path",
			queries: []string{"./apps/**", "[main...HEAD]"},
			roots:   []glob.Root{{Dir: "apps", Recursive: true}},
		},
		{
			name:    "git expression intersected with a path",
			queries: []string{"./apps/** | [main...HEAD]"},
			roots:   []glob.Root{{Dir: "apps", Recursive: true}},
		},
		{
			name:    "git expression as a graph target",
			queries: []string{"...[main...HEAD]"},
			roots:   everything,
		},
		{
			name:    "absolute path inside the working directory",
			queries: []string{absolute + "/a/**"},
			roots:   []glob.Root{{Dir: "a", Recursive: true}},
		},
		{
			name:    "absolute working directory",
			queries: []string{absolute},
			roots:   []glob.Root{{Dir: "."}},
		},
		{
			name:    "absolute globstar above the working directory",
			queries: []string{parent + "/**"},
			roots:   everything,
		},
		{
			name:    "absolute literal above the working directory",
			queries: []string{parent},
		},
		{
			name:    "absolute path beside the working directory",
			queries: []string{absolute + "-other/**"},
		},
		{
			name:    "absolute wildcard above the working directory",
			queries: []string{parent + "/*/a/**"},
			roots:   everything,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(logger.CreateLogger(), tt.queries)
			require.NoError(t, err)

			roots, reasons := filter.NewClassifier(filters).WalkRoots(workingDir)

			assert.ElementsMatch(t, tt.roots, roots, "reasons: %v", reasons)

			if slices.Equal(tt.roots, everything) {
				assert.NotEmpty(t, reasons)
			}
		})
	}
}
