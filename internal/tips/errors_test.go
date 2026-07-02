package tips_test

import (
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/stretchr/testify/require"
)

func TestInvalidTipNameErrorIs(t *testing.T) {
	t.Parallel()

	err := tips.NewInvalidTipNameError("foo", []string{"bar"})

	// errors.Is matches any InvalidTipNameError regardless of the requested name,
	// so a distinct instance (not the same pointer) still matches.
	require.ErrorIs(t, err, tips.NewInvalidTipNameError("other", nil))

	// A different error type does not match.
	require.NotErrorIs(t, err, io.EOF)
}
