package log

import "sort"

const (
	FieldKeyTime     = "time"
	FieldKeyLevel    = "level"
	FieldKeyPrefix   = "prefix"
	FieldKeyTFBinary = "tf-binary"
	FieldKeyMsg      = "msg"
)

// Fields type, used to pass to `WithFields`.
type Fields map[string]interface{}

func (fields Fields) Keys(removeKeys ...string) []string {
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
