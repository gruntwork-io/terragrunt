package dag_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/view/dag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderWithoutColor renders components the same way both production callers do
// (the runner's logUnitDeployOrderDAG and list's renderTree): generate the tree,
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

func TestGenerateDAGTreeColoredMatchesPlainAfterStrippingANSI(t *testing.T) {
	t.Parallel()

	// A chain plus siblings at each level forces both indenter continuation
	// bars ("│") and enumerator branches, so any spacing drift between the
	// styled indenter and the default 4-cell column shows up as misalignment.
	a := &dag.ListedComponent{Type: component.UnitKind, Path: "a"}
	b := &dag.ListedComponent{Type: component.UnitKind, Path: "b", Dependencies: []*dag.ListedComponent{a}}
	c := &dag.ListedComponent{Type: component.UnitKind, Path: "c", Dependencies: []*dag.ListedComponent{b}}
	d := &dag.ListedComponent{Type: component.UnitKind, Path: "d", Dependencies: []*dag.ListedComponent{b}}
	e := &dag.ListedComponent{Type: component.UnitKind, Path: "e", Dependencies: []*dag.ListedComponent{a}}

	render := func(shouldColor bool) string {
		styler := dag.NewTreeStyler(shouldColor)
		tr := dag.GenerateDAGTree(dag.ListedComponents{a, b, c, d, e}, styler)

		return styler.Style(tr).String()
	}

	colored := render(true)
	plain := render(false)

	// Guard against the parity check passing vacuously: if lipgloss ever
	// stops emitting escapes in this environment, fail loudly instead.
	require.Contains(t, colored, "\x1b[")

	sgr := regexp.MustCompile(`\x1b\[[0-9;]*m`)

	assert.Equal(t, plain, sgr.ReplaceAllString(colored, ""))
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

// SGR sequences lipgloss emits for the colorizer styles in NewColorizer.
const (
	unitSGR    = "\x1b[1;94m"
	stackSGR   = "\x1b[1;92m"
	headingSGR = "\x1b[1;93m"
	pathSGR    = "\x1b[2;37m"
	resetSGR   = "\x1b[m"
)

func TestColorizer(t *testing.T) {
	t.Parallel()

	const otherKind = component.Kind("file")

	testCases := []struct {
		render      func(c *dag.Colorizer) string
		name        string
		expected    string
		shouldColor bool
	}{
		{
			name:        "colorize bare unit path",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.Colorize(&dag.ListedComponent{Type: component.UnitKind, Path: "a"})
			},
			expected: unitSGR + "a" + resetSGR,
		},
		{
			name:        "colorize bare stack path",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.Colorize(&dag.ListedComponent{Type: component.StackKind, Path: "a"})
			},
			expected: stackSGR + "a" + resetSGR,
		},
		{
			name:        "colorize bare path of other kind stays plain",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.Colorize(&dag.ListedComponent{Type: otherKind, Path: "a"})
			},
			expected: "a",
		},
		{
			name:        "colorize nested unit path dims the directory",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.Colorize(&dag.ListedComponent{Type: component.UnitKind, Path: "live/a"})
			},
			expected: pathSGR + "live/" + resetSGR + unitSGR + "a" + resetSGR,
		},
		{
			name:        "colorize nested stack path dims the directory",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.Colorize(&dag.ListedComponent{Type: component.StackKind, Path: "live/a"})
			},
			expected: pathSGR + "live/" + resetSGR + stackSGR + "a" + resetSGR,
		},
		{
			name:        "colorize nested path of other kind stays plain",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.Colorize(&dag.ListedComponent{Type: otherKind, Path: "live/a"})
			},
			expected: "live/a",
		},
		{
			name:        "colorize kind unit",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.ColorizeKind("live/a", component.UnitKind)
			},
			expected: unitSGR + "live/a" + resetSGR,
		},
		{
			name:        "colorize kind stack",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.ColorizeKind("live/a", component.StackKind)
			},
			expected: stackSGR + "live/a" + resetSGR,
		},
		{
			name:        "colorize kind other uses path color",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.ColorizeKind("live", otherKind)
			},
			expected: pathSGR + "live" + resetSGR,
		},
		{
			name:        "colorize type unit is padded to stack width",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.ColorizeType(component.UnitKind)
			},
			expected: unitSGR + "unit " + resetSGR,
		},
		{
			name:        "colorize type stack",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.ColorizeType(component.StackKind)
			},
			expected: stackSGR + "stack" + resetSGR,
		},
		{
			name:        "colorize type other returns the raw kind",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.ColorizeType(otherKind)
			},
			expected: "file",
		},
		{
			name:        "colorize heading",
			shouldColor: true,
			render: func(c *dag.Colorizer) string {
				return c.ColorizeHeading("Dependencies")
			},
			expected: headingSGR + "Dependencies" + resetSGR,
		},
		{
			name:        "no color colorize nested unit path",
			shouldColor: false,
			render: func(c *dag.Colorizer) string {
				return c.Colorize(&dag.ListedComponent{Type: component.UnitKind, Path: "live/a"})
			},
			expected: "live/a",
		},
		{
			name:        "no color colorize nested stack path",
			shouldColor: false,
			render: func(c *dag.Colorizer) string {
				return c.Colorize(&dag.ListedComponent{Type: component.StackKind, Path: "live/a"})
			},
			expected: "live/a",
		},
		{
			name:        "no color colorize bare stack path",
			shouldColor: false,
			render: func(c *dag.Colorizer) string {
				return c.Colorize(&dag.ListedComponent{Type: component.StackKind, Path: "a"})
			},
			expected: "a",
		},
		{
			name:        "no color colorize kind other",
			shouldColor: false,
			render: func(c *dag.Colorizer) string {
				return c.ColorizeKind("live", otherKind)
			},
			expected: "live",
		},
		{
			name:        "no color colorize type unit keeps padding",
			shouldColor: false,
			render: func(c *dag.Colorizer) string {
				return c.ColorizeType(component.UnitKind)
			},
			expected: "unit ",
		},
		{
			name:        "no color colorize heading",
			shouldColor: false,
			render: func(c *dag.Colorizer) string {
				return c.ColorizeHeading("Dependencies")
			},
			expected: "Dependencies",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, tc.render(dag.NewColorizer(tc.shouldColor)))
		})
	}
}

func TestTreeStylerColorizerFollowsColorSetting(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		expected    string
		shouldColor bool
	}{
		{
			name:        "colored styler",
			shouldColor: true,
			expected:    headingSGR + "deps" + resetSGR,
		},
		{
			name:        "plain styler",
			shouldColor: false,
			expected:    "deps",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			colorizer := dag.NewTreeStyler(tc.shouldColor).Colorizer()
			require.NotNil(t, colorizer)

			assert.Equal(t, tc.expected, colorizer.ColorizeHeading("deps"))
		})
	}
}

func TestRenderDot(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		components func() dag.ListedComponents
		name       string
		expected   string
	}{
		{
			name: "no components",
			components: func() dag.ListedComponents {
				return nil
			},
			expected: "digraph {\n}\n",
		},
		{
			name: "components and dependencies are sorted by path",
			components: func() dag.ListedComponents {
				vpc := &dag.ListedComponent{Type: component.UnitKind, Path: "vpc"}
				db := &dag.ListedComponent{Type: component.UnitKind, Path: "db"}
				app := &dag.ListedComponent{
					Type:         component.UnitKind,
					Path:         "app",
					Dependencies: []*dag.ListedComponent{vpc, db},
				}

				return dag.ListedComponents{vpc, app, db}
			},
			expected: strings.Join([]string{
				"digraph {",
				"\t\"app\" ;",
				"\t\"app\" -> \"db\";",
				"\t\"app\" -> \"vpc\";",
				"\t\"db\" ;",
				"\t\"vpc\" ;",
				"}",
				"",
			}, "\n"),
		},
		{
			name: "excluded components are colored red",
			components: func() dag.ListedComponents {
				vpc := &dag.ListedComponent{Type: component.UnitKind, Path: "vpc", Excluded: true}
				app := &dag.ListedComponent{
					Type:         component.UnitKind,
					Path:         "app",
					Dependencies: []*dag.ListedComponent{vpc},
				}

				return dag.ListedComponents{app, vpc}
			},
			expected: strings.Join([]string{
				"digraph {",
				"\t\"app\" ;",
				"\t\"app\" -> \"vpc\";",
				"\t\"vpc\" [color=red];",
				"}",
				"",
			}, "\n"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf strings.Builder

			require.NoError(t, dag.RenderDot(&buf, tc.components()))
			assert.Equal(t, tc.expected, buf.String())
		})
	}
}

func TestRenderDotKeepsCallerComponentOrder(t *testing.T) {
	t.Parallel()

	b := &dag.ListedComponent{Type: component.UnitKind, Path: "b"}
	a := &dag.ListedComponent{Type: component.UnitKind, Path: "a"}
	components := dag.ListedComponents{b, a}

	var buf strings.Builder

	require.NoError(t, dag.RenderDot(&buf, components))

	assert.Same(t, b, components[0])
	assert.Same(t, a, components[1])
}

func TestRenderDotReturnsWriterError(t *testing.T) {
	t.Parallel()

	errWrite := errors.New("write failed")

	err := dag.RenderDot(failingWriter{err: errWrite}, dag.ListedComponents{
		&dag.ListedComponent{Type: component.UnitKind, Path: "a"},
	})

	require.ErrorIs(t, err, errWrite)
}

// failingWriter is an io.Writer that always returns err.
type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}
