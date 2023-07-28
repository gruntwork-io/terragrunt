package cli

import (
	libflag "flag"
	"fmt"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/maps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapFlagStringStringApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag          MapFlag[string, string]
		args          []string
		envs          map[string]string
		expectedValue map[string]string
		expectedErr   error
	}{
		{
			MapFlag[string, string]{Name: "foo-map-string", EnvVar: "FOO_MAP_STRING"},
			[]string{"--foo-map-string", "arg1-key=arg1-value", "--foo-map-string", "arg2-key = arg2-value"},
			map[string]string{"FOO_MAP_STRING": "env1-key=env1-value,env2-key=env2-value"},
			map[string]string{"arg1-key": "arg1-value", "arg2-key": "arg2-value"},
			nil,
		},
		{
			MapFlag[string, string]{Name: "foo-map-string", EnvVar: "FOO_MAP_STRING"},
			nil,
			map[string]string{"FOO_MAP_STRING": "env1-key=env1-value,env2-key = env2-value"},
			map[string]string{"env1-key": "env1-value", "env2-key": "env2-value"},
			nil,
		},
		{
			MapFlag[string, string]{Name: "foo-map-string", EnvVar: "FOO_MAP_STRING"},
			nil,
			nil,
			map[string]string{},
			nil,
		},
		{
			MapFlag[string, string]{Name: "foo-map-string", EnvVar: "FOO_MAP_STRING", Destination: mockDestValue(map[string]string{"default1-key": "default1-value", "default2-key": "default2-value"})},
			[]string{"--foo-map-string", "arg1-key=arg1-value", "--foo-map-string", "arg2-key=arg2-value"},
			map[string]string{"FOO_MAP_STRING": "env1-key=env1-value,env2-key=env2-value"},
			map[string]string{"arg1-key": "arg1-value", "arg2-key": "arg2-value"},
			nil,
		},
		{
			MapFlag[string, string]{Name: "foo-map-string", Destination: mockDestValue(map[string]string{"default1-key": "default1-value", "default2-key": "default2-value"})},
			nil,
			nil,
			map[string]string{"default1-key": "default1-value", "default2-key": "default2-value"},
			nil,
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testMapFlagApply(t, &testCase.flag, testCase.args, testCase.envs, testCase.expectedValue, testCase.expectedErr)
		})
	}
}

func TestMapFlagStringIntApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag          MapFlag[string, int]
		args          []string
		envs          map[string]string
		expectedValue map[string]int
		expectedErr   error
	}{
		{
			MapFlag[string, int]{Name: "foo-map-int", EnvVar: "FOO_MAP_INT"},
			[]string{"--foo-map-int", "arg1-key=10", "--foo-map-int", "arg2-key=11"},
			map[string]string{"FOO_MAP_INT": "env1-key=20,env2-key=21"},
			map[string]int{"arg1-key": 10, "arg2-key": 11},
			nil,
		},
		{
			MapFlag[string, int]{Name: "foo-map-int", EnvVar: "FOO_MAP_INT"},
			nil,
			map[string]string{"FOO_MAP_INT": "env1-key=20,env2-key=21"},
			map[string]int{"env1-key": 20, "env2-key": 21},
			nil,
		},

		{
			MapFlag[string, int]{Name: "foo-map-int", EnvVar: "FOO_MAP_INT", Destination: mockDestValue(map[string]int{"default1-key": 50, "default2-key": 51})},
			[]string{"--foo-map-int", "arg1-key=10", "--foo-map-int", "arg2-key=11"},
			map[string]string{"FOO_MAP_INT": "env1-key=20,env2-key=21"},
			map[string]int{"arg1-key": 10, "arg2-key": 11},
			nil,
		},
		{
			MapFlag[string, int]{Name: "foo-map-string", Destination: mockDestValue(map[string]int{"default1-key": 50, "default2-key": 51})},
			nil,
			nil,
			map[string]int{"default1-key": 50, "default2-key": 51},
			nil,
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testMapFlagApply(t, &testCase.flag, testCase.args, testCase.envs, testCase.expectedValue, testCase.expectedErr)
		})
	}
}

func testMapFlagApply[K MapFlagKeyType, V MapFlagValueType](t *testing.T, flag *MapFlag[K, V], args []string, envs map[string]string, expectedValue map[K]V, expectedErr error) {

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

	flag.LookupEnvFunc = func(key string) (string, bool) {
		if envs == nil {
			return "", false
		}

		if val, ok := envs[key]; ok {
			return val, true
		}
		return "", false
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

	assert.Equal(t, maps.Join(expectedValue, flag.EnvVarSep, flag.KeyValSep), flag.GetValue(), "GetValue()")

	assert.Equal(t, len(args) > 0, flag.Value().IsSet(), "IsSet()")
	assert.Equal(t, maps.Join(expectedDefaultValue, flag.EnvVarSep, flag.KeyValSep), flag.Value().GetDefaultText(), "GetDefaultText()")

	assert.False(t, flag.Value().IsBoolFlag(), "IsBoolFlag()")
	assert.True(t, flag.TakesValue(), "TakesValue()")
}
