package awsproviderpatch

import "fmt"

type MissingOverrideAttrError string

func (flagName MissingOverrideAttrError) Error() string {
	return fmt.Sprintf("You must specify at least one provider attribute to override via the --%s option.", string(flagName))
}

type TypeInferenceError struct {
	value         string
	underlyingErr error
}

func (err TypeInferenceError) Error() string {
	val := err.value
	return fmt.Sprintf(`Could not determine underlying type of JSON string %s. This usually happens when the JSON string is malformed, or if the value is not properly quoted (e.g., "%s"). Underlying error: %s`, val, val, err.underlyingErr)
}

type MalformedJSONValError struct {
	value         string
	underlyingErr error
}

func (err MalformedJSONValError) Error() string {
	val := err.value
	return fmt.Sprintf(`Error unmarshaling JSON string %s. This usually happens when the JSON string is malformed, or if the value is not properly quoted (e.g., "%s"). Underlying error: %s`, val, val, err.underlyingErr)
}
