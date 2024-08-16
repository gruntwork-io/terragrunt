package cli_test

import (
	"errors"
	libflag "flag"
	"fmt"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenericFlagStringApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag          cli.GenericFlag[string]
		args          []string
		envs          map[string]string
		expectedValue string
		expectedErr   error
	}{
		{
			cli.GenericFlag[string]{Name: "foo", EnvVar: "FOO"},
			[]string{"--foo", "arg-value"},
			map[string]string{"FOO": "env-value"},
			"arg-value",
			nil,
		},
		{
			cli.GenericFlag[string]{Name: "foo", EnvVar: "FOO"},
			nil,
			map[string]string{"FOO": "env-value"},
			"env-value",
			nil,
		},
		{
			cli.GenericFlag[string]{Name: "foo", EnvVar: "FOO"},
			nil,
			nil,
			"",
			nil,
		},
		{
			cli.GenericFlag[string]{Name: "foo", EnvVar: "FOO", Destination: mockDestValue("default-value")},
			[]string{"--foo", "arg-value"},
			map[string]string{"FOO": "env-value"},
			"arg-value",
			nil,
		},
		{
			cli.GenericFlag[string]{Name: "foo", Destination: mockDestValue("default-value")},
			nil,
			nil,
			"default-value",
			nil,
		},
		{
			cli.GenericFlag[string]{Name: "foo", EnvVar: "FOO"},
			[]string{"--foo", "arg-value1", "--foo", "arg-value2"},
			nil,
			"",
			errors.New(`invalid value "arg-value2" for flag -foo: setting the flag multiple times`),
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testGenericFlagApply(t, &testCase.flag, testCase.args, testCase.envs, testCase.expectedValue, testCase.expectedErr)
		})
	}
}

func TestGenericFlagIntApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag          cli.GenericFlag[int]
		args          []string
		envs          map[string]string
		expectedValue int
		expectedErr   error
	}{
		{
			cli.GenericFlag[int]{Name: "foo", EnvVar: "FOO"},
			[]string{"--foo", "10"},
			map[string]string{"FOO": "20"},
			10,
			nil,
		},
		{
			cli.GenericFlag[int]{Name: "foo", EnvVar: "FOO"},
			[]string{},
			map[string]string{"FOO": "20"},
			20,
			nil,
		},
		{
			cli.GenericFlag[int]{Name: "foo", Destination: mockDestValue(55)},
			nil,
			nil,
			55,
			nil,
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testGenericFlagApply(t, &testCase.flag, testCase.args, testCase.envs, testCase.expectedValue, testCase.expectedErr)
		})
	}
}

func TestGenericFlagInt64Apply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag          cli.GenericFlag[int64]
		args          []string
		envs          map[string]string
		expectedValue int64
		expectedErr   error
	}{
		{
			cli.GenericFlag[int64]{Name: "foo", EnvVar: "FOO"},
			[]string{"--foo", "10"},
			map[string]string{"FOO": "20"},
			10,
			nil,
		},
		{
			cli.GenericFlag[int64]{Name: "foo", EnvVar: "FOO"},
			[]string{},
			map[string]string{"FOO": "20"},
			20,
			nil,
		},
		{
			cli.GenericFlag[int64]{Name: "foo", Destination: mockDestValue(int64(55))},
			nil,
			nil,
			55,
			nil,
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testGenericFlagApply(t, &testCase.flag, testCase.args, testCase.envs, testCase.expectedValue, testCase.expectedErr)
		})
	}
}

func testGenericFlagApply[T cli.GenericType](t *testing.T, flag *cli.GenericFlag[T], args []string, envs map[string]string, expectedValue T, expectedErr error) {
	var (
		actualValue          T
		destDefined          bool
		expectedDefaultValue string
	)

	if flag.Destination == nil {
		destDefined = true
		flag.Destination = &actualValue
	} else {
		expectedDefaultValue = fmt.Sprintf("%v", *flag.Destination)
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
		actualValue = (flag.Value().Get()).(T)
	}

	assert.Equal(t, expectedValue, actualValue)
	assert.Equal(t, fmt.Sprintf("%v", expectedValue), flag.GetValue(), "GetValue()")

	assert.Equal(t, len(args) > 0, flag.Value().IsSet(), "IsSet()")
	assert.Equal(t, expectedDefaultValue, flag.Value().GetDefaultText(), "GetDefaultText()")

	assert.False(t, flag.Value().IsBoolFlag(), "IsBoolFlag()")
	assert.True(t, flag.TakesValue(), "TakesValue()")
}
