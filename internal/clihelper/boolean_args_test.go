package clihelper_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/stretchr/testify/assert"
)

func TestRemoveFlagWithLongBoolean(t *testing.T) {
	t.Parallel()

	// --json is in booleanFlagsMap (as "json")
	// Removing --json should NOT remove the next argument if it looks like a value but isn't
	// Wait, RemoveFlag removes the flag.
	// If I have "--json", "planfile". RemoveFlag("--json").
	// Should result in "planfile".

	// If I have "--json", "-no-color". RemoveFlag("--json").
	// Should result in "-no-color".

	// The issue described was: "RemoveFlag can wrongly skip a value for --flag booleans not listed in booleanFlags".
	// "If a boolean flag is passed as --long but only -long exists in booleanFlags, it will treat it as non-boolean and remove the next token"

	// Since we switched to booleanFlagsMap with bare names and normalizeFlag, --json should be recognized as boolean.

	args := &clihelper.IacArgs{
		Flags: []string{"--json", "planfile"},
	}

	// We want to remove --json.
	// It should be identified as boolean (bare "json" is in map).
	// So it should NOT consume "planfile" as a value.

	args.RemoveFlag("--json")

	assert.Equal(t, []string{"planfile"}, args.Flags)
}

func TestRemoveFlagWithUnknownFlag(t *testing.T) {
	t.Parallel()

	// Unknown flag --unknown. Not in map.
	// It should be treated as taking a value if the next arg doesn't start with -.

	args := &clihelper.IacArgs{
		Flags: []string{"--unknown", "val", "other"},
	}

	args.RemoveFlag("--unknown")

	assert.Equal(t, []string{"other"}, args.Flags)
}
