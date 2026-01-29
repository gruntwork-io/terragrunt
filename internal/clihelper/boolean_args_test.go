package clihelper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveFlagWithLongBoolean(t *testing.T) {
	t.Parallel()

	// --json is in booleanFlagsMap (as "json")
	// Removing --json should NOT remove the next argument if it looks like a value but isn't

	args := &IacArgs{
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

	args := &IacArgs{
		Flags: []string{"--unknown", "val", "other"},
	}

	args.RemoveFlag("--unknown")

	assert.Equal(t, []string{"other"}, args.Flags)
}

func TestHasFlagFalsePositive(t *testing.T) {
	t.Parallel()

	// out=foo should not match -out flag
	args := &IacArgs{
		Flags: []string{"-var", "out=foo"},
	}

	assert.False(t, args.HasFlag("-out"))
}

func TestRemoveFlagFalsePositive(t *testing.T) {
	t.Parallel()

	// Removing -out should not remove out=foo
	args := &IacArgs{
		Flags: []string{"-var", "out=foo"},
	}

	args.RemoveFlag("-out")

	assert.Equal(t, []string{"-var", "out=foo"}, args.Flags)
}

func TestHasFlagDoubleDashMatch(t *testing.T) {
	t.Parallel()

	args := &IacArgs{
		Flags: []string{"--help"},
	}

	assert.True(t, args.HasFlag("-help"))
	assert.True(t, args.HasFlag("--help"))
}

func TestRemoveFlagDoubleDashMatch(t *testing.T) {
	t.Parallel()

	args := &IacArgs{
		Flags: []string{"--help", "planfile"},
	}

	args.RemoveFlag("-help")

	assert.Equal(t, []string{"planfile"}, args.Flags)
}
