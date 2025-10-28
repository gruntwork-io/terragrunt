package queue_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoDependenciesMaintainsAlphabeticalOrder(t *testing.T) {
	t.Parallel()

	// Create configs with no dependencies - should maintain alphabetical order at front
	configs := component.Components{
		component.NewUnit("c"),
		component.NewUnit("a"),
		component.NewUnit("b"),
	}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	entries := q.Entries

	// Should be sorted alphabetically at front since none have dependencies
	assert.Equal(t, "a", entries[0].Component.Path())
	assert.Equal(t, "b", entries[1].Component.Path())
	assert.Equal(t, "c", entries[2].Component.Path())
}

func TestDependenciesOrderedByDependencyLevel(t *testing.T) {
	t.Parallel()

	// Create configs with dependencies - should order by dependency level
	aCfg := component.NewUnit("a")
	bCfg := component.NewUnit("b")
	bCfg.AddDependency(aCfg)

	cCfg := component.NewUnit("c")
	cCfg.AddDependency(bCfg)

	configs := component.Components{aCfg, bCfg, cCfg}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	entries := q.Entries

	// 'a' has no deps so should be at front
	// 'b' depends on 'a' so should be after
	// 'c' depends on 'b' so should be at back
	assert.Equal(t, "a", entries[0].Component.Path())
	assert.Equal(t, "b", entries[1].Component.Path())
	assert.Equal(t, "c", entries[2].Component.Path())
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
	A := component.NewUnit("A")
	B := component.NewUnit("B")
	C := component.NewUnit("C")
	C.AddDependency(A)

	D := component.NewUnit("D")
	D.AddDependency(A)
	D.AddDependency(B)

	E := component.NewUnit("E")
	E.AddDependency(C)

	F := component.NewUnit("F")
	F.AddDependency(C)

	configs := component.Components{F, E, D, C, B, A}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	entries := q.Entries

	// Verify ordering by dependency level and alphabetically within levels:
	// Level 0 (no deps): A, B
	// Level 1 (depends on level 0): C, D
	// Level 2 (depends on level 1): E, F
	assert.Equal(t, "A", entries[0].Component.Path())
	assert.Equal(t, "B", entries[1].Component.Path())
	assert.Equal(t, "C", entries[2].Component.Path())
	assert.Equal(t, "D", entries[3].Component.Path())
	assert.Equal(t, "E", entries[4].Component.Path())
	assert.Equal(t, "F", entries[5].Component.Path())

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
	A := component.NewUnit("A")
	B := component.NewUnit("B")
	B.AddDependency(A)

	C := component.NewUnit("C")
	C.AddDependency(A)

	D := component.NewUnit("D")
	D.AddDependency(A)
	configs := component.Components{D, C, B, A}

	// Run multiple times to verify deterministic ordering
	for range 5 {
		q, err := queue.NewQueue(configs)
		require.NoError(t, err)

		entries := q.Entries

		// A should be first (no deps)
		assert.Equal(t, "A", entries[0].Component.Path())

		// B, C, D should maintain alphabetical order since they're all at the same level
		assert.Equal(t, "B", entries[1].Component.Path())
		assert.Equal(t, "C", entries[2].Component.Path())
		assert.Equal(t, "D", entries[3].Component.Path())
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
	A := component.NewUnit("A")
	B := component.NewUnit("B")
	C := component.NewUnit("C")
	C.AddDependency(A)

	D := component.NewUnit("D")
	D.AddDependency(B)

	E := component.NewUnit("E")
	E.AddDependency(C)
	E.AddDependency(D)
	configs := component.Components{E, D, C, B, A}

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
	A := component.NewUnit("A")
	B := component.NewUnit("B")
	C := component.NewUnit("C")
	C.AddDependency(B)
	B.AddDependency(A)
	A.AddDependency(C) // Creates the cycle
	configs := component.Components{C, B, A}

	q, err := queue.NewQueue(configs)
	require.Error(t, err)
	assert.NotNil(t, q)
}

func TestErrorHandlingEmptyConfigList(t *testing.T) {
	t.Parallel()

	// Create an empty config list
	configs := component.Components{}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)
	assert.Empty(t, q.Entries)
}

// findIndex returns the index of the config with the given path in the slice
func findIndex(entries queue.Entries, path string) int {
	for i, cfg := range entries {
		if cfg.Component.Path() == path {
			return i
		}
	}

	return -1
}

func TestQueue_LinearDependencyExecution(t *testing.T) {
	t.Parallel()
	// A -> B -> C
	cfgA := component.NewUnit("A")
	cfgB := component.NewUnit("B")
	cfgB.AddDependency(cfgA)

	cfgC := component.NewUnit("C")
	cfgC.AddDependency(cfgB)
	configs := component.Components{cfgA, cfgB, cfgC}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	// Initially, not all are terminal
	assert.False(t, q.Finished(), "Finished should be false at start")

	// Check that all entries are ready initially and in order A, B, C
	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Initially only A should be ready")
	assert.Equal(t, queue.StatusReady, readyEntries[0].Status, "Entry %s should have StatusReady", readyEntries[0].Component.Path())
	assert.Equal(t, "A", readyEntries[0].Component.Path(), "First ready entry should be A")

	// Mark A as running and complete it
	entryA := readyEntries[0]
	entryA.Status = queue.StatusSucceeded

	assert.False(t, q.Finished(), "Finished should be false after A is done")

	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "After A is done, only B should be ready")
	assert.Equal(t, "B", readyEntries[0].Component.Path(), "Second ready entry should be B")

	// Mark B as running and complete it
	entryB := readyEntries[0]
	entryB.Status = queue.StatusSucceeded

	assert.False(t, q.Finished(), "Finished should be false after B is done")

	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "After B is done, only C should be ready")
	assert.Equal(t, "C", readyEntries[0].Component.Path(), "Third ready entry should be C")

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
	cfgA := component.NewUnit("A")
	cfgB := component.NewUnit("B")
	cfgB.AddDependency(cfgA)

	cfgC := component.NewUnit("C")
	cfgC.AddDependency(cfgA)
	configs := component.Components{cfgA, cfgB, cfgC}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	// 1. Initially, only A should be ready
	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Initially only A should be ready")
	assert.Equal(t, queue.StatusReady, readyEntries[0].Status, "Entry %s should have StatusReady", readyEntries[0].Component.Path())
	assert.Equal(t, "A", readyEntries[0].Component.Path(), "First ready entry should be A")

	// Mark A as running and complete it
	entryA := readyEntries[0]
	entryA.Status = queue.StatusSucceeded

	// 2. After A is done, both B and C should be ready (order doesn't matter)
	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 2, "After A is done, B and C should be ready")
	paths := []string{readyEntries[0].Component.Path(), readyEntries[1].Component.Path()}
	assert.Contains(t, paths, "B")
	assert.Contains(t, paths, "C")

	for _, entry := range readyEntries {
		assert.Equal(t, queue.StatusReady, entry.Status, "Entry %s should have StatusReady", entry.Component.Path())
	}

	// Mark B as running and complete it
	var entryB, entryC *queue.Entry

	for _, entry := range readyEntries {
		if entry.Component.Path() == "B" {
			entryB = entry
		}

		if entry.Component.Path() == "C" {
			entryC = entry
		}
	}

	entryB.Status = queue.StatusSucceeded

	// After B is done, C should still be ready (if not already marked)
	readyEntries = q.GetReadyWithDependencies()
	if entryC.Status != queue.StatusSucceeded {
		assert.Len(t, readyEntries, 1, "After B is done, C should still be ready")
		assert.Equal(t, "C", readyEntries[0].Component.Path())

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
	cfgA := component.NewUnit("A")
	cfgB := component.NewUnit("B")
	cfgB.AddDependency(cfgA)

	cfgC := component.NewUnit("C")
	cfgC.AddDependency(cfgA)
	configs := component.Components{cfgA, cfgB, cfgC}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	q.FailFast = true

	assert.False(t, q.Finished(), "Finished should be false at start")

	// Simulate A failing
	var entryA *queue.Entry

	for _, entry := range q.Entries {
		if entry.Component.Path() == "A" {
			entryA = entry
			break
		}
	}

	require.NotNil(t, entryA, "Entry A should exist")
	entryA.Status = queue.StatusRunning
	q.FailEntry(entryA)

	// B and C should be marked as early exit due to fail-fast
	for _, entry := range q.Entries {
		switch entry.Component.Path() {
		case "A":
			assert.Equal(t, queue.StatusFailed, entry.Status, "Entry %s should have StatusFailed", entry.Component.Path())
		case "B", "C":
			assert.Equal(t, queue.StatusEarlyExit, entry.Status, "Entry %s should have StatusEarlyExit", entry.Component.Path())
		}
	}

	// All entries should be listed as terminal (A: Failed, B/C: EarlyExit)
	for _, entry := range q.Entries {
		assert.True(t, entry.Status == queue.StatusFailed || entry.Status == queue.StatusEarlyExit, "Entry %s should be terminal", entry.Component.Path())
	}

	// Now all should be terminal
	assert.True(t, q.Finished(), "Finished should be true after fail-fast triggers")

	// No entries should be ready after fail-fast
	readyEntries := q.GetReadyWithDependencies()
	assert.Empty(t, readyEntries, "No entries should be ready after fail-fast triggers")
}

// buildMultiLevelDependencyTree returns the configs for the following dependency tree:
//
//	  A
//	 / \
//	B   C
//
// / \
// D   E
func buildMultiLevelDependencyTree() component.Components {
	cfgA := component.NewUnit("A")
	cfgB := component.NewUnit("B")
	cfgB.AddDependency(cfgA)

	cfgC := component.NewUnit("C")
	cfgC.AddDependency(cfgA)

	cfgD := component.NewUnit("D")
	cfgD.AddDependency(cfgB)

	cfgE := component.NewUnit("E")
	cfgE.AddDependency(cfgB)
	components := component.Components{cfgA, cfgB, cfgC, cfgD, cfgE}

	return components
}

func TestQueue_AdvancedDependencyOrder(t *testing.T) {
	t.Parallel()

	configs := buildMultiLevelDependencyTree()

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	// 1. Initially, only A should be ready
	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Initially only A should be ready")
	assert.Equal(t, "A", readyEntries[0].Component.Path())

	// Mark A as succeeded
	entryA := readyEntries[0]
	entryA.Status = queue.StatusSucceeded

	// 2. After A, B and C should be ready
	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 2, "After A, B and C should be ready")
	paths := []string{readyEntries[0].Component.Path(), readyEntries[1].Component.Path()}
	assert.Contains(t, paths, "B")
	assert.Contains(t, paths, "C")

	// Mark B as succeeded
	var entryB, entryC *queue.Entry

	for _, entry := range readyEntries {
		if entry.Component.Path() == "B" {
			entryB = entry
		}

		if entry.Component.Path() == "C" {
			entryC = entry
		}
	}

	entryB.Status = queue.StatusSucceeded

	// 3. After B is done, C should still be ready (if not already marked), and D and E should be ready
	readyEntries = q.GetReadyWithDependencies()

	readyPaths := map[string]bool{}
	for _, entry := range readyEntries {
		readyPaths[entry.Component.Path()] = true
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
		if entry.Component.Path() == "D" {
			entryD = entry
		}

		if entry.Component.Path() == "E" {
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
	assert.Equal(t, "A", readyEntries[0].Component.Path())

	// Mark A as succeeded
	entryA := readyEntries[0]
	entryA.Status = queue.StatusSucceeded

	// 2. After A, B and C should be ready
	readyEntries = q.GetReadyWithDependencies()

	var entryB, entryC *queue.Entry

	for _, entry := range readyEntries {
		if entry.Component.Path() == "B" {
			entryB = entry
		}

		if entry.Component.Path() == "C" {
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
	assert.Equal(t, "A", readyEntries[0].Component.Path())

	// Mark A as succeeded
	entryA := readyEntries[0]
	entryA.Status = queue.StatusSucceeded

	assert.False(t, q.Finished(), "Finished should be false after A is done")

	// 2. After A, B and C should be ready
	readyEntries = q.GetReadyWithDependencies()

	var entryB, entryC *queue.Entry

	for _, entry := range readyEntries {
		if entry.Component.Path() == "B" {
			entryB = entry
		}

		if entry.Component.Path() == "C" {
			entryC = entry
		}
	}

	assert.NotNil(t, entryB)
	assert.NotNil(t, entryC)

	// Mark B as failed
	entryB.Status = queue.StatusRunning
	q.FailEntry(entryB)

	assert.False(t, q.Finished(), "Finished should be false after B fails if C is not done")

	// D and E should be marked as early exit due to dependency on B
	assert.Equal(t, queue.StatusFailed, q.EntryByPath("B").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("D").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("E").Status)

	// C should still be ready
	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Only C should be ready after B fails")
	assert.Equal(t, "C", readyEntries[0].Component.Path())

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
	cfgA := component.NewUnit("A")
	cfgB := component.NewUnit("B")
	cfgB.AddDependency(cfgA)

	cfgC := component.NewUnit("C")
	cfgC.AddDependency(cfgB)
	configs := component.Components{cfgA, cfgB, cfgC}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	q.FailFast = true

	assert.False(t, q.Finished(), "Finished should be false at start")

	// Only A should be ready
	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Initially only A should be ready")
	assert.Equal(t, "A", readyEntries[0].Component.Path())

	// Mark A as running and then failed
	entryA := readyEntries[0]
	entryA.Status = queue.StatusRunning
	q.FailEntry(entryA)

	// After fail-fast, B and C should be early exit, A should be failed
	for _, entry := range q.Entries {
		switch entry.Component.Path() {
		case "A":
			assert.Equal(t, queue.StatusFailed, entry.Status, "Entry %s should have StatusFailed", entry.Component.Path())
		case "B", "C":
			assert.Equal(t, queue.StatusEarlyExit, entry.Status, "Entry %s should have StatusEarlyExit", entry.Component.Path())
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

func TestFailEntry_DirectAndRecursive(t *testing.T) {
	t.Parallel()
	// Build a graph: A -> B -> C, A -> D
	cfgA := component.NewUnit("A")
	cfgB := component.NewUnit("B")
	cfgB.AddDependency(cfgA)

	cfgC := component.NewUnit("C")
	cfgC.AddDependency(cfgB)

	cfgD := component.NewUnit("D")
	cfgD.AddDependency(cfgA)
	configs := component.Components{cfgA, cfgB, cfgC, cfgD}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	// Non-fail-fast: Should recursively mark all dependencies as StatusEarlyExit
	q.FailFast = false
	entryA := q.EntryByPath("A")
	q.FailEntry(entryA)
	assert.Equal(t, queue.StatusFailed, q.EntryByPath("A").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("B").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("C").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("D").Status)

	// Reset statuses for fail-fast test
	q, err = queue.NewQueue(configs)
	require.NoError(t, err)

	q.FailFast = true
	entryA = q.EntryByPath("A")
	q.FailEntry(entryA)
	assert.Equal(t, queue.StatusFailed, q.EntryByPath("A").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("B").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("C").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("D").Status)
}

func TestQueue_DestroyFail_PropagatesToDependencies_NonFailFast(t *testing.T) {
	t.Parallel()
	// Build a graph: A -> B -> C, A -> D
	cfgA := component.NewUnit("A")
	cfgB := component.NewUnit("B")
	cfgB.AddDependency(cfgA)

	cfgC := component.NewUnit("C")
	cfgC.AddDependency(cfgB)

	cfgD := component.NewUnit("D")
	cfgD.AddDependency(cfgA)
	configs := component.Components{cfgA, cfgB, cfgC, cfgD}

	// Set all configs to destroy (down) command
	for _, cfg := range configs {
		cfg.SetDiscoveryContext(&component.DiscoveryContext{Cmd: "destroy"})
	}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	q.FailFast = false

	// Fail C (should mark B and A as early exit, D should remain ready)
	entryC := q.EntryByPath("C")
	q.FailEntry(entryC)
	assert.Equal(t, queue.StatusFailed, q.EntryByPath("C").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("B").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("A").Status)
	assert.Equal(t, queue.StatusReady, q.EntryByPath("D").Status)
}

func TestQueue_DestroyFail_PropagatesToDependencies(t *testing.T) {
	t.Parallel()
	// Build a graph: A -> B -> C, A -> D
	cfgA := component.NewUnit("A")
	cfgB := component.NewUnit("B")
	cfgB.AddDependency(cfgA)

	cfgC := component.NewUnit("C")
	cfgC.AddDependency(cfgB)

	cfgD := component.NewUnit("D")
	cfgD.AddDependency(cfgA)
	configs := component.Components{cfgA, cfgB, cfgC, cfgD}

	// Set all configs to destroy (down) command
	for _, cfg := range configs {
		cfg.SetDiscoveryContext(&component.DiscoveryContext{Cmd: "destroy"})
	}

	// Only test fail-fast mode here
	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	q.FailFast = true
	entryC := q.EntryByPath("C")
	q.FailEntry(entryC)
	// All non-terminal entries should be early exit
	assert.Equal(t, queue.StatusFailed, q.EntryByPath("C").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("B").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("A").Status)
	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath("D").Status)
}

func TestDestroyCommandQueueOrderIsReverseOfDependencies(t *testing.T) {
	t.Parallel()

	// Create a simple chain: A -> B -> C
	cfgA := component.NewUnit("A")
	cfgB := component.NewUnit("B")
	cfgB.AddDependency(cfgA)

	cfgC := component.NewUnit("C")
	cfgC.AddDependency(cfgB)

	// Set all configs to destroy (down) command
	cfgA.SetDiscoveryContext(&component.DiscoveryContext{Cmd: "destroy"})
	cfgB.SetDiscoveryContext(&component.DiscoveryContext{Cmd: "destroy"})
	cfgC.SetDiscoveryContext(&component.DiscoveryContext{Cmd: "destroy"})

	configs := component.Components{cfgA, cfgB, cfgC}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	entries := q.Entries

	// For destroy, the queue should be in reverse dependency order: C, B, A
	assert.Equal(t, "C", entries[0].Component.Path())
	assert.Equal(t, "B", entries[1].Component.Path())
	assert.Equal(t, "A", entries[2].Component.Path())
}

func TestDestroyCommandQueueOrder_MultiLevelDependencyTree(t *testing.T) {
	t.Parallel()

	configs := buildMultiLevelDependencyTree()
	for _, cfg := range configs {
		cfg.SetDiscoveryContext(&component.DiscoveryContext{Cmd: "destroy"})
	}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	var processed []string

	for {
		ready := q.GetReadyWithDependencies()
		if len(ready) == 0 {
			break
		}

		for _, entry := range ready {
			processed = append(processed, entry.Component.Path())
			entry.Status = queue.StatusSucceeded
		}
	}

	// For destruction, the queue should be in reverse dependency order: E, D, C, B, A
	expected := []string{"C", "D", "E", "B", "A"}
	assert.Equal(t, expected, processed)
}

// TestQueue_DestroyWithIgnoreDependencyErrors_MaintainsOrder tests that when IgnoreDependencyErrors is true,
// the queue still respects dependency order for destroy operations. This is the bug reported in issue #4947.
// When a dependent fails, we should still wait for it to be in a terminal state before destroying the dependency.
func TestQueue_DestroyWithIgnoreDependencyErrors_MaintainsOrder(t *testing.T) {
	t.Parallel()

	// Build a graph: A -> B -> C
	// For destroy, the order should be: C (destroyed first), then B, then A
	cfgA := component.NewUnit("A")
	cfgB := component.NewUnit("B")
	cfgB.AddDependency(cfgA)

	cfgC := component.NewUnit("C")
	cfgC.AddDependency(cfgB)

	// Set all configs to destroy (down) command
	cfgA.SetDiscoveryContext(&component.DiscoveryContext{Cmd: "destroy"})
	cfgB.SetDiscoveryContext(&component.DiscoveryContext{Cmd: "destroy"})
	cfgC.SetDiscoveryContext(&component.DiscoveryContext{Cmd: "destroy"})

	configs := component.Components{cfgA, cfgB, cfgC}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	// Enable IgnoreDependencyErrors - this is the --queue-ignore-errors flag
	q.IgnoreDependencyErrors = true

	// Step 1: Only C should be ready (it has no dependents)
	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Initially only C should be ready for destruction")
	assert.Equal(t, "C", readyEntries[0].Component.Path(), "C should be the first entry ready for destruction")

	// Mark C as succeeded
	entryC := readyEntries[0]
	entryC.Status = queue.StatusSucceeded

	// Step 2: After C is destroyed, B should be ready (but NOT A yet, as A is a dependency of B)
	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "After C is destroyed, only B should be ready")
	assert.Equal(t, "B", readyEntries[0].Component.Path(), "B should be ready after C is destroyed")

	// Mark B as succeeded
	entryB := readyEntries[0]
	entryB.Status = queue.StatusSucceeded

	// Step 3: After B is destroyed, A should be ready
	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "After B is destroyed, only A should be ready")
	assert.Equal(t, "A", readyEntries[0].Component.Path(), "A should be ready last")

	// Mark A as succeeded
	entryA := readyEntries[0]
	entryA.Status = queue.StatusSucceeded

	// Step 4: All entries should be finished
	readyEntries = q.GetReadyWithDependencies()
	assert.Empty(t, readyEntries, "After all are destroyed, no entries should be ready")
	assert.True(t, q.Finished(), "Queue should be finished")
}

// TestQueue_DestroyWithIgnoreDependencyErrors_AllowsProgressAfterFailure tests that when IgnoreDependencyErrors is true
// and a dependent fails, we can still destroy the dependency once the dependent is in a terminal state.
func TestQueue_DestroyWithIgnoreDependencyErrors_AllowsProgressAfterFailure(t *testing.T) {
	t.Parallel()

	// Build a graph: A -> B -> C
	cfgA := component.NewUnit("A")
	cfgB := component.NewUnit("B")
	cfgB.AddDependency(cfgA)

	cfgC := component.NewUnit("C")
	cfgC.AddDependency(cfgB)

	// Set all configs to destroy (down) command
	cfgA.SetDiscoveryContext(&component.DiscoveryContext{Cmd: "destroy"})
	cfgB.SetDiscoveryContext(&component.DiscoveryContext{Cmd: "destroy"})
	cfgC.SetDiscoveryContext(&component.DiscoveryContext{Cmd: "destroy"})

	configs := component.Components{cfgA, cfgB, cfgC}

	q, err := queue.NewQueue(configs)
	require.NoError(t, err)

	q.IgnoreDependencyErrors = true

	// Step 1: Only C should be ready
	readyEntries := q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "Initially only C should be ready")
	assert.Equal(t, "C", readyEntries[0].Component.Path())

	// Mark C as FAILED (simulating a destroy failure)
	entryC := readyEntries[0]
	entryC.Status = queue.StatusRunning
	q.FailEntry(entryC)

	// With IgnoreDependencyErrors = true, B should NOT be marked as early exit
	// Instead, B should still be ready to run
	assert.Equal(t, queue.StatusFailed, q.EntryByPath("C").Status, "C should be failed")
	assert.Equal(t, queue.StatusReady, q.EntryByPath("B").Status, "B should still be ready (not early exit)")

	// Step 2: B should now be ready even though C failed
	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "After C fails, B should still be ready due to IgnoreDependencyErrors")
	assert.Equal(t, "B", readyEntries[0].Component.Path())

	// Mark B as succeeded
	entryB := readyEntries[0]
	entryB.Status = queue.StatusSucceeded

	// Step 3: After B succeeds, A should be ready
	readyEntries = q.GetReadyWithDependencies()
	assert.Len(t, readyEntries, 1, "After B succeeds, A should be ready")
	assert.Equal(t, "A", readyEntries[0].Component.Path())

	// Mark A as succeeded
	entryA := readyEntries[0]
	entryA.Status = queue.StatusSucceeded

	// Queue should be finished
	assert.True(t, q.Finished(), "Queue should be finished")
}
