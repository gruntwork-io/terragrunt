package cli_test

import (
	"errors"
	libflag "flag"
	"fmt"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenericFlagStringApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr   error
		envs          map[string]string
		expectedValue string
		args          []string
		flag          cli.GenericFlag[string]
	}{
		{
			flag:          cli.GenericFlag[string]{Name: "foo", EnvVars: []string{"FOO"}},
			args:          []string{"--foo", "arg-value"},
			envs:          map[string]string{"FOO": "env-value"},
			expectedValue: "arg-value",
		},
		{
			flag:          cli.GenericFlag[string]{Name: "foo", EnvVars: []string{"FOO"}},
			envs:          map[string]string{"FOO": "env-value"},
			expectedValue: "env-value",
		},
		{
			flag: cli.GenericFlag[string]{Name: "foo", EnvVars: []string{"FOO"}},
		},
		{
			flag:          cli.GenericFlag[string]{Name: "foo", EnvVars: []string{"FOO"}, Destination: mockDestValue("default-value")},
			args:          []string{"--foo", "arg-value"},
			envs:          map[string]string{"FOO": "env-value"},
			expectedValue: "arg-value",
		},
		{
			flag:          cli.GenericFlag[string]{Name: "foo", Destination: mockDestValue("default-value")},
			expectedValue: "default-value",
		},
		{
			flag:        cli.GenericFlag[string]{Name: "foo", EnvVars: []string{"FOO"}},
			args:        []string{"--foo", "arg-value1", "--foo", "arg-value2"},
			expectedErr: errors.New(`invalid value "arg-value2" for flag -foo: setting the flag multiple times`),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testGenericFlagApply(t, &tc.flag, tc.args, tc.envs, tc.expectedValue, tc.expectedErr)
		})
	}
}

func TestGenericFlagIntApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr   error
		envs          map[string]string
		args          []string
		flag          cli.GenericFlag[int]
		expectedValue int
	}{
		{
			flag:          cli.GenericFlag[int]{Name: "foo", EnvVars: []string{"FOO"}},
			args:          []string{"--foo", "10"},
			envs:          map[string]string{"FOO": "20"},
			expectedValue: 10,
		},
		{
			flag:          cli.GenericFlag[int]{Name: "foo", EnvVars: []string{"FOO"}},
			args:          []string{},
			envs:          map[string]string{"FOO": "20"},
			expectedValue: 20,
		},
		{
			flag:        cli.GenericFlag[int]{Name: "foo", EnvVars: []string{"FOO"}},
			args:        []string{},
			envs:        map[string]string{"FOO": "monkey"},
			expectedErr: errors.New(`invalid value "monkey" for env var FOO: must be 32-bit integer`),
		},
		{
			flag:          cli.GenericFlag[int]{Name: "foo", Destination: mockDestValue(55)},
			expectedValue: 55,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testGenericFlagApply(t, &tc.flag, tc.args, tc.envs, tc.expectedValue, tc.expectedErr)
		})
	}
}

func TestGenericFlagInt64Apply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr   error
		envs          map[string]string
		args          []string
		flag          cli.GenericFlag[int64]
		expectedValue int64
	}{
		{
			flag:          cli.GenericFlag[int64]{Name: "foo", EnvVars: []string{"FOO"}},
			args:          []string{"--foo", "10"},
			envs:          map[string]string{"FOO": "20"},
			expectedValue: 10,
		},
		{
			flag:          cli.GenericFlag[int64]{Name: "foo", EnvVars: []string{"FOO"}},
			args:          []string{},
			envs:          map[string]string{"FOO": "20"},
			expectedValue: 20,
		},
		{
			flag:        cli.GenericFlag[int64]{Name: "foo", EnvVars: []string{"FOO"}},
			args:        []string{},
			envs:        map[string]string{"FOO": "monkey"},
			expectedErr: errors.New(`invalid value "monkey" for env var FOO: must be 64-bit integer`),
		},
		{
			flag:          cli.GenericFlag[int64]{Name: "foo", Destination: mockDestValue(int64(55))},
			expectedValue: 55,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testGenericFlagApply(t, &tc.flag, tc.args, tc.envs, tc.expectedValue, tc.expectedErr)
		})
	}
}

func testGenericFlagApply[T cli.GenericType](t *testing.T, flag *cli.GenericFlag[T], args []string, envs map[string]string, expectedValue T, expectedErr error) {
	t.Helper()

	var (
		actualValue          T
		expectedDefaultValue string
	)

	if flag.Destination == nil {
		flag.Destination = new(T)
	}

	expectedDefaultValue = fmt.Sprintf("%v", *flag.Destination)

	flag.LookupEnvFunc = func(key string) []string {
		if envs == nil {
			return nil
		}

		if val, ok := envs[key]; ok {
			return []string{val}
		}

		return nil
	}

	flagSet := libflag.NewFlagSet("test-cmd", libflag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	err := flag.Apply(flagSet)
	if err == nil {
		err = flagSet.Parse(args)
	}

	if expectedErr != nil {
		require.Error(t, err)
		require.ErrorContains(t, expectedErr, err.Error())

		return
	}

	require.NoError(t, err)

	actualValue = (flag.Value().Get()).(T)

	assert.Equal(t, expectedValue, actualValue)
	assert.Equal(t, fmt.Sprintf("%v", expectedValue), flag.GetValue(), "GetValue()")

	assert.Equal(t, len(args) > 0 || len(envs) > 0, flag.Value().IsSet(), "IsSet()")
	assert.Equal(t, expectedDefaultValue, flag.GetInitialTextValue(), "GetDefaultText()")

	assert.False(t, flag.Value().IsBoolFlag(), "IsBoolFlag()")
	assert.True(t, flag.TakesValue(), "TakesValue()")
}
