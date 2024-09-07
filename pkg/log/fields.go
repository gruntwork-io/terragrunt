package log

import "sort"

const (
	FieldKeyPrefix   = "prefix"
	FieldKeyTFBinary = "tf-binary"
	FieldKeyMsg      = "msg"
	FieldKeyLevel    = "level"
	FieldKeyTime     = "time"
)

var logKeys = []string{
	FieldKeyMsg,
	FieldKeyLevel,
	FieldKeyTime,
}

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

// This is to not silently overwrite `time`, `msg` and `level` fields when
// dumping it. If this code wasn't there doing:
//
//	log.WithField("level", 1).Info("hello")
//
// Would just silently drop the user provided level. Instead with this code
// it'll logged as:
//
//	{"level": "info", "fields.level": 1, "msg": "hello", "time": "..."}
func (fields Fields) fixKeyClashes() {
	for _, key := range logKeys {
		if val, ok := fields[key]; ok {
			fields["fields."+key] = val
			delete(fields, key)
		}
	}
}
