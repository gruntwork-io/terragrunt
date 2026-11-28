package awsproviderpatch

import "fmt"

// MissingOverrideAttrError reports that `terragrunt aws-provider-patch` ran
// without an attribute to override. Its value is the name of the flag that sets
// one.
type MissingOverrideAttrError string

func (flagName MissingOverrideAttrError) Error() string {
	return fmt.Sprintf(
		"You must specify at least one provider attribute to override via the --%s option.",
		string(flagName),
	)
}

// TypeInferenceError reports an override value whose cty type cannot be
// inferred by parsing it as JSON.
type TypeInferenceError struct {
	underlyingErr error
	value         string
}

func (err TypeInferenceError) Error() string {
	val := err.value

	return fmt.Sprintf(
		`Could not determine underlying type of JSON string %s. This usually happens when the JSON string is malformed, or if the value is not properly quoted (e.g., "%s"). Underlying error: %s`,
		val,
		val,
		err.underlyingErr,
	)
}

// MalformedJSONValError reports an override value that fails to unmarshal as
// JSON into the cty type inferred for it.
type MalformedJSONValError struct {
	underlyingErr error
	value         string
}

func (err MalformedJSONValError) Error() string {
	val := err.value

	return fmt.Sprintf(
		`Error unmarshaling JSON string %s. This usually happens when the JSON string is malformed, or if the value is not properly quoted (e.g., "%s"). Underlying error: %s`,
		val,
		val,
		err.underlyingErr,
	)
}
