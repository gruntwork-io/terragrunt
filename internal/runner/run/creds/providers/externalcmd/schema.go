package externalcmd

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
	"github.com/xeipuuv/gojsonschema"
)

// SchemaValidationError represents a schema validation error with details.
type SchemaValidationError struct {
	Errors []string
}

func (e *SchemaValidationError) Error() string {
	return fmt.Sprintf(
		"Auth provider command response schema validation failed with %d error(s): %v",
		len(e.Errors),
		e.Errors,
	)
}

// ValidateResponse validates a JSON response against the auth provider command schema.
// Returns nil if valid, or a SchemaValidationError with details if invalid.
func ValidateResponse(data []byte) error {
	schemaBytes, err := json.Marshal(generateResponseSchema())
	if err != nil {
		return fmt.Errorf("failed to generate schema: %w", err)
	}

	schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)
	documentLoader := gojsonschema.NewBytesLoader(data)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("failed to validate response: %w", err)
	}

	if !result.Valid() {
		errors := make([]string, len(result.Errors()))
		for i, validationErr := range result.Errors() {
			errors[i] = validationErr.String()
		}

		return &SchemaValidationError{Errors: errors}
	}

	return nil
}

// generateResponseSchema generates the JSON schema for auth provider command response validation.
func generateResponseSchema() *jsonschema.Schema {
	reflector := jsonschema.Reflector{
		DoNotReference: true,
	}

	schema := reflector.Reflect(&Response{})
	schema.Description = "Schema for the JSON response expected from an auth provider command"
	schema.Title = "Terragrunt Auth Provider Command Response Schema"
	schema.ID = "https://docs.terragrunt.com/schemas/auth-provider-cmd/v1/schema.json"

	return schema
}
