package cli_test

import (
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
							Name:                         "cmd-bar",
							Flags:                        cli.Flags{&cli.BoolFlag{Name: "bar"}},
							Before:                       action(2, nil),
							Action:                       action(3, []string{"one", "-two"}),
							After:                        action(4, nil),
							DisabledErrorOnUndefinedFlag: true,
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
							Name:                         "cmd-bar",
							Flags:                        cli.Flags{&cli.BoolFlag{Name: "bar"}},
							Before:                       action(2, nil),
							After:                        action(3, nil),
							DisabledErrorOnUndefinedFlag: true,
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
							Name:                         "cmd-bar",
							Flags:                        cli.Flags{&cli.BoolFlag{Name: "bar"}},
							Before:                       action(2, nil),
							Action:                       action(3, []string{"one", "-two"}),
							After:                        action(4, nil),
							DisabledErrorOnUndefinedFlag: true,
						},
					},
					DisabledErrorOnUndefinedFlag: true,
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
							Name:                         "cmd-bar",
							Flags:                        cli.Flags{&cli.GenericFlag[string]{Name: "bar"}},
							Before:                       action(2, nil),
							Action:                       action(3, []string{"one", "-two"}),
							After:                        action(4, nil),
							DisabledErrorOnUndefinedFlag: true,
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

	for i, tcFn := range testCaseFuncs {
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

			tc := tcFn(action, skip)

			app := &cli.App{App: &urfaveCli.App{Writer: io.Discard}}
			ctx := cli.NewAppContext(t.Context(), app, tc.args)

			err := tc.command.Run(ctx, tc.args)
			if tc.expectedErr != nil {
				require.EqualError(t, err, tc.expectedErr.Error(), tc)
			} else {
				require.NoError(t, err, tc)
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

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actual := tc.command.HasName(tc.hasName)
			assert.Equal(t, tc.expected, actual, tc)
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

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actual := tc.command.Names()
			assert.Equal(t, tc.expected, actual, tc)
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

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actual := tc.command.Subcommand(tc.searchCmdName)
			assert.Equal(t, tc.expected, actual, tc)
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

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actual := tc.command.VisibleSubcommands()
			assert.Equal(t, tc.expected, actual, tc)
		})
	}
}
