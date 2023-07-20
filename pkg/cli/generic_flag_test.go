package cli

import (
	"errors"
	libflag "flag"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenericFlagStringApply(t *testing.T) {
	t.Parallel()

	mockDestValue := func(val string) *string {
		return &val
	}

	testCases := []struct {
		flag          GenericFlag[string]
		args          []string
		envs          map[string]string
		expectedValue string
		expectedErr   error
	}{
		{
			GenericFlag[string]{Name: "foo-string", EnvVar: "FOO_STRING"},
			[]string{"--foo-string", "arg-value"},
			map[string]string{"FOO_STRING": "env-value"},
			"arg-value",
			nil,
		},
		{
			GenericFlag[string]{Name: "foo-string", EnvVar: "FOO_STRING"},
			nil,
			map[string]string{"FOO_STRING": "env-value"},
			"env-value",
			nil,
		},
		{
			GenericFlag[string]{Name: "foo-string", EnvVar: "FOO_STRING"},
			nil,
			nil,
			"",
			nil,
		},
		{
			GenericFlag[string]{Name: "foo-string", EnvVar: "FOO_STRING", Destination: mockDestValue("default-value")},
			[]string{"--foo-string", "arg-value"},
			map[string]string{"FOO_STRING": "env-value"},
			"arg-value",
			nil,
		},
		{
			GenericFlag[string]{Name: "foo-string", Destination: mockDestValue("default-value")},
			nil,
			nil,
			"default-value",
			nil,
		},
		{
			GenericFlag[string]{Name: "foo-string", EnvVar: "FOO_STRING"},
			[]string{"--foo-string", "arg-value1", "--foo-string", "arg-value2"},
			nil,
			"",
			errors.New(`invalid value "arg-value2" for flag -foo-string: setting the flag multiple times`),
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

	mockDestValue := func(val int) *int {
		return &val
	}

	testCases := []struct {
		flag          GenericFlag[int]
		args          []string
		envs          map[string]string
		expectedValue int
		expectedErr   error
	}{
		{
			GenericFlag[int]{Name: "foo-int", EnvVar: "FOO_INT"},
			[]string{"--foo-int", "10"},
			map[string]string{"FOO_INT": "20"},
			10,
			nil,
		},
		{
			GenericFlag[int]{Name: "foo-int", EnvVar: "FOO_INT"},
			[]string{},
			map[string]string{"FOO_INT": "20"},
			20,
			nil,
		},
		{
			GenericFlag[int]{Name: "foo-int", Destination: mockDestValue(55)},
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

	mockDestValue := func(val int64) *int64 {
		return &val
	}

	testCases := []struct {
		flag          GenericFlag[int64]
		args          []string
		envs          map[string]string
		expectedValue int64
		expectedErr   error
	}{
		{
			GenericFlag[int64]{Name: "foo-int64", EnvVar: "FOO_INT64"},
			[]string{"--foo-int64", "10"},
			map[string]string{"FOO_INT64": "20"},
			10,
			nil,
		},
		{
			GenericFlag[int64]{Name: "foo-int64", EnvVar: "FOO_INT64"},
			[]string{},
			map[string]string{"FOO_INT64": "20"},
			20,
			nil,
		},
		{
			GenericFlag[int64]{Name: "foo-int64", Destination: mockDestValue(55)},
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

func testGenericFlagApply[T GenericType](t *testing.T, flag *GenericFlag[T], args []string, envs map[string]string, expectedValue T, expectedErr error) {
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
