package clihelper_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	urfaveCli "github.com/urfave/cli/v2"
)

func TestCommandRun(t *testing.T) {
	t.Parallel()

	type TestActionFunc func(expectedOrder int, expectedArgs []string) clihelper.ActionFunc

	type TestCase struct {
		expectedErr error
		args        []string
		command     clihelper.Command
	}

	testCaseFuncs := []func(action TestActionFunc, skip clihelper.ActionFunc) TestCase{
		func(action TestActionFunc, skip clihelper.ActionFunc) TestCase {
			return TestCase{
				args: []string{"--foo", "--foo", "cmd-bar", "--bar", "one", "-two"},
				command: clihelper.Command{
					Flags:  clihelper.Flags{&clihelper.BoolFlag{Name: "foo"}},
					Before: skip,
					Action: skip,
					After:  skip,
				},
				expectedErr: errors.New("invalid boolean flag foo: setting the flag multiple times"),
			}
		},

		func(action TestActionFunc, skip clihelper.ActionFunc) TestCase {
			return TestCase{
				args: []string{"--foo", "cmd-bar", "--bar", "one", "-two"},
				command: clihelper.Command{
					Flags:  clihelper.Flags{&clihelper.BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: skip,
					After:  action(5, nil),
					Subcommands: clihelper.Commands{
						&clihelper.Command{
							Name:   "cmd-cux",
							Flags:  clihelper.Flags{&clihelper.BoolFlag{Name: "bar"}},
							Before: skip,
							Action: skip,
							After:  skip,
						},
						&clihelper.Command{
							Name:                         "cmd-bar",
							Flags:                        clihelper.Flags{&clihelper.BoolFlag{Name: "bar"}},
							Before:                       action(2, nil),
							Action:                       action(3, []string{"one", "-two"}),
							After:                        action(4, nil),
							DisabledErrorOnUndefinedFlag: true,
						},
					},
				},
			}
		},
		func(action TestActionFunc, skip clihelper.ActionFunc) TestCase {
			return TestCase{
				args: []string{"--foo", "cmd-bar", "--bar", "one", "-two"},
				command: clihelper.Command{
					Flags:  clihelper.Flags{&clihelper.BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: skip,
					After:  action(4, nil),
					Subcommands: clihelper.Commands{
						&clihelper.Command{
							Name:                         "cmd-bar",
							Flags:                        clihelper.Flags{&clihelper.BoolFlag{Name: "bar"}},
							Before:                       action(2, nil),
							After:                        action(3, nil),
							DisabledErrorOnUndefinedFlag: true,
						},
					},
				},
			}
		},
		func(action TestActionFunc, skip clihelper.ActionFunc) TestCase {
			return TestCase{
				args: []string{"--foo", "--bar", "cmd-bar", "one", "-two"},
				command: clihelper.Command{
					Flags:  clihelper.Flags{&clihelper.BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: skip,
					After:  action(5, nil),
					Subcommands: clihelper.Commands{
						&clihelper.Command{
							Name:                         "cmd-bar",
							Flags:                        clihelper.Flags{&clihelper.BoolFlag{Name: "bar"}},
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
		func(action TestActionFunc, skip clihelper.ActionFunc) TestCase {
			return TestCase{
				args: []string{"--foo", "cmd-bar", "--bar", "value", "one", "-two"},
				command: clihelper.Command{
					Flags:  clihelper.Flags{&clihelper.BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: skip,
					After:  action(5, nil),
					Subcommands: clihelper.Commands{
						&clihelper.Command{
							Name:                         "cmd-bar",
							Flags:                        clihelper.Flags{&clihelper.GenericFlag[string]{Name: "bar"}},
							Before:                       action(2, nil),
							Action:                       action(3, []string{"one", "-two"}),
							After:                        action(4, nil),
							DisabledErrorOnUndefinedFlag: true,
						},
					},
				},
			}
		},
		func(action TestActionFunc, skip clihelper.ActionFunc) TestCase {
			return TestCase{
				args: []string{"--foo", "cmd-bar", "--bar", "value", "one", "-two"},
				command: clihelper.Command{
					Flags:  clihelper.Flags{&clihelper.BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: action(2, []string{"cmd-bar", "--bar", "value", "one", "-two"}),
					After:  action(3, nil),
					Subcommands: clihelper.Commands{
						&clihelper.Command{
							Name:        "cmd-bar",
							Flags:       clihelper.Flags{&clihelper.GenericFlag[string]{Name: "bar"}},
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

			action := func(expectedOrder int, expectedArgs []string) clihelper.ActionFunc {
				return func(ctx context.Context, cliCtx *clihelper.Context) error {
					(*actualOrder)++
					assert.Equal(t, expectedOrder, *actualOrder)

					if expectedArgs != nil {
						actualArgs := cliCtx.Args().Slice()
						assert.Equal(t, expectedArgs, actualArgs)
					}

					return nil
				}
			}

			skip := func(ctx context.Context, cliCtx *clihelper.Context) error {
				assert.Fail(t, "this action must be skipped")
				return nil
			}

			tc := tcFn(action, skip)

			app := &clihelper.App{App: &urfaveCli.App{Writer: io.Discard}}
			cliCtx := clihelper.NewAppContext(app, tc.args)

			err := tc.command.Run(t.Context(), cliCtx, tc.args)
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
		command  clihelper.Command
		expected bool
	}{
		{
			command: clihelper.Command{Name: "foo"},
			hasName: "bar",
		},
		{
			command:  clihelper.Command{Name: "foo", Aliases: []string{"bar"}},
			hasName:  "bar",
			expected: true,
		},
		{
			command:  clihelper.Command{Name: "bar"},
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
		command  clihelper.Command
	}{
		{
			command:  clihelper.Command{Name: "foo"},
			expected: []string{"foo"},
		},
		{
			command:  clihelper.Command{Name: "foo", Aliases: []string{"bar", "baz"}},
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
		expected      *clihelper.Command
		searchCmdName string
		command       clihelper.Command
	}{
		{
			command:       clihelper.Command{Name: "foo", Subcommands: clihelper.Commands{&clihelper.Command{Name: "bar"}, &clihelper.Command{Name: "baz"}}},
			searchCmdName: "baz",
			expected:      &clihelper.Command{Name: "baz"},
		},
		{
			command:       clihelper.Command{Name: "foo", Subcommands: clihelper.Commands{&clihelper.Command{Name: "bar"}, &clihelper.Command{Name: "baz"}}},
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
		expected clihelper.Commands
		command  clihelper.Command
	}{
		{
			command:  clihelper.Command{Name: "foo", Subcommands: clihelper.Commands{&clihelper.Command{Name: "bar"}, &clihelper.Command{Name: "baz", HelpName: "helpBaz"}}},
			expected: clihelper.Commands{{Name: "bar", HelpName: "bar"}, {Name: "baz", HelpName: "helpBaz"}},
		},
		{
			command:  clihelper.Command{Name: "foo", Subcommands: clihelper.Commands{&clihelper.Command{Name: "bar", Hidden: true}, &clihelper.Command{Name: "baz"}}},
			expected: clihelper.Commands{{Name: "baz", HelpName: "baz"}},
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
