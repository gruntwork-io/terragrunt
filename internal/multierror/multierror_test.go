package multierror_test

import (
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/multierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJoinNilWhenAllNil(t *testing.T) {
	t.Parallel()

	assert.NoError(t, multierror.Join())
	assert.NoError(t, multierror.Join(nil, nil))
}

func TestJoinDropsNil(t *testing.T) {
	t.Parallel()

	err := multierror.Join(nil, errors.New("boom"), nil)
	require.Error(t, err)
	assert.Equal(t, "error occurred:\n\n* boom\n", err.Error())
}

func TestJoinFormatsSingle(t *testing.T) {
	t.Parallel()

	err := multierror.Join(errors.New("first line\nsecond line"))
	require.Error(t, err)
	assert.Equal(t, "error occurred:\n\n* first line\n  second line\n", err.Error())
}

func TestJoinFormatsMultiple(t *testing.T) {
	t.Parallel()

	err := multierror.Join(errors.New("one"), errors.New("two"))
	require.Error(t, err)
	assert.Equal(t, "2 errors occurred:\n\n* one\n\n* two\n", err.Error())
}

func TestJoinFlattensNestedJoins(t *testing.T) {
	t.Parallel()

	// Both stdlib errors.Join and multierror.Join nest must flatten into a single list.
	inner := errors.Join(errors.New("two"), errors.New("three"))
	err := multierror.Join(errors.New("one"), inner, multierror.Join(errors.New("four")))

	require.Error(t, err)
	assert.Equal(t, "4 errors occurred:\n\n* one\n\n* two\n\n* three\n\n* four\n", err.Error())
}

func TestJoinPreservesErrorsIsAndAs(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("sentinel")
	err := multierror.Join(errors.New("noise"), errors.Join(sentinel))

	require.ErrorIs(t, err, sentinel)

	var target *customError
	require.ErrorAs(t, multierror.Join(&customError{}), &target)
}

func TestJoinBoundsRecursionDepth(t *testing.T) {
	t.Parallel()

	err := errors.New("leaf")
	for range 10_000 {
		err = errors.Join(err)
	}

	joined := multierror.Join(err)
	require.Error(t, joined)
	require.ErrorContains(t, joined, "leaf")
}

type customError struct{}

func (*customError) Error() string { return "custom" }
