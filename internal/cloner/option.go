package cloner

import "reflect"

const terragruntPkgPrefix = "github.com/gruntwork-io/terragrunt"

// Option represents an option to customize deep copied results.
type Option func(*Config)

// WithShadowCopyTypes returns a Option that force `Cloner` to create shadow copies
// of the types that are in the given `values`.
func WithShadowCopyTypes(values ...any) Option {
	return func(opt *Config) {
		for i := range values {
			opt.shadowCopyTypes = append(opt.shadowCopyTypes, reflect.TypeOf(values[i]))
		}
	}
}

// WithSkippingTypes returns a Option that force `Cloner` to skip copying types
// that are in the given `values`.
func WithSkippingTypes(values ...any) Option {
	return func(opt *Config) {
		for i := range values {
			opt.skippingTypes = append(opt.skippingTypes, reflect.TypeOf(values[i]))
		}
	}
}

func WithShadowCopyInversePkgPrefixes(prefixes ...string) Option {
	return func(opt *Config) {
		opt.shadowCopyInversePkgPrefixes = append(opt.shadowCopyInversePkgPrefixes, prefixes...)
	}
}

func WithShadowCopyThirdPartyTypes(prefixes ...string) Option {
	return WithShadowCopyInversePkgPrefixes(terragruntPkgPrefix)
}
