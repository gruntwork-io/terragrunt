package cli_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLIFlagHints pins the hint a misplaced flag earns. Each name below
// belongs to a real flag, just not to the command it was passed to, so the
// hint says where it does belong. Each hint is its own type, which is why
// these are three cases rather than one table.
func TestCLIFlagHints(t *testing.T) {
	t.Parallel()

	t.Run("global flag belonging to a command", func(t *testing.T) {
		t.Parallel()

		_, err := runCLI(t, oneUnit(t), "-raw", "init", "--working-dir", unitRoot)

		var hint *flags.GlobalFlagHintError

		require.ErrorAs(t, err, &hint)
		assert.Equal(t, flags.NewGlobalFlagHintError("raw", "stack output", "raw"), hint)
	})

	t.Run("flag belonging to a different command", func(t *testing.T) {
		t.Parallel()

		_, err := runCLI(t, oneUnit(t), "run", "--no-include-root", "--working-dir", unitRoot)

		var hint *flags.CommandFlagHintError

		require.ErrorAs(t, err, &hint)
		assert.Equal(
			t,
			flags.NewCommandFlagHintError("run", "no-include-root", "catalog", "no-include-root"),
			hint,
		)
	})

	t.Run("flag belonging to the wrapped binary", func(t *testing.T) {
		t.Parallel()

		_, err := runCLI(t, oneUnit(t), "run", "--platform", "--working-dir", unitRoot)

		var hint *flags.PassthroughFlagHintError

		require.ErrorAs(t, err, &hint)
		assert.Equal(t, flags.NewPassthroughFlagHintError("platform"), hint)
	})
}

// TestUsingAllAndGraphFlagsSimultaneously pins the refusal to run the queue
// two ways at once.
func TestUsingAllAndGraphFlagsSimultaneously(t *testing.T) {
	t.Parallel()

	_, err := runCLI(t, venvtest.New(), "run", "--graph", "--all")

	expectedErr := new(shared.AllGraphFlagsError)
	require.ErrorAs(t, err, &expectedErr)
}

// TestShowErrorWhenRunAllInvokedWithoutArguments pins that `run --all` with no
// command names what is missing. An empty queue would report success.
func TestShowErrorWhenRunAllInvokedWithoutArguments(t *testing.T) {
	t.Parallel()

	_, err := runCLI(t, oneUnit(t), "run", "--all", "--non-interactive", "--working-dir", unitRoot)

	var missingCommandError runall.MissingCommand

	require.ErrorAs(t, err, &missingCommandError)
}

// TestNoDefaultForwardingUnknownCommand pins that an unrecognized command is
// refused rather than handed to the wrapped binary, which would turn a typo
// into a run.
func TestNoDefaultForwardingUnknownCommand(t *testing.T) {
	t.Parallel()

	_, err := runCLI(
		t, oneUnit(t), "workspace", "list", "--non-interactive", "--working-dir", unitRoot,
	)

	var unknownCommandError cli.UnknownCommandError

	require.ErrorAs(t, err, &unknownCommandError)
	assert.Equal(t, cli.UnknownCommandError("workspace"), unknownCommandError)
}

// TestTerragruntRenderJSONHelp pins that --help on a subcommand prints that
// subcommand's usage and its flags.
func TestTerragruntRenderJSONHelp(t *testing.T) {
	t.Parallel()

	out, err := runCLI(t, venvtest.New(), "render", "--help", "--non-interactive")
	require.NoError(t, err)

	assert.Contains(t, out, "terragrunt render")
	assert.Contains(t, out, "--with-metadata")
}
