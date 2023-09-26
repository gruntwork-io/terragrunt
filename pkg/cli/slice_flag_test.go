package cli

import (
	libflag "flag"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSliceFlagStringApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag          SliceFlag[string]
		args          []string
		envs          map[string]string
		expectedValue []string
		expectedErr   error
	}{
		{
			SliceFlag[string]{Name: "foo", EnvVar: "FOO"},
			[]string{"--foo", "arg-value1", "--foo", "arg-value2"},
			map[string]string{"FOO": "env-value"},
			[]string{"arg-value1", "arg-value2"},
			nil,
		},
		{
			SliceFlag[string]{Name: "foo", EnvVar: "FOO"},
			nil,
			map[string]string{"FOO": "env-value1,env-value2"},
			[]string{"env-value1", "env-value2"},
			nil,
		},
		{
			SliceFlag[string]{Name: "foo", EnvVar: "FOO"},
			nil,
			nil,
			nil,
			nil,
		},
		{
			SliceFlag[string]{Name: "foo", EnvVar: "FOO", Destination: mockDestValue([]string{"default-value1", "default-value2"})},
			[]string{"--foo", "arg-value1", "--foo", "arg-value2"},
			map[string]string{"FOO": "env-value1,env-value2"},
			[]string{"arg-value1", "arg-value2"},
			nil,
		},
		{
			SliceFlag[string]{Name: "foo", Destination: mockDestValue([]string{"default-value1", "default-value2"})},
			nil,
			nil,
			[]string{"default-value1", "default-value2"},
			nil,
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testSliceFlagApply(t, &testCase.flag, testCase.args, testCase.envs, testCase.expectedValue, testCase.expectedErr)
		})
	}
}

func TestSliceFlagIntApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag          SliceFlag[int]
		args          []string
		envs          map[string]string
		expectedValue []int
		expectedErr   error
	}{
		{
			SliceFlag[int]{Name: "foo", EnvVar: "FOO"},
			[]string{"--foo", "10", "--foo", "11"},
			map[string]string{"FOO": "20,21"},
			[]int{10, 11},
			nil,
		},
		{
			SliceFlag[int]{Name: "foo", EnvVar: "FOO"},
			[]string{},
			map[string]string{"FOO": "20,21"},
			[]int{20, 21},
			nil,
		},
		{
			SliceFlag[int]{Name: "foo", Destination: mockDestValue([]int{50, 51})},
			nil,
			nil,
			[]int{50, 51},
			nil,
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testSliceFlagApply(t, &testCase.flag, testCase.args, testCase.envs, testCase.expectedValue, testCase.expectedErr)
		})
	}
}

func TestSliceFlagInt64Apply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag          SliceFlag[int64]
		args          []string
		envs          map[string]string
		expectedValue []int64
		expectedErr   error
	}{
		{
			SliceFlag[int64]{Name: "foo", EnvVar: "FOO"},
			[]string{"--foo", "10", "--foo", "11"},
			map[string]string{"FOO": "20,21"},
			[]int64{10, 11},
			nil,
		},
		{
			SliceFlag[int64]{Name: "foo", EnvVar: "FOO"},
			[]string{},
			map[string]string{"FOO": "20,21"},
			[]int64{20, 21},
			nil,
		},
		{
			SliceFlag[int64]{Name: "foo", Destination: mockDestValue([]int64{50, 51})},
			nil,
			nil,
			[]int64{50, 51},
			nil,
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testSliceFlagApply(t, &testCase.flag, testCase.args, testCase.envs, testCase.expectedValue, testCase.expectedErr)
		})
	}
}

func testSliceFlagApply[T SliceFlagType](t *testing.T, flag *SliceFlag[T], args []string, envs map[string]string, expectedValue []T, expectedErr error) {

	var (
		actualValue          []T
		destDefined          bool
		expectedDefaultValue []T
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
		actualValue = (flag.Value().Get()).([]T)
	}
	assert.Equal(t, expectedValue, actualValue)

	expectedStringValueFn := func(value []T) string {
		var stringValue []string
		for _, val := range value {
			stringValue = append(stringValue, fmt.Sprintf("%v", val))
		}

		return strings.Join(stringValue, flag.EnvVarSep)
	}

	assert.Equal(t, expectedStringValueFn(expectedValue), flag.GetValue(), "GetValue()")

	assert.Equal(t, len(args) > 0, flag.Value().IsSet(), "IsSet()")
	assert.Equal(t, expectedStringValueFn(expectedDefaultValue), flag.Value().GetDefaultText(), "GetDefaultText()")

	assert.False(t, flag.Value().IsBoolFlag(), "IsBoolFlag()")
	assert.True(t, flag.TakesValue(), "TakesValue()")
}
