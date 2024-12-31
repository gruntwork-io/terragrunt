package cloner

import "reflect"

// Option represents an option to customize deep copied results.
type Option func(*Config)

// DisallowTypes returns a Option that disallows copying any types
// that are in given values.
func DisallowTypes(val ...any) Option {
	return func(opt *Config) {
		for i := range val {
			opt.disallowTypes = append(opt.disallowTypes, reflect.TypeOf(val[i]))
		}
	}
}
