package cli

import (
	"os"
	"strings"
)

// lookupEnv retrieves the value of the environment variable named by the key. If the variable is present in the environment and the value is not empty the
// value is returned and the boolean is true. Otherwise the returned value will be empty and the boolean will be false.
func lookupEnv(key string) (string, bool) {
	if key == "" {
		return "", false
	}

	val, ok := os.LookupEnv(key)
	val = strings.TrimSpace(val)

	isPresent := ok && val != ""
	return val, isPresent
}
