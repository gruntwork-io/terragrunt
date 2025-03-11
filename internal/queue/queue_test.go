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
}
