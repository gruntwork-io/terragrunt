package queue_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoDependenciesMaintainsAlphabeticalOrder(t *testing.T) {
	t.Parallel()

	// Create configs with no dependencies - should maintain alphabetical order at front
	configs := []*discovery.DiscoveredConfig{
		{Path: "c", Dependencies: []*discovery.DiscoveredConfig{}},
		{Path: "a", Dependencies: []*discovery.DiscoveredConfig{}},
		{Path: "b", Dependencies: []*discovery.DiscoveredConfig{}},
	}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	entries := q.Entries

	// Should be sorted alphabetically at front since none have dependencies
	assert.Equal(t, "a", entries[0].Config.Path)
	assert.Equal(t, "b", entries[1].Config.Path)
	assert.Equal(t, "c", entries[2].Config.Path)
}

func TestDependenciesOrderedByDependencyLevel(t *testing.T) {
	t.Parallel()

	// Create configs with dependencies - should order by dependency level
	aCfg := &discovery.DiscoveredConfig{Path: "a", Dependencies: []*discovery.DiscoveredConfig{}}
	bCfg := &discovery.DiscoveredConfig{Path: "b", Dependencies: []*discovery.DiscoveredConfig{aCfg}}
	cCfg := &discovery.DiscoveredConfig{Path: "c", Dependencies: []*discovery.DiscoveredConfig{bCfg}}

	configs := []*discovery.DiscoveredConfig{aCfg, bCfg, cCfg}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	entries := q.Entries

	// 'a' has no deps so should be at front
	// 'b' depends on 'a' so should be after
	// 'c' depends on 'b' so should be at back
	assert.Equal(t, "a", entries[0].Config.Path)
	assert.Equal(t, "b", entries[1].Config.Path)
	assert.Equal(t, "c", entries[2].Config.Path)
}

func TestComplexDagOrderedByDependencyLevelAndAlphabetically(t *testing.T) {
	t.Parallel()

	// Create a more complex dependency graph:
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

	entries := q.Entries

	// Verify ordering by dependency level and alphabetically within levels:
	// Level 0 (no deps): A, B
	// Level 1 (depends on level 0): C, D
	// Level 2 (depends on level 1): E, F
	assert.Equal(t, "A", entries[0].Config.Path)
	assert.Equal(t, "B", entries[1].Config.Path)
	assert.Equal(t, "C", entries[2].Config.Path)
	assert.Equal(t, "D", entries[3].Config.Path)
	assert.Equal(t, "E", entries[4].Config.Path)
	assert.Equal(t, "F", entries[5].Config.Path)

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
}

func TestDeterministicOrderingOfParallelDependencies(t *testing.T) {
	t.Parallel()

	// Create a graph with parallel dependencies that could be ordered multiple ways:
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

		entries := q.Entries

		// A should be first (no deps)
		assert.Equal(t, "A", entries[0].Config.Path)

		// B, C, D should maintain alphabetical order since they're all at the same level
		assert.Equal(t, "B", entries[1].Config.Path)
		assert.Equal(t, "C", entries[2].Config.Path)
		assert.Equal(t, "D", entries[3].Config.Path)
	}
}

func TestDepthBasedOrderingVerification(t *testing.T) {
	t.Parallel()

	// Create a graph where depth matters:
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

	entries := q.Entries

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
}

func TestErrorHandlingCycle(t *testing.T) {
	t.Parallel()

	// Create a cycle: A -> B -> C -> A
	// Create a cycle: A -> B -> C -> A
	configs := []*discovery.DiscoveredConfig{
		{Path: "C", Dependencies: []*discovery.DiscoveredConfig{{Path: "B"}}},
		{Path: "B", Dependencies: []*discovery.DiscoveredConfig{{Path: "A"}}},
		{Path: "A", Dependencies: []*discovery.DiscoveredConfig{{Path: "C"}}},
	}

	q, err := queue.NewQueue(configs)
	require.Error(t, err)
	assert.NotNil(t, q)
}

func TestErrorHandlingEmptyConfigList(t *testing.T) {
	t.Parallel()

	// Create an empty config list
	configs := []*discovery.DiscoveredConfig{}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)
	assert.Empty(t, q.Entries)
}

// findIndex returns the index of the config with the given path in the slice
func findIndex(entries queue.Entries, path string) int {
	for i, cfg := range entries {
		if cfg.Config.Path == path {
			return i
		}
	}
	return -1
}

func TestQueue_LinearDependencyExecution(t *testing.T) {
	t.Parallel()
	// A -> B -> C
	cfgA := &discovery.DiscoveredConfig{Path: "A"}
	cfgB := &discovery.DiscoveredConfig{Path: "B", Dependencies: []*discovery.DiscoveredConfig{cfgA}}
	cfgC := &discovery.DiscoveredConfig{Path: "C", Dependencies: []*discovery.DiscoveredConfig{cfgB}}
	configs := []*discovery.DiscoveredConfig{cfgA, cfgB, cfgC}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	// Initially, not all are terminal
	assert.False(t, q.Finished(), "Finished should be false at start")

	// Check that all entries are ready initially and in order A, B, C
	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Initially only A should be ready")
	assert.Equal(t, queue.StatusReady, readyEntries[0].Status, "Entry %s should have StatusReady", readyEntries[0].Config.Path)
	assert.Equal(t, "A", readyEntries[0].Config.Path, "First ready entry should be A")

	// Mark A as running and complete it
	entryA := readyEntries[0]
	entryA.Status = queue.StatusSucceeded

	assert.False(t, q.Finished(), "Finished should be false after A is done")

	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "After A is done, only B should be ready")
	assert.Equal(t, "B", readyEntries[0].Config.Path, "Second ready entry should be B")

	// Mark B as running and complete it
	entryB := readyEntries[0]
	entryB.Status = queue.StatusSucceeded

	assert.False(t, q.Finished(), "Finished should be false after B is done")

	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "After B is done, only C should be ready")
	assert.Equal(t, "C", readyEntries[0].Config.Path, "Third ready entry should be C")

	// Mark C as running and complete it
	entryC := readyEntries[0]
	entryC.Status = queue.StatusSucceeded

	// Now all should be terminal
	assert.True(t, q.Finished(), "Finished should be true after all succeeded")

	readyEntries = q.GetReadyWithDependencies()
	assert.Empty(t, readyEntries, "After C is done, no entries should be ready")
}

func TestQueue_ParallelExecution(t *testing.T) {
	t.Parallel()
	//   A
	//  / \
	// B   C
	cfgA := &discovery.DiscoveredConfig{Path: "A"}
	cfgB := &discovery.DiscoveredConfig{Path: "B", Dependencies: []*discovery.DiscoveredConfig{cfgA}}
	cfgC := &discovery.DiscoveredConfig{Path: "C", Dependencies: []*discovery.DiscoveredConfig{cfgA}}
	configs := []*discovery.DiscoveredConfig{cfgA, cfgB, cfgC}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	// 1. Initially, only A should be ready
	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Initially only A should be ready")
	assert.Equal(t, queue.StatusReady, readyEntries[0].Status, "Entry %s should have StatusReady", readyEntries[0].Config.Path)
	assert.Equal(t, "A", readyEntries[0].Config.Path, "First ready entry should be A")

	// Mark A as running and complete it
	entryA := readyEntries[0]
	entryA.Status = queue.StatusSucceeded

	// 2. After A is done, both B and C should be ready (order doesn't matter)
	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 2, "After A is done, B and C should be ready")
	paths := []string{readyEntries[0].Config.Path, readyEntries[1].Config.Path}
	assert.Contains(t, paths, "B")
	assert.Contains(t, paths, "C")
	for _, entry := range readyEntries {
		assert.Equal(t, queue.StatusReady, entry.Status, "Entry %s should have StatusReady", entry.Config.Path)
	}

	// Mark B as running and complete it
	var entryB, entryC *queue.Entry
	for _, entry := range readyEntries {
		if entry.Config.Path == "B" {
			entryB = entry
		}
		if entry.Config.Path == "C" {
			entryC = entry
		}
	}
	entryB.Status = queue.StatusSucceeded

	// After B is done, C should still be ready (if not already marked)
	readyEntries = q.GetReadyWithDependencies()
	if entryC.Status != queue.StatusSucceeded {
		assert.Len(t, readyEntries, 1, "After B is done, C should still be ready")
		assert.Equal(t, "C", readyEntries[0].Config.Path)
		entryC.Status = queue.StatusSucceeded
	}

	// After C is done, nothing should be ready
	readyEntries = q.GetReadyWithDependencies()
	assert.Empty(t, readyEntries, "After B and C are done, no entries should be ready")
}

func TestQueue_FailFast(t *testing.T) {
	t.Parallel()
	//   A
	//  / \
	// B   C
	cfgA := &discovery.DiscoveredConfig{Path: "A"}
	cfgB := &discovery.DiscoveredConfig{Path: "B", Dependencies: []*discovery.DiscoveredConfig{cfgA}}
	cfgC := &discovery.DiscoveredConfig{Path: "C", Dependencies: []*discovery.DiscoveredConfig{cfgA}}
	configs := []*discovery.DiscoveredConfig{cfgA, cfgB, cfgC}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)
	q.FailFast = true

	assert.False(t, q.Finished(), "Finished should be false at start")

	// Simulate A failing
	var entryA *queue.Entry
	for _, entry := range q.Entries {
		if entry.Config.Path == "A" {
			entryA = entry
			break
		}
	}
	require.NotNil(t, entryA, "Entry A should exist")
	entryA.Status = queue.StatusRunning
	q.FailEntry(entryA)

	// B and C should be marked as early exit due to fail-fast
	for _, entry := range q.Entries {
		switch entry.Config.Path {
		case "A":
			assert.Equal(t, queue.StatusFailed, entry.Status, "Entry %s should have StatusFailed", entry.Config.Path)
		case "B", "C":
			assert.Equal(t, queue.StatusEarlyExit, entry.Status, "Entry %s should have StatusEarlyExit", entry.Config.Path)
		}
	}

	// All entries should be listed as terminal (A: Failed, B/C: EarlyExit)
	for _, entry := range q.Entries {
		assert.True(t, entry.Status == queue.StatusFailed || entry.Status == queue.StatusEarlyExit, "Entry %s should be terminal", entry.Config.Path)
	}

	// Now all should be terminal
	assert.True(t, q.Finished(), "Finished should be true after fail-fast triggers")

	// No entries should be ready after fail-fast
	readyEntries := q.GetReadyWithDependencies()
	assert.Empty(t, readyEntries, "No entries should be ready after fail-fast triggers")
}

// buildMultiLevelDependencyTree returns the configs for the following dependency tree:
//
//	    A
//	   / \
//	  B   C
//	 / \
//	D   E
func buildMultiLevelDependencyTree() []*discovery.DiscoveredConfig {
	cfgA := &discovery.DiscoveredConfig{Path: "A"}
	cfgB := &discovery.DiscoveredConfig{Path: "B", Dependencies: []*discovery.DiscoveredConfig{cfgA}}
	cfgC := &discovery.DiscoveredConfig{Path: "C", Dependencies: []*discovery.DiscoveredConfig{cfgA}}
	cfgD := &discovery.DiscoveredConfig{Path: "D", Dependencies: []*discovery.DiscoveredConfig{cfgB}}
	cfgE := &discovery.DiscoveredConfig{Path: "E", Dependencies: []*discovery.DiscoveredConfig{cfgB}}
	configs := []*discovery.DiscoveredConfig{cfgA, cfgB, cfgC, cfgD, cfgE}
	return configs
}

func TestQueue_AdvancedDependencyOrder(t *testing.T) {
	t.Parallel()
	configs := buildMultiLevelDependencyTree()

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	// 1. Initially, only A should be ready
	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Initially only A should be ready")
	assert.Equal(t, "A", readyEntries[0].Config.Path)

	// Mark A as succeeded
	entryA := readyEntries[0]
	entryA.Status = queue.StatusSucceeded

	// 2. After A, B and C should be ready
	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 2, "After A, B and C should be ready")
	paths := []string{readyEntries[0].Config.Path, readyEntries[1].Config.Path}
	assert.Contains(t, paths, "B")
	assert.Contains(t, paths, "C")

	// Mark B as succeeded
	var entryB, entryC *queue.Entry
	for _, entry := range readyEntries {
		if entry.Config.Path == "B" {
			entryB = entry
		}
		if entry.Config.Path == "C" {
			entryC = entry
		}
	}
	entryB.Status = queue.StatusSucceeded

	// 3. After B is done, C should still be ready (if not already marked), and D and E should be ready
	readyEntries = q.GetReadyWithDependencies()
	readyPaths := map[string]bool{}
	for _, entry := range readyEntries {
		readyPaths[entry.Config.Path] = true
	}
	// C may still be ready if not yet marked as succeeded
	assert.Contains(t, readyPaths, "C")
	assert.Contains(t, readyPaths, "D")
	assert.Contains(t, readyPaths, "E")
	assert.Len(t, readyEntries, 3, "After B is done, C, D, and E should be ready")

	// Mark C as succeeded
	entryC.Status = queue.StatusSucceeded

	// Mark D and E as succeeded
	var entryD, entryE *queue.Entry
	for _, entry := range readyEntries {
		if entry.Config.Path == "D" {
			entryD = entry
		}
		if entry.Config.Path == "E" {
			entryE = entry
		}
	}
	entryD.Status = queue.StatusSucceeded
	entryE.Status = queue.StatusSucceeded

	// 4. After all are done, nothing should be ready
	readyEntries = q.GetReadyWithDependencies()
	assert.Empty(t, readyEntries, "After all are done, no entries should be ready")
}

func TestQueue_AdvancedDependency_BFails(t *testing.T) {
	t.Parallel()
	configs := buildMultiLevelDependencyTree()

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)
	q.FailFast = true

	// 1. Initially, only A should be ready
	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Initially only A should be ready")
	assert.Equal(t, "A", readyEntries[0].Config.Path)

	// Mark A as succeeded
	entryA := readyEntries[0]
	entryA.Status = queue.StatusSucceeded

	// 2. After A, B and C should be ready
	readyEntries = q.GetReadyWithDependencies()
	var entryB, entryC *queue.Entry
	for _, entry := range readyEntries {
		if entry.Config.Path == "B" {
			entryB = entry
		}
		if entry.Config.Path == "C" {
			entryC = entry
		}
	}
	assert.NotNil(t, entryB)
	assert.NotNil(t, entryC)

	// Mark B as failed
	entryB.Status = queue.StatusRunning
	q.FailEntry(entryB)

	// Fail fast should mark all not-yet-started tasks as early exit
	assert.Equal(t, queue.StatusFailed, q.EntryByPath("B").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("D").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("E").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("C").Status)

	readyEntries = q.GetReadyWithDependencies()
	assert.Empty(t, readyEntries, "All entries should be terminal")
}

func TestQueue_AdvancedDependency_BFails_NoFailFast(t *testing.T) {
	t.Parallel()
	configs := buildMultiLevelDependencyTree()

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)
	q.FailFast = false

	assert.False(t, q.Finished(), "Finished should be false at start")

	// 1. Initially, only A should be ready
	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Initially only A should be ready")
	assert.Equal(t, "A", readyEntries[0].Config.Path)

	// Mark A as succeeded
	entryA := readyEntries[0]
	entryA.Status = queue.StatusSucceeded

	assert.False(t, q.Finished(), "Finished should be false after A is done")

	// 2. After A, B and C should be ready
	readyEntries = q.GetReadyWithDependencies()
	var entryB, entryC *queue.Entry
	for _, entry := range readyEntries {
		if entry.Config.Path == "B" {
			entryB = entry
		}
		if entry.Config.Path == "C" {
			entryC = entry
		}
	}
	assert.NotNil(t, entryB)
	assert.NotNil(t, entryC)

	// Mark B as failed
	entryB.Status = queue.StatusRunning
	q.FailEntry(entryB)

	assert.False(t, q.Finished(), "Finished should be false after B fails if C is not done")

	// D and E should be marked as failed due to dependency on B
	assert.Equal(t, queue.StatusFailed, q.EntryByPath("B").Status)
	assert.Equal(t, queue.StatusFailed, q.EntryByPath("D").Status)
	assert.Equal(t, queue.StatusFailed, q.EntryByPath("E").Status)

	// C should still be ready
	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Only C should be ready after B fails")
	assert.Equal(t, "C", readyEntries[0].Config.Path)

	// Mark C as succeeded
	entryC.Status = queue.StatusSucceeded

	// After C is done, now all should be terminal
	assert.True(t, q.Finished(), "Finished should be true after all entries are terminal")

	// After C is done, nothing should be ready
	readyEntries = q.GetReadyWithDependencies()
	assert.Empty(t, readyEntries, "After C is done, no entries should be ready")
}

func TestQueue_FailFast_SequentialOrder(t *testing.T) {
	t.Parallel()
	// A -> B -> C, where A fails and fail-fast is enabled
	cfgA := &discovery.DiscoveredConfig{Path: "A"}
	cfgB := &discovery.DiscoveredConfig{Path: "B", Dependencies: []*discovery.DiscoveredConfig{cfgA}}
	cfgC := &discovery.DiscoveredConfig{Path: "C", Dependencies: []*discovery.DiscoveredConfig{cfgB}}
	configs := []*discovery.DiscoveredConfig{cfgA, cfgB, cfgC}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)
	q.FailFast = true

	assert.False(t, q.Finished(), "Finished should be false at start")

	// Only A should be ready
	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Initially only A should be ready")
	assert.Equal(t, "A", readyEntries[0].Config.Path)

	// Mark A as running and then failed
	entryA := readyEntries[0]
	entryA.Status = queue.StatusRunning
	q.FailEntry(entryA)

	// After fail-fast, B and C should be early exit, A should be failed
	for _, entry := range q.Entries {
		switch entry.Config.Path {
		case "A":
			assert.Equal(t, queue.StatusFailed, entry.Status, "Entry %s should have StatusFailed", entry.Config.Path)
		case "B", "C":
			assert.Equal(t, queue.StatusEarlyExit, entry.Status, "Entry %s should have StatusEarlyExit", entry.Config.Path)
		}
	}

	// Finished should be true
	assert.True(t, q.Finished(), "Finished should be true after fail-fast triggers")

	// No entries should be ready
	readyEntries = q.GetReadyWithDependencies()
	assert.Empty(t, readyEntries, "No entries should be ready after fail-fast triggers")
}

func TestQueue_IgnoreDependencyOrder_MultiLevel(t *testing.T) {
	t.Parallel()
	configs := buildMultiLevelDependencyTree()

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)
	q.IgnoreDependencyOrder = true

	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 5, "Should be ready all entries")
}
