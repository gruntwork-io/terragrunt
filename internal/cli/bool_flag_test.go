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
		flag          cli.BoolFlag
		args          []string
		envs          map[string]string
		expectedValue bool
		expectedErr   error
	}{
		{
			cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}},
			[]string{"--foo"},
			map[string]string{"FOO": "false"},
			true,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}},
			nil,
			map[string]string{"FOO": "true"},
			true,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}},
			nil,
			nil,
			false,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}, Destination: mockDestValue(false)},
			[]string{"--foo"},
			map[string]string{"FOO": "false"},
			true,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", Destination: mockDestValue(true)},
			nil,
			nil,
			true,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", Destination: mockDestValue(true), Negative: true},
			[]string{"--foo"},
			nil,
			false,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}, Destination: mockDestValue(true), Negative: true},
			nil,
			map[string]string{"FOO": "true"},
			false,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}, Destination: mockDestValue(false), Negative: true},
			nil,
			map[string]string{"FOO": "false"},
			true,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}},
			[]string{"--foo", "--foo"},
			nil,
			false,
			errors.New(`invalid boolean flag foo: setting the flag multiple times`),
		},
		{
			cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}},
			nil,
			map[string]string{"FOO": ""},
			false,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVars: []string{"FOO"}},
			nil,
			map[string]string{"FOO": "monkey"},
			false,
			errors.New(`invalid value "monkey" for env var FOO: must be one of: "0", "1", "f", "t", "false", "true"`),
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testBoolFlagApply(t, &testCase.flag, testCase.args, testCase.envs, testCase.expectedValue, testCase.expectedErr)
		})
	}
}

func testBoolFlagApply(t *testing.T, flag *cli.BoolFlag, args []string, envs map[string]string, expectedValue bool, expectedErr error) {
	t.Helper()

	var (
		actualValue          bool
		expectedDefaultValue string
	)

	if val := flag.Destination; val != nil {
		expectedDefaultValue = strconv.FormatBool(*val)
	}

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
