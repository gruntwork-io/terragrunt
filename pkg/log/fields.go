package log

import "sort"

// Fields is the type used to pass arguments to `WithFields`.
type Fields map[string]interface{}

func (fields Fields) RemoveKeys(removeKeys ...string) []string {
	var keys []string

	for key := range fields {
		var skip bool

		for _, removeKey := range removeKeys {
			if key == removeKey {
				skip = true
				break
			}
		}

		if !skip {
			keys = append(keys, key)
		}
	}

	sort.Strings(keys)

	return keys
}
