// Package azurerm provides retry logic for Azure backend operations
package azurerm

import (
	"context"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Constants for retry configuration defaults
const (
	DefaultMaxRetries          = 3
	DefaultInitialDelaySeconds = 1
	DefaultMaxDelaySeconds     = 30
	DefaultBackoffMultiple     = 2.0
)

// Constants for jitter calculation
const (
	JitterDivisor        = 4      // For 25% jitter calculation
	JitterModulo         = 1000   // For random jitter generation
	JitterDivisorFloat64 = 1000.0 // Float64 version for division
)

// RetryConfig holds configuration for retry behavior when interacting with Azure services.
// This configuration controls how Terragrunt retries operations that fail due to
// transient errors such as network timeouts, service throttling, or temporary
// Azure service unavailability.
//
// The retry logic implements exponential backoff with optional jitter to avoid
// thundering herd problems when multiple operations retry simultaneously.
//
// Usage examples:
//
//	// Basic retry configuration
//	config := RetryConfig{
//	    MaxRetries:      3,
//	    InitialDelay:    2 * time.Second,
//	    MaxDelay:        30 * time.Second,
//	    BackoffMultiple: 2.0,
//	    Jitter:          true,
//	}
//
//	// Conservative retry configuration for production
//	config := RetryConfig{
//	    MaxRetries:      5,
//	    InitialDelay:    1 * time.Second,
//	    MaxDelay:        60 * time.Second,
//	    BackoffMultiple: 1.5,
//	    Jitter:          true,
//	}
//
//	// Aggressive retry configuration for development
//	config := RetryConfig{
//	    MaxRetries:      10,
//	    InitialDelay:    500 * time.Millisecond,
//	    MaxDelay:        15 * time.Second,
//	    BackoffMultiple: 2.0,
//	    Jitter:          false,
//	}
type RetryConfig struct {
	// MaxRetries specifies the maximum number of retry attempts.
	// After this many attempts, the operation will fail with the last error.
	// A value of 0 means no retries (fail immediately on first error).
	// Higher values provide more resilience but increase operation time.
	// Recommended range: 3-10 for most operations.
	// Default: 5
	MaxRetries int

	// InitialDelay specifies the initial delay before the first retry attempt.
	// This is the base delay that gets multiplied by BackoffMultiple for each retry.
	// Should be long enough to allow transient issues to resolve.
	// Too short values may not give Azure services time to recover.
	// Recommended range: 1-5 seconds for most operations.
	// Default: 2 seconds
	InitialDelay time.Duration

	// MaxDelay specifies the maximum delay between retry attempts.
	// This caps the exponential backoff to prevent excessively long waits.
	// Should be balanced between giving Azure services time to recover
	// and not making operations unacceptably slow.
	// Recommended range: 30-120 seconds for most operations.
	// Default: 60 seconds
	MaxDelay time.Duration

	// BackoffMultiple specifies the exponential backoff multiplier.
	// Each retry delay is calculated as: min(InitialDelay * BackoffMultiple^attempt, MaxDelay).
	// A value of 1.0 provides constant delays (no exponential backoff).
	// Higher values increase delays more aggressively.
	// Values greater than 3.0 may cause delays to reach MaxDelay too quickly.
	// Recommended range: 1.5-2.0 for most operations.
	// Default: 2.0
	BackoffMultiple float64

	// Jitter indicates whether to add random jitter to delay calculations.
	// When true, adds up to 20% random variation to delays to prevent
	// thundering herd problems when multiple operations retry simultaneously.
	// Recommended: true for most operations, especially in concurrent scenarios.
	// Set to false only for predictable delay requirements in testing.
	// Default: true
	Jitter bool
}

// DefaultRetryConfig returns a sensible default retry configuration for Azure operations
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      DefaultMaxRetries,
		InitialDelay:    DefaultInitialDelaySeconds * time.Second,
		MaxDelay:        DefaultMaxDelaySeconds * time.Second,
		BackoffMultiple: DefaultBackoffMultiple,
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
			return errors.Errorf("operation %s cancelled: %w", operation, ctx.Err())
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
			return errors.Errorf("operation %s cancelled during retry delay: %w", operation, ctx.Err())
		case <-timer.C:
		}
	}

	// All retries exhausted
	totalElapsed := time.Since(startTime)
	logger.Errorf("Operation %s failed after %d retries (elapsed: %v): %v",
		operation, config.MaxRetries+1, totalElapsed, lastErr)

	return WrapMaxRetriesExceededError(lastErr, operation, config.MaxRetries, totalElapsed)
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
		jitterRange := duration / JitterDivisor // 25% jitter
		// Generate random jitter between 0 and jitterRange
		nanoTime := time.Now().UnixNano()
		jitterMultiplier := float64(nanoTime%JitterModulo) / JitterDivisorFloat64 // 0.0 to 1.0
		jitter := time.Duration(float64(jitterRange) * jitterMultiplier)
		duration += jitter
	}

	return duration
}

// WrapTransientError wraps an error as a transient Azure error if it matches transient patterns
func WrapTransientError(err error, operation string) error {
	if err == nil {
		return nil
	}

	// Use ConvertAzureError for better error analysis
	azureErr := azurehelper.ConvertAzureError(err)

	var statusCode int
	if azureErr != nil {
		statusCode = azureErr.StatusCode
	} else {
		// Fallback to extracting status code from string if ConvertAzureError fails
		statusCode = extractStatusCode(err.Error())
	}

	if IsRetryableError(err) {
		return WrapTransientAzureError(err, operation, statusCode)
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
