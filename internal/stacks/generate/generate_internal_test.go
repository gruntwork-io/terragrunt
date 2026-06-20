package generate

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stackFile builds a canonical stack-file path under workingDir from path segments.
func stackFile(workingDir string, segments ...string) string {
	parts := append([]string{workingDir}, segments...)
	parts = append(parts, config.DefaultStackFile)

	return filepath.Join(parts...)
}

func set(files ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(files))
	for _, f := range files {
		out[f] = struct{}{}
	}

	return out
}

func graph(nodes ...*StackNode) map[string]*StackNode {
	out := make(map[string]*StackNode, len(nodes))
	for _, n := range nodes {
		out[n.FilePath] = n
	}

	return out
}

func withChild(parent, child *StackNode) *StackNode {
	child.Parent = parent
	parent.Children = append(parent.Children, child)

	return parent
}

// TestSelectDescendantNodes covers the selective-recursion rules used when a filter is active during stack generation.
func TestSelectDescendantNodes(t *testing.T) {
	t.Parallel()

	wd := filepath.FromSlash("/work")

	parent := stackFile(wd, "stacks", "first")
	nested := stackFile(wd, "stacks", "first", ".terragrunt-stack", "first")
	nestedSibling := stackFile(wd, "stacks", "first", ".terragrunt-stack", "second")
	topLevelSibling := stackFile(wd, "stacks", "second")
	deepA := stackFile(wd, "stacks", "first", ".terragrunt-stack", "first", ".terragrunt-stack", "a")
	deepB := stackFile(wd, "stacks", "first", ".terragrunt-stack", "first", ".terragrunt-stack", "b")

	testCases := []struct {
		name       string
		candidates []string
		nodes      map[string]*StackNode
		matched    map[string]struct{}
		excluded   map[string]struct{}
		want       []string
	}{
		{
			name:       "parent selected, child not named: full recursion keeps the child",
			candidates: []string{nested},
			nodes:      graph(NewStackNode(parent)),
			matched:    set(parent),
			want:       []string{nested},
		},
		{
			name:       "no filter: every candidate kept",
			candidates: []string{nested},
			nodes:      graph(NewStackNode(parent)),
			matched:    set(parent, nested),
			want:       []string{nested},
		},
		{
			name:       "selective: only the matched child is kept among siblings",
			candidates: []string{nested, nestedSibling},
			nodes:      graph(NewStackNode(parent)),
			matched:    set(parent, nested),
			want:       []string{nested},
		},
		{
			name:       "no child matches: all children of the parent are kept",
			candidates: []string{nested, nestedSibling},
			nodes:      graph(NewStackNode(parent)),
			matched:    set(parent),
			want:       []string{nested, nestedSibling},
		},
		{
			name:       "orphan top-level stack not matched is dropped",
			candidates: []string{topLevelSibling},
			nodes:      graph(NewStackNode(parent)),
			matched:    set(parent),
			want:       []string{},
		},
		{
			name:       "orphan top-level stack that matches is kept",
			candidates: []string{topLevelSibling},
			nodes:      graph(NewStackNode(parent)),
			matched:    set(parent, topLevelSibling),
			want:       []string{topLevelSibling},
		},
		{
			// A matched child already in the graph keeps selection stable across passes.
			name:       "selective stays stable when the matched child is already in the graph",
			candidates: []string{nestedSibling},
			nodes:      graph(withChild(NewStackNode(parent), NewStackNode(nested))),
			matched:    set(parent, nested),
			want:       []string{},
		},
		{
			// A deeper match under a fully recursed (unmatched) parent must not prune its sibling.
			name:       "deep match under a fully recursed parent keeps the sibling",
			candidates: []string{deepA, deepB},
			nodes:      graph(NewStackNode(parent), NewStackNode(nested)),
			matched:    set(parent, deepA),
			want:       []string{deepA, deepB},
		},
		{
			name:       "explicitly excluded child is dropped even under full recursion",
			candidates: []string{nested},
			nodes:      graph(NewStackNode(parent)),
			matched:    set(parent),
			excluded:   set(nested),
			want:       []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := selectDescendantNodes(tc.candidates, tc.nodes, tc.matched, tc.excluded, wd)

			assert.ElementsMatch(t, tc.want, got)
		})
	}
}

// TestStackRestrictedNegatives verifies only negated filters whose un-negated form stays stack-restricted are returned.
func TestStackRestrictedNegatives(t *testing.T) {
	t.Parallel()

	parse := func(query string) *filter.Filter {
		f, err := filter.Parse(query)
		require.NoError(t, err)

		return f
	}

	testCases := []struct {
		name      string
		input     filter.Filters
		wantCount int
	}{
		{
			name:      "positive stack filter contributes nothing",
			input:     filter.Filters{parse("./live | type=stack")},
			wantCount: 0,
		},
		{
			name:      "negated path with type=stack is un-negated and kept",
			input:     filter.Filters{parse("./live | type=stack"), parse("!./**/a | type=stack")},
			wantCount: 1,
		},
		{
			name:      "bare negated non-stack type is dropped",
			input:     filter.Filters{parse("./live | type=stack"), parse("!type=unit")},
			wantCount: 0,
		},
		{
			name:      "negated type=stack is not stack-restricted and dropped",
			input:     filter.Filters{parse("!type=stack")},
			wantCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := stackRestrictedNegatives(tc.input)

			assert.Len(t, got, tc.wantCount)

			// Each returned filter must be un-negated and stack-restricted, else everything matches as excluded.
			for _, f := range got {
				assert.False(t, filter.IsNegated(f.Expression()))
				assert.True(t, f.Expression().IsRestrictedToStacks())
			}
		})
	}
}
