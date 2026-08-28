package output_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/stacks/output"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

const nestTestStackDir = "/stack/.terragrunt-stack"

// expandedUnit builds a unit as the for_each decoder would leave it, carrying the
// iteration key that keeps its instances apart.
func expandedUnit(name, key, path string) output.UnitOutput {
	return output.UnitOutput{
		Unit: &config.Unit{
			Name: name,
			Path: path,
			Expansion: &hclparse.ExpansionBlock{
				EachKey: &key,
			},
		},
		Outputs: map[string]cty.Value{"id": cty.StringVal(name + "-" + key)},
		Path:    filepath.Join(nestTestStackDir, path),
	}
}

// countUnit builds a unit as the count decoder would leave it.
func countUnit(name string, index int, path string) output.UnitOutput {
	return output.UnitOutput{
		Unit: &config.Unit{
			Name: name,
			Path: path,
			Expansion: &hclparse.ExpansionBlock{
				CountIndex: &index,
			},
		},
		Outputs: map[string]cty.Value{"id": cty.StringVal(path)},
		Path:    filepath.Join(nestTestStackDir, path),
	}
}

// plainUnit builds a unit that declares no expansion.
func plainUnit(name, path string) output.UnitOutput {
	return output.UnitOutput{
		Unit:    &config.Unit{Name: name, Path: path},
		Outputs: map[string]cty.Value{"id": cty.StringVal(name)},
		Path:    filepath.Join(nestTestStackDir, path),
	}
}

func nest(t *testing.T, stacks map[string][]string, units ...output.UnitOutput) map[string]any {
	t.Helper()

	nested, err := output.NestOutputs(logger.CreateLogger(), stacks, units)
	require.NoError(t, err)

	return nested
}

// noEnclosingStacks is what StackOutput hands over for a stack file whose units all sit
// at the top level: a map with nothing in it, never a nil one.
func noEnclosingStacks() map[string][]string {
	return map[string][]string{}
}

// leaf walks the nested result to the outputs stored at address.
func leaf(t *testing.T, nested map[string]any, address ...string) cty.Value {
	t.Helper()

	current := nested

	for _, segment := range address[:len(address)-1] {
		next, ok := current[segment].(map[string]any)
		require.Truef(t, ok, "no branch at %q", segment)

		current = next
	}

	value, ok := current[address[len(address)-1]].(cty.Value)
	require.Truef(t, ok, "no outputs at %q", address[len(address)-1])

	return value
}

func TestNestOutputsKeysExpandedUnitsByIterationKey(t *testing.T) {
	t.Parallel()

	nested := nest(t, noEnclosingStacks(),
		expandedUnit("shard", "web", "shard/web"),
		expandedUnit("shard", "api", "shard/api"),
		plainUnit("vpc", "vpc"),
	)

	assert.Equal(t, cty.StringVal("shard-web"), leaf(t, nested, "shard", "web").AsValueMap()["id"])
	assert.Equal(t, cty.StringVal("shard-api"), leaf(t, nested, "shard", "api").AsValueMap()["id"])

	// An unexpanded unit keeps its bare address rather than gaining a key segment.
	assert.Equal(t, cty.StringVal("vpc"), leaf(t, nested, "vpc").AsValueMap()["id"])
}

func TestNestOutputsKeysCountUnitsByIndex(t *testing.T) {
	t.Parallel()

	nested := nest(t, noEnclosingStacks(),
		countUnit("shard", 0, "shard/0"),
		countUnit("shard", 1, "shard/1"),
	)

	assert.Equal(t, cty.StringVal("shard/0"), leaf(t, nested, "shard", "0").AsValueMap()["id"])
	assert.Equal(t, cty.StringVal("shard/1"), leaf(t, nested, "shard", "1").AsValueMap()["id"])
}

// TestNestOutputsKeepsDottedKeysWhole pins that an iteration key containing a dot stays
// one level. Joining addresses into a dotted string and splitting it again would bury the
// unit two levels down, out of reach of the address the user would write.
func TestNestOutputsKeepsDottedKeysWhole(t *testing.T) {
	t.Parallel()

	nested := nest(t, noEnclosingStacks(), expandedUnit("shard", "a.b", "shard/a-b"))

	assert.Equal(t, cty.StringVal("shard-a.b"), leaf(t, nested, "shard", "a.b").AsValueMap()["id"])
}

func TestNestOutputsNestsUnitsUnderTheirStacks(t *testing.T) {
	t.Parallel()

	stacks := map[string][]string{
		filepath.Join(nestTestStackDir, "team"): {"team"},
	}

	nested := nest(t, stacks, plainUnit("member", "team/member"))

	assert.Equal(t, cty.StringVal("member"), leaf(t, nested, "team", "member").AsValueMap()["id"])
}

// TestNestOutputsOrdersNestedStacksOutermostFirst pins the segment order when more than
// one stack encloses a unit. Reversing it would address the unit as team.org.member,
// which reads the nesting backwards.
func TestNestOutputsOrdersNestedStacksOutermostFirst(t *testing.T) {
	t.Parallel()

	stacks := map[string][]string{
		filepath.Join(nestTestStackDir, "org"):      {"org"},
		filepath.Join(nestTestStackDir, "org/team"): {"team"},
	}

	nested := nest(t, stacks, plainUnit("member", "org/team/member"))

	assert.Equal(
		t,
		cty.StringVal("member"),
		leaf(t, nested, "org", "team", "member").AsValueMap()["id"],
	)
}

// TestNestOutputsKeysExpandedStacks pins that units inside an expanded stack address
// through the stack's iteration key. Without it every instance of the stack would claim
// the same address and all but one unit would vanish from the output.
func TestNestOutputsKeysExpandedStacks(t *testing.T) {
	t.Parallel()

	stacks := map[string][]string{
		filepath.Join(nestTestStackDir, "team/0"): {"team", "0"},
		filepath.Join(nestTestStackDir, "team/1"): {"team", "1"},
	}

	nested := nest(t, stacks,
		plainUnit("member", "team/0/member"),
		plainUnit("member", "team/1/member"),
	)

	assert.NotNil(t, leaf(t, nested, "team", "0", "member"))
	assert.NotNil(t, leaf(t, nested, "team", "1", "member"))
}

// TestNestOutputsDoesNotConfuseSiblingIndexes pins that a stack generated to team/1 does
// not claim the units under team/10, which a plain substring test on the path would.
func TestNestOutputsDoesNotConfuseSiblingIndexes(t *testing.T) {
	t.Parallel()

	stacks := map[string][]string{
		filepath.Join(nestTestStackDir, "team/1"):  {"team", "1"},
		filepath.Join(nestTestStackDir, "team/10"): {"team", "10"},
	}

	nested := nest(t, stacks, plainUnit("member", "team/10/member"))

	assert.NotNil(t, leaf(t, nested, "team", "10", "member"))

	team, ok := nested["team"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, team, "1", "team/10 must not also nest under team/1")
}

// TestNestOutputsRejectsANilStackMap pins the narrower contract: an empty map is how a
// stack file with no stacks presents, so nil can only mean the caller skipped building
// one. Reading it as "no enclosing stacks" would leave two spellings of the same input.
func TestNestOutputsRejectsANilStackMap(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		_, _ = output.NestOutputs(logger.CreateLogger(), nil, []output.UnitOutput{
			plainUnit("vpc", "vpc"),
		})
	})
}

// TestNestOutputsRejectsCollidingAddresses pins that two units resolving to one address
// fail loudly rather than one silently overwriting the other.
func TestNestOutputsRejectsCollidingAddresses(t *testing.T) {
	t.Parallel()

	stacks := map[string][]string{
		filepath.Join(nestTestStackDir, "team"): {"team"},
	}

	// The unit expanded to team["member"] and the member unit inside the team stack both
	// resolve to team.member.
	_, err := output.NestOutputs(logger.CreateLogger(), stacks,
		[]output.UnitOutput{
			expandedUnit("team", "member", "team-member"),
			plainUnit("member", "team/member"),
		},
	)

	var collision output.UnitAddressCollisionError
	require.ErrorAs(t, err, &collision)
	assert.Equal(t, "team.member", collision.Address)
}
