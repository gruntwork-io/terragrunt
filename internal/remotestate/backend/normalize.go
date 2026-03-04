package backend

import (
	"maps"
	"reflect"
	"strconv"
	"strings"
)

// NormalizeBoolValues normalizes string boolean values in the config map
// back to native Go bools, using reflection on the given target struct
// to determine which keys expect boolean values.
//
// HCL ternary type unification can convert bools to strings (e.g. true → "true").
// This causes generated backend blocks to contain quoted "true"/"false" string
// literals instead of unquoted true/false boolean literals, which Terraform/OpenTofu
// rejects. This function fixes that by inspecting mapstructure tags on the target
// struct to identify boolean fields, then converting any string values that are
// valid boolean representations.
//
// The target parameter should be a pointer to the struct that the config map
// will eventually be decoded into (e.g. &ExtendedRemoteStateConfigS3{}).
// Fields with mapstructure:",squash" tags are recursed into automatically.
func NormalizeBoolValues(m Config, target any) Config {
	boolKeys := collectBoolKeys(reflect.TypeOf(target))

	if len(boolKeys) == 0 {
		return m
	}

	normalized := make(Config)
	maps.Copy(normalized, m)

	for key, val := range normalized {
		strVal, ok := val.(string)
		if !ok {
			continue
		}

		if !boolKeys[key] {
			continue
		}

		if boolVal, err := strconv.ParseBool(strVal); err == nil {
			normalized[key] = boolVal
		}
	}

	return normalized
}

// collectBoolKeys walks a struct type via reflection, reading mapstructure tags
// to build a set of config key names that map to bool fields.
func collectBoolKeys(t reflect.Type) map[string]bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil
	}

	keys := make(map[string]bool)

	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("mapstructure")

		// Handle squashed embedded structs
		if tag == ",squash" || tag == "" && field.Anonymous {
			for k, v := range collectBoolKeys(field.Type) {
				keys[k] = v
			}

			continue
		}

		if tag == "" || tag == "-" {
			continue
		}

		// Extract the key name (first comma-separated segment)
		key, _, _ := strings.Cut(tag, ",")
		if key == "" {
			continue
		}

		if field.Type.Kind() == reflect.Bool {
			keys[key] = true
		}
	}

	return keys
}
