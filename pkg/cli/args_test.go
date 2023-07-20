package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var mockArgs = []string{"one", "-foo", "two", "--bar", "value"}

func TestArgsSlice(t *testing.T) {
	t.Parallel()

	actual := newArgs(mockArgs).Slice()
	expected := mockArgs
	assert.Equal(t, expected, actual)
}

func TestArgsTail(t *testing.T) {
	t.Parallel()

	actual := newArgs(mockArgs).Tail()
	expected := mockArgs[1:]
	assert.Equal(t, expected, actual)
}

func TestArgsFirst(t *testing.T) {
	t.Parallel()

	actual := newArgs(mockArgs).First()
	expected := mockArgs[0]
	assert.Equal(t, expected, actual)
}

func TestArgsGet(t *testing.T) {
	t.Parallel()

	actual := newArgs(mockArgs).Get(2)
	expected := "two"
	assert.Equal(t, expected, actual)
}

func TestArgsLen(t *testing.T) {
	t.Parallel()

	actual := newArgs(mockArgs).Len()
	expected := 5
	assert.Equal(t, expected, actual)
}

func TestArgsPresent(t *testing.T) {
	t.Parallel()

	actual := newArgs(mockArgs).Present()
	expected := true
	assert.Equal(t, expected, actual)

	actual = newArgs([]string{}).Present()
	expected = false
	assert.Equal(t, expected, actual)
}

func TestArgsCommandName(t *testing.T) {
	t.Parallel()

	actual := newArgs(mockArgs).CommandName()
	expected := "one"
	assert.Equal(t, expected, actual)

	actual = newArgs(mockArgs[1:]).CommandName()
	expected = ""
	assert.Equal(t, expected, actual)
}

func TestArgsNormalize(t *testing.T) {
	t.Parallel()

	actual := newArgs(mockArgs).Normalize(SingleDashFlag).Slice()
	expected := []string{"one", "-foo", "two", "-bar", "value"}
	assert.Equal(t, expected, actual)

	actual = newArgs(mockArgs).Normalize(DoubleDashFlag).Slice()
	expected = []string{"one", "--foo", "two", "--bar", "value"}
	assert.Equal(t, expected, actual)
}
