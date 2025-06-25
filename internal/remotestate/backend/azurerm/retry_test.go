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
	assert.Equal(t, 2.0, config.BackoffMultiple)
	assert.True(t, config.Jitter)
}

func TestWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
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

func TestWithRetry_SuccessAfterRetries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
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
	assert.True(t, elapsed >= 30*time.Millisecond) // At least two delays
	assert.True(t, elapsed < 1*time.Second)        // But not too long
}

func TestWithRetry_MaxRetriesExceeded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
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
	require.True(t, errors.As(err, &maxRetriesErr))
	assert.Equal(t, "test operation", maxRetriesErr.Operation)
	assert.Equal(t, 2, maxRetriesErr.MaxRetries)
	assert.Contains(t, maxRetriesErr.Error(), "failed after 2 retries")
}

func TestWithRetry_NonRetryableError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
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

func TestWithRetry_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
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
		name        string
		error       error
		shouldRetry bool
	}{
		{
			name:        "nil error",
			error:       nil,
			shouldRetry: false,
		},
		{
			name: "transient azure error - retryable",
			error: azurerm.TransientAzureError{
				Underlying: errors.New("service unavailable"),
				Operation:  "test",
				StatusCode: 503,
			},
			shouldRetry: true,
		},
		{
			name: "transient azure error - non-retryable",
			error: azurerm.TransientAzureError{
				Underlying: errors.New("not found"),
				Operation:  "test",
				StatusCode: 404,
			},
			shouldRetry: false,
		},
		{
			name:        "HTTP 429 in message",
			error:       errors.New("request failed with status 429"),
			shouldRetry: true,
		},
		{
			name:        "too many requests in message",
			error:       errors.New("too many requests"),
			shouldRetry: true,
		},
		{
			name:        "HTTP 500 in message",
			error:       errors.New("internal server error 500"),
			shouldRetry: true,
		},
		{
			name:        "HTTP 503 in message",
			error:       errors.New("service unavailable 503"),
			shouldRetry: true,
		},
		{
			name:        "connection reset",
			error:       errors.New("connection reset by peer"),
			shouldRetry: true,
		},
		{
			name:        "timeout error",
			error:       errors.New("request timeout occurred"),
			shouldRetry: true,
		},
		{
			name:        "throttled error",
			error:       errors.New("request was throttled"),
			shouldRetry: true,
		},
		{
			name:        "non-retryable error",
			error:       errors.New("authentication failed"),
			shouldRetry: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
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
		assert.True(t, delay >= 2*time.Second, "Delay should be at least base delay")
		assert.True(t, delay <= 3*time.Second, "Delay should not be too much more than base + jitter")
	}
}

func TestWrapTransientError(t *testing.T) {
	t.Parallel()

	t.Run("nil error", func(t *testing.T) {
		result := azurerm.WrapTransientError(nil, "test")
		assert.Nil(t, result)
	})

	t.Run("retryable error", func(t *testing.T) {
		originalErr := errors.New("service unavailable 503")
		result := azurerm.WrapTransientError(originalErr, "test operation")

		var transientErr azurerm.TransientAzureError
		require.True(t, errors.As(result, &transientErr))
		assert.Equal(t, originalErr, transientErr.Underlying)
		assert.Equal(t, "test operation", transientErr.Operation)
		assert.Equal(t, 503, transientErr.StatusCode)
	})

	t.Run("non-retryable error", func(t *testing.T) {
		originalErr := errors.New("authentication failed")
		result := azurerm.WrapTransientError(originalErr, "test operation")

		assert.Equal(t, originalErr, result)
	})
}

func TestTransientAzureError_IsRetryable(t *testing.T) {
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
