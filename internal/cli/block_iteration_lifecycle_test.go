package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"

	"github.com/stretchr/testify/assert"
)

// TestBlockIterationUnitStaticToDynamicOrphansTheBarePath pins the address change a unit
// goes through when it gains an expansion. The generated directory moves from the bare path
// to one per key, and the bare path is left behind with the values it was generated with.
func TestBlockIterationUnitStaticToDynamicOrphansTheBarePath(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, unitTree(unitStatic))

	generateStack(t, v)
	assert.Equal(t, []string{"aurora"}, generatedUnits(t, fsys))

	switchConfig(t, fsys, config.DefaultStackFile, unitForEachSet)
	generateStack(t, v)

	// The orphan nests inside the directory its own replacements generate into, so the bare
	// address survives as a sibling of the keyed ones.
	assert.Equal(t, map[string]string{
		"aurora":     "aurora",
		"aurora/api": "api",
		"aurora/web": "web",
	}, generatedRoles(t, fsys))

	generateStack(t, v, sweepOrphans)
	assert.Equal(t, []string{"aurora/api", "aurora/web"}, generatedUnits(t, fsys))
}

// TestBlockIterationUnitDynamicToStaticOrphansEveryElement pins that dropping a unit's
// expansion orphans every keyed directory the expansion had generated.
func TestBlockIterationUnitDynamicToStaticOrphansEveryElement(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, unitTree(unitForEachSet))

	generateStack(t, v)
	assert.Equal(t, []string{"aurora/api", "aurora/web"}, generatedUnits(t, fsys))

	switchConfig(t, fsys, config.DefaultStackFile, unitStatic)
	generateStack(t, v)

	assert.Equal(t, map[string]string{
		"aurora":     "aurora",
		"aurora/api": "api",
		"aurora/web": "web",
	}, generatedRoles(t, fsys))

	generateStack(t, v, sweepOrphans)
	assert.Equal(t, []string{"aurora"}, generatedUnits(t, fsys))
}

// TestBlockIterationUnitCountDeletionShiftsLaterIndices pins the hazard that separates count
// from for_each. Dropping a middle element renumbers everything after it, so a directory
// that was never edited is rewritten with its successor's values, and the highest index is
// orphaned holding a copy of what now lives one directory down.
func TestBlockIterationUnitCountDeletionShiftsLaterIndices(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, unitTree(unitCount))

	generateStack(t, v)
	assert.Equal(t, map[string]string{
		"aurora/0": "alpha",
		"aurora/1": "beta",
		"aurora/2": "gamma",
	}, generatedRoles(t, fsys))

	switchConfig(t, fsys, config.DefaultStackFile, unitCountShrunk)
	generateStack(t, v)

	assert.Equal(t, map[string]string{
		"aurora/0": "alpha",
		"aurora/1": "gamma",
		"aurora/2": "gamma",
	}, generatedRoles(t, fsys))

	generateStack(t, v, sweepOrphans)
	assert.Equal(t, map[string]string{
		"aurora/0": "alpha",
		"aurora/1": "gamma",
	}, generatedRoles(t, fsys))
}

// TestBlockIterationUnitForEachDeletionOrphansOnlyTheRemovedKey pins that a for_each key
// names its own directory, so removing one leaves the surviving keys where they were.
func TestBlockIterationUnitForEachDeletionOrphansOnlyTheRemovedKey(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, unitTree(unitForEachSet))

	generateStack(t, v)
	assert.Equal(t, map[string]string{
		"aurora/api": "api",
		"aurora/web": "web",
	}, generatedRoles(t, fsys))

	switchConfig(t, fsys, config.DefaultStackFile, unitForEachSetShrunk)
	generateStack(t, v)

	assert.Equal(t, map[string]string{
		"aurora/api": "api",
		"aurora/web": "web",
	}, generatedRoles(t, fsys))

	generateStack(t, v, sweepOrphans)
	assert.Equal(t, []string{"aurora/api"}, generatedUnits(t, fsys))
}

// TestBlockIterationUnitForEachMapResolvesEachValuePerInstance pins that a map for_each keys
// the path by each.key while the body reads each.value.
func TestBlockIterationUnitForEachMapResolvesEachValuePerInstance(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, unitTree(unitForEachMap))

	generateStack(t, v)

	assert.Equal(t, map[string]string{
		"aurora/api": "backend",
		"aurora/web": "frontend",
	}, generatedRoles(t, fsys))
}

// TestBlockIterationUnitDisablingOrphansTheGeneratedPath pins that enabled = false is a
// deletion like any other. The unit stops generating and its directory stays behind.
func TestBlockIterationUnitDisablingOrphansTheGeneratedPath(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, unitTree(unitStatic))

	generateStack(t, v)
	assert.Equal(t, []string{"aurora"}, generatedUnits(t, fsys))

	switchConfig(t, fsys, config.DefaultStackFile, unitDisabled)
	generateStack(t, v)
	assert.Equal(t, []string{"aurora"}, generatedUnits(t, fsys))

	generateStack(t, v, sweepOrphans)
	assert.False(t, vfs.Exists(fsys, filepath.Join(liveDir, generatedStackDir)))
}

// TestBlockIterationStackStaticToDynamicOrphansTheBareTree pins the same address change one
// level up. An expanded stack block generates a whole tree per element, and the tree the
// bare path had generated is orphaned along with the units nested inside it.
func TestBlockIterationStackStaticToDynamicOrphansTheBareTree(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, stackTree(stackStatic))

	generateStack(t, v)
	assert.Equal(t, []string{"team/.terragrunt-stack/member"}, generatedUnits(t, fsys))

	switchConfig(t, fsys, config.DefaultStackFile, stackCount)
	generateStack(t, v)

	assert.Equal(t, []string{
		"team/.terragrunt-stack/member",
		"team/0/.terragrunt-stack/member",
		"team/1/.terragrunt-stack/member",
		"team/2/.terragrunt-stack/member",
	}, generatedUnits(t, fsys))

	generateStack(t, v, sweepOrphans)
	assert.Equal(t, []string{
		"team/0/.terragrunt-stack/member",
		"team/1/.terragrunt-stack/member",
		"team/2/.terragrunt-stack/member",
	}, generatedUnits(t, fsys))
}

// TestBlockIterationStackDynamicToStaticOrphansEveryTree pins that dropping a stack's
// expansion orphans every keyed tree the expansion had generated.
func TestBlockIterationStackDynamicToStaticOrphansEveryTree(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, stackTree(stackCount))

	generateStack(t, v)
	assert.Equal(t, []string{
		"team/0/.terragrunt-stack/member",
		"team/1/.terragrunt-stack/member",
		"team/2/.terragrunt-stack/member",
	}, generatedUnits(t, fsys))

	switchConfig(t, fsys, config.DefaultStackFile, stackStatic)
	generateStack(t, v)

	assert.Equal(t, []string{
		"team/.terragrunt-stack/member",
		"team/0/.terragrunt-stack/member",
		"team/1/.terragrunt-stack/member",
		"team/2/.terragrunt-stack/member",
	}, generatedUnits(t, fsys))

	generateStack(t, v, sweepOrphans)
	assert.Equal(t, []string{"team/.terragrunt-stack/member"}, generatedUnits(t, fsys))
}

// TestBlockIterationStackCountDeletionShiftsLaterIndices pins the index shift one level up.
// Dropping the middle element of a count renames the tree that followed it, so team/1 stops
// being beta and starts being gamma, while the tail tree is orphaned holding the old gamma.
func TestBlockIterationStackCountDeletionShiftsLaterIndices(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, stackTree(stackCount))

	generateStack(t, v)
	assert.Equal(t, map[string]string{
		"team/0/.terragrunt-stack/member": "alpha",
		"team/1/.terragrunt-stack/member": "beta",
		"team/2/.terragrunt-stack/member": "gamma",
	}, generatedRoles(t, fsys))

	switchConfig(t, fsys, config.DefaultStackFile, stackCountShrunk)
	generateStack(t, v)

	assert.Equal(t, map[string]string{
		"team/0/.terragrunt-stack/member": "alpha",
		"team/1/.terragrunt-stack/member": "gamma",
		"team/2/.terragrunt-stack/member": "gamma",
	}, generatedRoles(t, fsys))

	generateStack(t, v, sweepOrphans)
	assert.Equal(t, map[string]string{
		"team/0/.terragrunt-stack/member": "alpha",
		"team/1/.terragrunt-stack/member": "gamma",
	}, generatedRoles(t, fsys))
}

// TestBlockIterationStackForEachGeneratesOneTreePerKey pins that a for_each stack block
// names each tree by its key, so no index can shift under it.
func TestBlockIterationStackForEachGeneratesOneTreePerKey(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, stackTree(stackForEachSet))

	generateStack(t, v)

	assert.Equal(t, []string{
		"team/east/.terragrunt-stack/member",
		"team/west/.terragrunt-stack/member",
	}, generatedUnits(t, fsys))
}

// TestBlockIterationStackForEachDeletionOrphansOnlyTheRemovedKey pins that dropping a key
// orphans that tree and leaves every other one untouched.
func TestBlockIterationStackForEachDeletionOrphansOnlyTheRemovedKey(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, stackTree(stackForEachSet))

	generateStack(t, v)
	assert.Equal(t, map[string]string{
		"team/east/.terragrunt-stack/member": "east",
		"team/west/.terragrunt-stack/member": "west",
	}, generatedRoles(t, fsys))

	switchConfig(t, fsys, config.DefaultStackFile, stackForEachSetShrunk)
	generateStack(t, v)

	assert.Equal(t, map[string]string{
		"team/east/.terragrunt-stack/member": "east",
		"team/west/.terragrunt-stack/member": "west",
	}, generatedRoles(t, fsys))

	generateStack(t, v, sweepOrphans)
	assert.Equal(t, map[string]string{
		"team/east/.terragrunt-stack/member": "east",
	}, generatedRoles(t, fsys))
}

// TestBlockIterationStackForEachMapResolvesEachValuePerInstance pins that a map for_each
// keys the tree by each.key while the body reads each.value.
func TestBlockIterationStackForEachMapResolvesEachValuePerInstance(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, stackTree(stackForEachMap))

	generateStack(t, v)

	assert.Equal(t, map[string]string{
		"team/east/.terragrunt-stack/member": "primary",
		"team/west/.terragrunt-stack/member": "secondary",
	}, generatedRoles(t, fsys))
}

// TestBlockIterationStackDisablingOrphansTheGeneratedTree pins that disabling a stack that
// has already generated leaves its whole tree behind, since generation skips the block
// without cleaning up after it.
func TestBlockIterationStackDisablingOrphansTheGeneratedTree(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, stackTree(stackStatic))

	generateStack(t, v)
	assert.Equal(t, []string{"team/.terragrunt-stack/member"}, generatedUnits(t, fsys))

	switchConfig(t, fsys, config.DefaultStackFile, stackDisabled)
	generateStack(t, v)

	assert.Equal(t, []string{"team/.terragrunt-stack/member"}, generatedUnits(t, fsys))

	generateStack(t, v, sweepOrphans)
	assert.Empty(t, generatedUnits(t, fsys))
}

// TestBlockIterationStackDisablingGeneratesNothing pins that a disabled stack block
// generates no tree at all.
func TestBlockIterationStackDisablingGeneratesNothing(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, stackTree(stackDisabled))

	generateStack(t, v)

	assert.False(t, vfs.Exists(fsys, filepath.Join(liveDir, generatedStackDir)))
}
