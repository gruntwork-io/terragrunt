package clihelper_test

import (
	"context"
	libflag "flag"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	mockFlagFoo = &clihelper.GenericFlag[string]{Name: "foo"}
	mockFlagBar = &clihelper.SliceFlag[string]{Name: "bar"}
	mockFlagBaz = &clihelper.MapFlag[string, string]{Name: "baz"}

	newMockFlags = func() clihelper.Flags {
		return clihelper.Flags{
			mockFlagFoo,
			mockFlagBar,
			mockFlagBaz,
		}
	}
)

func TestFalgsGet(t *testing.T) {
	t.Parallel()

	actual := newMockFlags().Get("bar")
	expected := clihelper.Flag(mockFlagBar)
	assert.Equal(t, expected, actual)

	actual = newMockFlags().Get("break")
	expected = nil
	assert.Equal(t, expected, actual)
}

func TestFalgsAdd(t *testing.T) {
	t.Parallel()

	testNewFlag := &clihelper.GenericFlag[string]{Name: "qux"}

	actual := newMockFlags()
	actual = actual.Add(testNewFlag)

	expected := append(newMockFlags(), testNewFlag)
	assert.Equal(t, expected, actual)
}

func TestFalgsFilter(t *testing.T) {
	t.Parallel()

	actual := newMockFlags().Filter(([]string{"bar", "baz"})...)
	expected := clihelper.Flags{mockFlagBar, mockFlagBaz}
	assert.Equal(t, expected, actual)
}

func TestFalgsRunActions(t *testing.T) {
	t.Parallel()

	var actionHasBeenRun bool

	mockFlags := clihelper.Flags{
		&clihelper.SliceFlag[string]{Name: "bar"},
		&clihelper.GenericFlag[string]{Name: "foo", Action: func(ctx context.Context, cliCtx *clihelper.Context, val string) error {
			actionHasBeenRun = true
			return nil
		}},
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

	err := mockFlags.RunActions(t.Context(), nil)
	require.NoError(t, err)

	assert.True(t, actionHasBeenRun)
}
