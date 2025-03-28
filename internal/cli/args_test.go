package cli_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/stretchr/testify/assert"
)

var mockArgs = func() cli.Args { return cli.Args{"one", "-foo", "two", "--bar", "value"} }

func TestArgsSlice(t *testing.T) {
	t.Parallel()

	actual := mockArgs().Slice()
	expected := []string(mockArgs())
	assert.Equal(t, expected, actual)
}

func TestArgsTail(t *testing.T) {
	t.Parallel()

	actual := mockArgs().Tail()
	expected := mockArgs()[1:]
	assert.Equal(t, expected, actual)
}

func TestArgsFirst(t *testing.T) {
	t.Parallel()

	actual := mockArgs().First()
	expected := mockArgs()[0]
	assert.Equal(t, expected, actual)
}

func TestArgsGet(t *testing.T) {
	t.Parallel()

	actual := mockArgs().Get(2)
	expected := "two"
	assert.Equal(t, expected, actual)
}

func TestArgsLen(t *testing.T) {
	t.Parallel()

	actual := mockArgs().Len()
	expected := 5
	assert.Equal(t, expected, actual)
}

func TestArgsPresent(t *testing.T) {
	t.Parallel()

	actual := mockArgs().Present()
	expected := true
	assert.Equal(t, expected, actual)

	args := cli.Args([]string{})
	actual = args.Present()
	expected = false
	assert.Equal(t, expected, actual)
}

func TestArgsCommandName(t *testing.T) {
	t.Parallel()

	actual := mockArgs().CommandName()
	expected := "one"
	assert.Equal(t, expected, actual)

	args := mockArgs()[1:]
	actual = args.CommandName()
	expected = "two"
	assert.Equal(t, expected, actual)
}

func TestArgsNormalize(t *testing.T) {
	t.Parallel()

	actual := mockArgs().Normalize(cli.SingleDashFlag).Slice()
	expected := []string{"one", "-foo", "two", "-bar", "value"}
	assert.Equal(t, expected, actual)

	actual = mockArgs().Normalize(cli.DoubleDashFlag).Slice()
	expected = []string{"one", "--foo", "two", "--bar", "value"}
	assert.Equal(t, expected, actual)
}

func TestArgsRemove(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args           cli.Args
		expectedArgs   cli.Args
		removeName     string
		expectedResult cli.Args
	}{
		{
			mockArgs(),
			mockArgs(),
			"two",
			cli.Args{"one", "-foo", "--bar", "value"},
		},
		{
			mockArgs(),
			mockArgs(),
			"one",
			cli.Args{"-foo", "two", "--bar", "value"},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actual := tc.args.Remove(tc.removeName)
			assert.Equal(t, tc.expectedResult, actual)
			assert.Equal(t, tc.expectedArgs, tc.args)
		})
	}
}
