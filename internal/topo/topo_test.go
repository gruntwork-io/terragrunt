package topo_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/topo"
	"github.com/stretchr/testify/assert"
)

func TestGraphLayers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		add    func(g *topo.Graph[string])
		name   string
		layers [][]string
	}{
		{
			name: "empty",
			add:  func(*topo.Graph[string]) {},
		},
		{
			name: "chain",
			add: func(g *topo.Graph[string]) {
				g.Add("c", "b")
				g.Add("b", "a")
				g.Add("a")
			},
			layers: [][]string{{"a"}, {"b"}, {"c"}},
		},
		{
			name: "diamond",
			add: func(g *topo.Graph[string]) {
				g.Add("root")
				g.Add("left", "root")
				g.Add("right", "root")
				g.Add("joined", "left", "right")
			},
			layers: [][]string{{"root"}, {"left", "right"}, {"joined"}},
		},
		{
			name: "duplicate wait",
			add: func(g *topo.Graph[string]) {
				g.Add("a")
				g.Add("b", "a", "a")
			},
			layers: [][]string{{"a"}, {"b"}},
		},
		{
			name: "cycle",
			add: func(g *topo.Graph[string]) {
				g.Add("free")
				g.Add("a", "b")
				g.Add("b", "a")
				g.Add("after", "a", "free")
			},
			layers: [][]string{{"free"}},
		},
		{
			name: "self wait",
			add: func(g *topo.Graph[string]) {
				g.Add("a", "a")
			},
		},
		{
			name: "wait on node never added",
			add: func(g *topo.Graph[string]) {
				g.Add("a")
				g.Add("b", "a", "missing")
			},
			layers: [][]string{{"a"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := topo.New[string](0)
			tc.add(g)

			var layers [][]string

			for ready := g.Roots(); len(ready) > 0; {
				layers = append(layers, ready)

				var next []string

				for _, node := range ready {
					next = append(next, g.Done(node)...)
				}

				ready = next
			}

			assert.Equal(t, tc.layers, layers)
		})
	}
}

func TestGraphDoneWithheldBlocksDependents(t *testing.T) {
	t.Parallel()

	g := topo.New[string](0)
	g.Add("ok")
	g.Add("failed")
	g.Add("after-ok", "ok")
	g.Add("after-both", "ok", "failed")

	assert.Equal(t, []string{"ok", "failed"}, g.Roots())
	assert.Equal(t, []string{"after-ok"}, g.Done("ok"))
}
