// Package cloner provides functions to deep clone any Go data.
package cloner

import "reflect"

// Clone returns a deep cloned instance of the given `src` variable.
func Clone[T any](src T, opts ...Option) T { //nolint:ireturn
	conf := &Config{}
	for _, opt := range opts {
		opt(conf)
	}

	cloner := Cloner[T]{Config: conf}

	return cloner.Clone(src)
}

// WithShadowCopyTypes returns an `Option` that forces shadow copies
// of the types that are in the given `values`.
func WithShadowCopyTypes(values ...any) Option {
	return func(opt *Config) {
		for i := range values {
			opt.shadowCopyTypes = append(opt.shadowCopyTypes, reflect.TypeOf(values[i]))
		}
	}
}

// WithSkippingTypes returns an `Option` that forces skipping copying types
// that are in the given `values`.
func WithSkippingTypes(values ...any) Option {
	return func(opt *Config) {
		for i := range values {
			opt.skippingTypes = append(opt.skippingTypes, reflect.TypeOf(values[i]))
		}
	}
}

// WithShadowCopyInversePkgPrefixes returns an `Option` that forces shadow copies
// of types whose pkg paths do not match the given `prefixes`.
func WithShadowCopyInversePkgPrefixes(prefixes ...string) Option {
	return func(opt *Config) {
		opt.shadowCopyInversePkgPrefixes = append(opt.shadowCopyInversePkgPrefixes, prefixes...)
	}
}
