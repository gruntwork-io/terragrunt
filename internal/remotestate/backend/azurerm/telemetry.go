// Package azurerm provides telemetry and structured logging for Azure backend operations.
// This module tracks error patterns, operation metrics, and provides detailed logging
// for debugging and monitoring Azure backend interactions.
package azurerm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

// AzureTelemetryCollector provides structured telemetry collection for Azure backend operations
type AzureTelemetryCollector struct {
	telemeter *telemetry.Telemeter
	logger    log.Logger
}

// NewAzureTelemetryCollector creates a new telemetry collector for Azure operations
func NewAzureTelemetryCollector(telemeter *telemetry.Telemeter, logger log.Logger) *AzureTelemetryCollector {
	return &AzureTelemetryCollector{
		telemeter: telemeter,
		logger:    logger,
	}
}

// ErrorClassification represents different categories of Azure errors for telemetry
type ErrorClassification string

const (
	ErrorClassAuthentication   ErrorClassification = "authentication"
	ErrorClassConfiguration    ErrorClassification = "configuration"
	ErrorClassStorage          ErrorClassification = "storage"
	ErrorClassContainer        ErrorClassification = "container"
	ErrorClassNetwork          ErrorClassification = "network"
	ErrorClassPermissions      ErrorClassification = "permissions"
	ErrorClassTransient        ErrorClassification = "transient"
	ErrorClassValidation       ErrorClassification = "validation"
	ErrorClassResourceNotFound ErrorClassification = "resource_not_found"
	ErrorClassQuotaLimits      ErrorClassification = "quota_limits"
	ErrorClassUserInput        ErrorClassification = "user_input"
)

// OperationType represents different Azure backend operations for telemetry
type OperationType string

const (
	OperationBootstrap      OperationType = "bootstrap"
	OperationNeedsBootstrap OperationType = "needs_bootstrap"
	OperationDelete         OperationType = "delete"
	OperationDeleteBucket   OperationType = "delete_bucket"
	OperationMigrate        OperationType = "migrate"
	OperationContainerOp    OperationType = "container_operation"
	OperationStorageOp      OperationType = "storage_operation"
	OperationValidation     OperationType = "validation"
	OperationAuthentication OperationType = "authentication"
)

// AzureErrorMetrics is the metrics and context about Azure errors
type AzureErrorMetrics struct { // nolint: govet
	Additional     map[string]interface{} `json:"additional,omitempty"`      // 8-byte aligned (map)
	Duration       time.Duration          `json:"duration,omitempty"`        // 8-byte aligned (int64)
	ErrorType      string                 `json:"error_type"`                // 8-byte aligned (string)
	Classification ErrorClassification    `json:"classification"`            // 8-byte aligned (string)
	Operation      OperationType          `json:"operation"`                 // 8-byte aligned (string)
	ResourceType   string                 `json:"resource_type,omitempty"`   // 8-byte aligned (string)
	ResourceName   string                 `json:"resource_name,omitempty"`   // 8-byte aligned (string)
	SubscriptionID string                 `json:"subscription_id,omitempty"` // 8-byte aligned (string)
	Location       string                 `json:"location,omitempty"`        // 8-byte aligned (string)
	AuthMethod     string                 `json:"auth_method,omitempty"`     // 8-byte aligned (string)
	ErrorMessage   string                 `json:"error_message"`             // 8-byte aligned (string)
	StackTrace     string                 `json:"stack_trace,omitempty"`     // 8-byte aligned (string)
	StatusCode     int                    `json:"status_code,omitempty"`     // 4-byte aligned (int)
	RetryAttempts  int                    `json:"retry_attempts,omitempty"`  // 4-byte aligned (int)
	IsRetryable    bool                   `json:"is_retryable"`              // 1-byte aligned (bool)
}

// ClassifyError determines the classification of an Azure error for telemetry purposes
func ClassifyError(err error) ErrorClassification {
	if err == nil {
		return ""
	}

	errStr := strings.ToLower(err.Error())

	// Authentication errors
	if strings.Contains(errStr, "authentication") || strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "forbidden") || strings.Contains(errStr, "invalid credentials") ||
		strings.Contains(errStr, "token") || strings.Contains(errStr, "401") || strings.Contains(errStr, "403") {
		return ErrorClassAuthentication
	}

	// Configuration errors
	if strings.Contains(errStr, "missing") && (strings.Contains(errStr, "subscription") ||
		strings.Contains(errStr, "location") || strings.Contains(errStr, "resource group")) {
		return ErrorClassConfiguration
	}

	// Storage account errors
	if strings.Contains(errStr, "storage account") {
		if strings.Contains(errStr, "not found") || strings.Contains(errStr, "does not exist") {
			return ErrorClassResourceNotFound
		}

		return ErrorClassStorage
	}

	// Container errors
	if strings.Contains(errStr, "container") {
		if strings.Contains(errStr, "not found") || strings.Contains(errStr, "does not exist") {
			return ErrorClassResourceNotFound
		}

		if strings.Contains(errStr, "validation") || strings.Contains(errStr, "invalid") {
			return ErrorClassValidation
		}

		return ErrorClassContainer
	}

	// Network and transient errors
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "throttled") || strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "500") || strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") || strings.Contains(errStr, "504") {
		return ErrorClassTransient
	}

	// Permission errors
	if strings.Contains(errStr, "permission") || strings.Contains(errStr, "access denied") ||
		strings.Contains(errStr, "insufficient") {
		return ErrorClassPermissions
	}

	// Quota and limits
	if strings.Contains(errStr, "quota") || strings.Contains(errStr, "limit") ||
		strings.Contains(errStr, "exceeded") {
		return ErrorClassQuotaLimits
	}

	// Validation errors
	if strings.Contains(errStr, "validation") || strings.Contains(errStr, "invalid") ||
		strings.Contains(errStr, "must be") || strings.Contains(errStr, "cannot") {
		return ErrorClassValidation
	}

	// Default to user input for unclassified errors
	return ErrorClassUserInput
}

// LogError provides structured logging for Azure errors with telemetry collection
func (atc *AzureTelemetryCollector) LogError(ctx context.Context, err error, operation OperationType, metrics AzureErrorMetrics) {
	if err == nil {
		return
	}

	// Enhance metrics with classification if not already set
	if metrics.Classification == "" {
		metrics.Classification = ClassifyError(err)
	}

	if metrics.Operation == "" {
		metrics.Operation = operation
	}

	if metrics.ErrorMessage == "" {
		metrics.ErrorMessage = err.Error()
	}

	// Create structured log fields
	logFields := map[string]interface{}{
		"error_type":     metrics.ErrorType,
		"classification": string(metrics.Classification),
		"operation":      string(metrics.Operation),
		"is_retryable":   metrics.IsRetryable,
		"error_message":  metrics.ErrorMessage,
	}

	// Add optional fields if present
	if metrics.ResourceType != "" {
		logFields["resource_type"] = metrics.ResourceType
	}

	if metrics.ResourceName != "" {
		logFields["resource_name"] = metrics.ResourceName
	}

	if metrics.SubscriptionID != "" {
		logFields["subscription_id"] = MaskSubscriptionID(metrics.SubscriptionID)
	}

	if metrics.Location != "" {
		logFields["location"] = metrics.Location
	}

	if metrics.AuthMethod != "" {
		logFields["auth_method"] = metrics.AuthMethod
	}

	if metrics.StatusCode > 0 {
		logFields["status_code"] = metrics.StatusCode
	}

	if metrics.RetryAttempts > 0 {
		logFields["retry_attempts"] = metrics.RetryAttempts
	}

	if metrics.Duration > 0 {
		logFields["duration_ms"] = metrics.Duration.Milliseconds()
	}

	// Add any additional fields
	for k, v := range metrics.Additional {
		logFields[k] = v
	}

	// Log at appropriate level based on error classification
	switch metrics.Classification {
	case ErrorClassTransient:
		atc.logger.Warnf("Azure transient error: %s", FormatLogMessage(metrics, logFields))
	case ErrorClassConfiguration, ErrorClassValidation, ErrorClassUserInput:
		atc.logger.Errorf("Azure configuration error: %s", FormatLogMessage(metrics, logFields))
	case ErrorClassAuthentication, ErrorClassPermissions:
		atc.logger.Errorf("Azure authentication/permission error: %s", FormatLogMessage(metrics, logFields))
	case ErrorClassResourceNotFound:
		atc.logger.Warnf("Azure resource not found: %s", FormatLogMessage(metrics, logFields))
	case ErrorClassStorage, ErrorClassContainer:
		atc.logger.Errorf("Azure storage/container error: %s", FormatLogMessage(metrics, logFields))
	case ErrorClassNetwork:
		atc.logger.Warnf("Azure network error: %s", FormatLogMessage(metrics, logFields))
	case ErrorClassQuotaLimits:
		atc.logger.Errorf("Azure quota/limits error: %s", FormatLogMessage(metrics, logFields))
	default:
		atc.logger.Errorf("Azure error: %s", FormatLogMessage(metrics, logFields))
	}

	// Collect telemetry if telemeter is available
	if atc.telemeter != nil {
		atc.collectErrorTelemetry(ctx, metrics)
	}
}

// LogOperation provides structured logging for successful Azure operations
func (atc *AzureTelemetryCollector) LogOperation(ctx context.Context, operation OperationType, duration time.Duration, attrs map[string]interface{}) {
	logFields := map[string]interface{}{
		"operation":   string(operation),
		"duration_ms": duration.Milliseconds(),
		"status":      "success",
	}

	// Add additional attributes
	for k, v := range attrs {
		logFields[k] = v
	}

	atc.logger.Infof("Azure operation completed: %s", FormatSuccessMessage(operation, logFields))

	// Collect success telemetry
	if atc.telemeter != nil {
		atc.collectOperationTelemetry(ctx, operation, duration, logFields)
	}
}

// collectErrorTelemetry sends error metrics to the telemetry system
func (atc *AzureTelemetryCollector) collectErrorTelemetry(ctx context.Context, metrics AzureErrorMetrics) {
	telemetryAttrs := map[string]any{
		"error_classification": string(metrics.Classification),
		"operation_type":       string(metrics.Operation),
		"error_type":           metrics.ErrorType,
		"is_retryable":         metrics.IsRetryable,
	}

	// Add non-sensitive fields to telemetry
	if metrics.ResourceType != "" {
		telemetryAttrs["resource_type"] = metrics.ResourceType
	}

	if metrics.Location != "" {
		telemetryAttrs["location"] = metrics.Location
	}

	if metrics.AuthMethod != "" {
		telemetryAttrs["auth_method"] = metrics.AuthMethod
	}

	if metrics.StatusCode > 0 {
		telemetryAttrs["status_code"] = metrics.StatusCode
	}

	if metrics.RetryAttempts > 0 {
		telemetryAttrs["retry_attempts"] = metrics.RetryAttempts
	}

	// Collect telemetry for the error
	_ = atc.telemeter.Collect(ctx, "azure_backend_error", telemetryAttrs, func(childCtx context.Context) error {
		// This is just for telemetry collection, no actual operation
		return nil
	})
}

// collectOperationTelemetry sends operation metrics to the telemetry system
func (atc *AzureTelemetryCollector) collectOperationTelemetry(ctx context.Context, operation OperationType, duration time.Duration, attrs map[string]interface{}) {
	telemetryAttrs := map[string]any{
		"operation_type": string(operation),
		"duration_ms":    duration.Milliseconds(),
		"status":         "success",
	}

	// Add safe attributes to telemetry
	for k, v := range attrs {
		// Only include non-sensitive attributes in telemetry
		if !IsSensitiveAttribute(k) {
			telemetryAttrs[k] = v
		}
	}

	_ = atc.telemeter.Collect(ctx, "azure_backend_operation", telemetryAttrs, func(childCtx context.Context) error {
		return nil
	})
}

// Helper functions

// MaskSubscriptionID masks part of the subscription ID for privacy
// nolint: mnd
func MaskSubscriptionID(subscriptionID string) string {
	if len(subscriptionID) < 8 {
		return "****"
	}

	return subscriptionID[:4] + "****" + subscriptionID[len(subscriptionID)-4:]
}

// FormatLogMessage creates a human-readable log message with structured data
func FormatLogMessage(metrics AzureErrorMetrics, fields map[string]interface{}) string {
	var parts []string

	if metrics.Operation != "" {
		parts = append(parts, fmt.Sprintf("operation=%s", metrics.Operation))
	}

	if metrics.ResourceType != "" && metrics.ResourceName != "" {
		parts = append(parts, fmt.Sprintf("resource=%s/%s", metrics.ResourceType, metrics.ResourceName))
	}

	if metrics.StatusCode > 0 {
		parts = append(parts, fmt.Sprintf("status=%d", metrics.StatusCode))
	}

	if metrics.RetryAttempts > 0 {
		parts = append(parts, fmt.Sprintf("retries=%d", metrics.RetryAttempts))
	}

	message := metrics.ErrorMessage
	if len(parts) > 0 {
		message += fmt.Sprintf(" [%s]", strings.Join(parts, ", "))
	}

	return message
}

// FormatSuccessMessage creates a human-readable success log message
func FormatSuccessMessage(operation OperationType, fields map[string]interface{}) string {
	var parts []string

	if duration, ok := fields["duration_ms"].(int64); ok {
		parts = append(parts, fmt.Sprintf("duration=%dms", duration))
	}

	message := fmt.Sprintf("operation=%s", operation)
	if len(parts) > 0 {
		message += fmt.Sprintf(" [%s]", strings.Join(parts, ", "))
	}

	return message
}

// IsSensitiveAttribute checks if an attribute contains sensitive information
func IsSensitiveAttribute(key string) bool {
	sensitiveKeys := []string{
		"subscription_id", "client_id", "client_secret", "tenant_id",
		"access_key", "sas_token", "connection_string", "password",
	}

	keyLower := strings.ToLower(key)
	for _, sensitive := range sensitiveKeys {
		if strings.Contains(keyLower, sensitive) {
			return true
		}
	}

	return false
}
