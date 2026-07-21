package dag_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/view/dag"
	"github.com/stretchr/testify/assert"
)

// renderWithoutColor renders components the same way both production callers do
// (runnerpool's logUnitDeployOrderDAG and list's renderTree): generate the tree,
// apply the styler, and stringify. Color is disabled so the output is stable.
func renderWithoutColor(components dag.ListedComponents) string {
	styler := dag.NewTreeStyler(false)
	tr := dag.GenerateDAGTree(components, styler)

	return styler.Style(tr).String()
}

func TestGenerateDAGTreeRendersLinearChain(t *testing.T) {
	t.Parallel()

	a := &dag.ListedComponent{Type: component.UnitKind, Path: "a"}
	b := &dag.ListedComponent{
		Type:         component.UnitKind,
		Path:         "b",
		Dependencies: []*dag.ListedComponent{a},
	}
	c := &dag.ListedComponent{
		Type:         component.UnitKind,
		Path:         "c",
		Dependencies: []*dag.ListedComponent{b},
	}

	rendered := renderWithoutColor(dag.ListedComponents{a, b, c})

	expected := strings.Join([]string{
		".",
		"╰── a",
		"    ╰── b",
		"        ╰── c",
	}, "\n")

	assert.Equal(t, expected, rendered)
}

func TestGenerateDAGTreeSortsRootsBySubtreeSizeThenAlphabetically(t *testing.T) {
	t.Parallel()

	// solo-a and solo-b have subtree size 0 and sort alphabetically.
	// base anchors a two-node chain (subtree size 2), so it sorts last
	// even though "base" precedes "solo-a" alphabetically.
	soloB := &dag.ListedComponent{Type: component.UnitKind, Path: "solo-b"}
	soloA := &dag.ListedComponent{Type: component.UnitKind, Path: "solo-a"}
	base := &dag.ListedComponent{Type: component.UnitKind, Path: "base"}
	mid := &dag.ListedComponent{
		Type:         component.UnitKind,
		Path:         "mid",
		Dependencies: []*dag.ListedComponent{base},
	}
	top := &dag.ListedComponent{
		Type:         component.UnitKind,
		Path:         "top",
		Dependencies: []*dag.ListedComponent{mid},
	}

	rendered := renderWithoutColor(dag.ListedComponents{soloB, soloA, base, mid, top})

	expected := strings.Join([]string{
		".",
		"├── solo-a",
		"├── solo-b",
		"╰── base",
		"    ╰── mid",
		"        ╰── top",
	}, "\n")

	assert.Equal(t, expected, rendered)
}

func TestGenerateDAGTreeDuplicatesSharedDependentPerDependencyEdge(t *testing.T) {
	t.Parallel()

	// Diamond: b and c both depend on a; d depends on both b and c.
	// The shared dependent d intentionally renders once under each
	// dependency edge (see the GenerateDAGTree godoc).
	a := &dag.ListedComponent{Type: component.UnitKind, Path: "a"}
	b := &dag.ListedComponent{
		Type:         component.UnitKind,
		Path:         "b",
		Dependencies: []*dag.ListedComponent{a},
	}
	c := &dag.ListedComponent{
		Type:         component.UnitKind,
		Path:         "c",
		Dependencies: []*dag.ListedComponent{a},
	}
	d := &dag.ListedComponent{
		Type:         component.UnitKind,
		Path:         "d",
		Dependencies: []*dag.ListedComponent{b, c},
	}

	rendered := renderWithoutColor(dag.ListedComponents{a, b, c, d})

	expected := strings.Join([]string{
		".",
		"╰── a",
		"    ├── b",
		"    │   ╰── d",
		"    ╰── c",
		"        ╰── d",
	}, "\n")

	assert.Equal(t, expected, rendered)
}

func TestGenerateDAGTreeAttachesGrandchildOnlyToLastWiredDuplicate(t *testing.T) {
	t.Parallel()

	// Pins current behavior: when a shared dependent (d) is duplicated under
	// multiple dependency edges, a grandchild (f) attaches only to the
	// duplicate wired last (under c, the alphabetically last dependency).
	// The duplicate under b renders childless.
	a := &dag.ListedComponent{Type: component.UnitKind, Path: "a"}
	b := &dag.ListedComponent{
		Type:         component.UnitKind,
		Path:         "b",
		Dependencies: []*dag.ListedComponent{a},
	}
	c := &dag.ListedComponent{
		Type:         component.UnitKind,
		Path:         "c",
		Dependencies: []*dag.ListedComponent{a},
	}
	d := &dag.ListedComponent{
		Type:         component.UnitKind,
		Path:         "d",
		Dependencies: []*dag.ListedComponent{b, c},
	}
	f := &dag.ListedComponent{
		Type:         component.UnitKind,
		Path:         "f",
		Dependencies: []*dag.ListedComponent{d},
	}

	rendered := renderWithoutColor(dag.ListedComponents{a, b, c, d, f})

	expected := strings.Join([]string{
		".",
		"╰── a",
		"    ├── b",
		"    │   ╰── d",
		"    ╰── c",
		"        ╰── d",
		"            ╰── f",
	}, "\n")

	assert.Equal(t, expected, rendered)
}

func TestGenerateDAGTreeDropsNodesWithUnknownDependencyPaths(t *testing.T) {
	t.Parallel()

	// Pins current behavior: b declares a dependency on "./a", but the
	// component list only knows the path "a". Because the dependency path
	// matches no node, b is silently dropped from the tree instead of being
	// rendered as a root or surfaced as an error. This matters for the list
	// path, which rebuilds dependency nodes from path strings and can
	// produce such mismatches.
	a := &dag.ListedComponent{Type: component.UnitKind, Path: "a"}
	b := &dag.ListedComponent{
		Type: component.UnitKind,
		Path: "b",
		Dependencies: []*dag.ListedComponent{
			{Type: component.UnitKind, Path: "./a"},
		},
	}

	rendered := renderWithoutColor(dag.ListedComponents{a, b})

	expected := strings.Join([]string{
		".",
		"╰── a",
	}, "\n")

	assert.Equal(t, expected, rendered)
	assert.NotContains(t, rendered, "b")
}

func TestGenerateDAGTreeWithoutColorEmitsNoANSIEscapes(t *testing.T) {
	t.Parallel()

	a := &dag.ListedComponent{Type: component.UnitKind, Path: "live/a"}
	b := &dag.ListedComponent{
		Type:         component.StackKind,
		Path:         "live/b",
		Dependencies: []*dag.ListedComponent{a},
	}

	rendered := renderWithoutColor(dag.ListedComponents{a, b})

	assert.NotContains(t, rendered, "\x1b")
}

func TestFromComponentsWiresDependenciesForApplyOrder(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("vpc")
	app := component.NewUnit("app")
	app.AddDependency(vpc)

	listed := dag.FromComponents([]component.Component{vpc, app}, false)
	rendered := renderWithoutColor(listed)

	expected := strings.Join([]string{
		".",
		"╰── vpc",
		"    ╰── app",
	}, "\n")

	assert.Equal(t, expected, rendered)
}

func TestFromComponentsReversedInvertsEdgesForDestroyOrder(t *testing.T) {
	t.Parallel()

	// For destroy display the graph is inverted: app (the dependent)
	// becomes the root and vpc (its dependency) renders beneath it.
	vpc := component.NewUnit("vpc")
	app := component.NewUnit("app")
	app.AddDependency(vpc)

	listed := dag.FromComponentsReversed([]component.Component{vpc, app}, false)
	rendered := renderWithoutColor(listed)

	expected := strings.Join([]string{
		".",
		"╰── app",
		"    ╰── vpc",
	}, "\n")

	assert.Equal(t, expected, rendered)
}

func TestFromComponentsPathSelection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		expectedPath   string
		useDisplayPath bool
	}{
		{
			name:           "absolute path when display paths are disabled",
			useDisplayPath: false,
			expectedPath:   "/deploy/vpc",
		},
		{
			name:           "path relative to the discovery working dir when display paths are enabled",
			useDisplayPath: true,
			expectedPath:   "vpc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vpc := component.NewUnit("/deploy/vpc").
				WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: "/deploy",
				})

			listed := dag.FromComponents([]component.Component{vpc}, tc.useDisplayPath)

			assert.Len(t, listed, 1)
			assert.Equal(t, tc.expectedPath, listed[0].Path)
		})
	}
}

func TestListedComponentsContains(t *testing.T) {
	t.Parallel()

	components := dag.ListedComponents{
		&dag.ListedComponent{Type: component.UnitKind, Path: "a"},
		&dag.ListedComponent{Type: component.UnitKind, Path: "b"},
	}

	assert.True(t, components.Contains("a"))
	assert.False(t, components.Contains("./a"))
	assert.False(t, components.Contains("missing"))
}

func TestListedComponentsGet(t *testing.T) {
	t.Parallel()

	a := &dag.ListedComponent{Type: component.UnitKind, Path: "a"}
	components := dag.ListedComponents{a}

	assert.Same(t, a, components.Get("a"))
	assert.Nil(t, components.Get("missing"))
}

func TestListedComponentsSort(t *testing.T) {
	t.Parallel()

	components := dag.ListedComponents{
		&dag.ListedComponent{Type: component.UnitKind, Path: "c"},
		&dag.ListedComponent{Type: component.UnitKind, Path: "a"},
		&dag.ListedComponent{Type: component.UnitKind, Path: "b"},
	}

	components.Sort()

	paths := make([]string, 0, len(components))
	for _, c := range components {
		paths = append(paths, c.Path)
	}

	assert.Equal(t, []string{"a", "b", "c"}, paths)
}
