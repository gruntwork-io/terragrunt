package awsproviderpatch

import "fmt"

// MissingOverrideAttrError is an error type that is returned when the user
// does not specify any provider attributes to override.
type MissingOverrideAttrError string

// Error returns a string representation of the error.
func (flagName MissingOverrideAttrError) Error() string {
	return fmt.Sprintf("You must specify at least one provider attribute to override via the --%s option.", string(flagName)) //nolint:lll
}

// TypeInferenceError is an error type that is returned when the user
// specifies an invalid provider attribute to override.
type TypeInferenceError struct {
	value         string
	underlyingErr error
}

// Error returns a string representation of the error.
func (err TypeInferenceError) Error() string {
	val := err.value

	return fmt.Sprintf(`Could not determine underlying type of JSON string %s. This usually happens when the JSON string is malformed, or if the value is not properly quoted (e.g., "%s"). Underlying error: %s`, val, val, err.underlyingErr) //nolint:lll
}

// MalformedJSONValError is an error type that is returned when the user
// specifies a malformed JSON string.
type MalformedJSONValError struct {
	value         string
	underlyingErr error
}

// Error returns a string representation of the error.
func (err MalformedJSONValError) Error() string {
	val := err.value

	return fmt.Sprintf(`Error unmarshaling JSON string %s. This usually happens when the JSON string is malformed, or if the value is not properly quoted (e.g., "%s"). Underlying error: %s`, val, val, err.underlyingErr) //nolint:lll
}
