package cli_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBlockIterationDependencyStaticToDynamicReplacesBareAddressWithKeys pins the address
// change a dependency goes through when it gains an expansion. The bare address stops
// having outputs, and one keyed address appears per element.
func TestBlockIterationDependencyStaticToDynamicReplacesBareAddressWithKeys(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, dependencyTree(dependencyStatic))

	assert.Equal(t, map[string]any{
		"addresses": []any{"aurora"},
		"aurora_id": "aurora-web",
	}, renderedInputs(t, v))

	switchConfig(t, fsys, config.DefaultTerragruntConfigPath, dependencyForEachSet)

	assert.Equal(t, map[string]any{
		"addresses":   []any{"aurora"},
		"aurora_keys": []any{"api", "web"},
		"ids":         map[string]any{"api": "aurora-api", "web": "aurora-web"},
	}, renderedInputs(t, v))
}

// TestBlockIterationDependencyDynamicToStaticRestoresBareAddress pins that dropping the
// expansion brings the bare address back and takes the keyed ones away.
func TestBlockIterationDependencyDynamicToStaticRestoresBareAddress(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, dependencyTree(dependencyForEachSet))

	assert.Equal(t, []any{"api", "web"}, renderedInputs(t, v)["aurora_keys"])

	switchConfig(t, fsys, config.DefaultTerragruntConfigPath, dependencyStatic)

	assert.Equal(t, map[string]any{
		"addresses": []any{"aurora"},
		"aurora_id": "aurora-web",
	}, renderedInputs(t, v))
}

// TestBlockIterationDependencyForEachMapAddressesByKeyAndResolvesByValue pins that a map
// for_each addresses instances by each.key while the block body reads each.value.
func TestBlockIterationDependencyForEachMapAddressesByKeyAndResolvesByValue(t *testing.T) {
	t.Parallel()

	v, _ := blockIterationVenv(t, dependencyTree(dependencyForEachMap))

	assert.Equal(t, map[string]any{
		"aurora_keys": []any{"api", "web"},
		"ids":         map[string]any{"api": "backend", "web": "frontend"},
	}, renderedInputs(t, v))
}

// TestBlockIterationDependencyCountDeletionShiftsLaterIndices pins the count hazard on the
// addressing side. A count key is a position, so dropping a middle element repoints every
// later address at its successor without any of them being edited.
func TestBlockIterationDependencyCountDeletionShiftsLaterIndices(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, dependencyTree(dependencyCount))

	assert.Equal(t, map[string]any{
		"aurora_keys": []any{"0", "1", "2"},
		"ids":         map[string]any{"0": "web", "1": "api", "2": "edge"},
	}, renderedInputs(t, v))

	switchConfig(t, fsys, config.DefaultTerragruntConfigPath, dependencyCountShrunk)

	assert.Equal(t, map[string]any{
		"aurora_keys": []any{"0", "1"},
		"ids":         map[string]any{"0": "web", "1": "edge"},
	}, renderedInputs(t, v))
}

// TestBlockIterationDependencyForEachGrowthAddsOnlyTheNewKey pins that adding a for_each key
// adds one keyed address and leaves every existing address reaching the instance it did.
func TestBlockIterationDependencyForEachGrowthAddsOnlyTheNewKey(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, dependencyTree(dependencyForEachSet))

	assert.Equal(t, map[string]any{
		"addresses":   []any{"aurora"},
		"aurora_keys": []any{"api", "web"},
		"ids":         map[string]any{"api": "aurora-api", "web": "aurora-web"},
	}, renderedInputs(t, v))

	switchConfig(t, fsys, config.DefaultTerragruntConfigPath, dependencyForEachSetGrown)

	assert.Equal(t, map[string]any{
		"addresses":   []any{"aurora"},
		"aurora_keys": []any{"api", "edge", "web"},
		"ids": map[string]any{
			"api":  "aurora-api",
			"edge": "aurora-edge",
			"web":  "aurora-web",
		},
	}, renderedInputs(t, v))
}

// TestBlockIterationDependencyCountGrowthAppendsWithoutShifting pins that raising a count by
// appending to the list it reads adds the next index and leaves every earlier index reaching
// the instance it did.
func TestBlockIterationDependencyCountGrowthAppendsWithoutShifting(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, dependencyTree(dependencyCountShrunk))

	assert.Equal(t, map[string]any{
		"aurora_keys": []any{"0", "1"},
		"ids":         map[string]any{"0": "web", "1": "edge"},
	}, renderedInputs(t, v))

	switchConfig(t, fsys, config.DefaultTerragruntConfigPath, dependencyCountGrown)

	assert.Equal(t, map[string]any{
		"aurora_keys": []any{"0", "1", "2"},
		"ids":         map[string]any{"0": "web", "1": "edge", "2": "api"},
	}, renderedInputs(t, v))
}

// TestBlockIterationDependencyKeysWithQuotesAndDotsResolveInInputs pins that a for_each key
// holding a quote or a dot addresses its own instance from inputs. The quote is written with
// HCL's escape, and the dot stays inside the key.
func TestBlockIterationDependencyKeysWithQuotesAndDotsResolveInInputs(t *testing.T) {
	t.Parallel()

	v, _ := blockIterationVenv(t, dependencyTree(dependencyQuotedKeys))

	assert.Equal(t, map[string]any{
		"aurora_keys": []any{`a"b`, "c.d"},
		"dotted_id":   "c.d",
		"quoted_id":   `a"b`,
	}, renderedInputs(t, v))
}

// TestBlockIterationDependencyEnabledFalseKeepsTheBareAddress pins addressing parity. A
// disabled block keeps the bare address it always had, while an expansion block retires
// that address and puts one per element in its place.
func TestBlockIterationDependencyEnabledFalseKeepsTheBareAddress(t *testing.T) {
	t.Parallel()

	v, _ := blockIterationVenv(t, parityTree())

	assert.Equal(t, map[string]any{
		"addresses":   []any{"aurora", "vpc"},
		"aurora_keys": []any{"api", "web"},
		"vpc_id":      "vpc-id",
		"web_id":      "aurora-web-id",
	}, renderedInputs(t, v))
}

// TestBlockIterationDependencyEnabledFalseDropsOutOfTheRunGraph pins addressing parity in
// the run graph. Nothing waits on a disabled dependency, while the expanded block contributes
// one edge per element.
func TestBlockIterationDependencyEnabledFalseDropsOutOfTheRunGraph(t *testing.T) {
	t.Parallel()

	v, _ := blockIterationVenv(t, parityTree())

	stdout, err := runCLI(
		t, v, "find", "--dependencies", "--json", "--no-color", "--working-dir", blockIterationRoot,
	)
	require.NoError(t, err)

	assert.ElementsMatch(t, []foundUnit{
		{Type: "unit", Path: "aurora-api"},
		{Type: "unit", Path: "aurora-web"},
		{Type: "unit", Path: "live", Dependencies: []string{"aurora-api", "aurora-web"}},
		{Type: "unit", Path: "vpc"},
	}, decodeFoundUnits(t, stdout))
}

// TestBlockIterationDependencyFanOutResolvesEveryInstanceWithRacing pins that a block
// expanding into many instances resolves each of them exactly once. Terragrunt resolves the
// instances concurrently into one shared address map, so a slip there shows up as a lost,
// doubled, or crossed instance with no error to point at it.
func TestBlockIterationDependencyFanOutResolvesEveryInstanceWithRacing(t *testing.T) {
	t.Parallel()

	v, _ := blockIterationVenv(t, dependencyTree(dependencyFanOut))

	const instances = 24

	want := make(map[string]any, instances)
	for index := range instances {
		want[strconv.Itoa(index)] = "instance-" + strconv.Itoa(index)
	}

	assert.Equal(t, want, renderedInputs(t, v)["ids"])
}

// parityTree holds the parity configuration as the live unit, with the vpc unit it disables
// and the two aurora units it expands over beside it.
func parityTree() tree {
	return tree{}.
		file(filepath.Join("live", config.DefaultTerragruntConfigPath), dependencyParity).
		config("vpc", "").
		config("aurora-api", "").
		config("aurora-web", "")
}

type foundUnit struct {
	Type         string   `json:"type"`
	Path         string   `json:"path"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// decodeFoundUnits decodes find's JSON output with each unit's dependencies sorted, so a
// comparison pins membership alone.
func decodeFoundUnits(t *testing.T, stdout string) []foundUnit {
	t.Helper()

	var units []foundUnit
	require.NoError(t, json.Unmarshal([]byte(stdout), &units))

	for i := range units {
		slices.Sort(units[i].Dependencies)
	}

	return units
}
