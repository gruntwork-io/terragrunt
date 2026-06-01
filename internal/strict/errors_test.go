package strict_test

import (
	stderrors "errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/stretchr/testify/require"
)

func TestInvalidControlNameErrorIs(t *testing.T) {
	t.Parallel()

	err := strict.NewInvalidControlNameError([]string{"a", "b"})

	require.ErrorIs(t, err, &strict.InvalidControlNameError{})
	require.NotErrorIs(t, err, stderrors.New("other"))
}
