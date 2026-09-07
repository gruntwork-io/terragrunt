package test_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	testFixtureBlockIterationUnits  = "fixtures/block-iteration-lifecycle/units"
	testFixtureBlockIterationStacks = "fixtures/block-iteration-lifecycle/stacks"

	generatedStackDir = ".terragrunt-stack"
	generatedValues   = "terragrunt.values.hcl"

	// sweepOrphans deletes the generated tree before regenerating it, which is the only
	// supported way to clear what a transition orphaned.
	sweepOrphans = "--source-update"
)

// copyBlockIterationFixture copies fixture and returns the copy's root. Every variant in the
// tree sits at the same depth, so a config moved between them keeps resolving its sources.
func copyBlockIterationFixture(t *testing.T, fixture string) string {
	t.Helper()

	helpers.CleanupTerraformFolder(t, fixture)

	return filepath.Join(helpers.CopyEnvironment(t, fixture), fixture)
}

// switchStackConfig overwrites live's stack config with variant's, standing in for the edit a
// user makes when a block goes static to dynamic or back.
func switchStackConfig(t *testing.T, root, live, variant string) {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(root, variant, config.DefaultStackFile))
	require.NoError(t, err)

	require.NoError(
		t,
		os.WriteFile(filepath.Join(root, live, config.DefaultStackFile), content, 0o644),
	)
}

func generateStack(t *testing.T, dir string, args ...string) {
	t.Helper()

	helpers.RunTerragrunt(
		t,
		strings.Join(append(
			[]string{"terragrunt stack generate --experiment block-iteration --working-dir", dir},
			args...,
		), " "),
	)
}

// generatedUnits returns the slash-separated path of every generated unit under dir, relative
// to the generated stack directory. Units left behind by an earlier generation are included,
// because naming the whole set is what makes an orphan visible.
//
// A unit is identified by its generated values file, which the copied source tree of a nested
// stack does not carry.
func generatedUnits(t *testing.T, dir string) []string {
	t.Helper()

	stackDir := filepath.Join(dir, generatedStackDir)

	units := []string{}

	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || entry.Name() != generatedValues {
			return nil
		}

		rel, err := filepath.Rel(stackDir, filepath.Dir(path))
		if err != nil {
			return err
		}

		units = append(units, filepath.ToSlash(rel))

		return nil
	}

	require.NoError(t, filepath.WalkDir(stackDir, walk))

	slices.Sort(units)

	return units
}

// generatedRoles returns each generated unit's role value, keyed by the unit's path. The role
// is what a regenerated unit rewrites and an orphaned one keeps, so comparing the map across a
// transition separates the two.
func generatedRoles(t *testing.T, dir string) map[string]string {
	t.Helper()

	roles := map[string]string{}

	for _, unit := range generatedUnits(t, dir) {
		values, err := os.ReadFile(
			filepath.Join(dir, generatedStackDir, filepath.FromSlash(unit), generatedValues),
		)
		require.NoError(t, err)

		_, role, found := strings.Cut(string(values), `role = "`)
		require.Truef(t, found, "unit %s generated no role value", unit)

		role, _, _ = strings.Cut(role, `"`)
		roles[unit] = role
	}

	return roles
}

// TestBlockIterationUnitStaticToDynamicOrphansTheBarePath pins the address change a unit goes
// through when it gains an expansion. The generated directory moves from the bare path to one
// per key, and the bare path is left behind holding the values it was generated with.
func TestBlockIterationUnitStaticToDynamicOrphansTheBarePath(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationUnits)
	live := filepath.Join(root, "static")

	generateStack(t, live)
	assert.Equal(t, []string{"aurora"}, generatedUnits(t, live))

	switchStackConfig(t, root, "static", "for-each-set")
	generateStack(t, live)

	// The orphan nests inside the directory its own replacements generate into, so the bare
	// address survives as a sibling of the keyed ones rather than somewhere out of the way.
	assert.Equal(t, map[string]string{
		"aurora":     "aurora",
		"aurora/api": "api",
		"aurora/web": "web",
	}, generatedRoles(t, live))

	generateStack(t, live, sweepOrphans)
	assert.Equal(t, []string{"aurora/api", "aurora/web"}, generatedUnits(t, live))
}

// TestBlockIterationUnitDynamicToStaticOrphansEveryElement pins the reverse churn. Dropping a
// unit's expansion orphans every keyed directory the expansion had generated.
func TestBlockIterationUnitDynamicToStaticOrphansEveryElement(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationUnits)
	live := filepath.Join(root, "for-each-set")

	generateStack(t, live)
	assert.Equal(t, []string{"aurora/api", "aurora/web"}, generatedUnits(t, live))

	switchStackConfig(t, root, "for-each-set", "static")
	generateStack(t, live)

	assert.Equal(t, map[string]string{
		"aurora":     "aurora",
		"aurora/api": "api",
		"aurora/web": "web",
	}, generatedRoles(t, live))

	generateStack(t, live, sweepOrphans)
	assert.Equal(t, []string{"aurora"}, generatedUnits(t, live))
}

// TestBlockIterationUnitCountDeletionShiftsLaterIndices pins the hazard that separates count
// from for_each. Dropping a middle element renumbers everything after it, so a directory that
// was never meant to change is rewritten with its successor's values, and the highest index is
// orphaned holding a copy of what now lives one directory down.
func TestBlockIterationUnitCountDeletionShiftsLaterIndices(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationUnits)
	live := filepath.Join(root, "count")

	generateStack(t, live)
	assert.Equal(t, map[string]string{
		"aurora/0": "alpha",
		"aurora/1": "beta",
		"aurora/2": "gamma",
	}, generatedRoles(t, live))

	switchStackConfig(t, root, "count", "count-shrunk")
	generateStack(t, live)

	assert.Equal(t, map[string]string{
		"aurora/0": "alpha",
		"aurora/1": "gamma",
		"aurora/2": "gamma",
	}, generatedRoles(t, live))

	generateStack(t, live, sweepOrphans)
	assert.Equal(t, map[string]string{
		"aurora/0": "alpha",
		"aurora/1": "gamma",
	}, generatedRoles(t, live))
}

// TestBlockIterationUnitForEachDeletionOrphansOnlyTheRemovedKey pins the contrast. A for_each
// key names its own directory, so removing one leaves the surviving keys where they were.
func TestBlockIterationUnitForEachDeletionOrphansOnlyTheRemovedKey(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationUnits)
	live := filepath.Join(root, "for-each-set")

	generateStack(t, live)
	assert.Equal(t, map[string]string{
		"aurora/api": "api",
		"aurora/web": "web",
	}, generatedRoles(t, live))

	switchStackConfig(t, root, "for-each-set", "for-each-set-shrunk")
	generateStack(t, live)

	assert.Equal(t, map[string]string{
		"aurora/api": "api",
		"aurora/web": "web",
	}, generatedRoles(t, live))

	generateStack(t, live, sweepOrphans)
	assert.Equal(t, []string{"aurora/api"}, generatedUnits(t, live))
}

// TestBlockIterationUnitForEachMapResolvesEachValuePerInstance pins that a map for_each keys
// the path by each.key while the body reads each.value. The two are not interchangeable.
func TestBlockIterationUnitForEachMapResolvesEachValuePerInstance(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationUnits)
	live := filepath.Join(root, "for-each-map")

	generateStack(t, live)

	assert.Equal(t, map[string]string{
		"aurora/api": "backend",
		"aurora/web": "frontend",
	}, generatedRoles(t, live))
}

// TestBlockIterationUnitDisablingOrphansTheGeneratedPath pins that enabled = false is a
// deletion like any other. The unit stops generating and its directory stays behind.
func TestBlockIterationUnitDisablingOrphansTheGeneratedPath(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationUnits)
	live := filepath.Join(root, "static")

	generateStack(t, live)
	assert.Equal(t, []string{"aurora"}, generatedUnits(t, live))

	switchStackConfig(t, root, "static", "disabled")
	generateStack(t, live)
	assert.Equal(t, []string{"aurora"}, generatedUnits(t, live))

	generateStack(t, live, sweepOrphans)
	assert.NoDirExists(t, filepath.Join(live, generatedStackDir))
}

// TestBlockIterationStackStaticToDynamicOrphansTheBareTree pins the same address change one
// level up. An expanded stack block generates a whole tree per element, and the tree the bare
// path had generated is orphaned along with the units nested inside it.
func TestBlockIterationStackStaticToDynamicOrphansTheBareTree(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationStacks)
	live := filepath.Join(root, "static")

	generateStack(t, live)
	assert.Equal(t, []string{"team/.terragrunt-stack/member"}, generatedUnits(t, live))

	switchStackConfig(t, root, "static", "count")
	generateStack(t, live)

	assert.Equal(t, []string{
		"team/.terragrunt-stack/member",
		"team/0/.terragrunt-stack/member",
		"team/1/.terragrunt-stack/member",
	}, generatedUnits(t, live))

	generateStack(t, live, sweepOrphans)
	assert.Equal(t, []string{
		"team/0/.terragrunt-stack/member",
		"team/1/.terragrunt-stack/member",
	}, generatedUnits(t, live))
}

// TestBlockIterationStackDynamicToStaticOrphansEveryTree pins the reverse churn for stacks.
func TestBlockIterationStackDynamicToStaticOrphansEveryTree(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationStacks)
	live := filepath.Join(root, "count")

	generateStack(t, live)
	assert.Equal(t, []string{
		"team/0/.terragrunt-stack/member",
		"team/1/.terragrunt-stack/member",
	}, generatedUnits(t, live))

	switchStackConfig(t, root, "count", "static")
	generateStack(t, live)

	assert.Equal(t, []string{
		"team/.terragrunt-stack/member",
		"team/0/.terragrunt-stack/member",
		"team/1/.terragrunt-stack/member",
	}, generatedUnits(t, live))

	generateStack(t, live, sweepOrphans)
	assert.Equal(t, []string{"team/.terragrunt-stack/member"}, generatedUnits(t, live))
}

// TestBlockIterationStackCountDeletionOrphansTheDroppedIndex pins that dropping a stack
// element orphans the whole tree it had generated, nested units included.
func TestBlockIterationStackCountDeletionOrphansTheDroppedIndex(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationStacks)
	live := filepath.Join(root, "count")

	generateStack(t, live)

	switchStackConfig(t, root, "count", "count-shrunk")
	generateStack(t, live)

	assert.Equal(t, []string{
		"team/0/.terragrunt-stack/member",
		"team/1/.terragrunt-stack/member",
	}, generatedUnits(t, live))

	generateStack(t, live, sweepOrphans)
	assert.Equal(t, []string{"team/0/.terragrunt-stack/member"}, generatedUnits(t, live))
}

// TestBlockIterationStackForEachGeneratesOneTreePerKey pins a for_each stack block against the
// count case above. The key names the tree, so no index can shift under it.
func TestBlockIterationStackForEachGeneratesOneTreePerKey(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationStacks)
	live := filepath.Join(root, "for-each-set")

	generateStack(t, live)

	assert.Equal(t, []string{
		"team/east/.terragrunt-stack/member",
		"team/west/.terragrunt-stack/member",
	}, generatedUnits(t, live))
}

// TestBlockIterationStackDisablingGeneratesNothing pins that a disabled stack block generates
// no tree at all rather than an empty one.
func TestBlockIterationStackDisablingGeneratesNothing(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationStacks)
	live := filepath.Join(root, "disabled")

	generateStack(t, live)

	assert.NoDirExists(t, filepath.Join(live, generatedStackDir))
}
