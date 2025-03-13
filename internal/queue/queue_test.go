package queue_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

		q, err := queue.NewQueue(configs)
		require.NoError(t, err)

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

		q, err := queue.NewQueue(configs)
		require.NoError(t, err)

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

		q, err := queue.NewQueue(configs)
		require.NoError(t, err)

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
			q, err := queue.NewQueue(configs)
			require.NoError(t, err)

			entries := q.Entries()

			// A should be first (no deps)
			assert.Equal(t, "A", entries[0].Path)

			// B, C, D should maintain alphabetical order since they're all at the same level
			assert.Equal(t, "B", entries[1].Path)
			assert.Equal(t, "C", entries[2].Path)
			assert.Equal(t, "D", entries[3].Path)
		}
	})

	t.Run("depth-based ordering verification", func(t *testing.T) {
		t.Parallel()

		// Create a graph where depth matters:
		//   A (no deps, depth 0)
		//   B (no deps, depth 0)
		//   C -> A (depth 1)
		//   D -> B (depth 1)
		//   E -> C,D (depth 2)
		configs := []*discovery.DiscoveredConfig{
			{Path: "E", Dependencies: []*discovery.DiscoveredConfig{{Path: "C"}, {Path: "D"}}},
			{Path: "D", Dependencies: []*discovery.DiscoveredConfig{{Path: "B"}}},
			{Path: "C", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
			{Path: "B", Dependencies: []*discovery.DiscoveredConfig{}},
			{Path: "A", Dependencies: []*discovery.DiscoveredConfig{}},
		}

		q, err := queue.NewQueue(configs)
		require.NoError(t, err)

		entries := q.Entries()

		// Verify that items are grouped by their depth levels
		// Level 0: A,B (no deps)
		// Level 1: C,D (depend on level 0)
		// Level 2: E (depends on level 1)

		// First verify the basic ordering
		assert.Len(t, entries, 5, "Should have all 5 entries")

		// Find indices
		aIndex := findIndex(entries, "A")
		bIndex := findIndex(entries, "B")
		cIndex := findIndex(entries, "C")
		dIndex := findIndex(entries, "D")
		eIndex := findIndex(entries, "E")

		// Level 0 items should be at the start (indices 0 or 1)
		assert.LessOrEqual(t, aIndex, 1, "A should be in first two positions")
		assert.LessOrEqual(t, bIndex, 1, "B should be in first two positions")

		// Level 1 items should be in the middle (indices 2 or 3)
		assert.True(t, cIndex >= 2 && cIndex <= 3, "C should be in middle positions")
		assert.True(t, dIndex >= 2 && dIndex <= 3, "D should be in middle positions")

		// Level 2 item should be at the end (index 4)
		assert.Equal(t, 4, eIndex, "E should be in last position")
	})

	t.Run("error handling - cycle", func(t *testing.T) {
		t.Parallel()

		// Create a cycle: A -> B -> C -> A
		configs := []*discovery.DiscoveredConfig{
			{Path: "C", Dependencies: []*discovery.DiscoveredConfig{{Path: "B"}}},
			{Path: "B", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
			{Path: "A", Dependencies: []*discovery.DiscoveredConfig{{Path: "C"}}},
		}

		q, err := queue.NewQueue(configs)
		require.Error(t, err)
		assert.NotNil(t, q)
	})

	t.Run("error handling - empty config list", func(t *testing.T) {
		t.Parallel()

		q, err := queue.NewQueue([]*discovery.DiscoveredConfig{})
		require.NoError(t, err)
		assert.Empty(t, q.Entries())
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
