package queue_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/stretchr/testify/assert"
)

func TestNewQueue(t *testing.T) {
	t.Parallel()

	t.Run("no dependencies - maintains alphabetical order", func(t *testing.T) {
		t.Parallel()

		// Create configs with no dependencies - should maintain alphabetical order at front
		configs := []*discovery.DiscoveredConfig{
			{Path: "c", Dependencies: []*discovery.DiscoveredConfig{}},
			{Path: "a", Dependencies: []*discovery.DiscoveredConfig{}},
			{Path: "b", Dependencies: []*discovery.DiscoveredConfig{}},
		}

		q := queue.NewQueue(configs)
		entries := q.Entries()

		// Should be sorted alphabetically at front since none have dependencies
		assert.Equal(t, "a", entries[0].Path)
		assert.Equal(t, "b", entries[1].Path)
		assert.Equal(t, "c", entries[2].Path)
	})

	t.Run("dependencies - ordered by dependency level", func(t *testing.T) {
		t.Parallel()

		// Create configs with dependencies - should order by dependency level
		configs := []*discovery.DiscoveredConfig{
			{Path: "c", Dependencies: []*discovery.DiscoveredConfig{{Path: "b"}}},
			{Path: "b", Dependencies: []*discovery.DiscoveredConfig{{Path: "a"}}},
			{Path: "a", Dependencies: []*discovery.DiscoveredConfig{}},
		}

		q := queue.NewQueue(configs)
		entries := q.Entries()

		// 'a' has no deps so should be at front
		// 'b' depends on 'a' so should be after
		// 'c' depends on 'b' so should be at back
		assert.Equal(t, "a", entries[0].Path)
		assert.Equal(t, "b", entries[1].Path)
		assert.Equal(t, "c", entries[2].Path)
	})

	t.Run("complex dag - ordered by dependency level and alphabetically", func(t *testing.T) {
		t.Parallel()

		// Create a more complex dependency graph:
		//   A (no deps)
		//   B (no deps)
		//   C -> A
		//   D -> A,B
		//   E -> C
		//   F -> C
		configs := []*discovery.DiscoveredConfig{
			{Path: "F", Dependencies: []*discovery.DiscoveredConfig{{Path: "C"}}},
			{Path: "E", Dependencies: []*discovery.DiscoveredConfig{{Path: "C"}}},
			{Path: "D", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}, {Path: "B"}}},
			{Path: "C", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
			{Path: "B", Dependencies: []*discovery.DiscoveredConfig{}},
			{Path: "A", Dependencies: []*discovery.DiscoveredConfig{}},
		}

		q := queue.NewQueue(configs)
		entries := q.Entries()

		// Verify ordering by dependency level and alphabetically within levels:
		// Level 0 (no deps): A, B
		// Level 1 (depends on level 0): C, D
		// Level 2 (depends on level 1): E, F
		assert.Equal(t, "A", entries[0].Path)
		assert.Equal(t, "B", entries[1].Path)
		assert.Equal(t, "C", entries[2].Path)
		assert.Equal(t, "D", entries[3].Path)
		assert.Equal(t, "E", entries[4].Path)
		assert.Equal(t, "F", entries[5].Path)

		// Also verify relative ordering
		aIndex := findIndex(entries, "A")
		bIndex := findIndex(entries, "B")
		cIndex := findIndex(entries, "C")
		dIndex := findIndex(entries, "D")
		eIndex := findIndex(entries, "E")
		fIndex := findIndex(entries, "F")

		// Level 0 items should be before their dependents
		assert.Less(t, aIndex, cIndex, "A should come before C")
		assert.Less(t, aIndex, dIndex, "A should come before D")
		assert.Less(t, bIndex, dIndex, "B should come before D")

		// Level 1 items should be before their dependents
		assert.Less(t, cIndex, eIndex, "C should come before E")
		assert.Less(t, cIndex, fIndex, "C should come before F")
	})

	t.Run("deterministic ordering of parallel dependencies", func(t *testing.T) {
		t.Parallel()

		// Create a graph with parallel dependencies that could be ordered multiple ways:
		//   A (no deps)
		//   B -> A
		//   C -> A
		//   D -> A
		configs := []*discovery.DiscoveredConfig{
			{Path: "D", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
			{Path: "C", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
			{Path: "B", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
			{Path: "A", Dependencies: []*discovery.DiscoveredConfig{}},
		}

		// Run multiple times to verify deterministic ordering
		for range 5 {
			q := queue.NewQueue(configs)
			entries := q.Entries()

			// A should be first (no deps)
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
