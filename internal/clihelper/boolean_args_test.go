package clihelper_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/stretchr/testify/assert"
)

func TestRemoveFlagWithLongBoolean(t *testing.T) {
	t.Parallel()

	// --json is an unknown flag (not in valueTakingFlags).
	// Unknown flags are treated as boolean, so removing --json
	// should NOT consume "planfile" as a value.

	args := &clihelper.IacArgs{
		Flags: []string{"--json", "planfile"},
	}

	args.RemoveFlag("--json")

	assert.Equal(t, []string{"planfile"}, args.Flags)
}

func TestRemoveFlagWithUnknownFlag(t *testing.T) {
	t.Parallel()

	// Unknown flag --unknown is not in valueTakingFlags.
	// Unknown flags are treated as boolean, so removing --unknown
	// should NOT consume "val" as a value.

	args := &clihelper.IacArgs{
		Flags: []string{"--unknown", "val", "other"},
	}

	args.RemoveFlag("--unknown")

	assert.Equal(t, []string{"val", "other"}, args.Flags)
}

func TestHasFlagFalsePositive(t *testing.T) {
	t.Parallel()

	// out=foo should not match -out flag
	args := &clihelper.IacArgs{
		Flags: []string{"-var", "out=foo"},
	}

	assert.False(t, args.HasFlag("-out"))
}

func TestRemoveFlagFalsePositive(t *testing.T) {
	t.Parallel()

	// Removing -out should not remove out=foo
	args := &clihelper.IacArgs{
		Flags: []string{"-var", "out=foo"},
	}

	args.RemoveFlag("-out")

	assert.Equal(t, []string{"-var", "out=foo"}, args.Flags)
}

func TestHasFlagDoubleDashMatch(t *testing.T) {
	t.Parallel()

	args := &clihelper.IacArgs{
		Flags: []string{"--help"},
	}

	assert.True(t, args.HasFlag("-help"))
	assert.True(t, args.HasFlag("--help"))
}

func TestRemoveFlagDoubleDashMatch(t *testing.T) {
	t.Parallel()

	args := &clihelper.IacArgs{
		Flags: []string{"--help", "planfile"},
	}

	args.RemoveFlag("-help")

	assert.Equal(t, []string{"planfile"}, args.Flags)
}
