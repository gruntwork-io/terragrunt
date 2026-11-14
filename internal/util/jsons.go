package util

import (
	"encoding/json"
	"fmt"
	"strings"
)

// templateSequenceEscaper replaces the unescaped HCL template sequences ${ and %{ with their
// escaped forms $${ and %%{. Listing each escaped form first makes the replacement idempotent:
// an already-escaped $${ is matched at that position before ${ is tried, so it is emitted
// unchanged.
var templateSequenceEscaper = strings.NewReplacer("$${", "$${", "${", "$${", "%%{", "%%{", "%{", "%%{")

// AsTerraformEnvVarJSONValue converts the given value to a JSON value that can be passed to
// OpenTofu/Terraform as an environment variable that they parse as HCL. For the most part, this converts the value directly
// to JSON using Go's built-in json.Marshal. However, we have special handling
// for strings, which with normal JSON conversion would be wrapped in quotes, but when passing them to OpenTofu/Terraform via
// env vars, we need to NOT wrap them in quotes, so this method adds special handling for that case.
// For complex types (maps, lists, objects), string values containing ${...} or %{...} sequences are escaped to
// $${...} and %%{...} to prevent OpenTofu/Terraform's HCL parser from treating them as interpolations or
// template directives.
//
// A top-level string is returned as given. Whether its template sequences need escaping depends
// on the type constraint of the variable receiving it, so callers that know the constraint pass the
// value through [EscapeTemplateSequencesInString] first.
func AsTerraformEnvVarJSONValue(value any) (string, error) {
	switch val := value.(type) {
	case string:
		return val, nil
	default:
		escaped, err := escapeTemplateSequencesInValue(val, 0)
		if err != nil {
			return "", err
		}

		envVarValue, err := json.Marshal(escaped)
		if err != nil {
			return "", err
		}

		return string(envVarValue), nil
	}
}

// AsLiteralTerraformEnvVarJSONValue converts the given value to a JSON value for an environment
// variable that OpenTofu/Terraform read verbatim. Nothing is escaped, so a module that decodes the
// JSON gets every string as given.
//
// A top-level string is returned as given, without the quotes json.Marshal would add.
func AsLiteralTerraformEnvVarJSONValue(value any) (string, error) {
	if str, ok := value.(string); ok {
		return str, nil
	}

	envVarValue, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	return string(envVarValue), nil
}

// escapeTemplateSequencesInValue recursively walks a value tree and escapes the HCL template
// sequences ${...} and %{...} in string values to prevent OpenTofu/Terraform from treating them
// as interpolations or template directives when parsing complex type TF_VAR_* env vars.
//
// This unconditionally escapes both sequences in all string values within complex types.
// OpenTofu/Terraform's HCL parser would error on an unescaped one in a complex TF_VAR value.
//
// Nil maps and slices are preserved as nil so json.Marshal serializes them as null
// rather than {} or [], keeping OpenTofu/Terraform's null-vs-empty-collection semantics intact.
//
// Returns an error if the value tree is deeper than maxDepth (100), which prevents
// infinite recursion on malformed inputs.
func escapeTemplateSequencesInValue(value any, depth int) (any, error) {
	const maxDepth = 100
	if depth > maxDepth {
		return nil, fmt.Errorf(
			"escapeTemplateSequencesInValue: input exceeds maximum nesting depth of %d",
			maxDepth,
		)
	}

	switch v := value.(type) {
	case string:
		return EscapeTemplateSequencesInString(v), nil

	case map[string]any:
		if v == nil {
			return nil, nil
		}

		result := make(map[string]any, len(v))

		for key, val := range v {
			escaped, err := escapeTemplateSequencesInValue(val, depth+1)
			if err != nil {
				return nil, err
			}

			result[key] = escaped
		}

		return result, nil

	case []any:
		if v == nil {
			return nil, nil
		}

		result := make([]any, 0, len(v))

		for _, val := range v {
			escaped, err := escapeTemplateSequencesInValue(val, depth+1)
			if err != nil {
				return nil, err
			}

			result = append(result, escaped)
		}

		return result, nil

	case []string:
		if v == nil {
			return nil, nil
		}

		result := make([]any, 0, len(v))

		for _, s := range v {
			result = append(result, EscapeTemplateSequencesInString(s))
		}

		return result, nil

	case map[string]string:
		if v == nil {
			return nil, nil
		}

		result := make(map[string]any, len(v))

		for k, s := range v {
			result[k] = EscapeTemplateSequencesInString(s)
		}

		return result, nil

	default:
		return value, nil
	}
}

// EscapeTemplateSequencesInString escapes the HCL template sequences in a string by doubling
// their first character (${ → $${, %{ → %%{). This is idempotent: already-escaped $${ and %%{
// sequences are not double-escaped.
func EscapeTemplateSequencesInString(s string) string {
	return templateSequenceEscaper.Replace(s)
}
