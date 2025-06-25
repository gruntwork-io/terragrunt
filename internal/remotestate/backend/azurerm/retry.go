// Package azurerm provides retry logic for Azure backend operations
package azurerm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	tgerrors "github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// RetryConfig holds configuration for retry behavior
type RetryConfig struct {
	MaxRetries      int           // Maximum number of retry attempts
	InitialDelay    time.Duration // Initial delay between retries
	MaxDelay        time.Duration // Maximum delay between retries
	BackoffMultiple float64       // Exponential backoff multiplier
	Jitter          bool          // Whether to add jitter to delay
}

// DefaultRetryConfig returns a sensible default retry configuration for Azure operations
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      3,
		InitialDelay:    1 * time.Second,
		MaxDelay:        30 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          true,
	}
}

// RetryableOperation represents an operation that can be retried
type RetryableOperation func() error

// WithRetry executes an operation with exponential backoff retry logic for transient Azure errors
func WithRetry(ctx context.Context, logger log.Logger, operation string, config RetryConfig, op RetryableOperation) error {
	startTime := time.Now()
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("operation %s cancelled: %w", operation, ctx.Err())
		default:
		}

		err := op()
		if err == nil {
			if attempt > 0 {
				logger.Infof("Operation %s succeeded on attempt %d", operation, attempt+1)
			}
			return nil
		}

		lastErr = err

		// Check if this is a retryable error
		if !IsRetryableError(err) {
			logger.Debugf("Operation %s failed with non-retryable error: %v", operation, err)
			return err
		}

		// Don't sleep after the last attempt
		if attempt == config.MaxRetries {
			break
		}

		// Calculate delay with exponential backoff
		delay := CalculateDelay(attempt, config)

		logger.Warnf("Operation %s failed (attempt %d/%d), retrying in %v: %v",
			operation, attempt+1, config.MaxRetries+1, delay, err)

		// Sleep with context awareness
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("operation %s cancelled during retry delay: %w", operation, ctx.Err())
		case <-timer.C:
			// Continue to next retry
		}
	}

	// All retries exhausted
	totalElapsed := time.Since(startTime)
	logger.Errorf("Operation %s failed after %d retries (elapsed: %v): %v",
		operation, config.MaxRetries+1, totalElapsed, lastErr)

	return tgerrors.New(MaxRetriesExceededError{
		Underlying:   lastErr,
		Operation:    operation,
		MaxRetries:   config.MaxRetries,
		TotalElapsed: totalElapsed,
	})
}

// IsRetryableError determines if an error represents a transient condition that should be retried
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's already marked as a transient error
	var transientErr TransientAzureError
	if errors.As(err, &transientErr) {
		return transientErr.IsRetryable()
	}

	// Check for HTTP status codes in error messages (common pattern from Azure SDK)
	errorStr := strings.ToLower(err.Error())

	// HTTP 429 - Too Many Requests
	if strings.Contains(errorStr, "429") || strings.Contains(errorStr, "too many requests") ||
		strings.Contains(errorStr, "rate limit") {
		return true
	}

	// HTTP 500 - Internal Server Error
	if strings.Contains(errorStr, "500") || strings.Contains(errorStr, "internal server error") {
		return true
	}

	// HTTP 502 - Bad Gateway
	if strings.Contains(errorStr, "502") || strings.Contains(errorStr, "bad gateway") {
		return true
	}

	// HTTP 503 - Service Unavailable
	if strings.Contains(errorStr, "503") || strings.Contains(errorStr, "service unavailable") ||
		strings.Contains(errorStr, "temporarily unavailable") {
		return true
	}

	// HTTP 504 - Gateway Timeout
	if strings.Contains(errorStr, "504") || strings.Contains(errorStr, "gateway timeout") ||
		strings.Contains(errorStr, "request timeout") {
		return true
	}

	// Network-related errors
	if strings.Contains(errorStr, "connection reset") ||
		strings.Contains(errorStr, "connection refused") ||
		strings.Contains(errorStr, "network is unreachable") ||
		strings.Contains(errorStr, "timeout") ||
		strings.Contains(errorStr, "temporary failure") {
		return true
	}

	// Azure-specific transient errors
	if strings.Contains(errorStr, "throttled") ||
		strings.Contains(errorStr, "server busy") ||
		strings.Contains(errorStr, "operation timeout") ||
		strings.Contains(errorStr, "request rate is large") {
		return true
	}

	return false
}

// CalculateDelay computes the delay for the given attempt using exponential backoff
func CalculateDelay(attempt int, config RetryConfig) time.Duration {
	// Exponential backoff: delay = initialDelay * (backoffMultiple ^ attempt)
	delay := float64(config.InitialDelay) * math.Pow(config.BackoffMultiple, float64(attempt))

	// Cap at maximum delay
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	duration := time.Duration(delay)

	// Add jitter to avoid thundering herd
	if config.Jitter {
		jitterRange := duration / 4 // 25% jitter
		// Generate random jitter between 0 and jitterRange
		nanoTime := time.Now().UnixNano()
		jitterMultiplier := float64(nanoTime%1000) / 1000.0 // 0.0 to 1.0
		jitter := time.Duration(float64(jitterRange) * jitterMultiplier)
		duration = duration + jitter
	}

	return duration
}

// WrapTransientError wraps an error as a transient Azure error if it matches transient patterns
func WrapTransientError(err error, operation string) error {
	if err == nil {
		return nil
	}

	// Extract status code if possible (basic pattern matching)
	statusCode := extractStatusCode(err.Error())

	if IsRetryableError(err) {
		return tgerrors.New(TransientAzureError{
			Underlying: err,
			Operation:  operation,
			StatusCode: statusCode,
		})
	}

	return err
}

// extractStatusCode attempts to extract HTTP status code from error message
func extractStatusCode(errorStr string) int {
	// Common patterns for status codes in error messages
	if strings.Contains(errorStr, "429") {
		return http.StatusTooManyRequests
	}
	if strings.Contains(errorStr, "500") {
		return http.StatusInternalServerError
	}
	if strings.Contains(errorStr, "502") {
		return http.StatusBadGateway
	}
	if strings.Contains(errorStr, "503") {
		return http.StatusServiceUnavailable
	}
	if strings.Contains(errorStr, "504") {
		return http.StatusGatewayTimeout
	}

	return 0 // Unknown status code
}
