package cli_test

import (
	libflag "flag"
	"fmt"
	"io"
	"testing"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapFlagStringStringApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr   error
		envs          map[string]string
		expectedValue map[string]string
		args          []string
		flag          cli.MapFlag[string, string]
	}{
		{
			flag:          cli.MapFlag[string, string]{Name: "foo", EnvVars: []string{"FOO"}},
			args:          []string{"--foo", "arg1-key=arg1-value", "--foo", "arg2-key = arg2-value"},
			envs:          map[string]string{"FOO": "env1-key=env1-value,env2-key=env2-value"},
			expectedValue: map[string]string{"arg1-key": "arg1-value", "arg2-key": "arg2-value"},
		},
		{
			flag:          cli.MapFlag[string, string]{Name: "foo", EnvVars: []string{"FOO"}},
			envs:          map[string]string{"FOO": "env1-key=env1-value,env2-key = env2-value"},
			expectedValue: map[string]string{"env1-key": "env1-value", "env2-key": "env2-value"},
		},
		{
			flag:          cli.MapFlag[string, string]{Name: "foo", EnvVars: []string{"FOO"}},
			expectedValue: map[string]string{},
		},
		{
			flag:          cli.MapFlag[string, string]{Name: "foo", EnvVars: []string{"FOO"}, Destination: mockDestValue(map[string]string{"default1-key": "default1-value", "default2-key": "default2-value"})},
			args:          []string{"--foo", "arg1-key=arg1-value", "--foo", "arg2-key=arg2-value"},
			envs:          map[string]string{"FOO": "env1-key=env1-value,env2-key=env2-value"},
			expectedValue: map[string]string{"arg1-key": "arg1-value", "arg2-key": "arg2-value"},
		},
		{
			flag:          cli.MapFlag[string, string]{Name: "foo", Destination: mockDestValue(map[string]string{"default1-key": "default1-value", "default2-key": "default2-value"})},
			expectedValue: map[string]string{"default1-key": "default1-value", "default2-key": "default2-value"},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testMapFlagApply(t, &tc.flag, tc.args, tc.envs, tc.expectedValue, tc.expectedErr)
		})
	}
}

func TestMapFlagStringIntApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr   error
		envs          map[string]string
		expectedValue map[string]int
		args          []string
		flag          cli.MapFlag[string, int]
	}{
		{
			flag:          cli.MapFlag[string, int]{Name: "foo", EnvVars: []string{"FOO"}},
			args:          []string{"--foo", "arg1-key=10", "--foo", "arg2-key=11"},
			envs:          map[string]string{"FOO": "env1-key=20,env2-key=21"},
			expectedValue: map[string]int{"arg1-key": 10, "arg2-key": 11},
		},
		{
			flag:          cli.MapFlag[string, int]{Name: "foo", EnvVars: []string{"FOO"}},
			envs:          map[string]string{"FOO": "env1-key=20,env2-key=21"},
			expectedValue: map[string]int{"env1-key": 20, "env2-key": 21},
		},

		{
			flag:          cli.MapFlag[string, int]{Name: "foo", EnvVars: []string{"FOO"}, Destination: mockDestValue(map[string]int{"default1-key": 50, "default2-key": 51})},
			args:          []string{"--foo", "arg1-key=10", "--foo", "arg2-key=11"},
			envs:          map[string]string{"FOO": "env1-key=20,env2-key=21"},
			expectedValue: map[string]int{"arg1-key": 10, "arg2-key": 11},
		},
		{
			flag:          cli.MapFlag[string, int]{Name: "foo", Destination: mockDestValue(map[string]int{"default1-key": 50, "default2-key": 51})},
			expectedValue: map[string]int{"default1-key": 50, "default2-key": 51},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testMapFlagApply(t, &tc.flag, tc.args, tc.envs, tc.expectedValue, tc.expectedErr)
		})
	}
}

func testMapFlagApply[K cli.MapFlagKeyType, V cli.MapFlagValueType](t *testing.T, flag *cli.MapFlag[K, V], args []string, envs map[string]string, expectedValue map[K]V, expectedErr error) {
	t.Helper()

	var (
		actualValue          = map[K]V{}
		destDefined          bool
		expectedDefaultValue = map[K]V{}
	)

	if flag.Destination == nil {
		destDefined = true
		flag.Destination = &actualValue
	} else {
		expectedDefaultValue = *flag.Destination
	}

	flag.LookupEnvFunc = func(key string) []string {
		if envs == nil {
			return nil
		}

		if val, ok := envs[key]; ok {
			return flag.Splitter(val, flag.EnvVarSep)
		}

		return nil
	}

	flagSet := libflag.NewFlagSet("test-cmd", libflag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	err := flag.Apply(flagSet)
	require.NoError(t, err)

	err = flagSet.Parse(args)
	if expectedErr != nil {
		require.Equal(t, expectedErr, err)
		return
	}

	require.NoError(t, err)

	if !destDefined {
		actualValue = (flag.Value().Get()).(map[K]V)
	}

	assert.Subset(t, expectedValue, actualValue)

	assert.Equal(t, collections.MapJoin(expectedValue, flag.EnvVarSep, flag.KeyValSep), flag.GetValue(), "GetValue()")

	assert.Equal(t, len(args) > 0 || len(envs) > 0, flag.Value().IsSet(), "IsSet()")
	assert.Equal(t, collections.MapJoin(expectedDefaultValue, flag.EnvVarSep, flag.KeyValSep), flag.GetDefaultText(), "GetDefaultText()")

	assert.False(t, flag.Value().IsBoolFlag(), "IsBoolFlag()")
	assert.True(t, flag.TakesValue(), "TakesValue()")
}
