// Package topo orders nodes so that each one comes after every node it waits on.
package topo

import "slices"

// Graph tracks nodes and the nodes each one waits on. It hands out a node once everything it waits on is done, so a
// caller can process the graph one layer at a time.
//
// A node never becomes ready when it waits on a node that is never done, whether because of a cycle, a node that was
// never added, or a node the caller chose not to mark done.
type Graph[K comparable] struct {
	waiting    map[K]int
	dependents map[K][]K
	roots      []K
}

// New returns an empty graph with room for size nodes.
func New[K comparable](size int) *Graph[K] {
	return &Graph[K]{
		waiting:    make(map[K]int, size),
		dependents: make(map[K][]K, size),
	}
}

// Add records node, waiting on each of waitsOn. Add each node once. A node listed twice in waitsOn is waited on
// twice, and [Graph.Done] on it counts for both.
func (g *Graph[K]) Add(node K, waitsOn ...K) {
	if len(waitsOn) == 0 {
		g.roots = append(g.roots, node)
		return
	}

	g.waiting[node] = len(waitsOn)

	for _, dep := range waitsOn {
		g.dependents[dep] = append(g.dependents[dep], node)
	}
}

// Roots returns the nodes that wait on nothing, in the order they were added.
func (g *Graph[K]) Roots() []K {
	return slices.Clone(g.roots)
}

// Done marks node as done and returns the nodes it leaves with nothing to wait on, in the order they were added. Call
// it once per node.
func (g *Graph[K]) Done(node K) []K {
	var ready []K

	for _, dependent := range g.dependents[node] {
		g.waiting[dependent]--

		if g.waiting[dependent] == 0 {
			ready = append(ready, dependent)
		}
	}

	return ready
}
