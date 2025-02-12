package flags_test

import (
	"flag"
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockDestValue[T any](val T) *T {
	return &val
}

func TestFlag_TakesValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag     cli.Flag
		expected bool
	}{
		{
			&cli.BoolFlag{Name: "name", Destination: mockDestValue(false)},
			true,
		},
		{
			&cli.BoolFlag{Name: "name", Destination: mockDestValue(true)},
			false,
		},
		{
			&cli.BoolFlag{Name: "name", Negative: true, Destination: mockDestValue(true)},
			true,
		},
		{
			&cli.BoolFlag{Name: "name", Negative: true, Destination: mockDestValue(false)},
			false,
		},
		{
			&cli.GenericFlag[string]{Name: "name", Destination: mockDestValue("value")},
			true,
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testFlag := flags.NewFlag(testCase.flag)

			err := testFlag.Apply(new(flag.FlagSet))
			require.NoError(t, err)

			assert.Equal(t, testCase.expected, testFlag.TakesValue())
		})
	}
}
