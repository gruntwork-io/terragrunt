package telemetry

import "fmt"

// ErrorMissingEnvVariable error for missing environment variable.
type ErrorMissingEnvVariable struct {
	Vars []string
}

func (e *ErrorMissingEnvVariable) Error() string {
	return fmt.Sprintf("missing environment variable: %v", e.Vars)
}

// ErrorUnknownLogsExporter is the error for a logs exporter type that Terragrunt does not support.
type ErrorUnknownLogsExporter struct {
	Value string
}

func (e *ErrorUnknownLogsExporter) Error() string {
	return fmt.Sprintf(
		"unknown logs exporter %q: set TG_TELEMETRY_LOGS_EXPORTER to none, console, otlpHttp or otlpGrpc",
		e.Value,
	)
}

// ErrorInvalidEnvVariable is the error for an environment variable that does not hold a whole number above zero.
type ErrorInvalidEnvVariable struct {
	Name  string
	Value string
}

func (e *ErrorInvalidEnvVariable) Error() string {
	return fmt.Sprintf("invalid value %q for %s: set a whole number greater than 0", e.Value, e.Name)
}

// ErrorLogRecordsNotConfirmed is the error for log records that the exporter did not confirm.
type ErrorLogRecordsNotConfirmed struct {
	Unconfirmed uint64
	Emitted     uint64
}

func (e *ErrorLogRecordsNotConfirmed) Error() string {
	return fmt.Sprintf(
		"%d of %d log records were not confirmed by the exporter: check the collector, or raise %s and %s",
		e.Unconfirmed, e.Emitted, maxQueueSizeEnvName, maxBatchSizeEnvName,
	)
}
