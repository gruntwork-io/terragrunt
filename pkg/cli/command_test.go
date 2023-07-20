package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestCommandRunFlagParsing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args         []string
		flags        Flags
		expectedArgs []string
		expectedErr  error
	}{
		{
			[]string{"-foo", "one", "two"},
			Flags{
				&BoolFlag{Name: "foo"},
			},
			[]string{"one", "two"},
			nil,
		},
		{
			[]string{"one", "two"},
			nil,
			[]string{"one", "two"},
			nil,
		},
		{
			[]string{"one", "-foo"},
			Flags{
				&BoolFlag{Name: "foo"},
			},
			[]string{"one"},
			nil,
		},
		{
			[]string{"one", "-foo", "two", "-bar", "value"},
			Flags{
				&BoolFlag{Name: "foo"},
				&GenericFlag[string]{Name: "bar"},
			},
			[]string{"one", "two"},
			nil,
		},
		{
			[]string{"one", "-f", "two", "-b", "value"},
			Flags{
				&BoolFlag{Name: "foo", Aliases: []string{"f"}},
				&GenericFlag[string]{Name: "bar", Aliases: []string{"b"}},
			},
			[]string{"one", "two"},
			nil,
		},
		{
			[]string{"one", "-foo", "two", "-bar"},
			Flags{
				&BoolFlag{Name: "foo"},
				&GenericFlag[string]{Name: "bar"},
			},
			nil,
			errors.New("flag needs an argument: -bar"),
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			app := &App{App: &cli.App{Writer: io.Discard}}
			ctx := newContext(context.Background(), app)

			var actualArgs []string
			command := Command{
				Name:        "test-cmd",
				Aliases:     []string{"tc"},
				Usage:       "this is for testing",
				Description: "testing",
				Flags:       testCase.flags,
				Action: func(ctx *Context) error {
					actualArgs = ctx.Args().Slice()
					return nil
				},
			}

			err := command.Run(ctx, testCase.args...)
			if testCase.expectedErr != nil {
				require.EqualError(t, err, testCase.expectedErr.Error(), testCase)
			} else {
				require.NoError(t, err, testCase)
			}

			assert.Equal(t, testCase.expectedArgs, actualArgs, testCase)
		})
	}
}

func TestCommandRunCommandParsing(t *testing.T) {
	t.Parallel()

	var (
		actualArgs []string
		action     = func(ctx *Context) error {
			actualArgs = ctx.Args().Slice()
			return nil
		}
	)

	testCases := []struct {
		args         []string
		command      Command
		expectedArgs []string
		expectedErr  error
	}{
		{
			[]string{"--foo", "cmd-bar", "--bar", "one", "-two"},
			Command{
				Flags:  Flags{&BoolFlag{Name: "foo"}},
				Action: action,
				Subcommands: Commands{
					{
						Name:   "cmd-bar",
						Flags:  Flags{&BoolFlag{Name: "bar"}},
						Action: action,
					},
				},
			},
			[]string{"one", "-two"},
			nil,
		},
		{
			[]string{"--foo", "--bar", "cmd-bar", "one", "-two"},
			Command{
				Flags:  Flags{&BoolFlag{Name: "foo"}},
				Action: action,
				Subcommands: Commands{
					{
						Name:   "cmd-bar",
						Flags:  Flags{&BoolFlag{Name: "bar"}},
						Action: action,
					},
				},
			},
			[]string{"--bar", "cmd-bar", "one", "-two"},
			nil,
		},
		{
			[]string{"--foo", "cmd-bar", "--bar", "value", "one", "-two"},
			Command{
				Flags:  Flags{&BoolFlag{Name: "foo"}},
				Action: action,
				Subcommands: Commands{
					{
						Name:   "cmd-bar",
						Flags:  Flags{&GenericFlag[string]{Name: "bar"}},
						Action: action,
					},
				},
			},
			[]string{"one", "-two"},
			nil,
		},
	}

	for _, testCase := range testCases {
		actualArgs = []string{}

		app := &App{App: &cli.App{Writer: io.Discard}}
		ctx := newContext(context.Background(), app)

		err := testCase.command.Run(ctx, testCase.args...)
		if testCase.expectedErr != nil {
			require.EqualError(t, err, testCase.expectedErr.Error(), testCase)
		} else {
			require.NoError(t, err, testCase)
		}

		assert.Equal(t, testCase.expectedArgs, actualArgs, testCase)
	}
}

func TestCommandHasName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		command  Command
		hasName  string
		expected bool
	}{
		{
			Command{Name: "foo"},
			"bar",
			false,
		},
		{
			Command{Name: "foo", Aliases: []string{"bar"}},
			"bar",
			true,
		},
		{
			Command{Name: "bar"},
			"bar",
			true,
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actual := testCase.command.HasName(testCase.hasName)
			assert.Equal(t, testCase.expected, actual, testCase)
		})
	}
}

func TestCommandNames(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		command  Command
		expected []string
	}{
		{
			Command{Name: "foo"},
			[]string{"foo"},
		},
		{
			Command{Name: "foo", Aliases: []string{"bar", "baz"}},
			[]string{"foo", "bar", "baz"},
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actual := testCase.command.Names()
			assert.Equal(t, testCase.expected, actual, testCase)
		})
	}
}

func TestCommandSubcommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		command       Command
		searchCmdName string
		expected      *Command
	}{
		{
			Command{Name: "foo", Subcommands: Commands{{Name: "bar"}, {Name: "baz"}}},
			"baz",
			&Command{Name: "baz"},
		},
		{
			Command{Name: "foo", Subcommands: Commands{{Name: "bar"}, {Name: "baz"}}},
			"qux",
			nil,
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actual := testCase.command.Subcommand(testCase.searchCmdName)
			assert.Equal(t, testCase.expected, actual, testCase)
		})
	}
}

func TestCommandVisibleSubcommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		command  Command
		expected []*cli.Command
	}{
		{
			Command{Name: "foo", Subcommands: Commands{{Name: "bar"}, {Name: "baz", HelpName: "helpBaz"}}},
			[]*cli.Command{{Name: "bar", HelpName: "bar"}, {Name: "baz", HelpName: "helpBaz"}},
		},
		{
			Command{Name: "foo", Subcommands: Commands{{Name: "bar", Hidden: true}, {Name: "baz"}}},
			[]*cli.Command{{Name: "baz", HelpName: "baz"}},
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actual := testCase.command.VisibleSubcommands()
			assert.Equal(t, testCase.expected, actual, testCase)
		})
	}
}
