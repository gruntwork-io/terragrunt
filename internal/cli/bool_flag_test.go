package cli_test

import (
	libflag "flag"
	"fmt"
	"io"
	"maps"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoolFlagApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr   error
		envs          map[string]string
		args          []string
		flag          cli.BoolFlag
		expectedValue bool
	}{
		{
			flag:          cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}},
			args:          []string{"--foo"},
			envs:          map[string]string{"FOO": "false"},
			expectedValue: true,
			expectedErr:   nil,
		},
		{
			flag:          cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}},
			args:          nil,
			envs:          map[string]string{"FOO": "true"},
			expectedValue: true,
			expectedErr:   nil,
		},
		{
			flag:          cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}},
			args:          nil,
			envs:          nil,
			expectedValue: false,
			expectedErr:   nil,
		},
		{
			flag:          cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}, Destination: mockDestValue(false)},
			args:          []string{"--foo"},
			envs:          map[string]string{"FOO": "false"},
			expectedValue: true,
			expectedErr:   nil,
		},
		{
			flag:          cli.BoolFlag{Name: "foo", Destination: mockDestValue(true)},
			args:          nil,
			envs:          nil,
			expectedValue: true,
			expectedErr:   nil,
		},
		{
			flag:          cli.BoolFlag{Name: "foo", Destination: mockDestValue(true), Negative: true},
			args:          []string{"--foo"},
			envs:          nil,
			expectedValue: false,
			expectedErr:   nil,
		},
		{
			flag:          cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}, Destination: mockDestValue(true), Negative: true},
			args:          nil,
			envs:          map[string]string{"FOO": "true"},
			expectedValue: false,
			expectedErr:   nil,
		},
		{
			flag:          cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}, Destination: mockDestValue(false), Negative: true},
			args:          nil,
			envs:          map[string]string{"FOO": "false"},
			expectedValue: true,
			expectedErr:   nil,
		},
		{
			flag:          cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}},
			args:          []string{"--foo", "--foo"},
			envs:          nil,
			expectedValue: false,
			expectedErr:   errors.New(`invalid boolean flag foo: setting the flag multiple times`),
		},
		{
			flag:          cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}},
			args:          nil,
			envs:          map[string]string{"FOO": ""},
			expectedValue: false,
			expectedErr:   nil,
		},
		{
			flag:          cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}},
			args:          nil,
			envs:          map[string]string{"FOO": "monkey"},
			expectedValue: false,
			expectedErr:   errors.New(`invalid value "monkey" for env var FOO: must be one of: "0", "1", "f", "t", "false", "true"`),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testBoolFlagApply(t, &tc.flag, tc.args, tc.envs, tc.expectedValue, tc.expectedErr)
		})
	}
}

func testBoolFlagApply(t *testing.T, flag *cli.BoolFlag, args []string, envs map[string]string, expectedValue bool, expectedErr error) {
	t.Helper()

	var (
		actualValue          bool
		expectedDefaultValue string
	)

	if flag.Destination == nil {
		flag.Destination = new(bool)
	}

	expectedDefaultValue = strconv.FormatBool(*flag.Destination)

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

	actualValue = (flag.Value().Get()).(bool)

	assert.Equal(t, expectedValue, actualValue)

	if actualValue {
		assert.Equal(t, strconv.FormatBool(expectedValue), flag.GetValue(), "GetValue()")
	}

	maps.DeleteFunc(envs, func(k, v string) bool { return v == "" })

	assert.Equal(t, len(args) > 0 || len(envs) > 0, flag.Value().IsSet(), "IsSet()")
	assert.Equal(t, expectedDefaultValue, flag.GetDefaultText(), "GetDefaultText()")

	assert.True(t, flag.Value().IsBoolFlag(), "IsBoolFlag()")
	assert.False(t, flag.TakesValue(), "TakesValue()")
}
