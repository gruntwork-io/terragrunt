package queue_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/stretchr/testify/assert"
)

func TestNewQueue(t *testing.T) {
	t.Run("no dependencies", func(t *testing.T) {
		// Create configs with no dependencies
		configs := []*discovery.DiscoveredConfig{
			{Path: "first", Dependencies: []*discovery.DiscoveredConfig{}},
			{Path: "second", Dependencies: []*discovery.DiscoveredConfig{}},
			{Path: "third", Dependencies: []*discovery.DiscoveredConfig{}},
		}

		q := queue.NewQueue(configs)

		// Order should remain the same as input
		assert.Equal(t, "first", q.Entries()[0].Path)
		assert.Equal(t, "second", q.Entries()[1].Path)
		assert.Equal(t, "third", q.Entries()[2].Path)
	})

	t.Run("already ordered dependencies", func(t *testing.T) {
		// Create configs where dependencies are already in correct order
		configs := []*discovery.DiscoveredConfig{
			{Path: "first", Dependencies: []*discovery.DiscoveredConfig{}},
			{Path: "second", Dependencies: []*discovery.DiscoveredConfig{{Path: "first"}}},
			{Path: "third", Dependencies: []*discovery.DiscoveredConfig{{Path: "second"}}},
		}

		q := queue.NewQueue(configs)

		// Order should remain the same as input
		assert.Equal(t, "first", q.Entries()[0].Path)
		assert.Equal(t, "second", q.Entries()[1].Path)
		assert.Equal(t, "third", q.Entries()[2].Path)
	})

	t.Run("reorder needed for dependencies", func(t *testing.T) {
		// Create configs where order needs to be adjusted
		configs := []*discovery.DiscoveredConfig{
			{Path: "third", Dependencies: []*discovery.DiscoveredConfig{{Path: "second"}}},
			{Path: "second", Dependencies: []*discovery.DiscoveredConfig{{Path: "first"}}},
			{Path: "first", Dependencies: []*discovery.DiscoveredConfig{}},
		}

		q := queue.NewQueue(configs)

		// Order should be rearranged to satisfy dependencies
		assert.Equal(t, "first", q.Entries()[0].Path)
		assert.Equal(t, "second", q.Entries()[1].Path)
		assert.Equal(t, "third", q.Entries()[2].Path)
	})

	t.Run("complex dag with multiple paths", func(t *testing.T) {
		// Create a more complex dependency graph:
		//   A -> B -> C
		//   |         ^
		//   +-> D ----+
		//   |
		//   +-> E -> F
		configs := []*discovery.DiscoveredConfig{
			{Path: "F", Dependencies: []*discovery.DiscoveredConfig{{Path: "E"}}},
			{Path: "E", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
			{Path: "D", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
			{Path: "C", Dependencies: []*discovery.DiscoveredConfig{{Path: "B"}, {Path: "D"}}},
			{Path: "B", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
			{Path: "A", Dependencies: []*discovery.DiscoveredConfig{}},
		}

		q := queue.NewQueue(configs)
		entries := q.Entries()

		// Verify that dependencies are satisfied
		// A must come before B, D, and E
		aIndex := findIndex(entries, "A")
		bIndex := findIndex(entries, "B")
		dIndex := findIndex(entries, "D")
		eIndex := findIndex(entries, "E")
		assert.Less(t, aIndex, bIndex, "A should come before B")
		assert.Less(t, aIndex, dIndex, "A should come before D")
		assert.Less(t, aIndex, eIndex, "A should come before E")

		// B and D must come before C
		cIndex := findIndex(entries, "C")
		assert.Less(t, bIndex, cIndex, "B should come before C")
		assert.Less(t, dIndex, cIndex, "D should come before C")

		// E must come before F
		fIndex := findIndex(entries, "F")
		assert.Less(t, eIndex, fIndex, "E should come before F")
	})

	t.Run("deterministic ordering of parallel paths", func(t *testing.T) {
		// Create a graph with parallel paths that could be ordered multiple ways:
		//   A -> B
		//   A -> C
		//   A -> D
		configs := []*discovery.DiscoveredConfig{
			{Path: "D", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
			{Path: "C", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
			{Path: "B", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
			{Path: "A", Dependencies: []*discovery.DiscoveredConfig{}},
		}

		// Run multiple times to verify deterministic ordering
		for i := 0; i < 5; i++ {
			q := queue.NewQueue(configs)
			entries := q.Entries()

			// A should always be first
			assert.Equal(t, "A", entries[0].Path)

			// B, C, D should maintain alphabetical order since they're all at the same level
			assert.Equal(t, "B", entries[1].Path)
			assert.Equal(t, "C", entries[2].Path)
			assert.Equal(t, "D", entries[3].Path)
		}
	})
}

// findIndex returns the index of the config with the given path in the slice
func findIndex(configs []*discovery.DiscoveredConfig, path string) int {
	for i, cfg := range configs {
		if cfg.Path == path {
			return i
		}
	}
	return -1
}
