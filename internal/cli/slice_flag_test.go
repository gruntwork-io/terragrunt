package cli_test

import (
	libflag "flag"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSliceFlagStringApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr   error
		envs          map[string]string
		args          []string
		expectedValue []string
		flag          cli.SliceFlag[string]
	}{
		{
			flag:          cli.SliceFlag[string]{Name: "foo", EnvVars: []string{"FOO"}},
			args:          []string{"--foo", "arg-value1", "--foo", "arg-value2"},
			envs:          map[string]string{"FOO": "env-value"},
			expectedValue: []string{"arg-value1", "arg-value2"},
		},
		{
			flag:          cli.SliceFlag[string]{Name: "foo", EnvVars: []string{"FOO"}},
			envs:          map[string]string{"FOO": "env-value1,env-value2"},
			expectedValue: []string{"env-value1", "env-value2"},
		},
		{
			flag: cli.SliceFlag[string]{Name: "foo", EnvVars: []string{"FOO"}},
		},
		{
			flag:          cli.SliceFlag[string]{Name: "foo", EnvVars: []string{"FOO"}, Destination: mockDestValue([]string{"default-value1", "default-value2"})},
			args:          []string{"--foo", "arg-value1", "--foo", "arg-value2"},
			envs:          map[string]string{"FOO": "env-value1,env-value2"},
			expectedValue: []string{"arg-value1", "arg-value2"},
		},
		{
			flag:          cli.SliceFlag[string]{Name: "foo", Destination: mockDestValue([]string{"default-value1", "default-value2"})},
			expectedValue: []string{"default-value1", "default-value2"},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testSliceFlagApply(t, &tc.flag, tc.args, tc.envs, tc.expectedValue, tc.expectedErr)
		})
	}
}

func TestSliceFlagIntApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr   error
		envs          map[string]string
		args          []string
		expectedValue []int
		flag          cli.SliceFlag[int]
	}{
		{
			flag:          cli.SliceFlag[int]{Name: "foo", EnvVars: []string{"FOO"}},
			args:          []string{"--foo", "10", "--foo", "11"},
			envs:          map[string]string{"FOO": "20,21"},
			expectedValue: []int{10, 11},
		},
		{
			flag:          cli.SliceFlag[int]{Name: "foo", EnvVars: []string{"FOO"}},
			envs:          map[string]string{"FOO": "20,21"},
			expectedValue: []int{20, 21},
		},
		{
			flag:          cli.SliceFlag[int]{Name: "foo", Destination: mockDestValue([]int{50, 51})},
			expectedValue: []int{50, 51},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testSliceFlagApply(t, &tc.flag, tc.args, tc.envs, tc.expectedValue, tc.expectedErr)
		})
	}
}

func TestSliceFlagInt64Apply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr   error
		envs          map[string]string
		args          []string
		expectedValue []int64
		flag          cli.SliceFlag[int64]
	}{
		{
			flag:          cli.SliceFlag[int64]{Name: "foo", EnvVars: []string{"FOO"}},
			args:          []string{"--foo", "10", "--foo", "11"},
			envs:          map[string]string{"FOO": "20,21"},
			expectedValue: []int64{10, 11},
		},
		{
			flag:          cli.SliceFlag[int64]{Name: "foo", EnvVars: []string{"FOO"}},
			envs:          map[string]string{"FOO": "20,21"},
			expectedValue: []int64{20, 21},
		},
		{
			flag:          cli.SliceFlag[int64]{Name: "foo", Destination: mockDestValue([]int64{50, 51})},
			expectedValue: []int64{50, 51},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testSliceFlagApply(t, &tc.flag, tc.args, tc.envs, tc.expectedValue, tc.expectedErr)
		})
	}
}

func testSliceFlagApply[T cli.SliceFlagType](t *testing.T, flag *cli.SliceFlag[T], args []string, envs map[string]string, expectedValue []T, expectedErr error) {
	t.Helper()

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

	assert.Equal(t, len(args) > 0 || len(envs) > 0, flag.Value().IsSet(), "IsSet()")
	assert.Equal(t, expectedStringValueFn(expectedDefaultValue), flag.GetDefaultText(), "GetDefaultText()")

	assert.False(t, flag.Value().IsBoolFlag(), "IsBoolFlag()")
	assert.True(t, flag.TakesValue(), "TakesValue()")
}
