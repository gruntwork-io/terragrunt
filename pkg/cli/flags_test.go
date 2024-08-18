package cli_test

import (
	libflag "flag"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	mockFlagFoo = &cli.GenericFlag[string]{Name: "foo"}
	mockFlagBar = &cli.SliceFlag[string]{Name: "bar"}
	mockFlagBaz = &cli.MapFlag[string, string]{Name: "baz"}

	newMockFlags = func() cli.Flags {
		return cli.Flags{
			mockFlagFoo,
			mockFlagBar,
			mockFlagBaz,
		}
	}
)

func TestFalgsGet(t *testing.T) {
	t.Parallel()

	actual := newMockFlags().Get("bar")
	expected := cli.Flag(mockFlagBar)
	assert.Equal(t, expected, actual)

	actual = newMockFlags().Get("break")
	expected = nil
	assert.Equal(t, expected, actual)
}

func TestFalgsAdd(t *testing.T) {
	t.Parallel()

	testNewFlag := &cli.GenericFlag[string]{Name: "qux"}

	actual := newMockFlags()
	actual.Add(testNewFlag)

	expected := append(newMockFlags(), testNewFlag)
	assert.Equal(t, expected, actual)
}

func TestFalgsFilter(t *testing.T) {
	t.Parallel()

	actual := newMockFlags().Filter([]string{"bar", "baz"})
	expected := cli.Flags{mockFlagBar, mockFlagBaz}
	assert.Equal(t, expected, actual)
}

func TestFalgsRunActions(t *testing.T) {
	t.Parallel()

	var actionHasBeenRun bool

	mockFlags := cli.Flags{
		&cli.SliceFlag[string]{Name: "bar"},
		&cli.GenericFlag[string]{Name: "foo", Action: func(ctx *cli.Context) error { actionHasBeenRun = true; return nil }},
	}

	flagSet := libflag.NewFlagSet("test-cmd", libflag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	for _, flag := range mockFlags {
		err := flag.Apply(flagSet)
		require.NoError(t, err)

		err = flag.Value().Set("value")
		require.NoError(t, err)
	}

	assert.False(t, actionHasBeenRun)

	err := mockFlags.RunActions(nil)
	require.NoError(t, err)

	assert.True(t, actionHasBeenRun)
}
