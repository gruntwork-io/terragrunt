package flags_test

import (
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/stretchr/testify/assert"
)

func TestErrorHandler(t *testing.T) {
	t.Parallel()

	// Setup commands the error handler will search for flag hints.
	commands := clihelper.Commands{
		{
			Name: "catalog",
			Flags: clihelper.Flags{
				&clihelper.BoolFlag{Name: "no-include-root", Destination: new(bool)},
			},
		},
		{
			Name: "stack",
			Subcommands: clihelper.Commands{
				{
					Name: "output",
					Flags: clihelper.Flags{
						&clihelper.BoolFlag{Name: "raw", Destination: new(bool)},
					},
				},
			},
		},
	}

	handler := flags.ErrorHandler(commands)

	// newRootCtx creates a context at the root (global) level,
	// where ctx.Parent().Command is nil.
	newRootCtx := func() *clihelper.Context {
		app := clihelper.NewApp()
		appCtx := clihelper.NewAppContext(app, nil)
		rootCmd := &clihelper.Command{Name: "terragrunt", IsRoot: true}

		return appCtx.NewCommandContext(rootCmd, nil)
	}

	// newCommandCtx creates a context for a named subcommand,
	// where ctx.Parent().Command is the root command (non-nil).
	newCommandCtx := func(name string) *clihelper.Context {
		app := clihelper.NewApp()
		appCtx := clihelper.NewAppContext(app, nil)
		rootCmd := &clihelper.Command{Name: "terragrunt", IsRoot: true}
		rootCtx := appCtx.NewCommandContext(rootCmd, nil)
		cmd := &clihelper.Command{Name: name}

		return rootCtx.NewCommandContext(cmd, nil)
	}

	// newRunSubcommandCtx creates a context for a subcommand of "run"
	// (e.g., "providers" in "terragrunt run providers lock -platform ...").
	newRunSubcommandCtx := func(name string) *clihelper.Context {
		app := clihelper.NewApp()
		appCtx := clihelper.NewAppContext(app, nil)
		rootCmd := &clihelper.Command{Name: "terragrunt", IsRoot: true}
		rootCtx := appCtx.NewCommandContext(rootCmd, nil)
		runCmd := &clihelper.Command{Name: "run"}
		runCtx := rootCtx.NewCommandContext(runCmd, nil)
		cmd := &clihelper.Command{Name: name}

		return runCtx.NewCommandContext(cmd, nil)
	}

	testCases := []struct {
		ctx           *clihelper.Context
		err           error
		expectedError error
		name          string
	}{
		{
			name:          "non-undefined-flag error passes through unchanged",
			ctx:           newRootCtx(),
			err:           errors.New("some other error"),
			expectedError: errors.New("some other error"),
		},
		{
			name:          "known flag at global level returns GlobalFlagHintError",
			ctx:           newRootCtx(),
			err:           clihelper.UndefinedFlagError("raw"),
			expectedError: flags.NewGlobalFlagHintError("raw", "stack output", "raw"),
		},
		{
			name:          "known flag at command level returns CommandFlagHintError",
			ctx:           newCommandCtx("run"),
			err:           clihelper.UndefinedFlagError("no-include-root"),
			expectedError: flags.NewCommandFlagHintError("run", "no-include-root", "catalog", "no-include-root"),
		},
		{
			name:          "unknown flag on run command returns PassthroughFlagHintError",
			ctx:           newCommandCtx("run"),
			err:           clihelper.UndefinedFlagError("platform"),
			expectedError: flags.NewPassthroughFlagHintError("platform"),
		},
		{
			name:          "unknown flag on run subcommand returns PassthroughFlagHintError",
			ctx:           newRunSubcommandCtx("providers"),
			err:           clihelper.UndefinedFlagError("platform"),
			expectedError: flags.NewPassthroughFlagHintError("platform"),
		},
		{
			name:          "unknown flag on non-run command returns original error",
			ctx:           newCommandCtx("catalog"),
			err:           clihelper.UndefinedFlagError("platform"),
			expectedError: clihelper.UndefinedFlagError("platform"),
		},
		{
			name:          "unknown flag at global level returns original error",
			ctx:           newRootCtx(),
			err:           clihelper.UndefinedFlagError("platform"),
			expectedError: clihelper.UndefinedFlagError("platform"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := handler(tc.ctx, tc.err)
			assert.EqualError(t, result, tc.expectedError.Error())
		})
	}
}
