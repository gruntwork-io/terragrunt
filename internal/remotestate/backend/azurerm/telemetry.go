// Package azurerm provides telemetry and structured logging for Azure backend operations.
// This module tracks error patterns, operation metrics, and provides detailed logging
// for debugging and monitoring Azure backend interactions.
package azurerm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/azure/azureutil"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

// TelemetryCollectorSettings defines settings for the telemetry collector
type TelemetryCollectorSettings struct {
	// EnableDetailedMetrics enables collection of detailed performance metrics
	EnableDetailedMetrics bool
	// BufferSize sets the buffer size for metrics collection
	BufferSize int
	// EnableCaching enables caching of telemetry data
	EnableCaching bool
	// FlushInterval sets how often to flush telemetry data (in seconds)
	FlushInterval time.Duration
}

// TelemetryAdapter provides a bridge between azureutil.TelemetryCollector and AzureTelemetryCollector
type TelemetryAdapter struct {
	collector *AzureTelemetryCollector
	logger    log.Logger
}

// NewTelemetryAdapter creates a new adapter that implements azureutil.TelemetryCollector
func NewTelemetryAdapter(collector *AzureTelemetryCollector, l log.Logger) *TelemetryAdapter {
	return &TelemetryAdapter{
		collector: collector,
		logger:    l,
	}
}

// LogError implements azureutil.TelemetryCollector
func (a *TelemetryAdapter) LogError(ctx context.Context, err error, opType azureutil.OperationType, metrics azureutil.ErrorMetrics) {
	if a.collector != nil {
		// Convert azureutil types to our internal types
		azureMetrics := AzureErrorMetrics{
			ErrorType:      metrics.ErrorType,
			Classification: ErrorClassification(metrics.Classification),
			Operation:      OperationType(opType),
			ResourceType:   metrics.ResourceType,
			ResourceName:   metrics.ResourceName,
			SubscriptionID: metrics.SubscriptionID,
			Location:       metrics.Location,
			ErrorMessage:   metrics.ErrorMessage,
			StatusCode:     metrics.StatusCode,
			RetryAttempts:  metrics.RetryAttempts,
			IsRetryable:    metrics.IsRetryable,
		}

		// Log through our collector
		a.collector.LogError(ctx, err, OperationType(opType), azureMetrics)
	}
}

// LogOperation implements azureutil.TelemetryCollector
func (a *TelemetryAdapter) LogOperation(ctx context.Context, operation azureutil.OperationType, duration time.Duration, attrs map[string]interface{}) {
	if a.collector != nil {
		// Convert azureutil.OperationType to our internal OperationType
		localOp := OperationType(string(operation))
		a.collector.LogOperation(ctx, localOp, duration, attrs)
	}
}

// AzureTelemetryCollector provides structured telemetry collection for Azure backend operations
type AzureTelemetryCollector struct {
	telemeter *telemetry.Telemeter
	logger    log.Logger
}

// NewAzureTelemetryCollector creates a new telemetry collector for Azure operations
func NewAzureTelemetryCollector(l log.Logger) *AzureTelemetryCollector {
	return &AzureTelemetryCollector{
		logger: l,
	}
}

// NewAzureTelemetryCollectorWithSettings creates a new telemetry collector with specific settings
func NewAzureTelemetryCollectorWithSettings(l log.Logger, settings *TelemetryCollectorSettings) *AzureTelemetryCollector {
	collector := &AzureTelemetryCollector{
		logger: l,
	}

	// Apply settings to collector if provided
	if settings != nil {
		// For now, we store the settings for future use
		// In a full implementation, these would configure internal behavior
		_ = settings
	}

	return collector
}

// ErrorClassification represents different categories of Azure errors for telemetry
type ErrorClassification string

const (
	ErrorClassUnknown          ErrorClassification = "unknown"
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
	OperationBootstrap       OperationType = "bootstrap"
	OperationNeedsBootstrap  OperationType = "needs_bootstrap"
	OperationDelete          OperationType = "delete"
	OperationDeleteContainer OperationType = "delete_container"
	OperationDeleteAccount   OperationType = "delete_account"
	OperationMigrate         OperationType = "migrate"
	OperationContainerOp     OperationType = "container_operation"
	OperationStorageOp       OperationType = "storage_operation"
	OperationValidation      OperationType = "validation"
	OperationAuthentication  OperationType = "authentication"

	// New interface-based operations
	OperationBlobGet         OperationType = "blob_get"
	OperationBlobPut         OperationType = "blob_put"
	OperationBlobDelete      OperationType = "blob_delete"
	OperationBlobExists      OperationType = "blob_exists"
	OperationBlobList        OperationType = "blob_list"
	OperationContainerCreate OperationType = "container_create"
	OperationContainerDelete OperationType = "container_delete"
	OperationContainerExists OperationType = "container_exists"
	OperationStorageCreate   OperationType = "storage_create"
	OperationStorageDelete   OperationType = "storage_delete"
	OperationStorageExists   OperationType = "storage_exists"
	OperationStorageUpdate   OperationType = "storage_update"
	OperationVersionCheck    OperationType = "version_check"
	OperationRoleAssign      OperationType = "role_assign"
	OperationRoleRevoke      OperationType = "role_revoke"
	OperationRoleList        OperationType = "role_list"
	OperationAuthRefresh     OperationType = "auth_refresh"
	OperationAuthValidate    OperationType = "auth_validate"
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

	// First try to use ConvertAzureError for structured error analysis
	azureErr := azurehelper.ConvertAzureError(err)
	if azureErr != nil {
		// Use status codes for classification when available
		switch azureErr.StatusCode {
		case 401, 403: //nolint: mnd
			return ErrorClassAuthentication
		case 404: //nolint: mnd
			return ErrorClassResourceNotFound
		case 429: //nolint: mnd
			return ErrorClassTransient
		case 500, 502, 503, 504: //nolint: mnd
			return ErrorClassTransient
		}

		// Check specific Azure error codes
		switch azureErr.ErrorCode {
		case "StorageAccountNotFound":
			return ErrorClassResourceNotFound
		case "ContainerNotFound":
			return ErrorClassResourceNotFound
		case "AuthorizationFailed", "Forbidden", "Unauthorized":
			return ErrorClassAuthentication
		case "InsufficientAccountPermissions", "AccessDenied":
			return ErrorClassPermissions
		case "ThrottledRequest", "TooManyRequests":
			return ErrorClassTransient
		case "InternalError", "ServiceUnavailable":
			return ErrorClassTransient
		}
	}

	// Fallback to string-based detection for non-Azure errors or when ConvertAzureError fails
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

	// Permission errors - enhanced detection matching azurehelper.IsPermissionError patterns
	if strings.Contains(errStr, "permission") || strings.Contains(errStr, "access denied") ||
		strings.Contains(errStr, "insufficient") || strings.Contains(errStr, "forbidden") ||
		strings.Contains(errStr, "not authorized") || strings.Contains(errStr, "authorization failed") ||
		strings.Contains(errStr, "role assignment") || strings.Contains(errStr, "storage blob data owner") {
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

// LogError records an error with context for telemetry
func (t *AzureTelemetryCollector) LogErrorWithMetrics(ctx context.Context, err error, opType OperationType, metrics AzureErrorMetrics) {
	if t == nil {
		return
	}

	if err == nil {
		return
	}

	// Convert telemetry types if needed
	var operation OperationType
	switch opType {
	case "bootstrap":
		operation = OperationBootstrap
	case "delete":
		operation = OperationDelete
	default:
		operation = OperationType(opType)
	}

	// Convert ErrorMetrics to AzureErrorMetrics
	azureMetrics := AzureErrorMetrics{
		ErrorType:      metrics.ErrorType,
		Classification: ErrorClassification(metrics.Classification),
		Operation:      operation,
		ResourceType:   metrics.ResourceType,
		ResourceName:   metrics.ResourceName,
		SubscriptionID: metrics.SubscriptionID,
		Location:       metrics.Location,
		ErrorMessage:   metrics.ErrorMessage,
		StatusCode:     metrics.StatusCode,
		RetryAttempts:  metrics.RetryAttempts,
		IsRetryable:    metrics.IsRetryable,
	}

	if t.telemeter != nil {
		// Count the error occurrence
		t.telemeter.Count(ctx, "azure_backend_errors", 1)
	}

	// Log the error details
	errorDetails := fmt.Sprintf("Error Type: %s, Classification: %s, Operation: %s",
		azureMetrics.ErrorType, azureMetrics.Classification, azureMetrics.Operation)

	if azureMetrics.ResourceType != "" {
		errorDetails += fmt.Sprintf(", Resource Type: %s", azureMetrics.ResourceType)
	}
	if azureMetrics.ResourceName != "" {
		errorDetails += fmt.Sprintf(", Resource Name: %s", azureMetrics.ResourceName)
	}
	if azureMetrics.StatusCode != 0 {
		errorDetails += fmt.Sprintf(", Status Code: %d", azureMetrics.StatusCode)
	}

	t.logger.Errorf("Azure backend error: %v (%s)", err, errorDetails)
}
