package util

import (
	"encoding/json"
	"fmt"
	"strings"
)

// interpolationEscaper replaces unescaped HCL interpolation patterns (${...}) with
// their escaped form ($${...}). Listing $${ first makes the replacement idempotent:
// already-escaped $${...} is matched at that position before ${ is tried, so it is
// emitted unchanged.
var interpolationEscaper = strings.NewReplacer("$${", "$${", "${", "$${")

// AsTerraformEnvVarJSONValue converts the given value to a JSON value that can be passed to
// Terraform as an environment variable. For the most part, this converts the value directly
// to JSON using Go's built-in json.Marshal. However, we have special handling
// for strings, which with normal JSON conversion would be wrapped in quotes, but when passing them to Terraform via
// env vars, we need to NOT wrap them in quotes, so this method adds special handling for that case.
// For complex types (maps, lists, objects), string values containing ${...} patterns are escaped to $${...}
// to prevent Terraform's HCL parser from treating them as variable interpolations.
func AsTerraformEnvVarJSONValue(value any) (string, error) {
	switch val := value.(type) {
	case string:
		return val, nil
	default:
		escaped, err := escapeInterpolationPatternsInValue(val, 0)
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

// escapeInterpolationPatternsInValue recursively walks a value tree and escapes
// HCL interpolation patterns (${...}) in string values to prevent Terraform from
// treating them as variable references when parsing complex type TF_VAR_* env vars.
//
// This unconditionally escapes ${...} in all string values within complex types.
// This is intentional: Terraform's HCL parser would error on unescaped ${...} in
// complex TF_VAR values anyway (behaviour change: previously errored; now passes the
// literal value through).
//
// Nil maps and slices are preserved as nil so json.Marshal serializes them as null
// rather than {} or [], keeping Terraform's null-vs-empty-collection semantics intact.
//
// Returns an error if the value tree is deeper than maxDepth (100), which prevents
// infinite recursion on malformed inputs.
func escapeInterpolationPatternsInValue(value any, depth int) (any, error) {
	const maxDepth = 100
	if depth > maxDepth {
		return nil, fmt.Errorf("escapeInterpolationPatternsInValue: input exceeds maximum nesting depth of %d", maxDepth)
	}

	switch v := value.(type) {
	case string:
		return EscapeInterpolationInString(v), nil

	case map[string]any:
		if v == nil {
			return nil, nil
		}

		result := make(map[string]any, len(v))

		for key, val := range v {
			escaped, err := escapeInterpolationPatternsInValue(val, depth+1)
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
			escaped, err := escapeInterpolationPatternsInValue(val, depth+1)
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
			result = append(result, EscapeInterpolationInString(s))
		}

		return result, nil

	case map[string]string:
		if v == nil {
			return nil, nil
		}

		result := make(map[string]any, len(v))

		for k, s := range v {
			result[k] = EscapeInterpolationInString(s)
		}

		return result, nil

	default:
		return value, nil
	}
}

// EscapeInterpolationInString escapes HCL interpolation patterns (${...}) in a string
// by doubling the dollar sign (${ → $${). This is idempotent: already-escaped $${...}
// patterns are not double-escaped.
func EscapeInterpolationInString(s string) string {
	return interpolationEscaper.Replace(s)
}
