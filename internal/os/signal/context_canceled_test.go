package signal_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalFromContext(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		cause  error
		want   os.Signal
		name   string
		cancel bool
	}{
		{
			name: "context still running",
		},
		{
			name:   "canceled without a cause",
			cancel: true,
		},
		{
			name:   "canceled by an unrelated cause",
			cause:  errors.New("unrelated"),
			cancel: true,
		},
		{
			name:   "canceled by an error without a signal",
			cause:  signal.NewContextCanceledError(nil),
			cancel: true,
		},
		{
			name:   "canceled by a signal",
			cause:  signal.NewContextCanceledError(os.Interrupt),
			cancel: true,
			want:   os.Interrupt,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)

			if tc.cancel {
				cancel(tc.cause)
			}

			assert.Equal(t, tc.want, signal.SignalFromContext(ctx))
		})
	}
}

func TestContextCanceledError_IsContextCanceled(t *testing.T) {
	t.Parallel()

	err := signal.NewContextCanceledError(os.Interrupt)

	require.ErrorIs(t, err, context.Canceled, "the error should match context.Canceled")
	assert.Equal(t, context.Canceled.Error(), err.Error(), "the error should read as a plain cancellation")
	assert.Equal(t, os.Interrupt, err.Signal)
}
