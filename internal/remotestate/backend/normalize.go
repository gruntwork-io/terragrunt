package backend

import (
	"maps"
	"reflect"
	"strconv"
	"strings"
)

// NormalizeBoolValues converts string boolean values ("true"/"false") in the
// config map back to native Go bools. HCL ternary type unification can convert
// bools to strings, which causes generated backend blocks to contain quoted
// literals that Terraform/OpenTofu rejects.
//
// The target parameter should be a pointer to the config struct (e.g.
// &ExtendedRemoteStateConfigS3{}); its mapstructure tags determine which
// keys are boolean fields.
func NormalizeBoolValues(m Config, target any) Config {
	boolKeys := collectBoolKeys(reflect.TypeOf(target))

	if len(boolKeys) == 0 {
		return m
	}

	normalized := make(Config)
	maps.Copy(normalized, m)

	for key, val := range normalized {
		strVal, ok := val.(string)
		if _, isBool := boolKeys[key]; !ok || !isBool {
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
func collectBoolKeys(t reflect.Type) map[string]struct{} {
	if t == nil {
		return nil
	}

	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil
	}

	keys := make(map[string]struct{})

	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("mapstructure")

		// Handle squashed embedded structs
		if tag == ",squash" || (tag == "" && field.Anonymous) {
			maps.Copy(keys, collectBoolKeys(field.Type))

			continue
		}

		if key, ok := collectFieldBoolKey(&field, tag); ok {
			keys[key] = struct{}{}
		}
	}

	return keys
}

// collectFieldBoolKey returns the config key name for a bool field,
// or empty string and false if the field is not a bool.
func collectFieldBoolKey(field *reflect.StructField, tag string) (string, bool) {
	if tag == "" || tag == "-" {
		return "", false
	}

	key, _, _ := strings.Cut(tag, ",")
	if key == "" {
		return "", false
	}

	fieldType := field.Type
	if fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	return key, fieldType.Kind() == reflect.Bool
}
