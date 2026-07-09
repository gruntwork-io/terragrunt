package tui_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flatNode is a flattened tree entry used for asserting tree shape.
type flatNode struct {
	relPath string
	kind    tui.Kind
}

// flatten returns the tree's nodes in pre-order, excluding the root.
func flatten(root *tui.Node) []flatNode {
	out := make([]flatNode, 0, len(root.Children()))

	for _, c := range root.Children() {
		out = append(out, flatNode{relPath: c.RelPath(), kind: c.Kind()})
		out = append(out, flatten(c)...)
	}

	return out
}

func TestBuildTree(t *testing.T) {
	t.Parallel()

	const workingDir = "/work"

	abs := func(parts ...string) string {
		return filepath.Join(append([]string{workingDir}, parts...)...)
	}

	tests := []struct {
		name       string
		components component.Components
		want       []flatNode
	}{
		{
			name: "nested units and a stack are sorted alphabetically",
			components: component.Components{
				component.NewUnit(abs("prod", "vpc")),
				component.NewUnit(abs("prod", "eks")),
				component.NewStack(abs("prod", "rds")),
				component.NewUnit(abs("dev", "app")),
			},
			want: []flatNode{
				{relPath: "dev", kind: tui.KindDir},
				{relPath: filepath.Join("dev", "app"), kind: tui.KindUnit},
				{relPath: "prod", kind: tui.KindDir},
				{relPath: filepath.Join("prod", "eks"), kind: tui.KindUnit},
				{relPath: filepath.Join("prod", "rds"), kind: tui.KindStack},
				{relPath: filepath.Join("prod", "vpc"), kind: tui.KindUnit},
			},
		},
		{
			name: "a component nested under another upgrades the placeholder in place",
			components: component.Components{
				component.NewUnit(abs("prod", "vpc")),
				component.NewUnit(abs("prod")),
			},
			want: []flatNode{
				{relPath: "prod", kind: tui.KindUnit},
				{relPath: filepath.Join("prod", "vpc"), kind: tui.KindUnit},
			},
		},
		{
			name: "read files are not added to the tree",
			components: component.Components{
				component.NewUnit(abs("prod", "vpc")).WithReading(
					abs("root.hcl"),
					abs("prod", "vpc", "terragrunt.hcl"),
				),
			},
			want: []flatNode{
				{relPath: "prod", kind: tui.KindDir},
				{relPath: filepath.Join("prod", "vpc"), kind: tui.KindUnit},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := tui.BuildTree(workingDir, tt.components)

			assert.Equal(t, ".", root.Name())
			assert.Equal(t, tt.want, flatten(root))
		})
	}
}

func TestNavigation(t *testing.T) {
	t.Parallel()

	root := tui.BuildTree("/work", component.Components{
		component.NewUnit(filepath.Join("/work", "prod", "vpc")),
	})

	require.Len(t, root.Children(), 1)
	prod := root.Children()[0]

	// An empty filesystem is enough: these components have no on-disk files, so
	// loading surrounding entries finds nothing and navigation falls back to the
	// discovered tree.
	m := newModel(t, vfs.NewMemMapFS(), root, tui.ColorDisabled)

	// Descending from the root enters the prod directory.
	m = press(t, m, 'l')
	assert.Equal(t, prod, m.Current())

	// The vpc unit has no on-disk files here, so descending onto it finds nothing
	// to enter and leaves the current directory unchanged.
	m = press(t, m, 'l')
	assert.Equal(t, prod, m.Current())

	// Ascending returns to the root.
	m = press(t, m, 'h')
	assert.Equal(t, root, m.Current())

	// Ascending at the root stays put.
	m = press(t, m, 'h')
	assert.Equal(t, root, m.Current())
}
