package util

import (
	"encoding/json"
)

// Convert the given value to a JSON value that can be passed to Terraform as an environment variable. For the most
// part, this converts the value directly to JSON using Go's built-in json.Marshal. However, we have special handling
// for strings, which with normal JSON conversion would be wrapped in quotes, but when passing them to Terraform via
// env vars, we need to NOT wrap them in quotes, so this method adds special handling for that case.
func AsTerraformEnvVarJsonValue(value interface{}) (string, error) {
	switch val := value.(type) {
	case string:
		return val, nil
	default:
		envVarValue, err := json.Marshal(val)
		if err != nil {
			return "", err
		}
		return string(envVarValue), nil
	}
}
