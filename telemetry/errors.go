package telemetry

import "fmt"

// ErrorMissingEnvVariable error for missing environment variable.
type ErrorMissingEnvVariable struct {
	Vars []string
}

func (e *ErrorMissingEnvVariable) Error() string {
	return fmt.Sprintf("missing environment variable: %v", e.Vars)
}
