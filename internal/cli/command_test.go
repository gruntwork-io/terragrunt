package cli_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	urfaveCli "github.com/urfave/cli/v2"
)

func TestCommandRun(t *testing.T) {
	t.Parallel()

	type TestActionFunc func(expectedOrder int, expectedArgs []string) cli.ActionFunc

	type TestCase struct {
		expectedErr error
		args        []string
		command     cli.Command
	}

	testCaseFuncs := []func(action TestActionFunc, skip cli.ActionFunc) TestCase{
		func(action TestActionFunc, skip cli.ActionFunc) TestCase {
			return TestCase{
				args: []string{"--foo", "--foo", "cmd-bar", "--bar", "one", "-two"},
				command: cli.Command{
					Flags:  cli.Flags{&cli.BoolFlag{Name: "foo"}},
					Before: skip,
					Action: skip,
					After:  skip,
				},
				expectedErr: errors.New("invalid boolean flag foo: setting the flag multiple times"),
			}
		},

		func(action TestActionFunc, skip cli.ActionFunc) TestCase {
			return TestCase{
				args: []string{"--foo", "cmd-bar", "--bar", "one", "-two"},
				command: cli.Command{
					Flags:  cli.Flags{&cli.BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: skip,
					After:  action(5, nil),
					Subcommands: cli.Commands{
						&cli.Command{
							Name:   "cmd-cux",
							Flags:  cli.Flags{&cli.BoolFlag{Name: "bar"}},
							Before: skip,
							Action: skip,
							After:  skip,
						},
						&cli.Command{
							Name:   "cmd-bar",
							Flags:  cli.Flags{&cli.BoolFlag{Name: "bar"}},
							Before: action(2, nil),
							Action: action(3, []string{"one", "-two"}),
							After:  action(4, nil),
						},
					},
				},
			}
		},
		func(action TestActionFunc, skip cli.ActionFunc) TestCase {
			return TestCase{
				args: []string{"--foo", "cmd-bar", "--bar", "one", "-two"},
				command: cli.Command{
					Flags:  cli.Flags{&cli.BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: skip,
					After:  action(4, nil),
					Subcommands: cli.Commands{
						&cli.Command{
							Name:   "cmd-bar",
							Flags:  cli.Flags{&cli.BoolFlag{Name: "bar"}},
							Before: action(2, nil),
							After:  action(3, nil),
						},
					},
				},
			}
		},
		func(action TestActionFunc, skip cli.ActionFunc) TestCase {
			return TestCase{
				args: []string{"--foo", "--bar", "cmd-bar", "one", "-two"},
				command: cli.Command{
					Flags:  cli.Flags{&cli.BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: skip,
					After:  action(5, nil),
					Subcommands: cli.Commands{
						&cli.Command{
							Name:   "cmd-bar",
							Flags:  cli.Flags{&cli.BoolFlag{Name: "bar"}},
							Before: action(2, nil),
							Action: action(3, []string{"one", "-two"}),
							After:  action(4, nil),
						},
					},
				},
			}
		},
		func(action TestActionFunc, skip cli.ActionFunc) TestCase {
			return TestCase{
				args: []string{"--foo", "cmd-bar", "--bar", "value", "one", "-two"},
				command: cli.Command{
					Flags:  cli.Flags{&cli.BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: skip,
					After:  action(5, nil),
					Subcommands: cli.Commands{
						&cli.Command{
							Name:   "cmd-bar",
							Flags:  cli.Flags{&cli.GenericFlag[string]{Name: "bar"}},
							Before: action(2, nil),
							Action: action(3, []string{"one", "-two"}),
							After:  action(4, nil),
						},
					},
				},
			}
		},
		func(action TestActionFunc, skip cli.ActionFunc) TestCase {
			return TestCase{
				args: []string{"--foo", "cmd-bar", "--bar", "value", "one", "-two"},
				command: cli.Command{
					Flags:  cli.Flags{&cli.BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: action(2, []string{"cmd-bar", "--bar", "value", "one", "-two"}),
					After:  action(3, nil),
					Subcommands: cli.Commands{
						&cli.Command{
							Name:        "cmd-bar",
							Flags:       cli.Flags{&cli.GenericFlag[string]{Name: "bar"}},
							SkipRunning: true,
							Before:      skip,
							Action:      skip,
							After:       skip,
						},
					},
				},
			}
		},
	}

	for i, testCaseFn := range testCaseFuncs {
		testCaseFn := testCaseFn

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			var actualOrder = new(int)
			action := func(expectedOrder int, expectedArgs []string) cli.ActionFunc {
				return func(ctx *cli.Context) error {
					(*actualOrder)++
					assert.Equal(t, expectedOrder, *actualOrder)

					if expectedArgs != nil {
						actualArgs := ctx.Args().Slice()
						assert.Equal(t, expectedArgs, actualArgs)
					}

					return nil
				}
			}

			skip := func(ctx *cli.Context) error {
				assert.Fail(t, "this action must be skipped")
				return nil
			}

			testCase := testCaseFn(action, skip)

			app := &cli.App{App: &urfaveCli.App{Writer: io.Discard}}
			ctx := cli.NewAppContext(context.Background(), app, testCase.args)

			err := testCase.command.Run(ctx, testCase.args)
			if testCase.expectedErr != nil {
				require.EqualError(t, err, testCase.expectedErr.Error(), testCase)
			} else {
				require.NoError(t, err, testCase)
			}

		})
	}
}

func TestCommandHasName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		hasName  string
		command  cli.Command
		expected bool
	}{
		{
			command: cli.Command{Name: "foo"},
			hasName: "bar",
		},
		{
			command:  cli.Command{Name: "foo", Aliases: []string{"bar"}},
			hasName:  "bar",
			expected: true,
		},
		{
			command:  cli.Command{Name: "bar"},
			hasName:  "bar",
			expected: true,
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
		expected []string
		command  cli.Command
	}{
		{
			command:  cli.Command{Name: "foo"},
			expected: []string{"foo"},
		},
		{
			command:  cli.Command{Name: "foo", Aliases: []string{"bar", "baz"}},
			expected: []string{"foo", "bar", "baz"},
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
		expected      *cli.Command
		searchCmdName string
		command       cli.Command
	}{
		{
			command:       cli.Command{Name: "foo", Subcommands: cli.Commands{&cli.Command{Name: "bar"}, &cli.Command{Name: "baz"}}},
			searchCmdName: "baz",
			expected:      &cli.Command{Name: "baz"},
		},
		{
			command:       cli.Command{Name: "foo", Subcommands: cli.Commands{&cli.Command{Name: "bar"}, &cli.Command{Name: "baz"}}},
			searchCmdName: "qux",
			expected:      nil,
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
		expected cli.Commands
		command  cli.Command
	}{
		{
			command:  cli.Command{Name: "foo", Subcommands: cli.Commands{&cli.Command{Name: "bar"}, &cli.Command{Name: "baz", HelpName: "helpBaz"}}},
			expected: cli.Commands{{Name: "bar", HelpName: "bar"}, {Name: "baz", HelpName: "helpBaz"}},
		},
		{
			command:  cli.Command{Name: "foo", Subcommands: cli.Commands{&cli.Command{Name: "bar", Hidden: true}, &cli.Command{Name: "baz"}}},
			expected: cli.Commands{{Name: "baz", HelpName: "baz"}},
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
