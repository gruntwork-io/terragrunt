package cli_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/stretchr/testify/assert"
)

var mockArgs = cli.Args([]string{"one", "-foo", "two", "--bar", "value"})

func TestArgsSlice(t *testing.T) {
	t.Parallel()

	actual := mockArgs.Slice()
	expected := []string(mockArgs)
	assert.Equal(t, expected, actual)
}

func TestArgsTail(t *testing.T) {
	t.Parallel()

	actual := mockArgs.Tail()
	expected := []string(mockArgs[1:])
	assert.Equal(t, expected, actual)
}

func TestArgsFirst(t *testing.T) {
	t.Parallel()

	actual := mockArgs.First()
	expected := mockArgs[0]
	assert.Equal(t, expected, actual)
}

func TestArgsGet(t *testing.T) {
	t.Parallel()

	actual := mockArgs.Get(2)
	expected := "two"
	assert.Equal(t, expected, actual)
}

func TestArgsLen(t *testing.T) {
	t.Parallel()

	actual := mockArgs.Len()
	expected := 5
	assert.Equal(t, expected, actual)
}

func TestArgsPresent(t *testing.T) {
	t.Parallel()

	actual := mockArgs.Present()
	expected := true
	assert.Equal(t, expected, actual)

	mockArgs := cli.Args([]string{})
	actual = mockArgs.Present()
	expected = false
	assert.Equal(t, expected, actual)
}

func TestArgsCommandName(t *testing.T) {
	t.Parallel()

	actual := mockArgs.CommandName()
	expected := "one"
	assert.Equal(t, expected, actual)

	mockArgs := mockArgs[1:]
	actual = mockArgs.CommandName()
	expected = ""
	assert.Equal(t, expected, actual)
}

func TestArgsNormalize(t *testing.T) {
	t.Parallel()

	actual := mockArgs.Normalize(cli.SingleDashFlag).Slice()
	expected := []string{"one", "-foo", "two", "-bar", "value"}
	assert.Equal(t, expected, actual)

	actual = mockArgs.Normalize(cli.DoubleDashFlag).Slice()
	expected = []string{"one", "--foo", "two", "--bar", "value"}
	assert.Equal(t, expected, actual)
}
