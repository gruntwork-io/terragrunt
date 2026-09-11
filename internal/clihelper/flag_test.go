package clihelper_test

import (
	libflag "flag"
	"io"
	"reflect"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFlagEnvVar = "TG_TEST_FLAG_ENV"

func TestFlagIgnoresProcessEnv(t *testing.T) {
	t.Setenv(testFlagEnvVar, "from-process")

	flag := &clihelper.GenericFlag[string]{Name: "foo", EnvVars: []string{testFlagEnvVar}}

	set := libflag.NewFlagSet("test-cmd", libflag.ContinueOnError)
	set.SetOutput(io.Discard)

	require.NoError(t, flag.Apply(set, map[string]string{}))

	assert.False(t, flag.Value().IsEnvSet())
	assert.Empty(t, flag.Value().Get())
}

func TestFlagPrefersSuppliedEnvOverProcessEnv(t *testing.T) {
	t.Setenv(testFlagEnvVar, "from-process")

	flag := &clihelper.GenericFlag[string]{Name: "foo", EnvVars: []string{testFlagEnvVar}}

	set := libflag.NewFlagSet("test-cmd", libflag.ContinueOnError)
	set.SetOutput(io.Discard)

	require.NoError(t, flag.Apply(set, map[string]string{testFlagEnvVar: "from-venv"}))

	assert.Equal(t, "from-venv", flag.Value().Get())
}

func TestApplyFlagPanicsOnUnsetEnv(t *testing.T) {
	t.Parallel()

	flag := &clihelper.GenericFlag[string]{Name: "foo", EnvVars: []string{testFlagEnvVar}}

	set := libflag.NewFlagSet("test-cmd", libflag.ContinueOnError)
	set.SetOutput(io.Discard)

	assert.PanicsWithError(t, clihelper.ErrEnvUnset.Error(), func() {
		require.NoError(t, flag.Apply(set, nil))
	})
}

// TestFlagZeroValueStringDoesNotPanic mirrors stdlib flag's isZeroValue, which calls String on a zero value of
// each registered type whenever a FlagSet prints usage, including on every parse error.
func TestFlagZeroValueStringDoesNotPanic(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag clihelper.Flag
		name string
	}{
		{name: "bool", flag: &clihelper.BoolFlag{Name: "foo"}},
		{name: "generic", flag: &clihelper.GenericFlag[string]{Name: "foo"}},
		{name: "slice", flag: &clihelper.SliceFlag[string]{Name: "foo"}},
		{name: "map", flag: &clihelper.MapFlag[string, string]{Name: "foo"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			set := libflag.NewFlagSet("test-cmd", libflag.ContinueOnError)
			set.SetOutput(io.Discard)

			require.NoError(t, tc.flag.Apply(set, map[string]string{}))

			registered := set.Lookup("foo")
			require.NotNil(t, registered)

			zero, ok := reflect.New(reflect.TypeOf(registered.Value).Elem()).Interface().(libflag.Value)
			require.True(t, ok)

			assert.NotPanics(t, func() { assert.Empty(t, zero.String()) })
		})
	}
}
