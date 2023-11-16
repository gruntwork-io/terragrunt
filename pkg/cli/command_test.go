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

func TestCommandRun(t *testing.T) {
	t.Parallel()

	type TestActionFunc func(expectedOrder int, expectedArgs []string) ActionFunc

	type TestCase struct {
		args        []string
		command     Command
		expectedErr error
	}

	testCaseFuncs := []func(action TestActionFunc, skip ActionFunc) TestCase{
		func(action TestActionFunc, skip ActionFunc) TestCase {
			return TestCase{
				[]string{"--foo", "--foo", "cmd-bar", "--bar", "one", "-two"},
				Command{
					Flags:  Flags{&BoolFlag{Name: "foo"}},
					Before: skip,
					Action: skip,
					After:  skip,
				},
				errors.New("invalid boolean flag foo: setting the flag multiple times"),
			}
		},

		func(action TestActionFunc, skip ActionFunc) TestCase {
			return TestCase{
				[]string{"--foo", "cmd-bar", "--bar", "one", "-two"},
				Command{
					Flags:  Flags{&BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: skip,
					After:  action(5, nil),
					Subcommands: Commands{
						{
							Name:   "cmd-cux",
							Flags:  Flags{&BoolFlag{Name: "bar"}},
							Before: skip,
							Action: skip,
							After:  skip,
						},
						{
							Name:   "cmd-bar",
							Flags:  Flags{&BoolFlag{Name: "bar"}},
							Before: action(2, nil),
							Action: action(3, []string{"one", "-two"}),
							After:  action(4, nil),
						},
					},
				},
				nil,
			}
		},
		func(action TestActionFunc, skip ActionFunc) TestCase {
			return TestCase{
				[]string{"--foo", "cmd-bar", "--bar", "one", "-two"},
				Command{
					Flags:  Flags{&BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: skip,
					After:  action(4, nil),
					Subcommands: Commands{
						{
							Name:   "cmd-bar",
							Flags:  Flags{&BoolFlag{Name: "bar"}},
							Before: action(2, nil),
							After:  action(3, nil),
						},
					},
				},
				nil,
			}
		},
		func(action TestActionFunc, skip ActionFunc) TestCase {
			return TestCase{
				[]string{"--foo", "--bar", "cmd-bar", "one", "-two"},
				Command{
					Flags:  Flags{&BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: action(2, []string{"--bar", "cmd-bar", "one", "-two"}),
					After:  action(3, nil),
					Subcommands: Commands{
						{
							Name:   "cmd-bar",
							Flags:  Flags{&BoolFlag{Name: "bar"}},
							Before: skip,
							After:  skip,
							Action: skip,
						},
					},
				},
				nil,
			}
		},
		func(action TestActionFunc, skip ActionFunc) TestCase {
			return TestCase{
				[]string{"--foo", "cmd-bar", "--bar", "value", "one", "-two"},
				Command{
					Flags:  Flags{&BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: skip,
					After:  action(5, nil),
					Subcommands: Commands{
						{
							Name:   "cmd-bar",
							Flags:  Flags{&GenericFlag[string]{Name: "bar"}},
							Before: action(2, nil),
							Action: action(3, []string{"one", "-two"}),
							After:  action(4, nil),
						},
					},
				},
				nil,
			}
		},
		func(action TestActionFunc, skip ActionFunc) TestCase {
			return TestCase{
				[]string{"--foo", "cmd-bar", "--bar", "value", "one", "-two"},
				Command{
					Flags:  Flags{&BoolFlag{Name: "foo"}},
					Before: action(1, nil),
					Action: action(2, []string{"cmd-bar", "--bar", "value", "one", "-two"}),
					After:  action(3, nil),
					Subcommands: Commands{
						{
							Name:        "cmd-bar",
							Flags:       Flags{&GenericFlag[string]{Name: "bar"}},
							SkipRunning: true,
							Before:      skip,
							Action:      skip,
							After:       skip,
						},
					},
				},
				nil,
			}
		},
	}

	for i, testCaseFn := range testCaseFuncs {
		testCaseFn := testCaseFn

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			var actualOrder = new(int)
			action := func(expectedOrder int, expectedArgs []string) ActionFunc {
				return func(ctx *Context) error {
					(*actualOrder)++
					assert.Equal(t, expectedOrder, *actualOrder)

					if expectedArgs != nil {
						actualArgs := ctx.Args().Slice()
						assert.Equal(t, expectedArgs, actualArgs)
					}

					return nil
				}
			}

			skip := func(ctx *Context) error {
				assert.Fail(t, "this action must be skipped")
				return nil
			}

			testCase := testCaseFn(action, skip)

			app := &App{App: &cli.App{Writer: io.Discard}}
			ctx := newContext(context.Background(), app)

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
