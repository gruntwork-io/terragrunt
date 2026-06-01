package util

import (
	"os"
	"strings"
)

// EnvironMap returns the current process environment as a map of name to value.
// Entries without an '=' separator are skipped; keys are trimmed of surrounding
// whitespace, values pass through verbatim.
func EnvironMap() map[string]string {
	environ := os.Environ()
	out := make(map[string]string, len(environ))

	for _, e := range environ {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}

		out[strings.TrimSpace(k)] = v
	}

	return out
}
