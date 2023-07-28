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

func TestBoolFlagApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag          BoolFlag
		args          []string
		envs          map[string]string
		expectedValue bool
		expectedErr   error
	}{
		{
			BoolFlag{Name: "foo-bool", EnvVar: "FOO_BOOL"},
			[]string{"--foo-bool"},
			map[string]string{"FOO_BOOL": "false"},
			true,
			nil,
		},
		{
			BoolFlag{Name: "foo-bool", EnvVar: "FOO_BOOL"},
			nil,
			map[string]string{"FOO_BOOL": "true"},
			true,
			nil,
		},
		{
			BoolFlag{Name: "foo-bool", EnvVar: "FOO_BOOL"},
			nil,
			nil,
			false,
			nil,
		},
		{
			BoolFlag{Name: "foo-bool", EnvVar: "FOO_BOOL", Destination: mockDestValue(false)},
			[]string{"--foo-bool"},
			map[string]string{"FOO_BOOL": "false"},
			true,
			nil,
		},
		{
			BoolFlag{Name: "foo-bool", Destination: mockDestValue(true)},
			nil,
			nil,
			true,
			nil,
		},
		{
			BoolFlag{Name: "foo-bool", Destination: mockDestValue(true), Negative: true},
			[]string{"--foo-bool"},
			nil,
			false,
			nil,
		},
		{
			BoolFlag{Name: "foo-bool", EnvVar: "FOO_BOOL", Destination: mockDestValue(true), Negative: true},
			nil,
			map[string]string{"FOO_BOOL": "true"},
			false,
			nil,
		},
		{
			BoolFlag{Name: "foo-bool", EnvVar: "FOO_BOOL", Destination: mockDestValue(false), Negative: true},
			nil,
			map[string]string{"FOO_BOOL": "false"},
			true,
			nil,
		},
		{
			BoolFlag{Name: "foo-bool", EnvVar: "FOO_BOOL"},
			[]string{"--foo-bool", "--foo-bool"},
			nil,
			false,
			errors.New(`invalid boolean flag foo-bool: setting the flag multiple times`),
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

func testBoolFlagApply(t *testing.T, flag *BoolFlag, args []string, envs map[string]string, expectedValue bool, expectedErr error) {
	var (
		actualValue          bool
		destDefined          bool
		expectedDefaultValue string
	)

	if flag.Destination == nil {
		destDefined = true
		flag.Destination = &actualValue
	} else if val := *flag.Destination; val && !flag.Negative {
		expectedDefaultValue = fmt.Sprintf("%t", val)
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
		actualValue = (flag.Value().Get()).(bool)
	}

	assert.Equal(t, expectedValue, actualValue)
	if actualValue {
		assert.Equal(t, fmt.Sprintf("%t", expectedValue), flag.GetValue(), "GetValue()")
	}

	assert.Equal(t, len(args) > 0, flag.Value().IsSet(), "IsSet()")
	assert.Equal(t, expectedDefaultValue, flag.Value().GetDefaultText(), "GetDefaultText()")

	assert.True(t, flag.Value().IsBoolFlag(), "IsBoolFlag()")
	assert.False(t, flag.TakesValue(), "TakesValue()")
}
