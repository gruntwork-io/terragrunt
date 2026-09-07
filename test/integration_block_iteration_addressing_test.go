package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	testFixtureBlockIterationDependencies = "fixtures/block-iteration-lifecycle/dependencies/lifecycle"
	testFixtureBlockIterationParity       = "fixtures/block-iteration-lifecycle/dependencies/parity"
)

// switchUnitConfig overwrites live's config with variant's, standing in for the edit a user
// makes when a dependency goes static to dynamic or back.
func switchUnitConfig(t *testing.T, root, live, variant string) {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(root, variant, config.DefaultTerragruntConfigPath))
	require.NoError(t, err)

	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(root, live, config.DefaultTerragruntConfigPath),
			content,
			0o644,
		),
	)
}

// experimentFlag says whether a command opts into block-iteration. Naming it at the call site
// keeps a test that deliberately leaves the gate closed from reading like one that forgot.
type experimentFlag string

const (
	gateOpen   experimentFlag = "--experiment block-iteration"
	gateClosed experimentFlag = ""
)

// renderedInputs returns the inputs dir's config resolves to. Inputs are where a dependency
// address becomes observable. A reference that resolves proves the address exists, and its
// value proves which instance it reached.
func renderedInputs(t *testing.T, gate experimentFlag, dir string) map[string]any {
	t.Helper()

	command := []string{"terragrunt render --format json --non-interactive"}
	if gate != gateClosed {
		command = append(command, string(gate))
	}

	command = append(command, "--working-dir", dir)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, strings.Join(command, " "))
	require.NoError(t, err)

	rendered := struct {
		Inputs map[string]any `json:"inputs"`
	}{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &rendered))

	return rendered.Inputs
}

// TestBlockIterationDependencyStaticToDynamicReplacesBareAddressWithKeys pins the address
// change a dependency goes through when it gains an expansion. The bare address stops carrying
// outputs, and one keyed address appears per element.
func TestBlockIterationDependencyStaticToDynamicReplacesBareAddressWithKeys(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationDependencies)
	live := filepath.Join(root, "static")

	assert.Equal(t, map[string]any{
		"addresses": []any{"aurora"},
		"aurora_id": "aurora-web",
	}, renderedInputs(t, gateOpen, live))

	switchUnitConfig(t, root, "static", "for-each-set")

	assert.Equal(t, map[string]any{
		"addresses":   []any{"aurora"},
		"aurora_keys": []any{"api", "web"},
		"ids":         map[string]any{"api": "aurora-api", "web": "aurora-web"},
	}, renderedInputs(t, gateOpen, live))
}

// TestBlockIterationDependencyDynamicToStaticRestoresBareAddress pins the reverse churn:
// dropping the expansion brings the bare address back and takes the keyed ones away.
func TestBlockIterationDependencyDynamicToStaticRestoresBareAddress(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationDependencies)
	live := filepath.Join(root, "for-each-set")

	assert.Equal(t, []any{"api", "web"}, renderedInputs(t, gateOpen, live)["aurora_keys"])

	switchUnitConfig(t, root, "for-each-set", "static")

	assert.Equal(t, map[string]any{
		"addresses": []any{"aurora"},
		"aurora_id": "aurora-web",
	}, renderedInputs(t, gateOpen, live))
}

// TestBlockIterationDependencyForEachMapAddressesByKeyAndResolvesByValue pins that a map
// for_each addresses instances by each.key while the block body reads each.value.
func TestBlockIterationDependencyForEachMapAddressesByKeyAndResolvesByValue(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationDependencies)

	assert.Equal(t, map[string]any{
		"aurora_keys": []any{"api", "web"},
		"ids":         map[string]any{"api": "backend", "web": "frontend"},
	}, renderedInputs(t, gateOpen, filepath.Join(root, "for-each-map")))
}

// TestBlockIterationDependencyCountDeletionShiftsLaterIndices pins the count hazard on the
// addressing side. A count key is a position, so dropping a middle element repoints every
// later address at its successor without any of them being edited.
func TestBlockIterationDependencyCountDeletionShiftsLaterIndices(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationDependencies)
	live := filepath.Join(root, "count")

	assert.Equal(t, map[string]any{
		"aurora_keys": []any{"0", "1", "2"},
		"ids":         map[string]any{"0": "web", "1": "api", "2": "edge"},
	}, renderedInputs(t, gateOpen, live))

	switchUnitConfig(t, root, "count", "count-shrunk")

	assert.Equal(t, map[string]any{
		"aurora_keys": []any{"0", "1"},
		"ids":         map[string]any{"0": "web", "1": "edge"},
	}, renderedInputs(t, gateOpen, live))
}

// TestBlockIterationDependencyEnabledFalseKeepsTheBareAddress is the addressing-parity case.
// Disabling a dependency and expanding one are not two spellings of the same thing. A disabled
// block keeps the bare address it always had. An expansion block retires that address and puts
// one per element in its place.
func TestBlockIterationDependencyEnabledFalseKeepsTheBareAddress(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationParity)

	assert.Equal(t, map[string]any{
		"addresses":   []any{"aurora", "vpc"},
		"aurora_keys": []any{"api", "web"},
		"vpc_id":      "vpc-id",
		"web_id":      "aurora-web-id",
	}, renderedInputs(t, gateOpen, filepath.Join(root, "app")))
}

// TestBlockIterationDependencyEnabledFalseDropsOutOfTheRunGraph pins the other half of the
// addressing-parity contract. The address a disabled dependency keeps is an address, not an
// edge: nothing waits on it, while the expanded block contributes one edge per element.
func TestBlockIterationDependencyEnabledFalseDropsOutOfTheRunGraph(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationParity)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt find --no-color --dependencies --json --experiment block-iteration"+
			" --working-dir "+root,
	)
	require.NoError(t, err)
	assert.Empty(t, stderr)

	requireJSONEqualIgnoringArrayOrder(t, `[
  {"type":"unit","path":"app","dependencies":["aurora-api","aurora-web"]},
  {"type":"unit","path":"aurora-api"},
  {"type":"unit","path":"aurora-web"},
  {"type":"unit","path":"vpc"}
]`, stdout)
}

// TestBlockIterationDependencyFanOutResolvesEveryInstanceWithRacing pins that a block expanding
// into many instances resolves each of them exactly once. Terragrunt resolves the instances
// concurrently and collects them into one shared address map, so a slip there surfaces as a
// lost, doubled, or crossed instance rather than as a failure.
func TestBlockIterationDependencyFanOutResolvesEveryInstanceWithRacing(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationDependencies)

	const instances = 24

	want := make(map[string]any, instances)
	for index := range instances {
		want[strconv.Itoa(index)] = "instance-" + strconv.Itoa(index)
	}

	assert.Equal(t, want, renderedInputs(t, gateOpen, filepath.Join(root, "fan-out"))["ids"])
}
