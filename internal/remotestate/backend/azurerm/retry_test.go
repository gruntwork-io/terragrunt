package azurerm_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	azurerm "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestLogger() log.Logger {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	return log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter))
}

func TestDefaultRetryConfig(t *testing.T) {
	t.Parallel()

	config := azurerm.DefaultRetryConfig()

	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.InitialDelay)
	assert.Equal(t, 30*time.Second, config.MaxDelay)
	assert.InEpsilon(t, 2.0, config.BackoffMultiple, 0.001)
	assert.True(t, config.Jitter)
}

func TestWithRetrySuccessOnFirstAttempt(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := createTestLogger()
	config := azurerm.RetryConfig{
		MaxRetries:      3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	callCount := 0
	operation := func() error {
		callCount++

		return nil
	}

	err := azurerm.WithRetry(ctx, logger, "test operation", config, operation)

	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestWithRetrySuccessAfterRetries(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := createTestLogger()
	config := azurerm.RetryConfig{
		MaxRetries:      3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	callCount := 0
	operation := func() error {
		callCount++
		if callCount <= 2 {
			return azurerm.TransientAzureError{
				Underlying: errors.New("temporary failure"),
				Operation:  "test",
				StatusCode: 503,
			}
		}

		return nil
	}

	start := time.Now()
	err := azurerm.WithRetry(ctx, logger, "test operation", config, operation)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 3, callCount)
	// Should have some delay but not too much (we used small delays)
	assert.GreaterOrEqual(t, elapsed, 30*time.Millisecond) // At least two delays
	assert.Less(t, elapsed, 1*time.Second)                 // But not too long
}

func TestWithRetryMaxRetriesExceeded(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := createTestLogger()
	config := azurerm.RetryConfig{
		MaxRetries:      2,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	callCount := 0
	operation := func() error {
		callCount++

		return azurerm.TransientAzureError{
			Underlying: errors.New("permanent failure"),
			Operation:  "test",
			StatusCode: 503,
		}
	}

	err := azurerm.WithRetry(ctx, logger, "test operation", config, operation)

	require.Error(t, err)
	assert.Equal(t, 3, callCount) // MaxRetries + 1 = initial attempt + 2 retries

	var maxRetriesErr azurerm.MaxRetriesExceededError
	require.ErrorAs(t, err, &maxRetriesErr)
	assert.Equal(t, "test operation", maxRetriesErr.Operation)
	assert.Equal(t, 2, maxRetriesErr.MaxRetries)
	assert.Contains(t, maxRetriesErr.Error(), "failed after 2 retries")
}

func TestWithRetryNonRetryableError(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := createTestLogger()
	config := azurerm.RetryConfig{
		MaxRetries:      3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	callCount := 0
	nonRetryableErr := errors.New("non-retryable error")
	operation := func() error {
		callCount++
		return nonRetryableErr
	}

	err := azurerm.WithRetry(ctx, logger, "test operation", config, operation)

	require.Error(t, err)
	assert.Equal(t, 1, callCount) // Should not retry
	assert.Equal(t, nonRetryableErr, err)
}

func TestWithRetryContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	logger := createTestLogger()
	config := azurerm.RetryConfig{
		MaxRetries:      3,
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	callCount := 0
	operation := func() error {
		callCount++
		if callCount == 1 {
			// Cancel context after first failure to test cancellation during retry delay
			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()
		}

		return azurerm.TransientAzureError{
			Underlying: errors.New("transient failure"),
			Operation:  "test",
			StatusCode: 503,
		}
	}

	err := azurerm.WithRetry(ctx, logger, "test operation", config, operation)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
	assert.Equal(t, 1, callCount) // Should stop retrying when context is cancelled
}

func TestIsRetryableError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		error       error
		name        string
		shouldRetry bool
	}{
		{
			error:       nil,
			name:        "nil error",
			shouldRetry: false,
		},
		{
			error: azurerm.TransientAzureError{
				Underlying: errors.New("service unavailable"),
				Operation:  "test",
				StatusCode: 503,
			},
			name:        "transient azure error - retryable",
			shouldRetry: true,
		},
		{
			error: azurerm.TransientAzureError{
				Underlying: errors.New("not found"),
				Operation:  "test",
				StatusCode: 404,
			},
			name:        "transient azure error - non-retryable",
			shouldRetry: false,
		},
		{
			error:       errors.New("request failed with status 429"),
			name:        "HTTP 429 in message",
			shouldRetry: true,
		},
		{
			error:       errors.New("too many requests"),
			name:        "too many requests in message",
			shouldRetry: true,
		},
		{
			error:       errors.New("internal server error 500"),
			name:        "HTTP 500 in message",
			shouldRetry: true,
		},
		{
			error:       errors.New("service unavailable 503"),
			name:        "HTTP 503 in message",
			shouldRetry: true,
		},
		{
			error:       errors.New("connection reset by peer"),
			name:        "connection reset",
			shouldRetry: true,
		},
		{
			error:       errors.New("request timeout occurred"),
			name:        "timeout error",
			shouldRetry: true,
		},
		{
			error:       errors.New("request was throttled"),
			name:        "throttled error",
			shouldRetry: true,
		},
		{
			error:       errors.New("authentication failed"),
			name:        "non-retryable error",
			shouldRetry: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := azurerm.IsRetryableError(tc.error)
			assert.Equal(t, tc.shouldRetry, result, "Error: %v", tc.error)
		})
	}
}

func TestCalculateDelay(t *testing.T) {
	t.Parallel()

	config := azurerm.RetryConfig{
		InitialDelay:    1 * time.Second,
		MaxDelay:        10 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	testCases := []struct {
		attempt     int
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{0, 1 * time.Second, 1 * time.Second},
		{1, 2 * time.Second, 2 * time.Second},
		{2, 4 * time.Second, 4 * time.Second},
		{3, 8 * time.Second, 8 * time.Second},
		{4, 10 * time.Second, 10 * time.Second}, // Capped at MaxDelay
		{5, 10 * time.Second, 10 * time.Second}, // Still capped
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("attempt_%d", tc.attempt), func(t *testing.T) {
			t.Parallel()

			delay := azurerm.CalculateDelay(tc.attempt, config)
			assert.Equal(t, tc.expectedMin, delay)
		})
	}
}

func TestCalculateDelayWithJitter(t *testing.T) {
	t.Parallel()

	config := azurerm.RetryConfig{
		InitialDelay:    1 * time.Second,
		MaxDelay:        10 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          true,
	}

	// Test multiple times to check jitter variance
	delays := make([]time.Duration, 5)
	for i := 0; i < 5; i++ {
		delays[i] = azurerm.CalculateDelay(1, config)
	}

	// All delays should be around 2 seconds but with some variation
	for _, delay := range delays {
		assert.GreaterOrEqual(t, delay, 2*time.Second, "Delay should be at least base delay")
		assert.LessOrEqual(t, delay, 3*time.Second, "Delay should not be too much more than base + jitter")
	}
}

func TestWrapTransientError(t *testing.T) {
	t.Parallel()

	t.Run("nil error", func(t *testing.T) {
		t.Parallel()

		result := azurerm.WrapTransientError(nil, "test")
		assert.NoError(t, result)
	})

	t.Run("retryable error", func(t *testing.T) {
		t.Parallel()

		originalErr := errors.New("service unavailable 503")
		result := azurerm.WrapTransientError(originalErr, "test operation")

		var transientErr azurerm.TransientAzureError
		require.ErrorAs(t, result, &transientErr)
		assert.Equal(t, originalErr, transientErr.Underlying)
		assert.Equal(t, "test operation", transientErr.Operation)
		assert.Equal(t, 503, transientErr.StatusCode)
	})

	t.Run("non-retryable error", func(t *testing.T) {
		t.Parallel()

		originalErr := errors.New("authentication failed")
		result := azurerm.WrapTransientError(originalErr, "test operation")

		assert.Equal(t, originalErr, result)
	})
}

func TestTransientAzureErrorIsRetryable(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		statusCode  int
		shouldRetry bool
	}{
		{429, true},  // Too Many Requests
		{500, true},  // Internal Server Error
		{502, true},  // Bad Gateway
		{503, true},  // Service Unavailable
		{504, true},  // Gateway Timeout
		{400, false}, // Bad Request
		{401, false}, // Unauthorized
		{403, false}, // Forbidden
		{404, false}, // Not Found
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("status_%d", tc.statusCode), func(t *testing.T) {
			t.Parallel()

			err := azurerm.TransientAzureError{
				Underlying: errors.New("test error"),
				Operation:  "test",
				StatusCode: tc.statusCode,
			}

			assert.Equal(t, tc.shouldRetry, err.IsRetryable())
		})
	}
}

func TestMaxRetriesExceededError(t *testing.T) {
	t.Parallel()

	underlying := errors.New("underlying error")
	err := azurerm.MaxRetriesExceededError{
		Underlying:   underlying,
		Operation:    "test operation",
		MaxRetries:   3,
		TotalElapsed: 5 * time.Second,
	}

	assert.Contains(t, err.Error(), "test operation")
	assert.Contains(t, err.Error(), "failed after 3 retries")
	assert.Contains(t, err.Error(), "5s")
	assert.Equal(t, underlying, err.Unwrap())
}
