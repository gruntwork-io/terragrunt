package cli_test

import (
	"errors"
	libflag "flag"
	"fmt"
	"io"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"
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
			cli.BoolFlag{Name: "foo", EnvVar: "FOO"},
			[]string{"--foo"},
			map[string]string{"FOO": "false"},
			true,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVar: "FOO"},
			nil,
			map[string]string{"FOO": "true"},
			true,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVar: "FOO"},
			nil,
			nil,
			false,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVar: "FOO", Destination: mockDestValue(false)},
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
			cli.BoolFlag{Name: "foo", EnvVar: "FOO", Destination: mockDestValue(true), Negative: true},
			nil,
			map[string]string{"FOO": "true"},
			false,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVar: "FOO", Destination: mockDestValue(false), Negative: true},
			nil,
			map[string]string{"FOO": "false"},
			true,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVar: "FOO"},
			[]string{"--foo", "--foo"},
			nil,
			false,
			errors.New(`invalid boolean flag foo: setting the flag multiple times`),
		},
		{
			cli.BoolFlag{Name: "foo", EnvVar: "FOO"},
			nil,
			map[string]string{"FOO": ""},
			false,
			nil,
		},
		{
			cli.BoolFlag{Name: "foo", EnvVar: "FOO"},
			nil,
			map[string]string{"FOO": "monkey"},
			false,
			errors.New(`invalid boolean value "monkey" for FOO: must be one of: "0", "1", "f", "t", "false", "true"`),
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
		destDefined          bool
		expectedDefaultValue string
	)

	if flag.Destination == nil {
		destDefined = true
		flag.Destination = &actualValue
	} else if val := *flag.Destination; val && !flag.Negative {
		expectedDefaultValue = strconv.FormatBool(val)
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
	if err == nil {
		err = flagSet.Parse(args)
	}

	if expectedErr != nil {
		require.ErrorContains(t, expectedErr, err.Error())
		return
	}
	require.NoError(t, err)

	if !destDefined {
		actualValue = (flag.Value().Get()).(bool)
	}

	assert.Equal(t, expectedValue, actualValue)
	if actualValue {
		assert.Equal(t, strconv.FormatBool(expectedValue), flag.GetValue(), "GetValue()")
	}

	maps.DeleteFunc(envs, func(k, v string) bool { return v == "" })

	assert.Equal(t, len(args) > 0 || len(envs) > 0, flag.Value().IsSet(), "IsSet()")
	assert.Equal(t, expectedDefaultValue, flag.Value().GetDefaultText(), "GetDefaultText()")

	assert.True(t, flag.Value().IsBoolFlag(), "IsBoolFlag()")
	assert.False(t, flag.TakesValue(), "TakesValue()")
}
