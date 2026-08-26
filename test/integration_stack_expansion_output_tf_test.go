//go:build tf

package test_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const testFixtureStacksExpansionOutputs = "fixtures/stacks/expansion-outputs"

// applyExpansionOutputStack generates and applies the fixture, returning its root.
func applyExpansionOutputStack(t *testing.T) string {
	t.Helper()

	helpers.CleanupTerraformFolder(t, testFixtureStacksExpansionOutputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksExpansionOutputs)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksExpansionOutputs, "live")

	helpers.RunTerragrunt(
		t,
		"terragrunt stack run apply --experiment block-iteration --non-interactive --working-dir "+
			rootPath+" -- -auto-approve",
	)

	return rootPath
}

func stackOutputAt(t *testing.T, rootPath, address string) string {
	t.Helper()

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack output "+address+
			" --experiment block-iteration --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	return stdout
}

// TestTFStackOutputAddressesExpandedUnitsByKey pins the addressing scheme end to end.
// Every element of an expanded unit is reachable by its key, and a unit that declares no
// expansion still answers to its bare name.
func TestTFStackOutputAddressesExpandedUnitsByKey(t *testing.T) {
	t.Parallel()

	rootPath := applyExpansionOutputStack(t)

	testCases := []struct {
		address string
		want    string
	}{
		{address: `'aurora["web"].role'`, want: `"web"`},
		{address: `'aurora["api"].role'`, want: `"api"`},
		{address: `'shard[0].role'`, want: `"shard-0"`},
		{address: `'shard[1].role'`, want: `"shard-1"`},
		{address: "vpc.role", want: `"vpc"`},
	}

	for _, tc := range testCases {
		t.Run(tc.address, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, stackOutputAt(t, rootPath, tc.address), tc.want)
		})
	}
}

// TestTFStackOutputRawResolvesKeyedAddress pins that --format raw, the path automation
// reads, narrows a keyed address to the bare value.
func TestTFStackOutputRawResolvesKeyedAddress(t *testing.T) {
	t.Parallel()

	rootPath := applyExpansionOutputStack(t)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		`terragrunt stack output 'aurora["web"].role' --format raw`+
			" --experiment block-iteration --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)
	assert.Equal(t, "web", strings.TrimSpace(stdout))
}

// TestTFStackOutputListsEveryExpandedElement pins that an unfiltered run carries every
// element, rather than one instance of a block overwriting its siblings.
func TestTFStackOutputListsEveryExpandedElement(t *testing.T) {
	t.Parallel()

	stdout := stackOutputAt(t, applyExpansionOutputStack(t), "")

	for _, role := range []string{`"web"`, `"api"`, `"shard-0"`, `"shard-1"`, `"vpc"`} {
		assert.Contains(t, stdout, role)
	}
}
