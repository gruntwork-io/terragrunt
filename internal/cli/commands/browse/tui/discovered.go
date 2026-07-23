package tui

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
)

// discovered is the read model the background discovery pass produces, built
// once when discovery completes and only read afterward. Until then its zero
// value serves the loading state: the lookups return nothing and done is false.
type discovered struct {
	index     map[string]component.Component
	readFiles map[string]struct{}
	dirCounts map[string]dirCount
	done      bool
}

// dirCount is a directory's tally of discovered units and stacks at or below it.
type dirCount struct {
	units  int
	stacks int
}

// apply records the discovery result and annotates the loaded tree rooted at
// root, so later renders resolve counts, metadata, and read-file highlighting in
// place of their loading placeholders. A failed discovery still applies whatever
// partial components it produced; the caller flags the failure as a toast.
func (d *discovered) apply(res DiscoveryResult, root *Node) {
	d.index = make(map[string]component.Component, len(res.Components))
	d.readFiles = map[string]struct{}{}

	for _, c := range res.Components {
		d.index[c.Path()] = c

		for _, f := range c.Reading() {
			d.readFiles[f] = struct{}{}
		}
	}

	d.attachComponents(root)
	d.computeCounts(root)
	d.done = true
}

// attachComponents walks the loaded tree and attaches each node's discovered
// component, refining its kind to discovery's authority.
func (d *discovered) attachComponents(n *Node) {
	if c, ok := d.index[n.absPath]; ok {
		n.component = c
		n.kind = kindForComponent(c)
	}

	for _, child := range n.children {
		d.attachComponents(child)
	}
}

// computeCounts tallies, for every directory, the units and stacks discovered at
// or below it, so a directory's totals resolve with a single map lookup per
// render instead of a full scan of the discovery index. It draws on the index
// rather than the lazily loaded tree, so totals are correct even for directories
// that haven't been expanded.
func (d *discovered) computeCounts(root *Node) {
	counts := make(map[string]dirCount)

	for path, c := range d.index {
		isStack := c.Kind() == component.StackKind

		// Attribute the component to its own directory and every ancestor up to
		// the root, walking with filepath.Dir so the filesystem root is handled
		// without the trailing-separator special case a prefix test would need.
		for p := path; ; p = filepath.Dir(p) {
			dc := counts[p]
			if isStack {
				dc.stacks++
			} else {
				dc.units++
			}

			counts[p] = dc

			if p == root.absPath {
				break
			}

			if parent := filepath.Dir(p); parent == p {
				break
			}
		}
	}

	d.dirCounts = counts
}

// counts returns the number of discovered units and stacks at or below the
// node's path, from the tally computed once discovery completes.
func (d discovered) counts(n *Node) (units, stacks int) {
	dc := d.dirCounts[n.absPath]

	return dc.units, dc.stacks
}

// isRead reports whether path is one of the files a discovered component reads.
func (d discovered) isRead(path string) bool {
	_, ok := d.readFiles[path]

	return ok
}

// lookup returns the component discovered at path, if any.
func (d discovered) lookup(path string) (component.Component, bool) {
	c, ok := d.index[path]

	return c, ok
}
